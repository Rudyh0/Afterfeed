// Package library stores memories and conversations on the data volume and
// keeps the database index in sync with them.
//
// Every memory is a folder with its media files and a memory.json file, and
// every conversation is a folder with a conversation.json file and its
// attachments. Those files are the source of truth; the database is an index
// that can be rebuilt from them.
package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/rudyh0/afterfeed/internal/store"
)

// FormatVersion is the version of the memory.json and conversation.json formats.
const FormatVersion = 1

// Memory is the content of memory.json.
type Memory struct {
	Version  int    `json:"version"`
	ID       string `json:"id"`
	Platform string `json:"platform"`
	Kind     string `json:"kind"`
	Fields
	// Album is the name of the album on the platform, if any.
	Album string  `json:"album,omitempty"`
	Media []Media `json:"media"`
	Vault bool    `json:"vault,omitempty"`
	// Original holds the values from the export, set when a field is edited.
	Original  *Fields   `json:"original,omitempty"`
	Source    Source    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Fields are the values of a memory that can be edited.
type Fields struct {
	TakenAt time.Time `json:"taken_at"`
	// TZ is the IANA time zone the memory happened in. When it is empty the
	// server's time zone is used.
	TZ    string `json:"tz,omitempty"`
	Text  string `json:"text,omitempty"`
	Link  string `json:"link,omitempty"`
	Place *Place `json:"place,omitempty"`
}

// Place is where a memory happened.
type Place struct {
	Name        string   `json:"name,omitempty"`
	Address     string   `json:"address,omitempty"`
	Lat         *float64 `json:"lat,omitempty"`
	Lon         *float64 `json:"lon,omitempty"`
	CountryCode string   `json:"country_code,omitempty"`
	Region      string   `json:"region,omitempty"`
	City        string   `json:"city,omitempty"`
}

// HasCoordinates reports whether the place has a latitude and longitude.
func (p *Place) HasCoordinates() bool { return p != nil && p.Lat != nil && p.Lon != nil }

// Source records where a memory came from.
type Source struct {
	// Key identifies the item on its platform and stays the same across
	// exports, so importing again does not create duplicates.
	Key string `json:"key"`
	// Path is the file in the export the memory was read from.
	Path   string `json:"path,omitempty"`
	Import int64  `json:"import,omitempty"`
}

// Media is one photo or video of a memory.
type Media struct {
	ID string `json:"id"`
	// Key identifies the file in the export, usually its file name.
	Key      string     `json:"key"`
	File     string     `json:"file"`
	Kind     string     `json:"kind"`
	Mime     string     `json:"mime"`
	Size     int64      `json:"size"`
	SHA256   string     `json:"sha256"`
	Width    int        `json:"width,omitempty"`
	Height   int        `json:"height,omitempty"`
	Duration float64    `json:"duration,omitempty"`
	TakenAt  *time.Time `json:"taken_at,omitempty"`
	Lat      *float64   `json:"lat,omitempty"`
	Lon      *float64   `json:"lon,omitempty"`
	Text     string     `json:"text,omitempty"`
	// Overlay is a transparent image shown over the media, such as a
	// Snapchat caption or sticker.
	Overlay string `json:"overlay,omitempty"`
	// Source is the path of the file in the export.
	Source string `json:"source,omitempty"`
	// OCR is the text recognised in the photo or video frame. It is nil
	// until the file has been looked at, and empty when it has no text.
	OCR *string `json:"ocr,omitempty"`
	// OCRVersion is the version of the reader that read OCR. Media read by
	// an older reader are read again.
	OCRVersion int `json:"ocr_version,omitempty"`
}

// OCRText joins the text recognised in all media of a memory, for the
// search index. Codes written with dashes, dots or a space, such as number
// plates and postcodes, are added without them too, so that "GX707Z" finds
// "GX-707-Z" and a plate read as "GX-707Z".
func (m *Memory) OCRText() string {
	var parts []string
	for _, md := range m.Media {
		if md.OCR != nil && *md.OCR != "" {
			parts = append(parts, *md.OCR)
		}
	}
	text := strings.Join(parts, "\n")
	if codes := Codes(text); len(codes) > 0 {
		text += "\n" + strings.Join(codes, " ")
	}
	return text
}

var (
	codePattern     = regexp.MustCompile(`\b[A-Za-z0-9]{1,4}(?:[-.][A-Za-z0-9]{1,4}){1,3}\b`)
	postcodePattern = regexp.MustCompile(`\b[0-9]{4} [A-Za-z]{2}\b`)
)

// Codes returns the codes in a text with letters and digits written in
// parts, such as "GX-707-Z" or "9351 AB", joined: "GX707Z", "9351AB".
func Codes(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range append(codePattern.FindAllString(text, -1), postcodePattern.FindAllString(text, -1)...) {
		joined := strings.NewReplacer("-", "", ".", "", " ", "").Replace(m)
		hasDigit := strings.ContainsAny(joined, "0123456789")
		hasLetter := strings.IndexFunc(joined, unicode.IsLetter) >= 0
		if !hasDigit || !hasLetter || len(joined) < 4 || len(joined) > 10 || seen[joined] {
			continue
		}
		seen[joined] = true
		out = append(out, joined)
	}
	return out
}

// HashID derives a stable identifier from the given parts.
func HashID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:8])
}

// MemoryID returns the identifier of the memory with this source key.
func MemoryID(sourceKey string) string { return HashID("memory", sourceKey) }

// MediaID returns the identifier of a media file of a memory.
func MediaID(memoryID, mediaKey string) string { return HashID("media", memoryID, mediaKey) }

// Library manages the files on the data volume and their index.
type Library struct {
	memories string
	messages string
	st       *store.Store
	defLoc   *time.Location
	geocode  Geocoder
	// locks serialise changes to one memory.json: the editor, the importer
	// and text recognition each read, change and write it.
	locks [64]sync.Mutex
}

// LockMemory keeps others from changing a memory until unlock is called.
// Hold it from loading memory.json until it is saved and indexed.
func (l *Library) LockMemory(rel string) (unlock func()) {
	h := fnv.New32a()
	h.Write([]byte(rel))
	mu := &l.locks[h.Sum32()%uint32(len(l.locks))]
	mu.Lock()
	return mu.Unlock
}

// Geocoder finds the city and country for coordinates.
type Geocoder func(lat, lon float64) (city, countryCode string, ok bool)

// SetGeocoder sets the function used to name places.
func (l *Library) SetGeocoder(g Geocoder) { l.geocode = g }

// FillPlace adds the city and country to a place that only has coordinates.
func (l *Library) FillPlace(p *Place) {
	if l.geocode == nil || !p.HasCoordinates() || p.City != "" || p.CountryCode != "" {
		return
	}
	if city, cc, ok := l.geocode(*p.Lat, *p.Lon); ok {
		p.City, p.CountryCode = city, cc
	}
}

// PlaceFromMedia gives a memory without a place the location of its first
// photo or video that has one.
func PlaceFromMedia(m *Memory) {
	if m.Place != nil {
		return
	}
	for _, md := range m.Media {
		if md.Lat != nil && md.Lon != nil {
			lat, lon := *md.Lat, *md.Lon
			m.Place = &Place{Lat: &lat, Lon: &lon}
			return
		}
	}
}

// New creates a library rooted at the memories and messages folders.
func New(memoriesDir, messagesDir string, st *store.Store) *Library {
	return &Library{memories: memoriesDir, messages: messagesDir, st: st, defLoc: time.Local}
}

// Store returns the database.
func (l *Library) Store() *store.Store { return l.st }

// SetDefaultLocation sets the time zone for memories without one. It is the
// server's time zone unless changed.
func (l *Library) SetDefaultLocation(loc *time.Location) { l.defLoc = loc }

// Location returns the time zone a memory is shown in.
func (l *Library) Location(tz string) *time.Location {
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return l.defLoc
}

// MemoryDir returns the absolute path of a memory folder.
func (l *Library) MemoryDir(rel string) string {
	return filepath.Join(l.memories, filepath.FromSlash(rel))
}

// MemoryRel returns the folder of a memory, relative to the memories
// folder. It depends only on the time, platform and ID, so an import that
// was interrupted finds the same folder again.
func (l *Library) MemoryRel(m *Memory) string {
	local := m.TakenAt.In(l.Location(m.TZ))
	name := fmt.Sprintf("%s-%s-%s", local.Format("20060102-150405"), m.Platform, m.ID[:8])
	return local.Format("2006") + "/" + name
}

// CreateMemoryDir creates a new folder for a memory, named after its date,
// platform and id, and returns its path relative to the memories folder.
func (l *Library) CreateMemoryDir(m *Memory) (string, error) {
	rel := l.MemoryRel(m)
	if err := os.MkdirAll(l.MemoryDir(rel), 0o755); err != nil {
		return "", err
	}
	return rel, nil
}

// SaveMemory writes memory.json atomically.
func (l *Library) SaveMemory(rel string, m *Memory) error {
	m.Version = FormatVersion
	return writeJSONFile(filepath.Join(l.MemoryDir(rel), "memory.json"), m)
}

// LoadMemory reads memory.json.
func (l *Library) LoadMemory(rel string) (*Memory, error) {
	var m Memory
	if err := readJSONFile(filepath.Join(l.MemoryDir(rel), "memory.json"), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// IndexMemory adds or updates a memory and its media in the index.
func (l *Library) IndexMemory(ctx context.Context, tx *sql.Tx, rel string, m *Memory, importID int64) error {
	local := m.TakenAt.In(l.Location(m.TZ))
	var lat, lon any
	place := m.Place
	if place == nil {
		place = &Place{}
	}
	if place.HasCoordinates() {
		lat, lon = *place.Lat, *place.Lon
	} else {
		for _, md := range m.Media {
			if md.Lat != nil && md.Lon != nil {
				lat, lon = *md.Lat, *md.Lon
				break
			}
		}
	}
	photos, videos := 0, 0
	for _, md := range m.Media {
		switch md.Kind {
		case "photo":
			photos++
		case "video":
			videos++
		}
	}
	var imp any
	if importID > 0 {
		imp = importID
	}
	edited := 0
	if m.Original != nil {
		edited = 1
	}
	now := store.Now()
	_, err := tx.ExecContext(ctx, `INSERT INTO memories (
			id, source_key, platform, kind, taken_at, tz, local_date, month_day, year,
			text, link, album, place_name, lat, lon, country_code, region, city,
			media_count, photo_count, video_count, vault, edited, import_id, path, created_at, updated_at, ocr_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind, taken_at = excluded.taken_at, tz = excluded.tz,
			local_date = excluded.local_date, month_day = excluded.month_day, year = excluded.year,
			text = excluded.text, link = excluded.link, album = excluded.album, place_name = excluded.place_name,
			lat = excluded.lat, lon = excluded.lon, country_code = excluded.country_code,
			region = excluded.region, city = excluded.city, media_count = excluded.media_count,
			photo_count = excluded.photo_count, video_count = excluded.video_count,
			vault = excluded.vault, edited = excluded.edited, path = excluded.path,
			import_id = COALESCE(memories.import_id, excluded.import_id), updated_at = excluded.updated_at,
			ocr_text = excluded.ocr_text`,
		m.ID, m.Source.Key, m.Platform, m.Kind, store.FormatTime(m.TakenAt), nullIfEmpty(m.TZ),
		local.Format("2006-01-02"), local.Format("01-02"), local.Year(),
		m.Text, m.Link, m.Album, place.Name, lat, lon, place.CountryCode, place.Region, place.City,
		len(m.Media), photos, videos, boolInt(m.Vault), edited, imp, rel,
		store.FormatTime(m.CreatedAt), now, m.OCRText())
	if err != nil {
		return fmt.Errorf("index memory %s: %w", m.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media WHERE memory_id = ?`, m.ID); err != nil {
		return err
	}
	for i, md := range m.Media {
		var taken any
		if md.TakenAt != nil {
			taken = store.FormatTime(*md.TakenAt)
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO media (
				id, memory_id, position, media_key, kind, file, overlay, text, mime,
				width, height, duration, size, sha256, taken_at, lat, lon, ocr_text, ocr_version)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			md.ID, m.ID, i, md.Key, md.Kind, md.File, md.Overlay, md.Text, md.Mime,
			md.Width, md.Height, md.Duration, md.Size, md.SHA256, taken, md.Lat, md.Lon, md.OCR, md.OCRVersion)
		if err != nil {
			return fmt.Errorf("index media %s: %w", md.ID, err)
		}
	}
	return nil
}

// MemoryBySourceKey returns the id and folder of the memory with this source
// key, or empty strings when there is none.
func (l *Library) MemoryBySourceKey(ctx context.Context, key string) (id, rel string, err error) {
	err = l.st.R.QueryRowContext(ctx, `SELECT id, path FROM memories WHERE source_key = ?`, key).Scan(&id, &rel)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return id, rel, err
}

// RefreshMonthStats recalculates the number of memories per month.
func (l *Library) RefreshMonthStats(ctx context.Context) error {
	tx, err := l.st.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM month_stats`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO month_stats (year, month, memories, photos, videos)
		SELECT year, CAST(substr(local_date, 6, 2) AS INTEGER), COUNT(*), SUM(photo_count), SUM(video_count)
		FROM memories WHERE vault = 0 AND duplicate_of IS NULL
		GROUP BY 1, 2`); err != nil {
		return err
	}
	return tx.Commit()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// writeJSONFile writes v to path through a temporary file, so a crash never
// leaves a half-written file behind.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// DeleteMemory removes a memory folder and its index entries. Cached
// previews are removed by the caller, which knows the media ids.
func (l *Library) DeleteMemory(ctx context.Context, id, rel string) error {
	tx, err := l.st.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if rel != "" && !strings.Contains(rel, "..") {
		if err := os.RemoveAll(l.MemoryDir(rel)); err != nil {
			return err
		}
	}
	return l.RefreshMonthStats(ctx)
}

// GeocodeMissing names the places of memories that have coordinates but no
// city or country, for example memories imported before place names were
// available.
func (l *Library) GeocodeMissing(ctx context.Context) (int, error) {
	if l.geocode == nil {
		return 0, nil
	}
	rows, err := l.st.R.QueryContext(ctx, `SELECT path FROM memories
		WHERE lat IS NOT NULL AND city = '' AND country_code = ''`)
	if err != nil {
		return 0, err
	}
	var rels []string
	for rows.Next() {
		var rel string
		if rows.Scan(&rel) == nil {
			rels = append(rels, rel)
		}
	}
	rows.Close()
	n := 0
	b := newBatch(ctx, l.st.W, 200)
	for _, rel := range rels {
		if ctx.Err() != nil {
			break
		}
		m, changed, err := l.fillMissingPlace(rel)
		if err != nil {
			return n, err
		}
		if !changed {
			continue
		}
		rel := rel
		if err := b.add(func(tx *sql.Tx) error { return l.IndexMemory(ctx, tx, rel, m, 0) }); err != nil {
			return n, err
		}
		n++
	}
	return n, b.flush()
}

// fillMissingPlace names the place of one memory and saves it when that
// changed it.
func (l *Library) fillMissingPlace(rel string) (*Memory, bool, error) {
	defer l.LockMemory(rel)()
	m, err := l.LoadMemory(rel)
	if err != nil {
		return nil, false, nil
	}
	PlaceFromMedia(m)
	before := ""
	if m.Place != nil {
		before = m.Place.City + m.Place.CountryCode
	}
	l.FillPlace(m.Place)
	if m.Place == nil || m.Place.City+m.Place.CountryCode == before {
		return m, false, nil
	}
	return m, true, l.SaveMemory(rel, m)
}
