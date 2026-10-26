package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif" // register decoders for DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Info holds the properties of a media file.
type Info struct {
	Kind     Kind
	Mime     string
	Width    int
	Height   int
	Duration float64
	// TakenAt and the location come from the file's own metadata, when it has any.
	TakenAt time.Time
	HasZone bool
	Lat     *float64
	Lon     *float64
}

// Tools reports which external programs are available.
type Tools struct {
	FFprobe    string
	FFmpeg     string
	Vips       string
	VipsThumb  string
	VipsHeader string
	Exiftool   string
	Tesseract  string
}

// FindTools looks the programs up in PATH.
func FindTools() Tools {
	look := func(name string) string {
		p, err := exec.LookPath(name)
		if err != nil {
			return ""
		}
		return p
	}
	return Tools{
		FFprobe:    look("ffprobe"),
		FFmpeg:     look("ffmpeg"),
		Vips:       look("vips"),
		VipsThumb:  look("vipsthumbnail"),
		VipsHeader: look("vipsheader"),
		Exiftool:   look("exiftool"),
		Tesseract:  look("tesseract"),
	}
}

// Probe reads the properties of the file at path. t is the result of Sniff.
// Errors from external tools are not fatal: the returned Info then only has
// what could be read.
func (s *Service) Probe(ctx context.Context, path string, t Type) Info {
	info := Info{Kind: t.Kind, Mime: t.Mime}
	switch t.Kind {
	case Photo:
		s.probeImage(ctx, path, t, &info)
	case Video, Audio:
		s.probeAV(ctx, path, &info)
	}
	return info
}

func (s *Service) probeImage(ctx context.Context, path string, t Type, info *Info) {
	switch t.Mime {
	case "image/jpeg", "image/png", "image/gif":
		f, err := os.Open(path)
		if err != nil {
			return
		}
		cfg, _, err := image.DecodeConfig(bufio.NewReader(f))
		if err == nil {
			info.Width, info.Height = cfg.Width, cfg.Height
		}
		if t.Mime == "image/jpeg" {
			f.Seek(0, 0)
			if ex, err := ReadJPEGExif(bufio.NewReader(f)); err == nil {
				applyExif(info, ex)
			}
		}
		f.Close()
		if info.Width > 0 {
			return
		}
	}
	if s.tools.VipsHeader != "" {
		out, err := s.run(ctx, 20*time.Second, s.tools.VipsHeader, "-a", path)
		if err == nil {
			fields := parseColonFields(out)
			info.Width, _ = strconv.Atoi(fields["width"])
			info.Height, _ = strconv.Atoi(fields["height"])
			if o, _ := strconv.Atoi(fields["orientation"]); o >= 5 && o <= 8 {
				info.Width, info.Height = info.Height, info.Width
			}
		}
	}
	if t.Mime == "image/heic" || t.Mime == "image/avif" || t.Mime == "image/tiff" {
		s.exiftoolInfo(ctx, path, info)
	}
}

func applyExif(info *Info, ex *Exif) {
	if ex.Orientation >= 5 && ex.Orientation <= 8 {
		info.Width, info.Height = info.Height, info.Width
	}
	if !ex.TakenAt.IsZero() {
		info.TakenAt, info.HasZone = ex.TakenAt, ex.HasZone
	}
	info.Lat, info.Lon = ex.Lat, ex.Lon
}

// exiftoolInfo reads the capture time and location with exiftool, for formats
// the built-in reader does not handle.
func (s *Service) exiftoolInfo(ctx context.Context, path string, info *Info) {
	if s.tools.Exiftool == "" {
		return
	}
	out, err := s.run(ctx, 30*time.Second, s.tools.Exiftool, "-json", "-n", "-q",
		"-DateTimeOriginal", "-OffsetTimeOriginal", "-CreateDate", "-GPSLatitude", "-GPSLongitude", path)
	if err != nil {
		return
	}
	var res []map[string]any
	if json.Unmarshal(out, &res) != nil || len(res) == 0 {
		return
	}
	r := res[0]
	str := func(k string) string {
		v, _ := r[k].(string)
		return v
	}
	dt := str("DateTimeOriginal")
	if dt == "" {
		dt = str("CreateDate")
	}
	if dt != "" {
		if t, zone := parseExifTime(dt, str("OffsetTimeOriginal")); !t.IsZero() {
			info.TakenAt, info.HasZone = t, zone
		}
	}
	lat, lok := r["GPSLatitude"].(float64)
	lon, ook := r["GPSLongitude"].(float64)
	if lok && ook && !(lat == 0 && lon == 0) {
		info.Lat, info.Lon = &lat, &lon
	}
}

type ffprobeOutput struct {
	Streams []struct {
		CodecType string            `json:"codec_type"`
		Width     int               `json:"width"`
		Height    int               `json:"height"`
		Tags      map[string]string `json:"tags"`
		SideData  []struct {
			Rotation float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func (s *Service) probeAV(ctx context.Context, path string, info *Info) {
	if s.tools.FFprobe == "" {
		return
	}
	out, err := s.run(ctx, 60*time.Second, s.tools.FFprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path)
	if err != nil {
		return
	}
	var p ffprobeOutput
	if json.Unmarshal(out, &p) != nil {
		return
	}
	info.Duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	hasVideo := false
	for _, st := range p.Streams {
		if st.CodecType != "video" || hasVideo {
			continue
		}
		// Cover art in audio files shows up as a tiny video stream.
		if st.Tags["comment"] == "Cover (front)" {
			continue
		}
		hasVideo = true
		info.Width, info.Height = st.Width, st.Height
		rot := 0.0
		if r, err := strconv.ParseFloat(st.Tags["rotate"], 64); err == nil {
			rot = r
		}
		for _, sd := range st.SideData {
			if sd.Rotation != 0 {
				rot = sd.Rotation
			}
		}
		if r := int(math.Abs(rot)) % 180; r == 90 {
			info.Width, info.Height = info.Height, info.Width
		}
	}
	if info.Kind == Video && !hasVideo {
		info.Kind = Audio
		if info.Mime == "video/mp4" || info.Mime == "video/3gpp" {
			info.Mime = "audio/mp4"
		}
	}
	tags := map[string]string{}
	for k, v := range p.Format.Tags {
		tags[strings.ToLower(k)] = v
	}
	if ct := tags["com.apple.quicktime.creationdate"]; ct != "" {
		if t, err := time.Parse("2006-01-02T15:04:05-0700", ct); err == nil {
			info.TakenAt, info.HasZone = t, true
		}
	}
	if info.TakenAt.IsZero() {
		if ct := tags["creation_time"]; ct != "" {
			// Many apps write 1970 or 1904 here when they do not know the time.
			if t, err := time.Parse(time.RFC3339Nano, ct); err == nil && t.Year() > 1990 {
				info.TakenAt, info.HasZone = t, true
			}
		}
	}
	loc := tags["com.apple.quicktime.location.iso6709"]
	if loc == "" {
		loc = tags["location"]
	}
	if lat, lon, ok := parseISO6709(loc); ok {
		info.Lat, info.Lon = &lat, &lon
	}
}

var iso6709 = regexp.MustCompile(`^([+-]\d+(?:\.\d+)?)([+-]\d+(?:\.\d+)?)`)

func parseISO6709(s string) (float64, float64, bool) {
	m := iso6709.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(m[1], 64)
	lon, err2 := strconv.ParseFloat(m[2], 64)
	if err1 != nil || err2 != nil || math.Abs(lat) > 90 || math.Abs(lon) > 180 || (lat == 0 && lon == 0) {
		return 0, 0, false
	}
	return lat, lon, true
}

func parseColonFields(out []byte) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, seen := fields[k]; !seen {
			fields[k] = strings.TrimSpace(v)
		}
	}
	return fields
}

// run executes a tool with a timeout and returns its standard output.
func (s *Service) run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	return s.runEnv(ctx, timeout, nil, name, args...)
}

// runEnv is run with extra environment variables.
func (s *Service) runEnv(ctx context.Context, timeout time.Duration, env []string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, errors.Join(err, errors.New(msg))
	}
	return stdout.Bytes(), nil
}
