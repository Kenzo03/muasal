package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpHandler serves POST /mcp (MCP spec): a stateless MCP server whose tools
// call the REST API through root with the caller's token, so every API rule
// (visibility, validation, versions, audit, rate limit) applies unchanged.
func (s *Server) mcpHandler(root http.Handler) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer(&apiCaller{root: root, auth: r.Header.Get("Authorization"), remote: r.RemoteAddr, xff: r.Header.Get("X-Forwarded-For")})
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true,
		// Muasal sits behind Caddy (Host is the public name) and checks the token itself.
		DisableLocalhostProtection: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only a token: a session cookie would let any web page drive the tools.
		bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			s.mcpUnauthorized(w, false)
			return
		}
		tok, u := s.tokenUser(r.Context(), strings.TrimSpace(bearer))
		if u == nil {
			s.mcpUnauthorized(w, true)
			return
		}
		if !s.tokRate.Allow(strconv.FormatInt(tok.ID, 10)) {
			w.Header().Set("Retry-After", "60")
			writeProblem(w, http.StatusTooManyRequests, "rate_limited", "A token takes 60 requests a minute; wait a moment")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// mcpUnauthorized points the client at the sign-in metadata (RFC 9728).
// A token that was sent and refused also says so (RFC 6750 section 3).
func (s *Server) mcpUnauthorized(w http.ResponseWriter, sent bool) {
	hdr := "Bearer "
	if sent {
		hdr += `error="invalid_token", `
	}
	w.Header().Set("WWW-Authenticate", hdr+`resource_metadata="`+s.cfg.PublicURL+`/.well-known/oauth-protected-resource/mcp"`)
	writeProblem(w, http.StatusUnauthorized, "invalid_token", "Sign in to Muasal to use this endpoint")
}

// rawBody is a body already encoded, such as a multipart form.
type rawBody struct {
	contentType string
	data        []byte
}

// apiCaller sends a tool's requests to /api/v1 in-process, as the MCP caller.
type apiCaller struct {
	root              http.Handler
	auth, remote, xff string
}

// call decodes a 2xx JSON answer into out. Any other answer becomes an error
// carrying the problem's status, code, title and field errors, which the SDK
// returns to the agent as a tool error.
func (c *apiCaller) call(ctx context.Context, method, path string, headers map[string]string, body, out any) (http.Header, error) {
	var rd io.Reader
	contentType := "application/json"
	if raw, ok := body.(rawBody); ok {
		rd, contentType = bytes.NewReader(raw.data), raw.contentType
	} else if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://muasal.internal/api/v1"+path, rd)
	if err != nil {
		return nil, err
	}
	req.RemoteAddr = c.remote
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Content-Type", contentType)
	if c.xff != "" {
		req.Header.Set("X-Forwarded-For", c.xff)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c.root.ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code > 299 {
		var p Problem
		_ = json.Unmarshal(rec.Body.Bytes(), &p)
		msg := fmt.Sprintf("%d %s: %s", rec.Code, p.Code, p.Title)
		if p.Errors != nil {
			for _, f := range *p.Errors {
				msg += fmt.Sprintf("; %s: %s", f.Field, f.Message)
			}
		}
		return nil, errors.New(msg)
	}
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			return nil, err
		}
	}
	return rec.Header(), nil
}

// jsonResult returns v to the agent as JSON text.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}
