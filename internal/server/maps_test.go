package server

import (
	"math"
	"testing"
)

func TestClusterPoints(t *testing.T) {
	pts := func(zoom float64) []mapPoint {
		raw := []mapPoint{
			{id: "ams", lat: 52.3676, lon: 4.9041, media: 3},
			{id: "rot", lat: 51.9244, lon: 4.4777, media: 1},
			{id: "ber", lat: 52.5200, lon: 13.4050, media: 1},
		}
		for i := range raw {
			raw[i].x, raw[i].y = mercator(raw[i].lat, raw[i].lon, zoom)
		}
		return raw
	}
	// Zoomed out over Europe, Amsterdam and Rotterdam form one marker that
	// stays in the Netherlands, and Berlin stays apart.
	out := clusterPoints(pts(5), 60)
	if len(out) != 2 {
		t.Fatalf("zoom 5: %d clusters, want 2: %+v", len(out), out)
	}
	for _, c := range out {
		if c.Count == 2 {
			if c.Lon < 4.4 || c.Lon > 5 || c.Lat < 51.9 || c.Lat > 52.4 || c.MemoryID != "ams" {
				t.Errorf("Dutch cluster at %.3f,%.3f cover %s", c.Lat, c.Lon, c.MemoryID)
			}
		}
	}
	// Zoomed in, every city is its own marker.
	if out := clusterPoints(pts(10), 60); len(out) != 3 {
		t.Fatalf("zoom 10: %d clusters, want 3", len(out))
	}
}

func TestMercator(t *testing.T) {
	x, y := mercator(0, 0, 0)
	if math.Abs(x-128) > 1e-9 || math.Abs(y-128) > 1e-9 {
		t.Errorf("origin = %v,%v", x, y)
	}
	// North is up: a higher latitude has a smaller y.
	_, yn := mercator(52, 5, 3)
	_, ys := mercator(51, 5, 3)
	if yn >= ys {
		t.Error("north is not up")
	}
}
