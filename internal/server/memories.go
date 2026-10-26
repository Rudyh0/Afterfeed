package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/store"
)

func (s *Server) memoryRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/day/{md}", s.authed(s.day, false))
	m.HandleFunc("GET /api/years", s.authed(s.years, false))
	m.HandleFunc("GET /api/years/{year}", s.authed(s.year, false))
	m.HandleFunc("GET /api/years/{year}/{month}", s.authed(s.month, false))
	m.HandleFunc("GET /api/memories/{id}", s.authed(s.memory, false))
	m.HandleFunc("GET /api/search", s.authed(s.search, false))
	m.HandleFunc("POST /api/memories", s.authed(s.createMemory, false))
	m.HandleFunc("DELETE /api/memories/{id}", s.authed(s.deleteMemory, false))
	m.HandleFunc("GET /api/memories/{id}/download", s.authed(s.downloadMemory, false))
	m.HandleFunc("POST /api/downloads", s.authed(s.createDownload, false))
	m.HandleFunc("GET /api/downloads/{token}", s.authed(s.serveDownload, false))
}

// cardMedia is a photo or video as shown in lists.
type cardMedia struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Mime     string  `json:"mime"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Duration float64 `json:"duration,omitempty"`
	Overlay  bool    `json:"overlay,omitempty"`
	Text     string  `json:"text,omitempty"`
}

type placeJSON struct {
	Name        string   `json:"name,omitempty"`
	City        string   `json:"city,omitempty"`
	Region      string   `json:"region,omitempty"`
	CountryCode string   `json:"country_code,omitempty"`
	Lat         *float64 `json:"lat,omitempty"`
	Lon         *float64 `json:"lon,omitempty"`
}

// card is a memory as shown in lists: the day view, a month, search results.
type card struct {
	ID       string `json:"id"`
	Platform string `json:"platform"`
	Kind     string `json:"kind"`
	TakenAt  string `json:"taken_at"`
	// Date and Time are local to where the memory happened.
	Date       string      `json:"date"`
	Time       string      `json:"time"`
	Year       int         `json:"year"`
	TZ         string      `json:"tz,omitempty"`
	Text       string      `json:"text,omitempty"`
	Link       string      `json:"link,omitempty"`
	Album      string      `json:"album,omitempty"`
	Place      *placeJSON  `json:"place,omitempty"`
	Media      []cardMedia `json:"media"`
	MediaCount int         `json:"media_count"`
	PhotoCount int         `json:"photo_count"`
	VideoCount int         `json:"video_count"`
	Vault      bool        `json:"vault,omitempty"`
	Edited     bool        `json:"edited,omitempty"`
}

const cardColumns = `m.id, m.platform, m.kind, m.taken_at, COALESCE(m.tz, ''), m.local_date, m.year, m.text, m.link, m.album,
	m.place_name, m.city, m.region, m.country_code, m.lat, m.lon, m.media_count, m.photo_count, m.video_count, m.vault, m.edited`

// visible is the condition for memories shown outside the vault.
const visible = `m.vault = 0 AND m.duplicate_of IS NULL`

// cards runs a query that selects cardColumns and returns the cards with up
// to mediaLimit media each.
func (s *Server) cards(ctx context.Context, mediaLimit int, query string, args ...any) ([]*card, error) {
	rows, err := s.store.R.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var out []*card
	byID := map[string]*card{}
	for rows.Next() {
		c := &card{Media: []cardMedia{}}
		var takenAt string
		var p placeJSON
		var lat, lon sql.NullFloat64
		if err := rows.Scan(&c.ID, &c.Platform, &c.Kind, &takenAt, &c.TZ, &c.Date, &c.Year, &c.Text, &c.Link, &c.Album,
			&p.Name, &p.City, &p.Region, &p.CountryCode, &lat, &lon, &c.MediaCount, &c.PhotoCount, &c.VideoCount,
			&c.Vault, &c.Edited); err != nil {
			rows.Close()
			return nil, err
		}
		t, _ := store.ParseTime(takenAt)
		local := t.In(s.deps.Library.Location(c.TZ))
		c.TakenAt = t.Format(time.RFC3339)
		c.Time = local.Format("15:04")
		if lat.Valid && lon.Valid {
			p.Lat, p.Lon = &lat.Float64, &lon.Float64
		}
		if p.Name != "" || p.City != "" || p.Lat != nil {
			c.Place = &p
		}
		out = append(out, c)
		byID[c.ID] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 || mediaLimit == 0 {
		return out, nil
	}
	// Media for all cards in one query, in chunks to stay below SQLite's
	// limit on parameters.
	for start := 0; start < len(out); start += 500 {
		end := min(start+500, len(out))
		ids := make([]any, 0, end-start)
		for _, c := range out[start:end] {
			ids = append(ids, c.ID)
		}
		mrows, err := s.store.R.QueryContext(ctx, `SELECT memory_id, id, kind, mime, width, height, duration, overlay != '', text
			FROM media WHERE memory_id IN (`+placeholders(len(ids))+`) ORDER BY memory_id, position`, ids...)
		if err != nil {
			return nil, err
		}
		for mrows.Next() {
			var memID string
			var md cardMedia
			if err := mrows.Scan(&memID, &md.ID, &md.Kind, &md.Mime, &md.Width, &md.Height, &md.Duration, &md.Overlay, &md.Text); err != nil {
				mrows.Close()
				return nil, err
			}
			if c := byID[memID]; c != nil && (mediaLimit < 0 || len(c.Media) < mediaLimit) {
				c.Media = append(c.Media, md)
			}
		}
		mrows.Close()
	}
	return out, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

var monthDay = regexp.MustCompile(`^(\d{2})-(\d{2})$`)

// day returns the memories of one calendar day across all years.
func (s *Server) day(w http.ResponseWriter, r *http.Request) error {
	md := r.PathValue("md")
	m := monthDay.FindStringSubmatch(md)
	if m == nil {
		return errBadRequest("invalid_date", "use MM-DD")
	}
	month, _ := strconv.Atoi(m[1])
	dayNum, _ := strconv.Atoi(m[2])
	// 2000 is a leap year, so 29 February is valid.
	if _, err := time.Parse("2006-01-02", fmt.Sprintf("2000-%s", md)); err != nil || month < 1 || dayNum < 1 {
		return errBadRequest("invalid_date", "no such day")
	}
	ctx := r.Context()
	list, err := s.cards(ctx, 5, `SELECT `+cardColumns+` FROM memories m
		WHERE m.month_day = ? AND `+visible+` ORDER BY m.year DESC, m.taken_at ASC`, md)
	if err != nil {
		return err
	}
	if list == nil {
		list = []*card{}
	}
	resp := map[string]any{"date": md, "memories": list}
	if len(list) == 0 {
		// Point to the nearest days that do have memories.
		var prev, next sql.NullString
		s.store.R.QueryRowContext(ctx, `SELECT COALESCE(
				(SELECT MAX(month_day) FROM memories m WHERE month_day < ? AND `+visible+`),
				(SELECT MAX(month_day) FROM memories m WHERE `+visible+`))`, md).Scan(&prev)
		s.store.R.QueryRowContext(ctx, `SELECT COALESCE(
				(SELECT MIN(month_day) FROM memories m WHERE month_day > ? AND `+visible+`),
				(SELECT MIN(month_day) FROM memories m WHERE `+visible+`))`, md).Scan(&next)
		resp["previous_with_memories"] = prev.String
		resp["next_with_memories"] = next.String
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

type cover struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Overlay bool   `json:"overlay,omitempty"`
}

type monthJSON struct {
	Month    int     `json:"month"`
	Memories int     `json:"memories"`
	Photos   int     `json:"photos"`
	Videos   int     `json:"videos"`
	Covers   []cover `json:"covers"`
}

type yearJSON struct {
	Year     int     `json:"year"`
	Memories int     `json:"memories"`
	Photos   int     `json:"photos"`
	Videos   int     `json:"videos"`
	Covers   []cover `json:"covers,omitempty"`
}

// covers picks photos to represent each month (or year): the first photo
// of the memories with the most photos, a few per group.
func (s *Server) covers(ctx context.Context, perGroup int, byMonth bool, where string, args ...any) (map[[2]int][]cover, error) {
	group := "m.year"
	if byMonth {
		group = "m.year, CAST(substr(m.local_date, 6, 2) AS INTEGER)"
	}
	rows, err := s.store.R.QueryContext(ctx, `SELECT y, mo, id, kind, overlay FROM (
			SELECT m.year AS y, CAST(substr(m.local_date, 6, 2) AS INTEGER) AS mo, md.id, md.kind, md.overlay != '' AS overlay,
				ROW_NUMBER() OVER (PARTITION BY `+group+` ORDER BY m.photo_count + m.video_count DESC, m.taken_at) AS rn
			FROM memories m JOIN media md ON md.memory_id = m.id AND md.position = 0
			WHERE `+visible+` AND md.kind IN ('photo', 'video') AND `+where+`
		) WHERE rn <= ? ORDER BY y, mo`, append(args, perGroup)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[[2]int][]cover{}
	for rows.Next() {
		var y, mo int
		var c cover
		if err := rows.Scan(&y, &mo, &c.ID, &c.Kind, &c.Overlay); err != nil {
			return nil, err
		}
		key := [2]int{y, 0}
		if byMonth {
			key[1] = mo
		}
		out[key] = append(out[key], c)
	}
	return out, rows.Err()
}

func (s *Server) yearList(ctx context.Context) ([]yearJSON, error) {
	rows, err := s.store.R.QueryContext(ctx, `SELECT year, SUM(memories), SUM(photos), SUM(videos)
		FROM month_stats GROUP BY year ORDER BY year`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []yearJSON{}
	for rows.Next() {
		var y yearJSON
		if err := rows.Scan(&y.Year, &y.Memories, &y.Photos, &y.Videos); err != nil {
			return nil, err
		}
		out = append(out, y)
	}
	return out, rows.Err()
}

// years returns every year with its totals and a few covers.
func (s *Server) years(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	list, err := s.yearList(ctx)
	if err != nil {
		return err
	}
	covers, err := s.covers(ctx, 4, false, "1 = 1")
	if err != nil {
		return err
	}
	for i := range list {
		list[i].Covers = covers[[2]int{list[i].Year, 0}]
	}
	writeJSON(w, http.StatusOK, map[string]any{"years": list})
	return nil
}

// year returns the months of a year with their totals and covers.
func (s *Server) year(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	y, err := strconv.Atoi(r.PathValue("year"))
	if err != nil {
		return errNotFound()
	}
	all, err := s.yearList(ctx)
	if err != nil {
		return err
	}
	months := make([]monthJSON, 12)
	for i := range months {
		months[i] = monthJSON{Month: i + 1, Covers: []cover{}}
	}
	rows, err := s.store.R.QueryContext(ctx, `SELECT month, memories, photos, videos FROM month_stats WHERE year = ?`, y)
	if err != nil {
		return err
	}
	total := yearJSON{Year: y}
	for rows.Next() {
		var mo monthJSON
		if err := rows.Scan(&mo.Month, &mo.Memories, &mo.Photos, &mo.Videos); err != nil {
			rows.Close()
			return err
		}
		if mo.Month >= 1 && mo.Month <= 12 {
			mo.Covers = []cover{}
			months[mo.Month-1] = mo
			total.Memories += mo.Memories
			total.Photos += mo.Photos
			total.Videos += mo.Videos
		}
	}
	rows.Close()
	covers, err := s.covers(ctx, 4, true, "m.year = ?", y)
	if err != nil {
		return err
	}
	for i := range months {
		if c := covers[[2]int{y, i + 1}]; c != nil {
			months[i].Covers = c
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"year": total, "months": months, "years": all})
	return nil
}

// month returns the memories of one month, oldest first.
func (s *Server) month(w http.ResponseWriter, r *http.Request) error {
	y, err1 := strconv.Atoi(r.PathValue("year"))
	mo, err2 := strconv.Atoi(r.PathValue("month"))
	if err1 != nil || err2 != nil || mo < 1 || mo > 12 {
		return errNotFound()
	}
	prefix := fmt.Sprintf("%04d-%02d-", y, mo)
	list, err := s.cards(r.Context(), 5, `SELECT `+cardColumns+` FROM memories m
		WHERE m.year = ? AND m.local_date >= ? AND m.local_date < ? AND `+visible+`
		ORDER BY m.taken_at ASC`, y, prefix+"00", prefix+"99")
	if err != nil {
		return err
	}
	if list == nil {
		list = []*card{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"year": y, "month": mo, "memories": list})
	return nil
}

// memoryDetail is a memory with everything the detail view shows.
type memoryDetail struct {
	*card
	Media    []detailMedia `json:"media"`
	Source   sourceJSON    `json:"source"`
	Original *originalJSON `json:"original,omitempty"`
	// ServerTZ is the time zone used for memories without one.
	ServerTZ string `json:"server_tz"`
}

// originalJSON is how a memory was before it was edited.
type originalJSON struct {
	Date  string     `json:"date"`
	Time  string     `json:"time"`
	TZ    string     `json:"tz,omitempty"`
	Text  string     `json:"text"`
	Place *placeJSON `json:"place,omitempty"`
}

type detailMedia struct {
	cardMedia
	File   string `json:"file"`
	Size   int64  `json:"size"`
	Source string `json:"source,omitempty"`
}

type sourceJSON struct {
	Platform   string `json:"platform"`
	ImportedAt string `json:"imported_at,omitempty"`
	Path       string `json:"path,omitempty"`
}

func (s *Server) loadCard(ctx context.Context, id string) (*card, error) {
	list, err := s.cards(ctx, 0, `SELECT `+cardColumns+` FROM memories m WHERE m.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errNotFound()
	}
	return list[0], nil
}

func (s *Server) memory(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	c, err := s.loadCard(ctx, r.PathValue("id"))
	if err != nil {
		return err
	}
	if c.Vault {
		if !s.vaultOpen(r) {
			return errNotFound()
		}
		s.deps.Auth.TouchVault(ctx, sessionFrom(ctx))
	}
	var rel string
	var importID sql.NullInt64
	s.store.R.QueryRowContext(ctx, `SELECT path, import_id FROM memories WHERE id = ?`, c.ID).Scan(&rel, &importID)
	mem, err := s.deps.Library.LoadMemory(rel)
	if err != nil {
		return err
	}
	d := memoryDetail{card: c, Media: []detailMedia{}, ServerTZ: s.deps.Library.Location("").String()}
	if o := mem.Original; o != nil {
		local := o.TakenAt.In(s.deps.Library.Location(o.TZ))
		d.Original = &originalJSON{Date: local.Format("2006-01-02"), Time: local.Format("15:04"), TZ: o.TZ, Text: o.Text}
		if p := o.Place; p != nil {
			d.Original.Place = &placeJSON{Name: p.Name, City: p.City, Region: p.Region, CountryCode: p.CountryCode, Lat: p.Lat, Lon: p.Lon}
		}
	}
	for _, md := range mem.Media {
		d.Media = append(d.Media, detailMedia{
			cardMedia: cardMedia{ID: md.ID, Kind: md.Kind, Mime: md.Mime, Width: md.Width, Height: md.Height,
				Duration: md.Duration, Overlay: md.Overlay != "", Text: md.Text},
			File:   md.File,
			Size:   md.Size,
			Source: md.Source,
		})
	}
	d.Source = sourceJSON{Platform: mem.Platform, Path: mem.Source.Path}
	if importID.Valid {
		var finished sql.NullString
		s.store.R.QueryRowContext(ctx, `SELECT COALESCE(finished_at, created_at) FROM imports WHERE id = ?`, importID.Int64).Scan(&finished)
		d.Source.ImportedAt = finished.String
	}
	if d.Source.ImportedAt == "" {
		d.Source.ImportedAt = store.FormatTime(mem.CreatedAt)
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id := r.PathValue("id")
	var rel, platform string
	var vault bool
	err := s.store.R.QueryRowContext(ctx, `SELECT path, platform, vault FROM memories WHERE id = ?`, id).Scan(&rel, &platform, &vault)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound()
	}
	if err != nil {
		return err
	}
	if vault && !s.vaultOpen(r) {
		return errNotFound()
	}
	// Imported memories stay: importing again would bring them back.
	if platform != "afterfeed" {
		return errForbidden("memory.not_own")
	}
	var mediaIDs []string
	if rows, err := s.store.R.QueryContext(ctx, `SELECT id FROM media WHERE memory_id = ?`, id); err == nil {
		for rows.Next() {
			var mid string
			if rows.Scan(&mid) == nil {
				mediaIDs = append(mediaIDs, mid)
			}
		}
		rows.Close()
	}
	if err := s.deps.Library.DeleteMemory(ctx, id, rel); err != nil {
		return err
	}
	for _, mid := range mediaIDs {
		s.deps.Media.Remove(mid)
	}
	writeJSON(w, http.StatusOK, map[string]any{})
	return nil
}
