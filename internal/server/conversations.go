package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

func (s *Server) conversationRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/conversations", s.authed(s.listConversations, false))
	m.HandleFunc("GET /api/conversations/{id}", s.authed(s.conversation, false))
	m.HandleFunc("PATCH /api/conversations/{id}", s.authed(s.editConversation, false))
	m.HandleFunc("GET /api/conversations/{id}/search", s.authed(s.searchConversation, false))
	m.HandleFunc("GET /api/message-files/{id}/waveform", s.authed(s.waveform, false))
}

type conversationJSON struct {
	ID           string   `json:"id"`
	Platform     string   `json:"platform"`
	Title        string   `json:"title"`
	IsGroup      bool     `json:"is_group"`
	Participants []string `json:"participants"`
	Self         string   `json:"self,omitempty"`
	FirstAt      int64    `json:"first_at"`
	LastAt       int64    `json:"last_at"`
	LastText     string   `json:"last_text"`
	Messages     int      `json:"message_count"`
	Files        int      `json:"file_count"`
	Vault        bool     `json:"vault,omitempty"`
}

const conversationColumns = `c.id, c.platform, c.title, c.is_group, c.participants_json, c.self_name,
	COALESCE(c.first_at, 0), COALESCE(c.last_at, 0), c.last_text, c.message_count, c.file_count, c.vault`

func scanConversation(row interface{ Scan(...any) error }) (*conversationJSON, error) {
	var c conversationJSON
	var participants string
	if err := row.Scan(&c.ID, &c.Platform, &c.Title, &c.IsGroup, &participants, &c.Self,
		&c.FirstAt, &c.LastAt, &c.LastText, &c.Messages, &c.Files, &c.Vault); err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(participants), &c.Participants)
	if c.Participants == nil {
		c.Participants = []string{}
	}
	return &c, nil
}

// listConversations lists conversations of a platform, most recent first,
// with the number of conversations per platform. With ?counts=1 it returns
// only the numbers.
func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	q := r.URL.Query()
	vault := q.Get("vault") == "1"
	if vault && !s.vaultOpen(r) {
		return errForbidden("vault.locked")
	}
	vaultCond := "c.vault = 0"
	if vault {
		vaultCond = "c.vault = 1"
	}
	platforms := []map[string]any{}
	rows, err := s.store.R.QueryContext(ctx, `SELECT c.platform, COUNT(*) FROM conversations c WHERE `+vaultCond+`
		GROUP BY c.platform ORDER BY COUNT(*) DESC`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p string
		var n int
		if rows.Scan(&p, &n) == nil {
			platforms = append(platforms, map[string]any{"platform": p, "conversations": n})
		}
	}
	rows.Close()
	if q.Get("counts") == "1" {
		writeJSON(w, http.StatusOK, map[string]any{"platforms": platforms})
		return nil
	}

	conds := []string{vaultCond}
	var args []any
	if p := q.Get("platform"); p != "" {
		conds = append(conds, "c.platform = ?")
		args = append(args, p)
	}
	if text := strings.TrimSpace(q.Get("q")); text != "" {
		conds = append(conds, "(c.title LIKE ? ESCAPE '\\' OR c.participants_json LIKE ? ESCAPE '\\')")
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text) + "%"
		args = append(args, like, like)
	}
	rows, err = s.store.R.QueryContext(ctx, `SELECT `+conversationColumns+` FROM conversations c
		WHERE `+strings.Join(conds, " AND ")+` ORDER BY c.last_at DESC LIMIT 500`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []*conversationJSON{}
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return err
		}
		list = append(list, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"platforms": platforms, "conversations": list})
	return nil
}

type messageJSON struct {
	ID        int64              `json:"id"`
	Sender    string             `json:"sender"`
	Self      bool               `json:"self"`
	At        int64              `json:"at"`
	Kind      string             `json:"kind"`
	Text      string             `json:"text,omitempty"`
	Link      string             `json:"link,omitempty"`
	Files     []messageFileJSON  `json:"files,omitempty"`
	Reactions []library.Reaction `json:"reactions,omitempty"`
}

// messageFileJSON is a file in a message, without its path on the server.
type messageFileJSON struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Name     string  `json:"name,omitempty"`
	Mime     string  `json:"mime,omitempty"`
	Size     int64   `json:"size,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

func (s *Server) loadConversation(r *http.Request, id string) (*conversationJSON, error) {
	c, err := scanConversation(s.store.R.QueryRowContext(r.Context(), `SELECT `+conversationColumns+`
		FROM conversations c WHERE c.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound()
	}
	if err != nil {
		return nil, err
	}
	if c.Vault {
		if !s.vaultOpen(r) {
			return nil, errNotFound()
		}
		s.deps.Auth.TouchVault(r.Context(), sessionFrom(r.Context()))
	}
	return c, nil
}

// conversation returns a page of messages: the newest, those before or
// after a message, those from a moment on, or those around one message.
func (s *Server) conversation(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	c, err := s.loadConversation(r, r.PathValue("id"))
	if err != nil {
		return err
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 120
	}
	var msgs []messageJSON
	var moreBefore, moreAfter bool
	switch {
	case q.Get("around") != "":
		id, _ := strconv.ParseInt(q.Get("around"), 10, 64)
		var at int64
		if err := s.store.R.QueryRowContext(ctx, `SELECT sent_at FROM messages WHERE id = ? AND conversation_id = ?`, id, c.ID).Scan(&at); err != nil {
			return errNotFound()
		}
		before, mb, err := s.messages(ctx, c.ID, "sent_at < ? OR (sent_at = ? AND id < ?)", []any{at, at, id}, "DESC", limit/2)
		if err != nil {
			return err
		}
		after, ma, err := s.messages(ctx, c.ID, "sent_at > ? OR (sent_at = ? AND id >= ?)", []any{at, at, id}, "ASC", limit/2)
		if err != nil {
			return err
		}
		msgs, moreBefore, moreAfter = append(reverse(before), after...), mb, ma
	case q.Get("after") != "":
		// Messages after a known one: ?after=<time>&after_id=<id>.
		at, _ := strconv.ParseInt(q.Get("after"), 10, 64)
		id, _ := strconv.ParseInt(q.Get("after_id"), 10, 64)
		msgs, moreAfter, err = s.messages(ctx, c.ID, "sent_at > ? OR (sent_at = ? AND id > ?)", []any{at, at, id}, "ASC", limit)
		moreBefore = true
	case q.Get("from") != "":
		// The first messages from a moment on, for "go to date".
		from, _ := strconv.ParseInt(q.Get("from"), 10, 64)
		msgs, moreAfter, err = s.messages(ctx, c.ID, "sent_at >= ?", []any{from}, "ASC", limit)
		if err != nil {
			return err
		}
		var n int
		s.store.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND sent_at < ?`, c.ID, from).Scan(&n)
		moreBefore = n > 0
	default:
		// The newest messages, or those before a known one: ?before=<time>&before_id=<id>.
		before, beforeID := int64(1)<<62, int64(0)
		if b := q.Get("before"); b != "" {
			before, _ = strconv.ParseInt(b, 10, 64)
			beforeID, _ = strconv.ParseInt(q.Get("before_id"), 10, 64)
			moreAfter = true
		}
		var list []messageJSON
		list, moreBefore, err = s.messages(ctx, c.ID, "sent_at < ? OR (sent_at = ? AND id < ?)", []any{before, before, beforeID}, "DESC", limit)
		msgs = reverse(list)
	}
	if err != nil {
		return err
	}
	if msgs == nil {
		msgs = []messageJSON{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation": c, "messages": msgs, "more_before": moreBefore, "more_after": moreAfter,
	})
	return nil
}

func reverse(list []messageJSON) []messageJSON {
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list
}

// messages reads up to limit messages and reports whether there are more.
func (s *Server) messages(ctx context.Context, convID, cond string, args []any, order string, limit int) ([]messageJSON, bool, error) {
	rows, err := s.store.R.QueryContext(ctx, `SELECT id, sender, is_self, sent_at, kind, text, link, files_json, reactions_json
		FROM messages WHERE conversation_id = ? AND (`+cond+`) ORDER BY sent_at `+order+`, id `+order+` LIMIT ?`,
		append(append([]any{convID}, args...), limit+1)...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []messageJSON
	for rows.Next() {
		var m messageJSON
		var files, reactions string
		if err := rows.Scan(&m.ID, &m.Sender, &m.Self, &m.At, &m.Kind, &m.Text, &m.Link, &files, &reactions); err != nil {
			return nil, false, err
		}
		json.Unmarshal([]byte(files), &m.Files)
		json.Unmarshal([]byte(reactions), &m.Reactions)
		out = append(out, m)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, rows.Err()
}

// searchConversation finds messages in one conversation, newest first.
func (s *Server) searchConversation(w http.ResponseWriter, r *http.Request) error {
	c, err := s.loadConversation(r, r.PathValue("id"))
	if err != nil {
		return err
	}
	match := ftsQuery(r.URL.Query().Get("q"))
	hits := []messageHit{}
	if match != "" {
		hits, err = s.searchMessages(r.Context(), `msg.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?) AND msg.conversation_id = ?`,
			[]any{match, c.ID}, "DESC", 200, 0)
		if err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": hits})
	return nil
}

// editConversation moves a conversation in or out of the vault.
func (s *Server) editConversation(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req struct {
		Vault *bool `json:"vault"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	c, err := s.loadConversation(r, r.PathValue("id"))
	if err != nil {
		return err
	}
	if req.Vault == nil || *req.Vault == c.Vault {
		writeJSON(w, http.StatusOK, c)
		return nil
	}
	var rel string
	if err := s.store.R.QueryRowContext(ctx, `SELECT path FROM conversations WHERE id = ?`, c.ID).Scan(&rel); err != nil {
		return err
	}
	conv, err := s.deps.Library.LoadConversation(rel)
	if err != nil {
		return err
	}
	conv.Vault = *req.Vault
	conv.UpdatedAt = time.Now().UTC()
	if err := s.deps.Library.SaveConversation(rel, conv); err != nil {
		return err
	}
	if _, err := s.store.W.ExecContext(ctx, `UPDATE conversations SET vault = ?, updated_at = ? WHERE id = ?`,
		*req.Vault, time.Now().UTC().Format(time.RFC3339), c.ID); err != nil {
		return err
	}
	c.Vault = *req.Vault
	writeJSON(w, http.StatusOK, c)
	return nil
}

// waveform returns the loudness curve of a voice message.
func (s *Server) waveform(w http.ResponseWriter, r *http.Request) error {
	var id, mime, file, dir string
	var vault bool
	err := s.store.R.QueryRowContext(r.Context(), `SELECT f.id, f.mime, f.file, c.path, c.vault
		FROM message_files f JOIN conversations c ON c.id = f.conversation_id WHERE f.id = ?`, r.PathValue("id")).
		Scan(&id, &mime, &file, &dir, &vault)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && vault && !s.vaultOpen(r)) {
		return errNotFound()
	}
	if err != nil {
		return err
	}
	src := media.Source{ID: "m" + id, Path: filepath.Join(s.deps.Library.ConversationDir(dir), filepath.FromSlash(file)), Kind: media.Audio, Mime: mime}
	points, err := s.deps.Media.Waveform(r.Context(), src)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"points": []float64{}})
		return nil
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	data, _ := json.Marshal(map[string]any{"points": points})
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
	return nil
}
