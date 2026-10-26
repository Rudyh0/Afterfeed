package server

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/rudyh0/afterfeed/internal/auth"
)

func (s *Server) sourceRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/sources", s.authed(s.listSources, false))
	m.HandleFunc("POST /api/sources/{platform}/remove", s.authed(s.removeSource, false))
}

// sourceInfo is one platform in the archive, with everything that removing
// it would delete.
type sourceInfo struct {
	Platform      string `json:"platform"`
	FirstYear     int    `json:"first_year,omitempty"`
	LastYear      int    `json:"last_year,omitempty"`
	Memories      int    `json:"memories"`
	Conversations int    `json:"conversations"`
	// Uploads are the export files uploaded for it, and their size.
	Uploads     int   `json:"uploads"`
	UploadsSize int64 `json:"uploads_size"`
	// Removable is false for memories made in Afterfeed.
	Removable bool `json:"removable"`
}

// listSources returns the platforms in the archive. The counts include
// what is in the vault: removing a source removes that too.
func (s *Server) listSources(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	byPlatform := map[string]*sourceInfo{}
	var order []string
	get := func(p string) *sourceInfo {
		if src := byPlatform[p]; src != nil {
			return src
		}
		src := &sourceInfo{Platform: p, Removable: p != "afterfeed"}
		byPlatform[p] = src
		order = append(order, p)
		return src
	}
	rows, err := s.store.R.QueryContext(ctx, `SELECT platform, MIN(year), MAX(year), COUNT(*)
		FROM memories GROUP BY platform ORDER BY COUNT(*) DESC`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p string
		var first, last, n int
		if err := rows.Scan(&p, &first, &last, &n); err != nil {
			rows.Close()
			return err
		}
		src := get(p)
		src.FirstYear, src.LastYear, src.Memories = first, last, n
	}
	rows.Close()
	rows, err = s.store.R.QueryContext(ctx, `SELECT platform, COUNT(*) FROM conversations GROUP BY platform`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p string
		var n int
		if err := rows.Scan(&p, &n); err != nil {
			rows.Close()
			return err
		}
		get(p).Conversations = n
	}
	rows.Close()
	out := make([]*sourceInfo, 0, len(order))
	for _, p := range order {
		src := byPlatform[p]
		if s.deps.Imports != nil && src.Removable {
			src.Uploads, src.UploadsSize, _ = s.deps.Imports.SourceUploads(ctx, p)
		}
		out = append(out, src)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": out})
	return nil
}

// removeSource deletes everything imported from a platform, after checking
// the account password: memories, conversations, their files on the data
// volume, cached previews and the import history. Uploaded export files are
// kept unless asked otherwise, so the source can be imported again.
func (s *Server) removeSource(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password      string `json:"password"`
		DeleteUploads bool   `json:"delete_uploads"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	platform := r.PathValue("platform")
	known := []string{"facebook", "instagram", "x", "snapchat"}
	if s.deps.Imports != nil {
		known = s.deps.Imports.Platforms()
	}
	if !slices.Contains(known, platform) {
		return errBadRequest("source.unknown", "unknown or own source")
	}
	ctx := r.Context()
	u := userFrom(ctx)
	key := "source-remove:" + u.Username
	if err := s.deps.Auth.CheckAttempts(ctx, key); err != nil {
		if rl := rateLimited(err); rl != nil {
			return rl
		}
		return err
	}
	if !auth.VerifyPassword(req.Password, u.PasswordHash) {
		s.deps.Auth.FailAttempt(ctx, key)
		return errBadRequest("auth.invalid_credentials", "password is wrong")
	}
	s.deps.Auth.ResetAttempts(ctx, key)
	if s.deps.Imports != nil && s.deps.Imports.Running(ctx) {
		return &apiError{Status: http.StatusConflict, Code: "source.import_running", Message: "an import is running"}
	}

	// Closing the page must not stop it halfway.
	ctx = context.WithoutCancel(ctx)
	began := time.Now()
	removed, err := s.deps.Library.RemovePlatform(ctx, platform)
	if err != nil {
		return err
	}
	for _, id := range removed.FileIDs {
		s.deps.Media.Remove(id)
	}
	if s.deps.Imports != nil {
		if err := s.deps.Imports.ForgetSource(ctx, platform, req.DeleteUploads); err != nil {
			return err
		}
	}
	slog.Info("source removed", "platform", platform, "memories", removed.Memories,
		"conversations", removed.Conversations, "uploads_deleted", req.DeleteUploads,
		"took", time.Since(began).Round(time.Millisecond))
	writeJSON(w, http.StatusOK, map[string]any{"memories": removed.Memories, "conversations": removed.Conversations})
	return nil
}
