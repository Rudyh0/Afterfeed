package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Variant is a derived version of a media file, stored in the cache.
type Variant string

const (
	// Thumb is a small image for grids: 640 pixels on the longest side.
	Thumb Variant = "thumb"
	// Preview is a large image for the detail view: 2048 pixels on the longest side.
	Preview Variant = "preview"
	// Poster is a still frame of a video.
	Poster Variant = "poster"
)

const (
	thumbSize   = 640
	previewSize = 2048
	posterSize  = 1600
)

// ErrUnsupported is returned when a variant cannot be made for a file, for
// example because the tools are missing.
var ErrUnsupported = errors.New("variant not available")

// Source identifies a media file for creating variants.
type Source struct {
	ID   string
	Path string
	Kind Kind
	Mime string
	// Overlay is an optional transparent image placed over the media, such
	// as a Snapchat caption.
	Overlay string
}

// Service probes files and manages the cache of variants.
type Service struct {
	cacheDir string
	tools    Tools
	sem      chan struct{}
	// background limits work that nobody is waiting for, so one slot of
	// sem stays free for what is on screen.
	background chan struct{}
	// docService is the address of a Gotenberg server that converts office
	// documents to PDF, for previews. Empty when there is none.
	docService string

	mu       sync.Mutex
	inflight map[string]*call
	// langs caches the OCR languages that are installed.
	langs *string
}

type call struct {
	done chan struct{}
	path string
	err  error
}

// New creates a media service that keeps variants in cacheDir.
func New(cacheDir string) *Service {
	slots := max(2, runtime.NumCPU())
	return &Service{
		cacheDir:   cacheDir,
		tools:      FindTools(),
		sem:        make(chan struct{}, slots),
		background: make(chan struct{}, slots-1),
		inflight:   map[string]*call{},
	}
}

// Tools returns the external programs that were found.
func (s *Service) Tools() Tools { return s.tools }

// SetDocumentService sets the address of the document conversion service.
func (s *Service) SetDocumentService(url string) { s.docService = strings.TrimRight(url, "/") }

// CachePath returns where a variant of a media file is stored.
func (s *Service) CachePath(id string, v Variant) string {
	ext := ".webp"
	if v == Poster {
		ext = ".jpg"
	}
	prefix := "xx"
	if len(id) >= 2 {
		prefix = id[:2]
	}
	return filepath.Join(s.cacheDir, "media", prefix, id+"."+string(v)+ext)
}

// Remove deletes every cached variant of a media file.
func (s *Service) Remove(id string) {
	for _, v := range []Variant{Thumb, Preview, Poster} {
		os.Remove(s.CachePath(id, v))
	}
}

// Has reports whether a variant is already in the cache.
func (s *Service) Has(id string, v Variant) bool {
	_, err := os.Stat(s.CachePath(id, v))
	return err == nil
}

// BackgroundWorkers is how many variants Prepare makes at the same time.
func (s *Service) BackgroundWorkers() int { return cap(s.background) }

// Prepare makes a variant ahead of time. It leaves room for requests from
// the web app, which go first.
func (s *Service) Prepare(ctx context.Context, src Source, v Variant) error {
	select {
	case s.background <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.background }()
	_, err := s.Get(ctx, src, v)
	return err
}

// Get returns the path of a variant, creating it when it is not cached yet.
// Concurrent requests for the same variant share one creation.
func (s *Service) Get(ctx context.Context, src Source, v Variant) (string, error) {
	dst := s.CachePath(src.ID, v)
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	s.mu.Lock()
	if c, ok := s.inflight[dst]; ok {
		s.mu.Unlock()
		select {
		case <-c.done:
			return c.path, c.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	c := &call{done: make(chan struct{})}
	s.inflight[dst] = c
	s.mu.Unlock()

	// The creation is not tied to one request: another request may be waiting for it.
	c.path, c.err = s.create(context.WithoutCancel(ctx), src, v, dst)
	close(c.done)
	s.mu.Lock()
	delete(s.inflight, dst)
	s.mu.Unlock()
	return c.path, c.err
}

func (s *Service) create(ctx context.Context, src Source, v Variant, dst string) (string, error) {
	// The thumbnail and preview of a video are made from its poster frame.
	// Get the poster before taking a work slot: a thumbnail that held a slot
	// while waiting for its poster could wait forever when every slot is
	// taken by such thumbnails.
	var poster string
	if src.Kind == Video && v != Poster {
		p, err := s.Get(ctx, Source{ID: src.ID, Path: src.Path, Kind: Video}, Poster)
		if err != nil {
			return "", err
		}
		poster = p
	}

	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-s.sem }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return "", err
	}
	tmp := dst + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 36) + filepath.Ext(dst)
	defer os.Remove(tmp)

	var err error
	switch {
	case v == Poster && src.Kind == Video:
		err = s.videoFrame(ctx, src.Path, tmp, posterSize)
	case src.Kind == Video:
		err = s.imageVariant(ctx, poster, src.Overlay, sizeOf(v), tmp)
	case src.Kind == Photo && v != Poster:
		err = s.imageVariant(ctx, src.Path, src.Overlay, sizeOf(v), tmp)
	case src.Kind == Document && v != Poster:
		err = s.documentImage(ctx, src, sizeOf(v), tmp)
	default:
		err = ErrUnsupported
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func sizeOf(v Variant) int {
	if v == Preview {
		return previewSize
	}
	return thumbSize
}

func (s *Service) videoFrame(ctx context.Context, in, out string, size int) error {
	if s.tools.FFmpeg == "" {
		return ErrUnsupported
	}
	scale := fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease", size, size)
	// Seek one second in, which skips black first frames; short videos fall back to the start.
	for _, at := range []string{"1", "0"} {
		_, err := s.run(ctx, 2*time.Minute, s.tools.FFmpeg, "-nostdin", "-v", "error", "-ss", at, "-i", in,
			"-frames:v", "1", "-vf", scale, "-q:v", "3", "-y", out)
		if err == nil {
			if st, err := os.Stat(out); err == nil && st.Size() > 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("no frame in %s", filepath.Base(in))
}

// imageVariant scales an image to fit in size×size, places the overlay on top
// when there is one, and writes WebP.
func (s *Service) imageVariant(ctx context.Context, in, overlay string, size int, out string) error {
	if s.tools.VipsThumb == "" {
		return ErrUnsupported
	}
	target := out + "[Q=80,strip]"
	if strings.HasSuffix(out, ".png") {
		target = out
	}
	if !strings.Contains(in, "[") {
		in += "[0]"
	}
	if overlay == "" {
		_, err := s.run(ctx, 2*time.Minute, s.tools.VipsThumb, in, "--size", fmt.Sprintf("%dx%d>", size, size), "-o", target)
		return err
	}
	base := out + ".base.png"
	ov := out + ".overlay.png"
	defer os.Remove(base)
	defer os.Remove(ov)
	if _, err := s.run(ctx, 2*time.Minute, s.tools.VipsThumb, in, "--size", fmt.Sprintf("%dx%d>", size, size), "-o", base); err != nil {
		return err
	}
	hdr, err := s.run(ctx, 20*time.Second, s.tools.VipsHeader, "-a", base)
	if err != nil {
		return err
	}
	f := parseColonFields(hdr)
	w, h := f["width"], f["height"]
	if w == "" || h == "" {
		return errors.New("unknown size")
	}
	// The overlay is stretched to exactly the size of the image it belongs to.
	if _, err := s.run(ctx, time.Minute, s.tools.VipsThumb, overlay, "--size", w+"x"+h+"!", "-o", ov); err != nil {
		// A broken overlay should not hide the photo.
		_, err := s.run(ctx, time.Minute, s.tools.Vips, "copy", base, target)
		return err
	}
	_, err = s.run(ctx, time.Minute, s.tools.Vips, "composite2", base, ov, target, "over")
	return err
}

// Composite writes the image with its overlay at full size, for downloads.
func (s *Service) Composite(ctx context.Context, in, overlay, out string) error {
	if s.tools.Vips == "" {
		return ErrUnsupported
	}
	hdr, err := s.run(ctx, 20*time.Second, s.tools.VipsHeader, "-a", in)
	if err != nil {
		return err
	}
	f := parseColonFields(hdr)
	ov := out + ".overlay.png"
	defer os.Remove(ov)
	if _, err := s.run(ctx, time.Minute, s.tools.VipsThumb, overlay, "--size", f["width"]+"x"+f["height"]+"!", "-o", ov); err != nil {
		return err
	}
	target := out
	if strings.HasSuffix(strings.ToLower(out), ".jpg") {
		target += "[Q=92]"
	}
	_, err = s.run(ctx, 2*time.Minute, s.tools.Vips, "composite2", in, ov, target, "over")
	return err
}
