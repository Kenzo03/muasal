package httpapi

import "net/http"

// The generated ServerInterface needs every operation; user administration lands in the next task.
func (s *Server) SetupPassword(w http.ResponseWriter, r *http.Request)             { notImplemented(w) }
func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request)                  { notImplemented(w) }
func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request)                 { notImplemented(w) }
func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request)                { notImplemented(w) }
func (s *Server) UpdateUser(w http.ResponseWriter, r *http.Request, id int64)      { notImplemented(w) }
func (s *Server) CreateSetupLink(w http.ResponseWriter, r *http.Request, id int64) { notImplemented(w) }

func notImplemented(w http.ResponseWriter) {
	writeProblem(w, http.StatusNotImplemented, "not_implemented", "Not implemented yet")
}
