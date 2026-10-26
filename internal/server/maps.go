package server

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/rudyh0/afterfeed/internal/geo"
)

func (s *Server) mapRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/map/clusters", s.authed(s.mapClusters, false))
	m.HandleFunc("GET /api/map/memories", s.authed(s.mapMemories, false))
	m.HandleFunc("GET /api/map/bounds", s.authed(s.mapBounds, false))
	m.HandleFunc("GET /api/geo/search", s.authed(s.geoSearch, false))
	m.HandleFunc("GET /api/geo/reverse", s.authed(s.geoReverse, false))
}

// bbox is a map area: west, south, east, north.
type bbox struct{ w, s, e, n float64 }

func parseBBox(v string) (bbox, bool) {
	parts := strings.Split(v, ",")
	if len(parts) != 4 {
		return bbox{}, false
	}
	var f [4]float64
	for i, p := range parts {
		x, err := strconv.ParseFloat(p, 64)
		if err != nil || math.IsNaN(x) {
			return bbox{}, false
		}
		f[i] = x
	}
	b := bbox{w: f[0], s: f[1], e: f[2], n: f[3]}
	if b.e-b.w >= 360 {
		b.w, b.e = -180, 180
	}
	b.w = wrapLon(b.w)
	b.e = wrapLon(b.e)
	b.s = math.Max(-90, b.s)
	b.n = math.Min(90, b.n)
	return b, true
}

func wrapLon(x float64) float64 {
	for x < -180 {
		x += 360
	}
	for x > 180 {
		x -= 360
	}
	return x
}

// mapFilter builds the condition for memories with a location, inside the
// box and matching the filters.
func mapFilter(r *http.Request) (string, []any) {
	conds := []string{visible, "m.lat IS NOT NULL", "m.lon IS NOT NULL"}
	var args []any
	q := r.URL.Query()
	if b, ok := parseBBox(q.Get("bbox")); ok {
		conds = append(conds, "m.lat BETWEEN ? AND ?")
		args = append(args, b.s, b.n)
		if b.w <= b.e {
			conds = append(conds, "m.lon BETWEEN ? AND ?")
			args = append(args, b.w, b.e)
		} else { // the area crosses the date line
			conds = append(conds, "(m.lon >= ? OR m.lon <= ?)")
			args = append(args, b.w, b.e)
		}
	}
	if y, err := strconv.Atoi(q.Get("year")); err == nil {
		conds = append(conds, "m.year = ?")
		args = append(args, y)
	}
	if p := q.Get("platform"); p != "" {
		conds = append(conds, "m.platform = ?")
		args = append(args, p)
	}
	if c := q.Get("country"); c != "" {
		conds = append(conds, "m.country_code = ?")
		args = append(args, strings.ToUpper(c))
	}
	return strings.Join(conds, " AND "), args
}

type cluster struct {
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Count    int     `json:"count"`
	MemoryID string  `json:"memory_id"`
	MediaID  string  `json:"media_id,omitempty"`
	Kind     string  `json:"kind,omitempty"`
}

// mapClusters groups memories in squares of about 60 pixels at the zoom
// level, so the map shows a manageable number of markers.
func (s *Server) mapClusters(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	zoom, _ := strconv.ParseFloat(r.URL.Query().Get("zoom"), 64)
	zoom = math.Max(0, math.Min(22, zoom))
	cellDeg := 360 / (512 * math.Pow(2, zoom)) * 60
	where, args := mapFilter(r)
	rows, err := s.store.R.QueryContext(ctx, `SELECT COUNT(*), AVG(m.lat), AVG(m.lon), MAX(m.photo_count + m.video_count), m.id
		FROM memories m WHERE `+where+`
		GROUP BY CAST((m.lat + 90) / ? AS INTEGER), CAST((m.lon + 180) / ? AS INTEGER)
		LIMIT 2000`, append(args, cellDeg, cellDeg)...)
	if err != nil {
		return err
	}
	out := []cluster{}
	for rows.Next() {
		var c cluster
		var media int
		if err := rows.Scan(&c.Count, &c.Lat, &c.Lon, &media, &c.MemoryID); err != nil {
			rows.Close()
			return err
		}
		out = append(out, c)
	}
	rows.Close()
	if err := s.coverMedia(ctx, out); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": out})
	return nil
}

// coverMedia adds the first photo or video of each cluster's memory.
func (s *Server) coverMedia(ctx context.Context, cs []cluster) error {
	if len(cs) == 0 {
		return nil
	}
	byID := map[string][]int{}
	ids := make([]any, 0, len(cs))
	for i, c := range cs {
		if _, ok := byID[c.MemoryID]; !ok {
			ids = append(ids, c.MemoryID)
		}
		byID[c.MemoryID] = append(byID[c.MemoryID], i)
	}
	rows, err := s.store.R.QueryContext(ctx, `SELECT memory_id, id, kind FROM media
		WHERE position = 0 AND kind IN ('photo', 'video') AND memory_id IN (`+placeholders(len(ids))+`)`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var mid, id, kind string
		if err := rows.Scan(&mid, &id, &kind); err != nil {
			return err
		}
		for _, i := range byID[mid] {
			cs[i].MediaID, cs[i].Kind = id, kind
		}
	}
	return rows.Err()
}

type placeGroup struct {
	City        string  `json:"city"`
	CountryCode string  `json:"country_code"`
	Count       int     `json:"count"`
	First       string  `json:"first"`
	Last        string  `json:"last"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
}

// mapMemories lists the memories in the visible area, with the places and
// years they are from.
func (s *Server) mapMemories(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	where, args := mapFilter(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var total int
	if err := s.store.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories m WHERE `+where, args...).Scan(&total); err != nil {
		return err
	}
	list, err := s.cards(ctx, 4, `SELECT `+cardColumns+` FROM memories m WHERE `+where+`
		ORDER BY m.taken_at DESC LIMIT ? OFFSET ?`, append(args, limit, max(0, offset))...)
	if err != nil {
		return err
	}
	if list == nil {
		list = []*card{}
	}
	resp := map[string]any{"total": total, "memories": list}
	if offset == 0 {
		rows, err := s.store.R.QueryContext(ctx, `SELECT m.city, m.country_code, COUNT(*), MIN(m.local_date), MAX(m.local_date),
				AVG(m.lat), AVG(m.lon)
			FROM memories m WHERE `+where+` GROUP BY m.city, m.country_code ORDER BY COUNT(*) DESC LIMIT 8`, args...)
		if err != nil {
			return err
		}
		places := []placeGroup{}
		for rows.Next() {
			var g placeGroup
			if err := rows.Scan(&g.City, &g.CountryCode, &g.Count, &g.First, &g.Last, &g.Lat, &g.Lon); err != nil {
				rows.Close()
				return err
			}
			places = append(places, g)
		}
		rows.Close()
		years := []int{}
		yrows, err := s.store.R.QueryContext(ctx, `SELECT DISTINCT m.year FROM memories m WHERE `+where+` ORDER BY m.year`, args...)
		if err != nil {
			return err
		}
		for yrows.Next() {
			var y int
			if yrows.Scan(&y) == nil {
				years = append(years, y)
			}
		}
		yrows.Close()
		var countries []string
		crows, err := s.store.R.QueryContext(ctx, `SELECT m.country_code FROM memories m WHERE `+where+` AND m.country_code != ''
			GROUP BY m.country_code ORDER BY COUNT(*) DESC LIMIT 3`, args...)
		if err == nil {
			for crows.Next() {
				var c string
				if crows.Scan(&c) == nil {
					countries = append(countries, c)
				}
			}
			crows.Close()
		}
		resp["places"] = places
		resp["years"] = years
		resp["countries"] = countries
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

// mapBounds returns the area that contains the memories matching the
// filters, or one memory.
func (s *Server) mapBounds(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if id := r.URL.Query().Get("memory"); id != "" {
		var lat, lon sql.NullFloat64
		var vault bool
		err := s.store.R.QueryRowContext(ctx, `SELECT lat, lon, vault FROM memories WHERE id = ?`, id).Scan(&lat, &lon, &vault)
		if err != nil || !lat.Valid || (vault && !s.vaultOpen(r)) {
			return errNotFound()
		}
		writeJSON(w, http.StatusOK, map[string]any{"bbox": []float64{lon.Float64, lat.Float64, lon.Float64, lat.Float64}})
		return nil
	}
	where, args := mapFilter(r)
	var minLat, minLon, maxLat, maxLon sql.NullFloat64
	if err := s.store.R.QueryRowContext(ctx, `SELECT MIN(m.lat), MIN(m.lon), MAX(m.lat), MAX(m.lon) FROM memories m WHERE `+where, args...).
		Scan(&minLat, &minLon, &maxLat, &maxLon); err != nil {
		return err
	}
	if !minLat.Valid {
		writeJSON(w, http.StatusOK, map[string]any{"bbox": nil})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"bbox": []float64{minLon.Float64, minLat.Float64, maxLon.Float64, maxLat.Float64}})
	return nil
}

func (s *Server) geoSearch(w http.ResponseWriter, r *http.Request) error {
	hits := geo.Default().Search(r.URL.Query().Get("q"), 8)
	if hits == nil {
		hits = []geo.Place{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": hits})
	return nil
}

func (s *Server) geoReverse(w http.ResponseWriter, r *http.Request) error {
	lat, err1 := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, err2 := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if err1 != nil || err2 != nil {
		return errBadRequest("invalid_coordinates", "lat and lon are required")
	}
	p, ok := geo.Default().Nearest(lat, lon, 30)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"place": nil})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"place": p})
	return nil
}
