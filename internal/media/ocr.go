package media

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rudyh0/afterfeed/internal/textfind"
)

// OCRLanguages are the Tesseract languages used to read text in images,
// such as "eng+nld". Empty uses all languages that are installed; "off"
// turns reading text off.
var OCRLanguages = os.Getenv("AFTERFEED_OCR_LANGUAGES")

// CanReadText reports whether text recognition is available.
func (s *Service) CanReadText() bool {
	return s.tools.Tesseract != "" && s.tools.VipsThumb != "" && s.ocrLanguages() != ""
}

// ocrLanguages returns the languages to use: the configured ones, or the
// known languages whose models are installed.
func (s *Service) ocrLanguages() string {
	switch l := strings.TrimSpace(OCRLanguages); strings.ToLower(l) {
	case "":
	case "off", "none", "false", "0":
		return ""
	default:
		return l
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.langs != nil {
		return *s.langs
	}
	var found []string
	for _, dir := range tessdataDirs() {
		for _, l := range []string{"eng", "nld", "deu", "fra", "spa"} {
			if _, err := os.Stat(filepath.Join(dir, l+".traineddata")); err == nil && !contains(found, l) {
				found = append(found, l)
			}
		}
	}
	langs := strings.Join(found, "+")
	s.langs = &langs
	return langs
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// tessdataDirs are the usual places of Tesseract's language models.
func tessdataDirs() []string {
	dirs := []string{}
	if p := os.Getenv("TESSDATA_PREFIX"); p != "" {
		dirs = append(dirs, p, filepath.Join(p, "tessdata"))
	}
	dirs = append(dirs, "/usr/share/tessdata", "/usr/share/tesseract-ocr/5/tessdata", "/usr/share/tesseract-ocr/4.00/tessdata",
		"/usr/local/share/tessdata")
	return dirs
}

// ocrSize is the longest side of the image that lines are cut from:
// enough for small print, plates and signs.
const ocrSize = 2048

// TextVersion identifies how text in images is read. Media read by an
// older version are read again. Version 1 read whole pages with Tesseract;
// version 2 first finds the lines of text with a detection model, which
// also finds text in scenes, such as signs and number plates.
const TextVersion = 2

// ReadText returns the text in a photo or in the poster frame of a video,
// including a caption overlay. A detection model finds the lines of text;
// Tesseract reads them. Words the recogniser is unsure about are left out,
// so photos without text give an empty result rather than noise.
//
// It is background work: it waits for a free background slot, so what is
// on screen goes first.
func (s *Service) ReadText(ctx context.Context, src Source) (string, error) {
	if !s.CanReadText() || (src.Kind != Photo && src.Kind != Video) {
		return "", ErrUnsupported
	}
	det, err := textfind.Default()
	if err != nil {
		return "", err
	}
	select {
	case s.background <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-s.background }()

	// A video is read from its poster frame, which the thumbnail already
	// needed. Get it before taking a work slot, like create does.
	in := src.Path
	if src.Kind == Video {
		p, err := s.Get(ctx, Source{ID: src.ID, Path: src.Path, Kind: Video}, Poster)
		if err != nil {
			return "", err
		}
		in = p
	}
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-s.sem }()

	if err := os.MkdirAll(filepath.Join(s.cacheDir, "media"), 0o750); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(filepath.Join(s.cacheDir, "media"), "ocr-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	// The image is made for reading only and not kept: the large preview
	// is made when someone opens the photo.
	page := filepath.Join(dir, "page.png")
	if preview := s.CachePath(src.ID, Preview); fileExists(preview) {
		_, err = s.run(ctx, time.Minute, s.tools.VipsThumb, preview, "--size", fmt.Sprintf("%dx%d>", ocrSize, ocrSize),
			"-o", page+"[compression=1]")
	} else {
		err = s.imageVariant(ctx, in, src.Overlay, ocrSize, page)
	}
	if err != nil {
		return "", err
	}
	img, err := decodePNG(page)
	if err != nil {
		return "", err
	}
	rgb := textfind.NewRGB(img)
	lines, err := det.Find(rgb)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", nil
	}
	sheet := textfind.Sheet(rgb, lines)
	if sheet == nil {
		return "", nil
	}
	sheetPath := filepath.Join(dir, "lines.png")
	if err := encodePNG(sheetPath, sheet); err != nil {
		return "", err
	}
	// Tesseract's own threads would compete with the other workers.
	out, err := s.runEnv(ctx, 2*time.Minute, []string{"OMP_THREAD_LIMIT=1"}, s.tools.Tesseract, sheetPath, "stdout",
		"-l", s.ocrLanguages(), "--psm", "6", "-c", "tessedit_create_tsv=1")
	if err != nil {
		return "", err
	}
	return parseTSV(string(out)), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func decodePNG(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(bufio.NewReader(f))
}

func encodePNG(p string, img image.Image) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// parseTSV turns Tesseract's word list into lines of text. Words it is
// fairly sure of are kept, if they have at least two letters or digits or
// are a number; a line is kept when one of its words is clearer still.
// Single letters are mostly icons and edges.
func parseTSV(tsv string) string {
	type word struct {
		text string
		conf float64
	}
	var lines []string
	var cur []word
	key := ""
	flush := func() {
		strong := false
		var keep []string
		for _, w := range cur {
			n := letters(w.text) + digits(w.text)
			if w.conf < 60 || n == 0 || (n == 1 && digits(w.text) == 0) {
				continue
			}
			keep = append(keep, w.text)
			if w.conf >= 70 && n >= 2 {
				strong = true
			}
		}
		if strong {
			lines = append(lines, strings.Join(keep, " "))
		}
		cur = cur[:0]
	}
	for i, row := range strings.Split(tsv, "\n") {
		f := strings.Split(row, "\t")
		if i == 0 || len(f) < 12 || f[0] != "5" {
			continue
		}
		text := strings.TrimSpace(f[11])
		if text == "" {
			continue
		}
		k := f[2] + "." + f[3] + "." + f[4] // block, paragraph, line
		if k != key {
			flush()
			key = k
		}
		conf, _ := strconv.ParseFloat(f[10], 64)
		cur = append(cur, word{text, conf})
	}
	flush()
	return strings.Join(lines, "\n")
}

func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

func digits(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}
