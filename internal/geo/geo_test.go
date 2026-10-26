package geo

import "testing"

func TestNearest(t *testing.T) {
	x := Default()
	cases := []struct {
		lat, lon float64
		country  string
	}{
		{37.9715, 23.7257, "GR"}, // Acropolis
		{52.358, 4.868, "NL"},    // Vondelpark
		{53.05, 4.80, "NL"},      // Texel
		{41.3851, 2.1734, "ES"},  // Barcelona
		{64.1466, -21.9426, "IS"},
		{-33.8688, 151.2093, "AU"},
	}
	for _, c := range cases {
		p, ok := x.Nearest(c.lat, c.lon, 30)
		if !ok || p.Country != c.country {
			t.Errorf("Nearest(%v, %v) = %+v, %v; want country %s", c.lat, c.lon, p, ok, c.country)
		}
	}
	if _, ok := x.Nearest(0, -30, 30); ok {
		t.Error("middle of the Atlantic should have no place")
	}
}

func TestSearch(t *testing.T) {
	x := Default()
	hits := x.Search("amster", 5)
	if len(hits) == 0 || hits[0].Name != "Amsterdam" {
		t.Fatalf("search amster = %+v", hits)
	}
	if hits := x.Search("münch", 3); len(hits) == 0 {
		t.Error("no match with accent")
	}
	if hits := x.Search("chania", 3); len(hits) == 0 || hits[0].Country != "GR" {
		t.Errorf("chania = %+v", hits)
	}
}

func TestSearchInCountry(t *testing.T) {
	x := Default()
	got := x.SearchIn("a", 3, "NL")
	if len(got) == 0 || got[0].Country != "NL" {
		t.Fatalf("expected a Dutch place first, got %+v", got)
	}
	if exact := x.SearchIn("paris", 1, "NL"); len(exact) == 0 || exact[0].Country != "FR" {
		t.Fatalf("an exact, much larger match should still win: %+v", exact)
	}
}
