package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
)

func (s *Server) editRoutes() {
	m := s.mux
	m.HandleFunc("PATCH /api/memories/{id}", s.authed(s.editMemory, false))
	m.HandleFunc("POST /api/memories/bulk", s.authed(s.bulkEdit, false))
}

// placeInput is a place as the editor sends it.
type placeInput struct {
	Name string   `json:"name"`
	Lat  *float64 `json:"lat"`
	Lon  *float64 `json:"lon"`
}

func (p *placeInput) place() *library.Place {
	if p == nil {
		return nil
	}
	name := strings.TrimSpace(p.Name)
	if name == "" && (p.Lat == nil || p.Lon == nil) {
		return nil
	}
	pl := &library.Place{Name: name}
	if p.Lat != nil && p.Lon != nil && *p.Lat >= -90 && *p.Lat <= 90 && *p.Lon >= -180 && *p.Lon <= 180 {
		pl.Lat, pl.Lon = p.Lat, p.Lon
	}
	return pl
}

// editRequest changes fields of one memory. A field that is absent stays as
// it is; "place": null removes the place.
type editRequest struct {
	Date    *string          `json:"date"`
	Time    *string          `json:"time"`
	TZ      *string          `json:"tz"`
	Text    *string          `json:"text"`
	Place   *json.RawMessage `json:"place"`
	Restore []string         `json:"restore"`
	Vault   *bool            `json:"vault"`
}

// loadForEdit loads a memory the session may change. The memory stays
// locked until unlock is called, after it has been saved.
func (s *Server) loadForEdit(r *http.Request, id string) (m *library.Memory, rel string, unlock func(), err error) {
	var vault bool
	err = s.store.R.QueryRowContext(r.Context(), `SELECT path, vault FROM memories WHERE id = ?`, id).Scan(&rel, &vault)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil, errNotFound()
	}
	if err != nil {
		return nil, "", nil, err
	}
	if vault && !s.vaultOpen(r) {
		return nil, "", nil, errNotFound()
	}
	unlock = s.deps.Library.LockMemory(rel)
	m, err = s.deps.Library.LoadMemory(rel)
	if err != nil {
		unlock()
		return nil, "", nil, err
	}
	return m, rel, unlock, nil
}

// remember stores the values from the export before the first change.
func remember(m *library.Memory) {
	if m.Original == nil {
		o := m.Fields
		if m.Place != nil {
			p := *m.Place
			o.Place = &p
		}
		m.Original = &o
	}
}

// settle forgets the original values when every field is back to them.
func settle(m *library.Memory) {
	o := m.Original
	if o == nil {
		return
	}
	if o.TakenAt.Equal(m.TakenAt) && o.TZ == m.TZ && o.Text == m.Text && samePlace(o.Place, m.Place) {
		m.Original = nil
	}
}

func samePlace(a, b *library.Place) bool {
	if a == nil || b == nil {
		return a == b
	}
	eq := func(x, y *float64) bool { return (x == nil && y == nil) || (x != nil && y != nil && *x == *y) }
	return a.Name == b.Name && eq(a.Lat, b.Lat) && eq(a.Lon, b.Lon)
}

func (s *Server) editMemory(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req editRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	m, rel, unlock, err := s.loadForEdit(r, r.PathValue("id"))
	if err != nil {
		return err
	}
	defer unlock()

	for _, f := range req.Restore {
		if m.Original == nil {
			break
		}
		switch f {
		case "taken_at":
			m.TakenAt, m.TZ = m.Original.TakenAt, m.Original.TZ
		case "text":
			m.Text = m.Original.Text
		case "place":
			m.Place = m.Original.Place
		}
	}

	if req.Date != nil || req.Time != nil || req.TZ != nil {
		tz := m.TZ
		if req.TZ != nil {
			tz = *req.TZ
		}
		loc := s.deps.Library.Location(tz)
		if tz != "" && loc.String() != tz {
			return errBadRequest("edit.invalid_tz", tz)
		}
		local := m.TakenAt.In(s.deps.Library.Location(m.TZ))
		date, clock := local.Format("2006-01-02"), local.Format("15:04")
		if req.Date != nil {
			date = *req.Date
		}
		if req.Time != nil {
			clock = *req.Time
		}
		t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, loc)
		if err != nil {
			return errBadRequest("edit.invalid_date", err.Error())
		}
		if !t.Equal(m.TakenAt) || tz != m.TZ {
			remember(m)
			m.TakenAt, m.TZ = t.UTC(), tz
		}
	}
	if req.Text != nil && strings.TrimSpace(*req.Text) != m.Text {
		remember(m)
		m.Text = strings.TrimSpace(*req.Text)
	}
	if req.Place != nil {
		var in *placeInput
		if string(*req.Place) != "null" {
			in = &placeInput{}
			if err := json.Unmarshal(*req.Place, in); err != nil {
				return errBadRequest("invalid_json", err.Error())
			}
		}
		p := in.place()
		if !samePlace(p, m.Place) {
			remember(m)
			s.deps.Library.FillPlace(p)
			m.Place = p
		}
	}
	if req.Vault != nil {
		if !*req.Vault && m.Vault && !s.vaultOpen(r) {
			return errForbidden("vault.locked")
		}
		m.Vault = *req.Vault
	}
	settle(m)
	if err := s.saveEdited(ctx, rel, m); err != nil {
		return err
	}
	if err := s.deps.Library.RefreshMonthStats(ctx); err != nil {
		return err
	}
	if m.Vault && !s.vaultOpen(r) {
		// Moved into a locked vault: the memory is no longer shown.
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return s.memory(w, r)
}

// saveEdited writes memory.json and the index.
func (s *Server) saveEdited(ctx context.Context, rel string, m *library.Memory) error {
	m.UpdatedAt = time.Now().UTC()
	if err := s.deps.Library.SaveMemory(rel, m); err != nil {
		return err
	}
	tx, err := s.store.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := s.deps.Library.IndexMemory(ctx, tx, rel, m, 0); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// bulkRequest changes several memories at once.
type bulkRequest struct {
	IDs   []string `json:"ids"`
	Place *struct {
		placeInput
		// Overwrite also changes memories that already have a place.
		Overwrite bool `json:"overwrite"`
	} `json:"place"`
	// ShiftMinutes moves every memory by the same amount of time.
	ShiftMinutes *int `json:"shift_minutes"`
	// Date, Time and TZ give every memory the same date and time.
	Date  *string `json:"date"`
	Time  *string `json:"time"`
	TZ    *string `json:"tz"`
	Vault *bool   `json:"vault"`
}

func (s *Server) bulkEdit(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var req bulkRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if len(req.IDs) == 0 || len(req.IDs) > 5000 {
		return errBadRequest("bulk.invalid", "select 1 to 5000 memories")
	}
	var setTime *time.Time
	tz := ""
	if req.Date != nil && req.Time != nil {
		if req.TZ != nil {
			tz = *req.TZ
		}
		t, err := time.ParseInLocation("2006-01-02 15:04", *req.Date+" "+*req.Time, s.deps.Library.Location(tz))
		if err != nil {
			return errBadRequest("edit.invalid_date", err.Error())
		}
		setTime = &t
	}
	if req.Vault != nil && !*req.Vault && !s.vaultOpen(r) {
		return errForbidden("vault.locked")
	}
	changed := 0
	for _, id := range req.IDs {
		ok, err := s.bulkEditOne(r, id, &req, setTime, tz)
		if err != nil {
			return err
		}
		if ok {
			changed++
		}
	}
	if err := s.deps.Library.RefreshMonthStats(ctx); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed})
	return nil
}

func memoryEqual(a, b *library.Memory) bool {
	return a.TakenAt.Equal(b.TakenAt) && a.TZ == b.TZ && a.Text == b.Text && samePlace(a.Place, b.Place) &&
		a.Vault == b.Vault && (a.Original == nil) == (b.Original == nil)
}

// bulkEditOne applies a bulk change to one memory and reports whether it
// changed. Memories that cannot be found are skipped.
func (s *Server) bulkEditOne(r *http.Request, id string, req *bulkRequest, setTime *time.Time, tz string) (bool, error) {
	m, rel, unlock, err := s.loadForEdit(r, id)
	if err != nil {
		return false, nil
	}
	defer unlock()
	before := *m
	if req.Place != nil && (m.Place == nil || !m.Place.HasCoordinates() || req.Place.Overwrite) {
		p := req.Place.place()
		if !samePlace(p, m.Place) {
			remember(m)
			s.deps.Library.FillPlace(p)
			m.Place = p
		}
	}
	if req.ShiftMinutes != nil && *req.ShiftMinutes != 0 {
		remember(m)
		m.TakenAt = m.TakenAt.Add(time.Duration(*req.ShiftMinutes) * time.Minute)
	} else if setTime != nil && !setTime.Equal(m.TakenAt) {
		remember(m)
		m.TakenAt, m.TZ = setTime.UTC(), tz
	}
	if req.Vault != nil {
		m.Vault = *req.Vault
	}
	settle(m)
	if before.UpdatedAt.Equal(m.UpdatedAt) && memoryEqual(&before, m) {
		return false, nil
	}
	return true, s.saveEdited(r.Context(), rel, m)
}
