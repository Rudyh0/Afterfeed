package server

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"sort"
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

// mapClusters groups memories that would be within about 60 pixels of each
// other on the map at this zoom level, so the map shows a manageable number
// of markers. Positions are compared in map pixels (Web Mercator), where a
// degree of latitude is taller than a degree of longitude away from the
// equator.
func (s *Server) mapClusters(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	zoom, _ := strconv.ParseFloat(r.URL.Query().Get("zoom"), 64)
	zoom = math.Max(0, math.Min(22, zoom))
	where, args := mapFilter(r)
	rows, err := s.store.R.QueryContext(ctx, `SELECT m.id, m.lat, m.lon, m.photo_count + m.video_count
		FROM memories m WHERE `+where+` ORDER BY m.taken_at DESC LIMIT 200000`, args...)
	if err != nil {
		return err
	}
	var points []mapPoint
	for rows.Next() {
		var p mapPoint
		if err := rows.Scan(&p.id, &p.lat, &p.lon, &p.media); err != nil {
			rows.Close()
			return err
		}
		p.x, p.y = mercator(p.lat, p.lon, zoom)
		points = append(points, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	out := clusterPoints(points, 60)
	if err := s.coverMedia(ctx, out); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": out})
	return nil
}

type mapPoint struct {
	id       string
	lat, lon float64
	x, y     float64
	media    int
}

// mercator returns the position of a point in pixels of a 256-pixel tile
// world at the zoom level.
func mercator(lat, lon, zoom float64) (float64, float64) {
	size := 256 * math.Pow(2, zoom)
	lat = math.Max(-85.05, math.Min(85.05, lat))
	phi := lat * math.Pi / 180
	x := (lon + 180) / 360 * size
	y := (1 - math.Log(math.Tan(phi)+1/math.Cos(phi))/math.Pi) / 2 * size
	return x, y
}

// clusterPoints groups points on a grid of cells of the given size, then
// merges groups whose centres are closer than that, which the grid alone
// leaves when points fall on both sides of a cell edge.
func clusterPoints(points []mapPoint, cell float64) []cluster {
	type group struct {
		x, y      float64 // sum of pixel positions
		lat, lon  float64 // sum of positions
		n         int
		best      string
		bestMedia int
	}
	cells := map[[2]int64]*group{}
	var order []*group
	for _, p := range points {
		k := [2]int64{int64(math.Floor(p.x / cell)), int64(math.Floor(p.y / cell))}
		g := cells[k]
		if g == nil {
			g = &group{best: p.id, bestMedia: -1}
			cells[k] = g
			order = append(order, g)
		}
		g.x += p.x
		g.y += p.y
		g.lat += p.lat
		g.lon += p.lon
		g.n++
		// The cover is the memory with the most photos and videos, the
		// newest of those (points come newest first).
		if p.media > g.bestMedia {
			g.best, g.bestMedia = p.id, p.media
		}
	}
	// Biggest groups first, so smaller neighbours join them.
	sort.SliceStable(order, func(i, j int) bool { return order[i].n > order[j].n })
	var kept []*group
	for _, g := range order {
		cx, cy := g.x/float64(g.n), g.y/float64(g.n)
		merged := false
		for _, k := range kept {
			kx, ky := k.x/float64(k.n), k.y/float64(k.n)
			if math.Hypot(cx-kx, cy-ky) < cell {
				k.x += g.x
				k.y += g.y
				k.lat += g.lat
				k.lon += g.lon
				k.n += g.n
				if g.bestMedia > k.bestMedia {
					k.best, k.bestMedia = g.best, g.bestMedia
				}
				merged = true
				break
			}
		}
		if !merged {
			kept = append(kept, g)
		}
		if len(kept) >= 2000 {
			break
		}
	}
	out := make([]cluster, 0, len(kept))
	for _, g := range kept {
		out = append(out, cluster{Lat: g.lat / float64(g.n), Lon: g.lon / float64(g.n), Count: g.n, MemoryID: g.best})
	}
	return out
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
	country := r.URL.Query().Get("country")
	if country == "" {
		// Prefer the country where most memories are.
		s.store.R.QueryRowContext(r.Context(), `SELECT country_code FROM memories WHERE country_code != ''
			GROUP BY country_code ORDER BY COUNT(*) DESC LIMIT 1`).Scan(&country)
	}
	hits := geo.Default().SearchIn(r.URL.Query().Get("q"), 8, country)
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
