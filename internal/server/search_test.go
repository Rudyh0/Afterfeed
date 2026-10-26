package server

import (
	"context"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
)

func TestFTSQuery(t *testing.T) {
	cases := map[string]string{
		"athene":            `"athene"*`,
		"  Café de Jaren ":  `"Café"* "de"* "Jaren"*`,
		`he said "hi" OR x`: `"he"* "said"* "hi"* "OR"* "x"*`,
		"--":                ``,
		"rock'n'roll":       `"rock'n'roll"*`,
		"GX-707-Z":          `("GX-707-Z"* OR "GX707Z"*)`,
		"Jean-Luc":          `"Jean-Luc"*`,
	}
	for in, want := range cases {
		if got := ftsQuery(in); got != want {
			t.Errorf("ftsQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

// A number plate in a photo is found however it is typed, also when the
// reader missed a dash.
func TestSearchPlateInPhoto(t *testing.T) {
	c, deps := newTestServer(t)
	if code := c.do("POST", "/api/setup", map[string]string{"username": "rudy", "password": "a long enough password"}, nil); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	lib := deps.Library
	now := time.Date(2023, 10, 14, 17, 0, 0, 0, time.UTC)
	read := "Renault\nGX-707Z"
	mem := &library.Memory{
		ID: library.MemoryID("test:car"), Platform: "snapchat", Kind: "memory",
		Fields: library.Fields{TakenAt: now}, Source: library.Source{Key: "test:car"},
		CreatedAt: now, UpdatedAt: now,
		Media: []library.Media{{ID: "m1", Key: "1", File: "1.jpg", Kind: "photo", Mime: "image/jpeg", OCR: &read, OCRVersion: 2}},
	}
	rel, err := lib.CreateMemoryDir(mem)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, _ := lib.Store().W.BeginTx(ctx, nil)
	if err := lib.IndexMemory(ctx, tx, rel, mem, 0); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	for _, q := range []string{"GX-707-Z", "gx707z", "GX707", "gx-707", "renault"} {
		var res struct {
			Media []mediaHit `json:"media"`
		}
		c.do("GET", "/api/search?q="+url.QueryEscape(q), nil, &res)
		if len(res.Media) != 1 {
			t.Errorf("search %q found %d photos, want 1", q, len(res.Media))
		}
	}
	var res struct {
		Media []mediaHit `json:"media"`
	}
	c.do("GET", "/api/search?q=GX-708", nil, &res)
	if len(res.Media) != 0 {
		t.Errorf("another plate found %d photos", len(res.Media))
	}
}

func TestCodes(t *testing.T) {
	got := library.Codes("Kenteken GX-707-Z en GX-707Z\nPostcode 9351 AB, tel. 06-1234 5678\nZomer op Texel 2021")
	want := []string{"GX707Z", "9351AB"}
	if !slices.Equal(got, want) {
		t.Errorf("Codes = %q, want %q", got, want)
	}
}
