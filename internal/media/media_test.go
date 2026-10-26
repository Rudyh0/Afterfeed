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
	"strings"
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

func TestReadText(t *testing.T) {
	s := New(t.TempDir())
	if !s.CanReadText() {
		t.Skip("tesseract or its language models are not installed")
	}
	photo := "testdata/caption.jpg"
	if _, err := os.Stat(photo); err != nil {
		t.Skip("no test photo")
	}
	text, err := s.ReadText(context.Background(), Source{ID: "ocr1", Path: photo, Kind: Photo, Mime: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Texel", "Strandpaviljoen"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
}

func TestParseTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t0\t0\t1\t1\t96\tZomer\n" +
		"5\t1\t1\t1\t1\t2\t0\t0\t1\t1\t95\top\n" +
		"5\t1\t1\t1\t1\t3\t0\t0\t1\t1\t95\tTexel\n" +
		"5\t1\t2\t1\t1\t1\t0\t0\t1\t1\t91\tO\n" +
		"5\t1\t3\t1\t1\t1\t0\t0\t1\t1\t30\tStrand\n" +
		"5\t1\t4\t1\t1\t1\t0\t0\t1\t1\t74\tGX-707-Z\n"
	if got := parseTSV(tsv); got != "Zomer op Texel\nGX-707-Z" {
		t.Errorf("parseTSV = %q", got)
	}
}

// A Snapchat caption is in the overlay, not in the photo itself.
func TestReadTextInOverlay(t *testing.T) {
	s := New(t.TempDir())
	if !s.CanReadText() || s.tools.Vips == "" {
		t.Skip("tesseract, its language models or vips are not installed")
	}
	dir := t.TempDir()
	photo := filepath.Join(dir, "photo.jpg")
	overlay := filepath.Join(dir, "overlay.png")
	run := func(args ...string) {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	run("vips", "black", filepath.Join(dir, "black.v"), "1080", "1920", "--bands", "3")
	run("vips", "linear", filepath.Join(dir, "black.v"), photo, "1", "90", "--uchar")
	if out, err := exec.Command("vips", "text", filepath.Join(dir, "text.v"), "Pannenkoeken bij oma", "--dpi", "600",
		"--font", "sans").CombinedOutput(); err != nil {
		t.Skipf("vips cannot draw text here: %v %s", err, out)
	}
	// White text with an alpha channel from the rendered text.
	run("vips", "embed", filepath.Join(dir, "text.v"), filepath.Join(dir, "mask.v"), "60", "900", "1080", "1920")
	run("vips", "bandjoin_const", filepath.Join(dir, "mask.v"), filepath.Join(dir, "rgb.v"), "0 0")
	run("vips", "linear", filepath.Join(dir, "rgb.v"), filepath.Join(dir, "white.v"), "0 0 0", "255 255 255", "--uchar")
	run("vips", "bandjoin", filepath.Join(dir, "white.v")+" "+filepath.Join(dir, "mask.v"), overlay)

	text, err := s.ReadText(context.Background(), Source{ID: "ocr2", Path: photo, Kind: Photo, Mime: "image/jpeg", Overlay: overlay})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Pannenkoeken") {
		t.Errorf("text %q lacks the caption", text)
	}
}

// Text in a scene, like a number plate on a car, is found too; a photo
// without text gives nothing.
func TestReadTextPlate(t *testing.T) {
	s := New(t.TempDir())
	if !s.CanReadText() || s.tools.Vips == "" {
		t.Skip("tesseract, its language models or vips are not installed")
	}
	dir := t.TempDir()
	v := func(name string) string { return filepath.Join(dir, name) }
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("vips", args...).CombinedOutput(); err != nil {
			t.Fatalf("vips %v: %v %s", args, err, out)
		}
	}
	// A grainy grey photo.
	run("gaussnoise", v("bg.v"), "1500", "2000", "--mean", "128", "--sigma", "40")
	run("cast", v("bg.v"), v("bg8.v"), "uchar")
	run("bandjoin", v("bg8.v")+" "+v("bg8.v")+" "+v("bg8.v"), v("bg3.v"))
	run("copy", v("bg3.v"), v("empty.jpg"))
	text, err := s.ReadText(context.Background(), Source{ID: "empty", Path: v("empty.jpg"), Kind: Photo, Mime: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		t.Errorf("a photo without text gave %q", text)
	}
	// A yellow plate with black letters, slightly tilted.
	if out, err := exec.Command("vips", "text", v("t.v"), "GX-707-Z", "--dpi", "600", "--font", "sans bold").CombinedOutput(); err != nil {
		t.Skipf("vips cannot draw text here: %v %s", err, out)
	}
	run("embed", v("t.v"), v("tpad.v"), "30", "18", "540", "110")
	run("invert", v("tpad.v"), v("ink.v"))
	run("black", v("yel.v"), "540", "110", "--bands", "3")
	run("linear", v("yel.v"), v("yel2.v"), "0 0 0", "235 190 30", "--uchar")
	run("bandjoin", v("ink.v")+" "+v("ink.v")+" "+v("ink.v"), v("ink3.v"))
	run("multiply", v("yel2.v"), v("ink3.v"), v("m.v"))
	run("linear", v("m.v"), v("plate.v"), "0.0039215", "0", "--uchar")
	run("rotate", v("plate.v"), v("plater.v"), "4", "--background", "128 128 128")
	run("insert", v("bg3.v"), v("plater.v"), v("car.jpg"), "450", "1300")

	text, err = s.ReadText(context.Background(), Source{ID: "plate", Path: v("car.jpg"), Kind: Photo, Mime: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "GX-707-Z") {
		t.Errorf("text %q lacks the plate", text)
	}
}
