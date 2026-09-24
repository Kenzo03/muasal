package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	writeProblem(rec, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
		FieldError{Field: "email", Code: "invalid", Message: "Enter a valid email address"})
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content type %q", got)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Status != 422 || p.Code != "validation_failed" || p.Type != "/problems/validation_failed" ||
		p.Errors == nil || (*p.Errors)[0].Field != "email" {
		t.Fatalf("unexpected problem: %+v", p)
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"a@b.c","password":"x","is_admin":true}`))
	var in LoginRequest
	if decodeJSON(rec, req, &in) {
		t.Fatal("an unknown field must be rejected")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}
