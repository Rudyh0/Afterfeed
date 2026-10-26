package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSniff(t *testing.T) {
	cases := []struct {
		head []byte
		name string
		want Kind
		mime string
	}{
		{[]byte("\xff\xd8\xff\xe0...."), "x.png", Photo, "image/jpeg"},
		{[]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), "IMG_1.HEIC", Photo, "image/heic"},
		{[]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00isommp42"), "clip", Video, "video/mp4"},
		{[]byte("\x00\x00\x00\x14ftypqt  "), "clip.mov", Video, "video/quicktime"},
		{[]byte("\x00\x00\x00\x18ftypM4A \x00\x00\x00\x00"), "voice.mp4", Audio, "audio/mp4"},
		{[]byte("%PDF-1.7"), "file", Document, "application/pdf"},
		{[]byte("PK\x03\x04"), "Budget.xlsx", Document, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{[]byte("OggS"), "audio", Audio, "audio/ogg"},
		{[]byte("hello"), "notes.txt", Document, "text/plain"},
		{[]byte("\x00\x01"), "blob.unknown", Other, "application/octet-stream"},
	}
	for _, c := range cases {
		got := Sniff(c.head, c.name)
		if got.Kind != c.want || got.Mime != c.mime {
			t.Errorf("Sniff(%q, %s) = %+v, want %s %s", c.head, c.name, got, c.want, c.mime)
		}
	}
}

func TestExifFromExiftool(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not installed")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "a.jpg")
	writeJPEG(t, p, 40, 20)
	cmd := exec.Command("exiftool", "-q", "-overwrite_original",
		"-DateTimeOriginal=2019:10:25 19:47:00", "-OffsetTimeOriginal=+02:00", "-Orientation#=6",
		"-GPSLatitude=37.9715", "-GPSLatitudeRef=N", "-GPSLongitude=23.7257", "-GPSLongitudeRef=E", p)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exiftool: %v %s", err, out)
	}
	s := New(dir)
	info := s.Probe(context.Background(), p, Type{Photo, "image/jpeg", ".jpg"})
	if info.Width != 20 || info.Height != 40 {
		t.Errorf("size = %dx%d, want 20x40 after orientation", info.Width, info.Height)
	}
	if !info.HasZone || info.TakenAt.UTC().Format("2006-01-02 15:04") != "2019-10-25 17:47" {
		t.Errorf("taken = %v zone=%v", info.TakenAt, info.HasZone)
	}
	if info.Lat == nil || *info.Lat < 37.97 || *info.Lat > 37.98 || *info.Lon < 23.72 {
		t.Errorf("gps = %v %v", info.Lat, info.Lon)
	}
}

func TestVariants(t *testing.T) {
	s := New(t.TempDir())
	if s.tools.VipsThumb == "" || s.tools.FFmpeg == "" {
		t.Skip("vips or ffmpeg not installed")
	}
	dir := t.TempDir()
	photo := filepath.Join(dir, "p.jpg")
	writeJPEG(t, photo, 3000, 2000)
	overlay := filepath.Join(dir, "o.png")
	img := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	img.Set(10, 10, color.NRGBA{255, 0, 0, 255})
	f, _ := os.Create(overlay)
	png.Encode(f, img)
	f.Close()

	ctx := context.Background()
	for _, ov := range []string{"", overlay} {
		id := "ab12" + filepath.Base(ov)
		p, err := s.Get(ctx, Source{ID: id, Path: photo, Kind: Photo, Mime: "image/jpeg", Overlay: ov}, Thumb)
		if err != nil {
			t.Fatalf("thumb (overlay %q): %v", ov, err)
		}
		info := s.Probe(ctx, p, Type{Photo, "image/webp", ".webp"})
		if info.Width != 640 || info.Height != 427 {
			t.Errorf("thumb size = %dx%d", info.Width, info.Height)
		}
	}

	video := filepath.Join(dir, "v.mp4")
	out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=10",
		"-pix_fmt", "yuv420p", "-y", video).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	vinfo := s.Probe(ctx, video, Type{Video, "video/mp4", ".mp4"})
	if vinfo.Width != 320 || vinfo.Duration < 1.9 {
		t.Errorf("video info = %+v", vinfo)
	}
	if _, err := s.Get(ctx, Source{ID: "cd34", Path: video, Kind: Video}, Thumb); err != nil {
		t.Fatalf("video thumb: %v", err)
	}
	if !s.Has("cd34", Poster) {
		t.Error("poster not cached")
	}
}

func writeJPEG(t *testing.T, p string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x += 7 {
		img.Set(x, x%h, color.RGBA{200, 100, 50, 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A video's thumbnail needs its poster frame first. With every work slot
// taken by video thumbnails, they must not wait for each other forever.
func TestVideoThumbsDoNotDeadlock(t *testing.T) {
	s := New(t.TempDir())
	if s.tools.VipsThumb == "" || s.tools.FFmpeg == "" {
		t.Skip("vips or ffmpeg not installed")
	}
	s.sem = make(chan struct{}, 2)
	dir := t.TempDir()
	var videos []string
	for i := range 4 {
		v := filepath.Join(dir, fmt.Sprintf("v%d.mp4", i))
		out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=10",
			"-pix_fmt", "yuv420p", "-y", v).CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg: %v %s", err, out)
		}
		videos = append(videos, v)
	}
	done := make(chan error, len(videos)*2)
	for i, v := range videos {
		for _, variant := range []Variant{Thumb, Preview} {
			go func() {
				_, err := s.Get(context.Background(), Source{ID: fmt.Sprintf("vv%02d", i), Path: v, Kind: Video}, variant)
				done <- err
			}()
		}
	}
	timeout := time.After(60 * time.Second)
	for range len(videos) * 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-timeout:
			t.Fatal("video thumbnails are stuck waiting for each other")
		}
	}
}
