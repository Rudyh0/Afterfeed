package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strings"
	"time"
)

// Exif holds the few EXIF fields Afterfeed uses.
type Exif struct {
	Orientation int
	// TakenAt is the capture time. When the file has no time zone offset it is
	// set in UTC with HasZone false, and should be read as local time.
	TakenAt time.Time
	HasZone bool
	Lat     *float64
	Lon     *float64
}

var errNoExif = errors.New("no exif")

// ReadJPEGExif reads EXIF data from a JPEG stream. It stops reading at the
// start of the image data.
func ReadJPEGExif(r io.Reader) (*Exif, error) {
	var marker [2]byte
	if _, err := io.ReadFull(r, marker[:]); err != nil || marker != [2]byte{0xff, 0xd8} {
		return nil, errNoExif
	}
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, errNoExif
		}
		if hdr[0] != 0xff {
			return nil, errNoExif
		}
		m := hdr[1]
		if m == 0xda || m == 0xd9 { // start of scan or end of image
			return nil, errNoExif
		}
		size := int(binary.BigEndian.Uint16(hdr[2:])) - 2
		if size < 0 {
			return nil, errNoExif
		}
		if m == 0xe1 && size > 6 {
			buf := make([]byte, size)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, errNoExif
			}
			if bytes.HasPrefix(buf, []byte("Exif\x00\x00")) {
				return ParseTIFFExif(buf[6:])
			}
			continue
		}
		if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
			return nil, errNoExif
		}
	}
}

// ParseTIFFExif parses the TIFF structure inside an EXIF block.
func ParseTIFFExif(b []byte) (*Exif, error) {
	if len(b) < 8 {
		return nil, errNoExif
	}
	var bo binary.ByteOrder
	switch string(b[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return nil, errNoExif
	}
	p := &tiff{b: b, bo: bo}
	out := &Exif{}
	ifd0 := p.ifd(int(bo.Uint32(b[4:])))
	if v, ok := ifd0[0x0112]; ok {
		out.Orientation = int(p.uint(v))
	}
	var dateTime, offset string
	if v, ok := ifd0[0x0132]; ok {
		dateTime = p.str(v)
	}
	if v, ok := ifd0[0x8769]; ok {
		exif := p.ifd(int(p.uint(v)))
		if v, ok := exif[0x9003]; ok {
			dateTime = p.str(v)
		} else if v, ok := exif[0x9004]; ok && dateTime == "" {
			dateTime = p.str(v)
		}
		if v, ok := exif[0x9011]; ok {
			offset = p.str(v)
		}
	}
	if dateTime != "" {
		out.TakenAt, out.HasZone = parseExifTime(dateTime, offset)
	}
	if v, ok := ifd0[0x8825]; ok {
		gps := p.ifd(int(p.uint(v)))
		lat, lok := p.coord(gps[2], gps[1])
		lon, ook := p.coord(gps[4], gps[3])
		if lok && ook && !(lat == 0 && lon == 0) && math.Abs(lat) <= 90 && math.Abs(lon) <= 180 {
			out.Lat, out.Lon = &lat, &lon
		}
	}
	return out, nil
}

func parseExifTime(s, offset string) (time.Time, bool) {
	s = strings.TrimRight(strings.TrimSpace(s), "\x00")
	t, err := time.Parse("2006:01:02 15:04:05", s)
	if err != nil || t.Year() < 1900 {
		return time.Time{}, false
	}
	offset = strings.TrimRight(strings.TrimSpace(offset), "\x00")
	if offset != "" {
		if z, err := time.Parse("-07:00", offset); err == nil {
			_, off := z.Zone()
			loc := time.FixedZone("", off)
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc), true
		}
	}
	return t, false
}

type tiffEntry struct {
	typ   uint16
	count uint32
	value []byte // the 4 value bytes, or the data they point to
}

type tiff struct {
	b  []byte
	bo binary.ByteOrder
}

var typeSize = map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}

func (p *tiff) ifd(off int) map[uint16]tiffEntry {
	out := map[uint16]tiffEntry{}
	if off <= 0 || off+2 > len(p.b) {
		return out
	}
	n := int(p.bo.Uint16(p.b[off:]))
	for i := 0; i < n && i < 512; i++ {
		e := off + 2 + i*12
		if e+12 > len(p.b) {
			break
		}
		tag := p.bo.Uint16(p.b[e:])
		typ := p.bo.Uint16(p.b[e+2:])
		count := p.bo.Uint32(p.b[e+4:])
		size := typeSize[typ] * int(count)
		if size == 0 || count > 1<<20 {
			continue
		}
		var val []byte
		if size <= 4 {
			val = p.b[e+8 : e+8+size]
		} else {
			o := int(p.bo.Uint32(p.b[e+8:]))
			if o < 0 || o+size > len(p.b) {
				continue
			}
			val = p.b[o : o+size]
		}
		out[tag] = tiffEntry{typ: typ, count: count, value: val}
	}
	return out
}

func (p *tiff) uint(e tiffEntry) uint32 {
	switch e.typ {
	case 3:
		if len(e.value) >= 2 {
			return uint32(p.bo.Uint16(e.value))
		}
	case 4, 9:
		if len(e.value) >= 4 {
			return p.bo.Uint32(e.value)
		}
	case 1, 7:
		if len(e.value) >= 1 {
			return uint32(e.value[0])
		}
	}
	return 0
}

func (p *tiff) str(e tiffEntry) string {
	if e.typ != 2 {
		return ""
	}
	return strings.TrimRight(string(e.value), "\x00 ")
}

func (p *tiff) rational(b []byte) float64 {
	if len(b) < 8 {
		return 0
	}
	num, den := p.bo.Uint32(b), p.bo.Uint32(b[4:])
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func (p *tiff) coord(v, ref tiffEntry) (float64, bool) {
	if v.typ != 5 || v.count < 3 || len(v.value) < 24 {
		return 0, false
	}
	d := p.rational(v.value) + p.rational(v.value[8:])/60 + p.rational(v.value[16:])/3600
	r := strings.ToUpper(p.str(ref))
	if r == "S" || r == "W" {
		d = -d
	}
	return d, true
}
