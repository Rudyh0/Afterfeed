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

// A cluster spread over the mainland and an island is placed where most of
// its memories are, not at their average in the sea.
func TestClusterOnLand(t *testing.T) {
	raw := []mapPoint{
		{id: "bcn1", lat: 41.3874, lon: 2.1686}, {id: "bcn2", lat: 41.3851, lon: 2.1734},
		{id: "bcn3", lat: 41.4036, lon: 2.1744}, {id: "pmi1", lat: 39.5696, lon: 2.6502},
		{id: "pmi2", lat: 39.5710, lon: 2.6470}, {id: "vlc", lat: 39.4699, lon: -0.3763},
	}
	for i := range raw {
		raw[i].x, raw[i].y = mercator(raw[i].lat, raw[i].lon, 4)
	}
	out := clusterPoints(raw, 60)
	if len(out) != 1 {
		t.Fatalf("%d clusters, want 1: %+v", len(out), out)
	}
	c := out[0]
	if math.Abs(c.Lat-41.39) > 0.1 || math.Abs(c.Lon-2.17) > 0.1 {
		t.Errorf("cluster at %.3f,%.3f, want Barcelona", c.Lat, c.Lon)
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
