// Package geo finds place names for coordinates without any online
// service. It uses a list of populated places from GeoNames
// (https://www.geonames.org, CC BY 4.0), embedded in the binary; see
// tools/geodata.
package geo

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

//go:embed cities.tsv.gz
var data []byte

// Place is a populated place.
type Place struct {
	Name    string  `json:"name"`
	Country string  `json:"country_code"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Pop     int     `json:"population"`
}

// Index looks places up by location and by name.
type Index struct {
	places []Place
	folded []string
	grid   map[int32][]int32
}

var (
	once  sync.Once
	index *Index
)

// Default returns the embedded index, loading it on first use.
func Default() *Index {
	once.Do(func() {
		var err error
		index, err = load(data)
		if err != nil {
			slog.Error("load place names", "err", err)
			index = &Index{grid: map[int32][]int32{}}
		}
	})
	return index
}

func load(gz []byte) (*Index, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	x := &Index{grid: map[int32][]int32{}}
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		f := strings.SplitN(sc.Text(), "\t", 5)
		if len(f) != 5 {
			continue
		}
		lat, err1 := strconv.Atoi(f[0])
		lon, err2 := strconv.Atoi(f[1])
		pop, _ := strconv.Atoi(f[3])
		if err1 != nil || err2 != nil {
			continue
		}
		p := Place{Name: f[4], Country: f[2], Lat: float64(lat) / 1e5, Lon: float64(lon) / 1e5, Pop: pop}
		i := int32(len(x.places))
		x.places = append(x.places, p)
		x.folded = append(x.folded, Fold(p.Name))
		key := cell(p.Lat, p.Lon)
		x.grid[key] = append(x.grid[key], i)
	}
	return x, sc.Err()
}

// cell is the key of the 1×1 degree square a point is in.
func cell(lat, lon float64) int32 {
	y := int32(math.Floor(lat + 90))
	xx := int32(math.Floor(lon+180)) % 360
	if xx < 0 {
		xx += 360
	}
	return y*360 + xx
}

// Nearest returns the closest place within maxKm kilometres.
func (x *Index) Nearest(lat, lon, maxKm float64) (Place, bool) {
	best := -1
	bestD := maxKm
	// One degree of longitude gets shorter towards the poles, so search
	// wider there.
	span := 1
	if c := math.Cos(lat * math.Pi / 180); c > 0 && c < 0.5 {
		span = int(math.Min(6, math.Ceil(0.5/c)))
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -span; dx <= span; dx++ {
			for _, i := range x.grid[cell(lat+float64(dy), lon+float64(dx))] {
				p := x.places[i]
				if d := Distance(lat, lon, p.Lat, p.Lon); d < bestD {
					best, bestD = int(i), d
				}
			}
		}
	}
	if best < 0 {
		return Place{}, false
	}
	return x.places[best], true
}

// Search returns places whose name starts with q, the largest first.
func (x *Index) Search(q string, limit int) []Place {
	q = Fold(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var hits []int
	for i, name := range x.folded {
		if strings.HasPrefix(name, q) {
			hits = append(hits, i)
		}
	}
	sort.Slice(hits, func(a, b int) bool {
		pa, pb := x.places[hits[a]], x.places[hits[b]]
		// Exact matches first, then by size.
		ea, eb := x.folded[hits[a]] == q, x.folded[hits[b]] == q
		if ea != eb {
			return ea
		}
		return pa.Pop > pb.Pop
	})
	out := make([]Place, 0, min(limit, len(hits)))
	for _, i := range hits {
		if len(out) == limit {
			break
		}
		out = append(out, x.places[i])
	}
	return out
}

// Distance returns the distance between two points in kilometres.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp := p2 - p1
	dl := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Fold lower-cases a name and removes accents, for matching what people type.
func Fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if a, ok := fold[r]; ok {
			b.WriteString(a)
		} else if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var fold = func() map[rune]string {
	m := map[rune]string{}
	groups := map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ďđ", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥħ",
		"i": "ìíîïĩīĭįı", "j": "ĵ", "k": "ķ", "l": "ĺļľŀł", "n": "ñńņňŉ", "o": "òóôõöøōŏő",
		"r": "ŕŗř", "s": "śŝşšș", "t": "ţťŧț", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž",
		"ae": "æ", "oe": "œ", "ss": "ß",
	}
	for ascii, letters := range groups {
		for _, r := range letters {
			m[r] = ascii
		}
	}
	return m
}()
