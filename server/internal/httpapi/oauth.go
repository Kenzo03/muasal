package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/kenzo03/muasal/server/internal/db"
)

// MCP sign-in (MCP spec): Muasal is the authorization server for its own
// /mcp endpoint. Clients register themselves, a signed-in user approves them on
// /oauth/authorize, and the code they get back buys an ordinary API token.

// mcpResource is the protected resource the tokens are for.
func (s *Server) mcpResource() string { return s.cfg.PublicURL + "/mcp" }

// protectedResource is RFC 9728's metadata.
func (s *Server) protectedResource(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 s.mcpResource(),
		"authorization_servers":    []string{s.cfg.PublicURL},
		"bearer_methods_supported": []string{"header"},
	})
}

// authServerMetadata is RFC 8414's metadata.
func (s *Server) authServerMetadata(w http.ResponseWriter, _ *http.Request) {
	u := s.cfg.PublicURL
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                u,
		"authorization_endpoint":                u + "/oauth/authorize",
		"token_endpoint":                        u + "/oauth/token",
		"registration_endpoint":                 u + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

// writeOAuthError answers in RFC 6749's error format, which OAuth clients parse.
func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// validRedirectURI allows https anywhere and http only on the loopback
// address, where native clients listen (RFC 8252). No fragments.
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || strings.Contains(raw, "#") {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}

// registerClient is RFC 7591 dynamic registration for public clients.
// Registering grants nothing: a signed-in user still has to approve.
func (s *Server) registerClient(w http.ResponseWriter, r *http.Request) {
	if !s.ipLimit.Allow(clientIP(r)) {
		writeOAuthError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests from this address; wait a minute")
		return
	}
	var in struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	// Clients send more metadata than this; unknown fields are ignored.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "Send the client metadata as JSON")
		return
	}
	name := strings.TrimSpace(in.ClientName)
	if name == "" {
		name = "MCP client"
	}
	if utf8.RuneCountInString(name) > 100 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "client_name takes at most 100 characters")
		return
	}
	if len(in.RedirectURIs) < 1 || len(in.RedirectURIs) > 5 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Give 1 to 5 redirect URIs")
		return
	}
	for _, u := range in.RedirectURIs {
		if !validRedirectURI(u) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Redirect URIs use https, or http on localhost, with no fragment")
			return
		}
	}
	c, err := s.q.CreateOAuthClient(r.Context(), db.CreateOAuthClientParams{ID: rand.Text(), Name: name, RedirectUris: in.RedirectURIs})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": c.ID, "client_name": c.Name, "redirect_uris": c.RedirectUris,
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code"}, "response_types": []string{"code"},
	})
}
