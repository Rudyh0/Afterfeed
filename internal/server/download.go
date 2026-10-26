package server

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
)

// Downloads carry the memory's date, location and description inside the
// file, so photos show up at the right time and place in other apps. Most
// exports strip this from the files; the originals on the data volume are
// never changed.

func (s *Server) memoryForMedia(ctx context.Context, mediaID string) (*library.Memory, string, error) {
	var rel string
	var vault bool
	err := s.store.R.QueryRowContext(ctx, `SELECT mm.path, mm.vault FROM media md JOIN memories mm ON mm.id = md.memory_id
		WHERE md.id = ?`, mediaID).Scan(&rel, &vault)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", errNotFound()
	}
	if err != nil {
		return nil, "", err
	}
	mem, err := s.deps.Library.LoadMemory(rel)
	if err != nil {
		return nil, "", err
	}
	return mem, rel, nil
}

// downloadFile writes a copy of a media file with its metadata to a
// temporary file and returns its path and download name.
func (s *Server) downloadFile(ctx context.Context, mem *library.Memory, rel string, md library.Media, index int) (string, string, error) {
	dir := s.deps.Library.MemoryDir(rel)
	src := filepath.Join(dir, md.File)
	ext := strings.ToLower(filepath.Ext(md.File))
	taken := mem.TakenAt
	if md.TakenAt != nil {
		taken = *md.TakenAt
	}
	local := taken.In(s.deps.Library.Location(mem.TZ))
	name := fmt.Sprintf("%s-%s-%02d%s", mem.Platform, local.Format("20060102-150405"), index+1, ext)

	tmpDir := filepath.Join(s.cfg.CacheDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		return "", "", err
	}
	b := make([]byte, 8)
	rand.Read(b)
	tmp := filepath.Join(tmpDir, "dl-"+hex.EncodeToString(b)+ext)

	// A photo with a Snapchat overlay is downloaded as it was shown.
	if md.Overlay != "" && md.Kind == "photo" {
		tmp = strings.TrimSuffix(tmp, ext) + ".jpg"
		name = strings.TrimSuffix(name, ext) + ".jpg"
		if err := s.deps.Media.Composite(ctx, src, filepath.Join(dir, md.Overlay), tmp); err != nil {
			slog.Debug("composite for download", "err", err)
			tmp = strings.TrimSuffix(tmp, ".jpg") + ext
			name = strings.TrimSuffix(name, ".jpg") + ext
			if err := copyPath(src, tmp); err != nil {
				return "", "", err
			}
		}
	} else if err := copyPath(src, tmp); err != nil {
		return "", "", err
	}

	lat, lon := md.Lat, md.Lon
	if (lat == nil || lon == nil) && mem.Place.HasCoordinates() {
		lat, lon = mem.Place.Lat, mem.Place.Lon
	}
	text := md.Text
	if text == "" {
		text = mem.Text
	}
	if err := s.writeMetadata(ctx, tmp, md.Kind, local, lat, lon, text); err != nil {
		slog.Debug("write metadata", "file", md.File, "err", err)
	}
	return tmp, name, nil
}

func copyPath(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// writeMetadata writes date, time, location and description into a file
// with exiftool. Formats exiftool cannot write are left as they are.
func (s *Server) writeMetadata(ctx context.Context, path, kind string, local time.Time, lat, lon *float64, text string) error {
	tool := s.deps.Media.Tools().Exiftool
	if tool == "" {
		return errors.New("exiftool not available")
	}
	ext := strings.ToLower(filepath.Ext(path))
	_, offset := local.Zone()
	zone := fmt.Sprintf("%+03d:%02d", offset/3600, abs(offset%3600)/60)
	stamp := local.Format("2006:01:02 15:04:05")
	args := []string{"-q", "-q", "-overwrite_original", "-m"}
	switch {
	case kind == "photo" && (ext == ".jpg" || ext == ".jpeg" || ext == ".heic" || ext == ".png" || ext == ".tif" || ext == ".webp"):
		args = append(args, "-DateTimeOriginal="+stamp, "-CreateDate="+stamp, "-OffsetTimeOriginal="+zone, "-OffsetTime="+zone)
		if lat != nil && lon != nil {
			args = append(args,
				fmt.Sprintf("-GPSLatitude=%f", math.Abs(*lat)), "-GPSLatitudeRef="+ref(*lat, "N", "S"),
				fmt.Sprintf("-GPSLongitude=%f", math.Abs(*lon)), "-GPSLongitudeRef="+ref(*lon, "E", "W"))
		}
		if text != "" {
			args = append(args, "-XMP-dc:Description="+text)
		}
	case kind == "video" && (ext == ".mp4" || ext == ".mov" || ext == ".m4v"):
		// With QuickTimeUTC, exiftool stores the time in UTC as the format
		// requires; the zone tells it which time this is.
		args = append(args, "-api", "QuickTimeUTC=1", "-QuickTime:CreateDate="+stamp+zone,
			"-QuickTime:ModifyDate="+stamp+zone, "-Keys:CreationDate="+stamp+zone)
		if lat != nil && lon != nil {
			args = append(args, fmt.Sprintf("-Keys:GPSCoordinates=%f, %f", *lat, *lon))
		}
		if text != "" {
			args = append(args, "-Keys:Description="+text)
		}
	default:
		return nil
	}
	args = append(args, path)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, tool, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ref(v float64, pos, neg string) string {
	if v < 0 {
		return neg
	}
	return pos
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// downloadMedia serves one media file with its metadata.
func (s *Server) downloadMedia(w http.ResponseWriter, r *http.Request, mediaID string) error {
	ctx := r.Context()
	mem, rel, err := s.memoryForMedia(ctx, mediaID)
	if err != nil {
		return err
	}
	if mem.Vault && !s.vaultOpen(r) {
		return errNotFound()
	}
	for i, md := range mem.Media {
		if md.ID != mediaID {
			continue
		}
		tmp, name, err := s.downloadFile(ctx, mem, rel, md, i)
		if err != nil {
			return err
		}
		defer os.Remove(tmp)
		mime := md.Mime
		if strings.HasSuffix(tmp, ".jpg") && md.Overlay != "" {
			mime = "image/jpeg"
		}
		return serveFile(w, r, tmp, mime, name, false)
	}
	return errNotFound()
}

// downloadMemory serves all media of a memory as one zip file.
func (s *Server) downloadMemory(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var rel string
	if err := s.store.R.QueryRowContext(ctx, `SELECT path FROM memories WHERE id = ?`, r.PathValue("id")).Scan(&rel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNotFound()
		}
		return err
	}
	mem, err := s.deps.Library.LoadMemory(rel)
	if err != nil {
		return err
	}
	if mem.Vault && !s.vaultOpen(r) {
		return errNotFound()
	}
	if len(mem.Media) == 0 {
		return errNotFound()
	}
	local := mem.TakenAt.In(s.deps.Library.Location(mem.TZ))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, mem.Platform, local.Format("20060102-150405")))
	w.Header().Set("Cache-Control", "no-store")
	zw := zip.NewWriter(w)
	for i, md := range mem.Media {
		tmp, name, err := s.downloadFile(ctx, mem, rel, md, i)
		if err != nil {
			slog.Warn("download", "file", md.File, "err", err)
			continue
		}
		err = func() error {
			defer os.Remove(tmp)
			f, err := os.Open(tmp)
			if err != nil {
				return err
			}
			defer f.Close()
			fi, _ := f.Stat()
			hdr := &zip.FileHeader{Name: name, Method: zip.Store, Modified: local}
			hdr.SetMode(0o644)
			if fi != nil {
				hdr.UncompressedSize64 = uint64(fi.Size())
			}
			zf, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			_, err = io.Copy(zf, f)
			return err
		}()
		if err != nil {
			// The response has started; all we can do is stop.
			slog.Warn("download zip", "err", err)
			return nil
		}
	}
	zw.Close()
	return nil
}
