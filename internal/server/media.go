package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/media"
)

func (s *Server) mediaRoutes() {
	s.mux.HandleFunc("GET /api/media/{id}/{variant}", s.authed(s.serveMedia, false))
	s.mux.HandleFunc("GET /api/message-files/{id}/{variant}", s.authed(s.serveMessageFile, false))
	s.mux.HandleFunc("GET /api/library", s.authed(s.librarySummary, false))
}

// vaultOpen reports whether the vault is unlocked in this session.
func (s *Server) vaultOpen(r *http.Request) bool {
	sess := sessionFrom(r.Context())
	return sess != nil && s.deps.Auth.VaultOpen(sess, s.vaultAutolock(r.Context()))
}

type mediaRow struct {
	id, kind, mime, file, overlay, dir string
	takenAt                            string
	position                           int
	vault                              bool
}

func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) error {
	var m mediaRow
	err := s.store.R.QueryRowContext(r.Context(), `SELECT md.id, md.kind, md.mime, md.file, md.overlay, mm.path,
			mm.taken_at, md.position, mm.vault
		FROM media md JOIN memories mm ON mm.id = md.memory_id WHERE md.id = ?`, r.PathValue("id")).
		Scan(&m.id, &m.kind, &m.mime, &m.file, &m.overlay, &m.dir, &m.takenAt, &m.position, &m.vault)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound()
	}
	if err != nil {
		return err
	}
	if m.vault && !s.vaultOpen(r) {
		return errNotFound()
	}
	dir := s.deps.Library.MemoryDir(m.dir)
	src := media.Source{ID: m.id, Path: filepath.Join(dir, m.file), Kind: media.Kind(m.kind), Mime: m.mime}
	if m.overlay != "" {
		src.Overlay = filepath.Join(dir, m.overlay)
	}
	switch v := r.PathValue("variant"); v {
	case "thumb", "preview", "poster":
		return s.serveVariant(w, r, src, media.Variant(v))
	case "original":
		return serveFile(w, r, src.Path, m.mime, "", false)
	case "overlay":
		if src.Overlay == "" {
			return errNotFound()
		}
		return serveFile(w, r, src.Overlay, "image/png", "", false)
	case "download":
		return s.downloadMedia(w, r, m.id)
	}
	return errNotFound()
}

// serveVariant serves a cached thumbnail, preview or poster, creating it
// when needed. Without the image tools, photos that browsers can show are
// served as they are.
func (s *Server) serveVariant(w http.ResponseWriter, r *http.Request, src media.Source, v media.Variant) error {
	p, err := s.deps.Media.Get(r.Context(), src, v)
	if err != nil {
		if src.Kind == media.Photo && media.BrowserImage(src.Mime) {
			return serveFile(w, r, src.Path, src.Mime, "", false)
		}
		if errors.Is(err, media.ErrUnsupported) {
			return errNotFound()
		}
		return err
	}
	mime := "image/webp"
	if v == media.Poster {
		mime = "image/jpeg"
	}
	return serveFile(w, r, p, mime, "", true)
}

// serveFile serves a file with support for range requests, which video
// players need.
func serveFile(w http.ResponseWriter, r *http.Request, p, mime, downloadName string, immutable bool) error {
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return errNotFound()
		}
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	h := w.Header()
	if mime != "" {
		h.Set("Content-Type", mime)
	}
	if immutable {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "private, max-age=3600")
	}
	if downloadName != "" {
		h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(downloadName, `"`, "")))
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
	return nil
}

func (s *Server) serveMessageFile(w http.ResponseWriter, r *http.Request) error {
	var id, kind, mime, file, name, dir string
	var vault bool
	err := s.store.R.QueryRowContext(r.Context(), `SELECT f.id, f.kind, f.mime, f.file, f.name, c.path, c.vault
		FROM message_files f JOIN conversations c ON c.id = f.conversation_id WHERE f.id = ?`, r.PathValue("id")).
		Scan(&id, &kind, &mime, &file, &name, &dir, &vault)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound()
	}
	if err != nil {
		return err
	}
	if vault && !s.vaultOpen(r) {
		return errNotFound()
	}
	p := filepath.Join(s.deps.Library.ConversationDir(dir), filepath.FromSlash(file))
	src := media.Source{ID: "m" + id, Path: p, Kind: media.Kind(kind), Mime: mime}
	switch v := r.PathValue("variant"); v {
	case "thumb", "preview", "poster":
		return s.serveVariant(w, r, src, media.Variant(v))
	case "original":
		return serveFile(w, r, p, mime, "", false)
	case "download":
		if name == "" {
			name = filepath.Base(file)
		}
		return serveFile(w, r, p, mime, name, false)
	}
	return errNotFound()
}

// Source is one platform in the library, for the menu.
type Source struct {
	Platform      string `json:"platform"`
	FirstYear     int    `json:"first_year"`
	LastYear      int    `json:"last_year"`
	Memories      int    `json:"memories"`
	Conversations int    `json:"conversations"`
}

func (s *Server) librarySummary(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	rows, err := s.store.R.QueryContext(ctx, `SELECT platform, MIN(year), MAX(year), COUNT(*)
		FROM memories WHERE vault = 0 GROUP BY platform ORDER BY COUNT(*) DESC`)
	if err != nil {
		return err
	}
	sources := []*Source{}
	byPlatform := map[string]*Source{}
	for rows.Next() {
		src := &Source{}
		if err := rows.Scan(&src.Platform, &src.FirstYear, &src.LastYear, &src.Memories); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, src)
		byPlatform[src.Platform] = src
	}
	rows.Close()
	crow, err := s.store.R.QueryContext(ctx, `SELECT platform, COUNT(*) FROM conversations WHERE vault = 0 GROUP BY platform`)
	if err != nil {
		return err
	}
	for crow.Next() {
		var p string
		var n int
		if err := crow.Scan(&p, &n); err != nil {
			crow.Close()
			return err
		}
		if src := byPlatform[p]; src != nil {
			src.Conversations = n
		} else {
			sources = append(sources, &Source{Platform: p, Conversations: n})
		}
	}
	crow.Close()
	var total, years int
	var first, last sql.NullInt64
	s.store.R.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT year), MIN(year), MAX(year) FROM memories WHERE vault = 0`).
		Scan(&total, &years, &first, &last)
	writeJSON(w, http.StatusOK, map[string]any{
		"sources":    sources,
		"memories":   total,
		"years":      years,
		"first_year": first.Int64,
		"last_year":  last.Int64,
		"today":      time.Now().Format("2006-01-02"),
	})
	return nil
}
