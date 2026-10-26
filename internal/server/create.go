package server

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

// newMemoryData is the form of a new memory, sent as the "data" part of a
// multipart request together with the files.
type newMemoryData struct {
	Text string `json:"text"`
	// Date and Time are local to TZ. When DateAuto is true, the capture date
	// of the first photo or video is used if it has one.
	Date     string `json:"date"`
	Time     string `json:"time"`
	TZ       string `json:"tz"`
	DateAuto bool   `json:"date_auto"`
	Place    *struct {
		Name string   `json:"name"`
		Lat  *float64 `json:"lat"`
		Lon  *float64 `json:"lon"`
	} `json:"place"`
	Vault bool `json:"vault"`
}

const maxNewMemoryBytes = 20 << 30

type uploadedFile struct {
	path string
	name string
	t    media.Type
	size int64
	sum  string
	info media.Info
}

func (s *Server) createMemory(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	mr, err := r.MultipartReader()
	if err != nil {
		return errBadRequest("invalid_form", err.Error())
	}
	b := make([]byte, 8)
	rand.Read(b)
	tmpDir := filepath.Join(s.cfg.StagingDir(), "new-"+hex.EncodeToString(b))
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	var data newMemoryData
	var files []uploadedFile
	var total int64
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errBadRequest("invalid_form", err.Error())
		}
		switch part.FormName() {
		case "data":
			if err := json.NewDecoder(io.LimitReader(part, 1<<20)).Decode(&data); err != nil {
				return errBadRequest("invalid_json", err.Error())
			}
		case "file":
			f, err := saveUpload(part, tmpDir, len(files), maxNewMemoryBytes-total)
			if err != nil {
				return err
			}
			total += f.size
			files = append(files, *f)
		}
		part.Close()
	}
	data.Text = strings.TrimSpace(data.Text)
	if len(files) == 0 && data.Text == "" {
		return errBadRequest("new_memory.empty", "add a photo, a video or text")
	}

	loc := s.deps.Library.Location(data.TZ)
	tz := ""
	if data.TZ != "" && loc.String() == data.TZ {
		tz = data.TZ
	}
	taken, err := time.ParseInLocation("2006-01-02 15:04", data.Date+" "+data.Time, loc)
	if err != nil {
		taken = time.Now()
	}
	for i := range files {
		files[i].info = s.deps.Media.Probe(ctx, files[i].path, files[i].t)
	}
	if data.DateAuto {
		for _, f := range files {
			if !f.info.TakenAt.IsZero() {
				if f.info.HasZone {
					taken = f.info.TakenAt
				} else {
					// A time without zone is the local time where the photo was taken.
					t := f.info.TakenAt
					taken = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
				}
				break
			}
		}
	}

	now := time.Now().UTC()
	key := "afterfeed:" + hex.EncodeToString(b) + fmt.Sprint(now.UnixNano())
	mem := &library.Memory{
		ID:       library.MemoryID(key),
		Platform: "afterfeed",
		Kind:     "memory",
		Fields: library.Fields{
			TakenAt: taken.UTC().Truncate(time.Second),
			TZ:      tz,
			Text:    data.Text,
		},
		Vault:     data.Vault,
		Source:    library.Source{Key: key},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if p := data.Place; p != nil && (strings.TrimSpace(p.Name) != "" || (p.Lat != nil && p.Lon != nil)) {
		mem.Place = &library.Place{Name: strings.TrimSpace(p.Name), Lat: p.Lat, Lon: p.Lon}
	} else {
		for _, f := range files {
			if f.info.Lat != nil && f.info.Lon != nil {
				mem.Place = &library.Place{Lat: f.info.Lat, Lon: f.info.Lon}
				break
			}
		}
	}

	rel, err := s.deps.Library.CreateMemoryDir(mem)
	if err != nil {
		return err
	}
	dir := s.deps.Library.MemoryDir(rel)
	for i, f := range files {
		name := fmt.Sprintf("%02d%s", i+1, f.t.Ext)
		if err := os.Rename(f.path, filepath.Join(dir, name)); err != nil {
			os.RemoveAll(dir)
			return err
		}
		mkey := fmt.Sprintf("%02d-%s", i+1, strings.ToLower(f.name))
		md := library.Media{
			ID: library.MediaID(mem.ID, mkey), Key: mkey, File: name, Kind: string(f.info.Kind), Mime: f.info.Mime,
			Size: f.size, SHA256: f.sum, Width: f.info.Width, Height: f.info.Height, Duration: f.info.Duration,
			Lat: f.info.Lat, Lon: f.info.Lon, Source: f.name,
		}
		if !f.info.TakenAt.IsZero() {
			t := f.info.TakenAt.UTC()
			md.TakenAt = &t
		}
		mem.Media = append(mem.Media, md)
	}
	library.PlaceFromMedia(mem)
	s.deps.Library.FillPlace(mem.Place)
	if err := s.deps.Library.SaveMemory(rel, mem); err != nil {
		os.RemoveAll(dir)
		return err
	}
	tx, err := s.store.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := s.deps.Library.IndexMemory(ctx, tx, rel, mem, 0); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := s.deps.Library.RefreshMonthStats(ctx); err != nil {
		return err
	}
	if s.deps.Imports != nil {
		s.deps.Imports.PrepareNew(mem.ID)
	}
	c, err := s.loadCard(ctx, mem.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, c)
	return nil
}

// saveUpload stores one uploaded file and recognises its type.
func saveUpload(part io.Reader, dir string, n int, limit int64) (*uploadedFile, error) {
	name := "file"
	if p, ok := part.(interface{ FileName() string }); ok && p.FileName() != "" {
		name = filepath.Base(p.FileName())
	}
	br := bufio.NewReaderSize(part, 64<<10)
	head, _ := br.Peek(media.SniffLen)
	t := media.Sniff(head, name)
	if t.Kind != media.Photo && t.Kind != media.Video {
		return nil, errBadRequest("new_memory.unsupported", name)
	}
	p := filepath.Join(dir, fmt.Sprintf("%02d%s", n, t.Ext))
	out, err := os.Create(p)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(br, limit+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if size > limit {
		return nil, errBadRequest("new_memory.too_large", name)
	}
	return &uploadedFile{path: p, name: name, t: t, size: size, sum: hex.EncodeToString(h.Sum(nil))}, nil
}
