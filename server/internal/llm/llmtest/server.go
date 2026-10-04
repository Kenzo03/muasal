// Package llmtest is a fake OpenAI-compatible model server for tests: word-hash
// embeddings, so vector search behaves sensibly without a model, and scripted
// chat answers streamed in small pieces.
package llmtest

import (
	"cmp"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode"
)

// Server records what it was asked. Set the fields before the calls you script.
type Server struct {
	*httptest.Server
	mu sync.Mutex

	Dim          int                                                      // embedding dimension, 1024 by default
	Answer       func(system, user string, schema json.RawMessage) string // the chat answer's text
	Down         bool                                                     // answer every call with 503
	DownBody     string                                                   // the 503's body, "model server stopped" by default
	RejectSchema bool                                                     // answer json_schema requests with 400
	Key          string                                                   // when set, require "Bearer <Key>"
	Missing      string                                                   // a model this server lacks: 404 as Ollama says it

	ChatCalls  int
	EmbedCalls int
	Embedded   []string       // every text embedded, in order
	LastChat   map[string]any // the last chat request body
}

// New starts a server that the test closes.
func New(t *testing.T) *Server {
	s := &Server{Dim: 1024, Answer: func(string, string, json.RawMessage) string { return `{"claims":[]}` }}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// URL is the base URL a client uses, ending in /v1.
func (s *Server) BaseURL() string { return s.Server.URL + "/v1" }

// Calls reports the chat and embedding calls so far.
func (s *Server) Calls() (chat, embed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ChatCalls, s.EmbedCalls
}

// Set changes the server's behavior under its lock.
func (s *Server) Set(f func(s *Server)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	down, downBody, key, dim, answer, reject, missing := s.Down, s.DownBody, s.Key, s.Dim, s.Answer, s.RejectSchema, s.Missing
	s.mu.Unlock()
	if down {
		http.Error(w, cmp.Or(downBody, "model server stopped"), http.StatusServiceUnavailable)
		return
	}
	if key != "" && r.Header.Get("Authorization") != "Bearer "+key {
		http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/v1/models":
		writeJSON(w, map[string]any{"data": []map[string]string{{"id": "qwen3.5:4b"}, {"id": "bge-m3"}}})
	case "/v1/embeddings":
		var in struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if missing != "" && in.Model == missing {
			http.Error(w, `{"error":{"message":"model \"`+missing+`\" not found, try pulling it first"}}`, http.StatusNotFound)
			return
		}
		s.mu.Lock()
		s.EmbedCalls++
		s.Embedded = append(s.Embedded, in.Input...)
		s.mu.Unlock()
		data := make([]map[string]any, len(in.Input))
		for i, text := range in.Input {
			data[i] = map[string]any{"index": i, "embedding": Vector(text, dim)}
		}
		writeJSON(w, map[string]any{"data": data})
	case "/v1/chat/completions":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if missing != "" && body["model"] == missing {
			http.Error(w, `{"error":{"message":"model \"`+missing+`\" not found, try pulling it first"}}`, http.StatusNotFound)
			return
		}
		rf, _ := body["response_format"].(map[string]any)
		if reject && rf["type"] == "json_schema" {
			http.Error(w, `{"error":{"message":"response_format json_schema is not supported"}}`, http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.ChatCalls++
		s.LastChat = body
		s.mu.Unlock()
		var system, user string
		if msgs, ok := body["messages"].([]any); ok && len(msgs) == 2 {
			system, _ = msgs[0].(map[string]any)["content"].(string)
			user, _ = msgs[1].(map[string]any)["content"].(string)
		}
		var schema json.RawMessage
		if js, ok := rf["json_schema"].(map[string]any); ok {
			schema, _ = json.Marshal(js["schema"])
		}
		text := answer(system, user, schema)
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < len(text); i += 7 { // small pieces, split mid-token
			piece := text[i:min(i+7, len(text))]
			b, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]string{"content": piece}}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	default:
		http.NotFound(w, r)
	}
}

// Vector is a deterministic stand-in for an embedding: each lower-cased word
// adds weight to one hashed dimension, and the result has unit length. Texts
// that share words are close in cosine distance.
func Vector(text string, dim int) []float32 {
	v := make([]float32, dim)
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		h := fnv.New32a()
		h.Write([]byte(w))
		v[int(h.Sum32())%dim]++
	}
	var n float64
	for _, x := range v {
		n += float64(x * x)
	}
	if n == 0 {
		v[0] = 1
		return v
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / math.Sqrt(n))
	}
	return v
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
