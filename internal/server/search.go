package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rudyh0/afterfeed/internal/library"
)

// searchQuery is a parsed search. The web app recognises dates, seasons and
// places in what the user types, in their language, and sends them as
// separate parameters; the rest is full-text search.
type searchQuery struct {
	text     string
	place    string
	from     string // YYYY-MM-DD, inclusive
	to       string // YYYY-MM-DD, inclusive
	months   []int
	platform string
	country  string
	newest   bool
}

func parseSearch(r *http.Request) searchQuery {
	q := r.URL.Query()
	sq := searchQuery{
		text:     strings.TrimSpace(q.Get("q")),
		place:    strings.TrimSpace(q.Get("place")),
		from:     validDate(q.Get("from")),
		to:       validDate(q.Get("to")),
		platform: q.Get("platform"),
		country:  strings.ToUpper(q.Get("country")),
		newest:   q.Get("sort") != "oldest",
	}
	for _, m := range strings.Split(q.Get("months"), ",") {
		if n, err := strconv.Atoi(m); err == nil && n >= 1 && n <= 12 {
			sq.months = append(sq.months, n)
		}
	}
	return sq
}

func validDate(s string) string {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return ""
	}
	return s
}

func (q searchQuery) empty() bool {
	return q.text == "" && q.place == "" && q.from == "" && q.to == "" && len(q.months) == 0 &&
		q.platform == "" && q.country == ""
}

// ftsQuery turns user text into an FTS5 query: every word must match, as a
// prefix, so "athe" finds "Athens".
func ftsQuery(text string) string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '-')
	})
	parts := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, "'-")
		if w == "" {
			continue
		}
		q := `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
		// A code with dashes, such as a number plate, also matches its
		// form without them, which the index has for text in photos.
		if joined := strings.ReplaceAll(w, "-", ""); joined != w && library.Codes(w) != nil {
			q = `(` + q + ` OR "` + strings.ReplaceAll(joined, `"`, `""`) + `"*)`
		}
		parts = append(parts, q)
	}
	return strings.Join(parts, " ")
}

// memoryFilter builds the WHERE clause for memories.
func (q searchQuery) memoryFilter() (string, []any) {
	conds := []string{visible}
	var args []any
	match := ftsQuery(q.text)
	if place := ftsQuery(q.place); place != "" {
		placeMatch := "{place_name city region}: (" + place + ")"
		if match != "" {
			match = "(" + match + ") AND " + placeMatch
		} else {
			match = placeMatch
		}
	}
	if match != "" {
		conds = append(conds, `m.rowid IN (SELECT rowid FROM memories_fts WHERE memories_fts MATCH ?)`)
		args = append(args, match)
	}
	if q.from != "" {
		conds = append(conds, `m.local_date >= ?`)
		args = append(args, q.from)
	}
	if q.to != "" {
		conds = append(conds, `m.local_date <= ?`)
		args = append(args, q.to)
	}
	if len(q.months) > 0 {
		ph := make([]string, len(q.months))
		for i, mo := range q.months {
			ph[i] = "?"
			args = append(args, strconv.Itoa(100 + mo)[1:])
		}
		conds = append(conds, `substr(m.local_date, 6, 2) IN (`+strings.Join(ph, ",")+`)`)
	}
	if q.platform != "" {
		conds = append(conds, `m.platform = ?`)
		args = append(args, q.platform)
	}
	if q.country != "" {
		conds = append(conds, `m.country_code = ?`)
		args = append(args, q.country)
	}
	return strings.Join(conds, " AND "), args
}

// mediaHit is a photo or video in search results.
type mediaHit struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Overlay  bool    `json:"overlay,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	MemoryID string  `json:"memory_id"`
	Date     string  `json:"date"`
	Position int     `json:"position"`
}

// messageHit is a message in search results.
type messageHit struct {
	ID             int64  `json:"id"`
	ConversationID string `json:"conversation_id"`
	Platform       string `json:"platform"`
	Title          string `json:"title"`
	Sender         string `json:"sender"`
	Self           bool   `json:"self"`
	At             int64  `json:"at"`
	// Snippet marks matches with \u0002 and \u0003.
	Snippet string `json:"snippet"`
}

type searchCounts struct {
	All      int `json:"all"`
	Photos   int `json:"photos"`
	Videos   int `json:"videos"`
	Posts    int `json:"posts"`
	Messages int `json:"messages"`
	Checkins int `json:"checkins"`
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	q := parseSearch(r)
	typ := r.URL.Query().Get("type")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	offset = max(offset, 0)
	resp := map[string]any{"counts": searchCounts{}, "media": []mediaHit{}, "posts": []*card{}, "messages": []messageHit{}, "checkins": []*card{}}
	if q.empty() {
		writeJSON(w, http.StatusOK, resp)
		return nil
	}
	where, args := q.memoryFilter()
	order := "DESC"
	if !q.newest {
		order = "ASC"
	}

	var counts searchCounts
	err := s.store.R.QueryRowContext(ctx, `SELECT COUNT(*),
			COALESCE(SUM(m.photo_count), 0), COALESCE(SUM(m.video_count), 0),
			COALESCE(SUM(m.text != '' AND m.kind != 'checkin'), 0), COALESCE(SUM(m.kind = 'checkin'), 0)
		FROM memories m WHERE `+where, args...).Scan(&counts.All, &counts.Photos, &counts.Videos, &counts.Posts, &counts.Checkins)
	if err != nil {
		return err
	}
	msgWhere, msgArgs, msgOK := s.messageFilter(q)
	if msgOK {
		s.store.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages msg JOIN conversations c ON c.id = msg.conversation_id
			WHERE `+msgWhere, msgArgs...).Scan(&counts.Messages)
	}
	counts.All += counts.Messages
	resp["counts"] = counts

	limits := map[string]int{"media": 12, "posts": 10, "messages": 5, "checkins": 5}
	offsets := map[string]int{}
	if typ != "" && typ != "all" {
		for k := range limits {
			limits[k] = 0
		}
		key := typ
		if typ == "photos" || typ == "videos" {
			key = "media"
		}
		limits[key] = limit
		offsets[key] = offset
	}

	if n := limits["media"]; n > 0 {
		kinds := "'photo', 'video'"
		switch typ {
		case "photos":
			kinds = "'photo'"
		case "videos":
			kinds = "'video'"
		}
		rows, err := s.store.R.QueryContext(ctx, `SELECT md.id, md.kind, md.overlay != '', md.duration, m.id, m.local_date, md.position
			FROM memories m JOIN media md ON md.memory_id = m.id
			WHERE `+where+` AND md.kind IN (`+kinds+`)
			ORDER BY m.taken_at `+order+`, md.position LIMIT ? OFFSET ?`, append(args, n, offsets["media"])...)
		if err != nil {
			return err
		}
		hits := []mediaHit{}
		for rows.Next() {
			var h mediaHit
			if err := rows.Scan(&h.ID, &h.Kind, &h.Overlay, &h.Duration, &h.MemoryID, &h.Date, &h.Position); err != nil {
				rows.Close()
				return err
			}
			hits = append(hits, h)
		}
		rows.Close()
		resp["media"] = hits
	}
	if n := limits["posts"]; n > 0 {
		list, err := s.cards(ctx, 5, `SELECT `+cardColumns+` FROM memories m WHERE `+where+` AND m.text != '' AND m.kind != 'checkin'
			ORDER BY m.taken_at `+order+` LIMIT ? OFFSET ?`, append(args, n, offsets["posts"])...)
		if err != nil {
			return err
		}
		if list != nil {
			resp["posts"] = list
		}
	}
	if n := limits["checkins"]; n > 0 {
		list, err := s.cards(ctx, 1, `SELECT `+cardColumns+` FROM memories m WHERE `+where+` AND m.kind = 'checkin'
			ORDER BY m.taken_at `+order+` LIMIT ? OFFSET ?`, append(args, n, offsets["checkins"])...)
		if err != nil {
			return err
		}
		if list != nil {
			resp["checkins"] = list
		}
	}
	if n := limits["messages"]; n > 0 && msgOK {
		hits, err := s.searchMessages(ctx, msgWhere, msgArgs, order, n, offsets["messages"])
		if err != nil {
			return err
		}
		resp["messages"] = hits
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

// messageFilter builds the WHERE clause for messages. Messages are only
// searched for text, since they have no places.
func (s *Server) messageFilter(q searchQuery) (string, []any, bool) {
	match := ftsQuery(q.text)
	if match == "" || q.place != "" || q.country != "" {
		return "", nil, false
	}
	conds := []string{`c.vault = 0`, `msg.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)`}
	args := []any{match}
	loc := s.deps.Library.Location("")
	if q.from != "" {
		t, _ := time.ParseInLocation("2006-01-02", q.from, loc)
		conds = append(conds, `msg.sent_at >= ?`)
		args = append(args, t.UnixMilli())
	}
	if q.to != "" {
		t, _ := time.ParseInLocation("2006-01-02", q.to, loc)
		conds = append(conds, `msg.sent_at < ?`)
		args = append(args, t.AddDate(0, 0, 1).UnixMilli())
	}
	if q.platform != "" {
		conds = append(conds, `c.platform = ?`)
		args = append(args, q.platform)
	}
	if len(q.months) > 0 {
		// Months without a year are matched in the server's time zone.
		_, off := time.Now().In(loc).Zone()
		ph := make([]string, len(q.months))
		for i, mo := range q.months {
			ph[i] = "?"
			args = append(args, strconv.Itoa(100 + mo)[1:])
		}
		conds = append(conds, `strftime('%m', (msg.sent_at / 1000) + `+strconv.Itoa(off)+`, 'unixepoch') IN (`+strings.Join(ph, ",")+`)`)
	}
	return strings.Join(conds, " AND "), args, true
}

func (s *Server) searchMessages(ctx context.Context, where string, args []any, order string, limit, offset int) ([]messageHit, error) {
	// snippet() needs the FTS table in the FROM clause.
	rows, err := s.store.R.QueryContext(ctx, `SELECT msg.id, c.id, c.platform, c.title, msg.sender, msg.is_self, msg.sent_at,
			snippet(messages_fts, 0, char(2), char(3), '…', 16)
		FROM messages_fts JOIN messages msg ON msg.id = messages_fts.rowid JOIN conversations c ON c.id = msg.conversation_id
		WHERE messages_fts MATCH ? AND `+where+`
		ORDER BY msg.sent_at `+order+` LIMIT ? OFFSET ?`, append(append([]any{args[0]}, args...), limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []messageHit{}
	for rows.Next() {
		var h messageHit
		if err := rows.Scan(&h.ID, &h.ConversationID, &h.Platform, &h.Title, &h.Sender, &h.Self, &h.At, &h.Snippet); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
