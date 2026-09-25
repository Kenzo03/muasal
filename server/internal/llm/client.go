// Package llm is one small client for OpenAI-compatible APIs: a local Ollama or
// vLLM, or a cloud provider with the customer's key (FSD §13.4, R-AI-4). The
// browser never talks to a model server; only this package does.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to one base URL (ending in /v1) and one model.
type Client struct {
	base, model, key string
	http             *http.Client
}

// New returns a client; key may be empty for a local server without auth.
func New(baseURL, model, key string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), model: model, key: key, http: hc}
}

// Model names the model this client uses.
func (c *Client) Model() string { return c.model }

// APIError is a non-2xx answer; Body is cut to 500 bytes and never holds our key.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("model server answered %d: %s", e.Status, e.Body)
}

// Models lists the model ids the server offers (Test connection, §13.4).
func (c *Client) Models(ctx context.Context) ([]string, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/models", nil, &out); err != nil {
		return nil, err
	}
	ids := make([]string, len(out.Data))
	for i, m := range out.Data {
		ids[i] = m.ID
	}
	return ids, nil
}

// Embed returns one vector per text, in order (§13.2).
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/embeddings", map[string]any{"model": c.model, "input": texts}, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("asked for %d embeddings, got %d", len(texts), len(out.Data))
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) {
			return nil, fmt.Errorf("embedding index %d out of range", d.Index)
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
}

// ChatRequest is one constrained answer (§11.5).
type ChatRequest struct {
	System, User string
	Schema       json.RawMessage // JSON schema of the answer
	Temperature  float64
	MaxTokens    int
	Seed         *int // fixed during evaluation
}

// ChatStream starts a streamed chat and returns a reader of the answer's text,
// the concatenated content deltas. The request asks for the JSON schema and no
// thinking pass; a provider that refuses those gets the same request without
// reasoning_effort, then in plain JSON mode (§11.5). The server validates the
// answer either way.
func (c *Client) ChatStream(ctx context.Context, r ChatRequest) (io.ReadCloser, error) {
	body := map[string]any{
		"model":            c.model,
		"stream":           true,
		"temperature":      r.Temperature,
		"top_p":            0.9,
		"max_tokens":       r.MaxTokens,
		"reasoning_effort": "none",
		"messages": []map[string]string{
			{"role": "system", "content": r.System},
			{"role": "user", "content": r.User},
		},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "answer", "strict": true, "schema": r.Schema},
		},
	}
	if r.Seed != nil {
		body["seed"] = *r.Seed
	}
	fallbacks := []func(){
		func() { delete(body, "reasoning_effort") },
		func() { body["response_format"] = map[string]any{"type": "json_object"} },
	}
	for i := 0; ; i++ {
		res, err := c.send(ctx, http.MethodPost, "/chat/completions", body)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest && i < len(fallbacks) {
			fallbacks[i]()
			continue
		}
		if err != nil {
			return nil, err
		}
		return streamContent(res.Body), nil
	}
}

// streamContent turns an SSE body of chat.completion.chunk events into the
// text of their content deltas.
func streamContent(body io.ReadCloser) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer body.Close()
		sc := bufio.NewScanner(body)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			data, ok := strings.CutPrefix(line, "data:")
			if !ok {
				continue
			}
			data = strings.TrimSpace(data)
			if data == "[DONE]" {
				break
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				pw.CloseWithError(fmt.Errorf("bad stream event: %w", err))
				return
			}
			if chunk.Error != nil {
				pw.CloseWithError(errors.New("model server: " + chunk.Error.Message))
				return
			}
			for _, ch := range chunk.Choices {
				if _, err := io.WriteString(pw, ch.Delta.Content); err != nil {
					return // the reader went away
				}
			}
		}
		pw.CloseWithError(sc.Err()) // nil means io.EOF for the reader
	}()
	return pr
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	res, err := c.send(ctx, method, path, in)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return json.NewDecoder(res.Body).Decode(out)
}

// send makes one request. A 429 retries with backoff (1 s, 2 s, 4 s, …) until
// the caller's deadline (§11.7).
func (c *Client) send(ctx context.Context, method, path string, in any) (*http.Response, error) {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return nil, err
		}
	}
	for wait := time.Second; ; wait *= 2 {
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.key != "" {
			req.Header.Set("Authorization", "Bearer "+c.key)
		}
		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode/100 == 2 {
			return res, nil
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 500))
		res.Body.Close()
		apiErr := &APIError{Status: res.StatusCode, Body: strings.TrimSpace(string(b))}
		if res.StatusCode != http.StatusTooManyRequests {
			return nil, apiErr
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, apiErr
		}
	}
}
