package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// documentImage renders the first page of a document. PDF files are
// rendered directly; office documents are converted to PDF by the document
// service first, when one is configured.
func (s *Service) documentImage(ctx context.Context, src Source, size int, out string) error {
	pdf := src.Path
	if src.Mime != "application/pdf" {
		if s.docService == "" {
			return ErrUnsupported
		}
		converted := s.CachePath(src.ID, "document")
		converted = converted[:len(converted)-len(filepath.Ext(converted))] + ".pdf"
		if _, err := os.Stat(converted); err != nil {
			if err := s.convertToPDF(ctx, src.Path, converted); err != nil {
				return err
			}
		}
		pdf = converted
	}
	return s.imageVariant(ctx, pdf+"[page=0]", "", size, out)
}

// convertToPDF sends a document to Gotenberg's LibreOffice route.
func (s *Service) convertToPDF(ctx context.Context, in, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("files", filepath.Base(in))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, io.LimitReader(f, 200<<20)); err != nil {
		return err
	}
	mw.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.docService+"/forms/libreoffice/convert", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("document service: %s: %s", res.Status, msg)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return err
	}
	tmp := out + ".tmp"
	o, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, io.LimitReader(res.Body, 500<<20)); err != nil {
		o.Close()
		os.Remove(tmp)
		return err
	}
	o.Close()
	return os.Rename(tmp, out)
}

// waveformPoints is the number of bars in a voice message waveform.
const waveformPoints = 48

// Waveform returns the loudness of an audio file in a few dozen steps,
// between 0 and 1, for drawing voice messages.
func (s *Service) Waveform(ctx context.Context, src Source) ([]float64, error) {
	cache := s.CachePath(src.ID, "waveform")
	cache = cache[:len(cache)-len(filepath.Ext(cache))] + ".json"
	if data, err := os.ReadFile(cache); err == nil {
		var w []float64
		if json.Unmarshal(data, &w) == nil {
			return w, nil
		}
	}
	if s.tools.FFmpeg == "" {
		return nil, ErrUnsupported
	}
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.sem }()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.tools.FFmpeg, "-nostdin", "-v", "error", "-i", src.Path,
		"-ac", "1", "-ar", "4000", "-f", "s16le", "-t", "600", "-")
	pcm, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	n := len(pcm) / 2
	if n == 0 {
		return nil, errors.New("no audio")
	}
	out := make([]float64, waveformPoints)
	per := max(1, n/waveformPoints)
	peak := 0.0
	for i := range out {
		sum := 0.0
		count := 0
		for j := i * per; j < min(n, (i+1)*per); j++ {
			v := float64(int16(binary.LittleEndian.Uint16(pcm[j*2:]))) / 32768
			sum += v * v
			count++
		}
		if count > 0 {
			out[i] = math.Sqrt(sum / float64(count))
		}
		peak = math.Max(peak, out[i])
	}
	if peak > 0 {
		for i := range out {
			out[i] = math.Round(out[i]/peak*100) / 100
		}
	}
	if data, err := json.Marshal(out); err == nil {
		os.MkdirAll(filepath.Dir(cache), 0o750)
		os.WriteFile(cache, data, 0o640)
	}
	return out, nil
}
