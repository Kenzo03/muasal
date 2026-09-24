package httpapi

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes v as a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeProblem writes an RFC 9457 problem (FSD §17.1). code is stable; the UI translates it.
func writeProblem(w http.ResponseWriter, status int, code, title string, fields ...FieldError) {
	p := Problem{Type: "/problems/" + code, Title: title, Status: status, Code: code}
	if len(fields) > 0 {
		p.Errors = &fields
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// decodeJSON reads a JSON body of at most 1 MiB into dst and rejects unknown
// fields, so a client cannot slip in fields such as is_admin.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "The request body is not valid JSON for this endpoint")
		return false
	}
	return true
}
