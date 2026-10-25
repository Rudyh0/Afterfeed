package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// apiError is an error with an HTTP status and a stable code. The frontend
// translates the code; the message is for logs and developers.
type apiError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// RetryAfter is set when the client must wait before trying again, in seconds.
	RetryAfter int `json:"retry_after,omitempty"`
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

func errBadRequest(code, msg string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: code, Message: msg}
}

func errNotFound() *apiError {
	return &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}
}

func errUnauthorized() *apiError {
	return &apiError{Status: http.StatusUnauthorized, Code: "auth.required", Message: "sign in required"}
}

func errForbidden(code string) *apiError {
	return &apiError{Status: http.StatusForbidden, Code: code, Message: "forbidden"}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("write json", "err", err)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	if !errors.As(err, &ae) {
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		ae = &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: "internal error"}
	}
	writeJSON(w, ae.Status, map[string]any{"error": ae})
}

// readJSON decodes a JSON request body into v. Bodies are limited to 1 MiB;
// large files use the upload endpoints instead.
func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errBadRequest("invalid_json", err.Error())
	}
	return nil
}

// handlerFunc is an HTTP handler that returns an error instead of writing it.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (s *Server) handle(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeError(w, r, err)
		}
	}
}
