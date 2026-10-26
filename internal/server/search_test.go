package server

import "testing"

func TestFTSQuery(t *testing.T) {
	cases := map[string]string{
		"athene":            `"athene"*`,
		"  Café de Jaren ":  `"Café"* "de"* "Jaren"*`,
		`he said "hi" OR x`: `"he"* "said"* "hi"* "OR"* "x"*`,
		"--":                ``,
		"rock'n'roll":       `"rock'n'roll"*`,
	}
	for in, want := range cases {
		if got := ftsQuery(in); got != want {
			t.Errorf("ftsQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
