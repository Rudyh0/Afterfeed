package importer

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rudyh0/afterfeed/internal/media"
)

// previewsLeft is the number of media files still waiting for a thumbnail.
var previewsLeft atomic.Int64

// PreviewsLeft returns how many thumbnails are still being prepared.
func (m *Manager) PreviewsLeft() int64 { return previewsLeft.Load() }

func (m *Manager) kickPreviews() {
	select {
	case m.previewWake <- struct{}{}:
	default:
	}
}

// previewWorker prepares thumbnails for media that do not have one yet, so
// the timeline is fast from the first visit. Thumbnails that are missing
// when a page asks for them are also made on request.
func (m *Manager) previewWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.previewWake:
		}
		// Let a running import finish first: it competes for the disk.
		for m.importRunning() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
		if err := m.preparePreviews(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("prepare previews", "err", err)
		}
	}
}

func (m *Manager) importRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.cancels) > 0
}

type previewJob struct {
	src media.Source
}

func (m *Manager) preparePreviews(ctx context.Context) error {
	rows, err := m.st.R.QueryContext(ctx, `SELECT md.id, md.kind, md.mime, md.file, md.overlay, mm.path
		FROM media md JOIN memories mm ON mm.id = md.memory_id
		WHERE md.kind IN ('photo', 'video') ORDER BY mm.taken_at DESC`)
	if err != nil {
		return err
	}
	var jobs []previewJob
	for rows.Next() {
		var id, kind, mime, file, overlay, rel string
		if err := rows.Scan(&id, &kind, &mime, &file, &overlay, &rel); err != nil {
			rows.Close()
			return err
		}
		if m.media.Has(id, media.Thumb) {
			continue
		}
		dir := m.lib.MemoryDir(rel)
		src := media.Source{ID: id, Path: filepath.Join(dir, file), Kind: media.Kind(kind), Mime: mime}
		if overlay != "" {
			src.Overlay = filepath.Join(dir, overlay)
		}
		jobs = append(jobs, previewJob{src: src})
	}
	rows.Close()
	if len(jobs) == 0 {
		return nil
	}
	slog.Info("preparing previews", "count", len(jobs))
	previewsLeft.Store(int64(len(jobs)))
	defer previewsLeft.Store(0)

	ch := make(chan previewJob)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				if _, err := m.media.Get(ctx, j.src, media.Thumb); err != nil && ctx.Err() == nil {
					slog.Debug("thumbnail", "id", j.src.ID, "err", err)
				}
				previewsLeft.Add(-1)
			}
		}()
	}
	for _, j := range jobs {
		if m.importRunning() {
			// An import started; continue afterwards.
			m.kickPreviews()
			break
		}
		select {
		case ch <- j:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(ch)
	wg.Wait()
	return nil
}
