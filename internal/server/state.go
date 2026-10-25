package server

import (
	"net/http"

	"github.com/rudyh0/afterfeed/internal/version"
)

// state tells the web app what to show on start: the Take-in for a new
// installation, the sign-in screen, or the app itself.
func (s *Server) state(w http.ResponseWriter, r *http.Request) error {
	var users int
	if err := s.store.R.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        version.Version,
		"setup_required": users == 0,
		"authenticated":  false,
	})
	return nil
}
