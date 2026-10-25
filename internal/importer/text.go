package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// FixMojibake repairs text from Meta exports, which store UTF-8 bytes as
// separate characters (é becomes Ã©). Text that is not affected is returned
// unchanged.
func FixMojibake(s string) string {
	if s == "" {
		return s
	}
	needs := false
	for _, r := range s {
		if r > 0xff {
			return s // real non-Latin-1 characters: the text is not mangled
		}
		if r >= 0x80 {
			needs = true
		}
	}
	if !needs {
		return s
	}
	b := make([]byte, 0, len(s))
	for _, r := range s {
		b = append(b, byte(r))
	}
	if !utf8.Valid(b) {
		return s
	}
	return string(b)
}

// Clean trims text and normalises line endings.
func Clean(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}

// Unix converts a timestamp in seconds, milliseconds or microseconds since
// 1970 to a time, guessing the unit from the size.
func Unix(v float64) time.Time {
	switch {
	case v <= 0 || math.IsNaN(v):
		return time.Time{}
	case v > 1e17: // nanoseconds
		return time.Unix(0, int64(v)).UTC()
	case v > 1e14: // microseconds
		return time.UnixMicro(int64(v)).UTC()
	case v > 1e11: // milliseconds
		return time.UnixMilli(int64(v)).UTC()
	}
	sec, frac := math.Modf(v)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC()
}

// ParseNumber reads a number from a JSON value that may be a number or a string.
func ParseNumber(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return 0
}

// ValidCoordinates reports whether lat and lon are a real location. Exports
// use 0,0 for unknown.
func ValidCoordinates(lat, lon float64) bool {
	return !(lat == 0 && lon == 0) && math.Abs(lat) <= 90 && math.Abs(lon) <= 180 &&
		!math.IsNaN(lat) && !math.IsNaN(lon)
}

// Coordinates returns pointers to lat and lon when they are valid.
func Coordinates(lat, lon float64) (*float64, *float64) {
	if !ValidCoordinates(lat, lon) {
		return nil, nil
	}
	return &lat, &lon
}

// Hash returns a short stable hash of the parts, for building keys.
func Hash(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:8])
}

// messageKey derives a key for a message from what identifies it. It does not
// depend on the conversation, so the same message in two exports of one chat
// (such as Messenger's normal and encrypted exports) is stored once.
func messageKey(platform string, m *Message) string {
	if m.Key != "" {
		return platform + ":" + m.Key
	}
	files := make([]string, 0, len(m.Files))
	for _, f := range m.Files {
		files = append(files, f.key())
	}
	return platform + ":" + Hash(strconv.FormatInt(m.At.UnixMilli(), 10), m.Sender, m.Text, m.Link, strings.Join(files, ","))
}

func (m *Media) key() string {
	if m.Key != "" {
		return m.Key
	}
	if m.File != nil {
		return strings.ToLower(m.File.Name())
	}
	ref := m.Ref
	if i := strings.LastIndexAny(ref, "/\\"); i >= 0 {
		ref = ref[i+1:]
	}
	return strings.ToLower(ref)
}
