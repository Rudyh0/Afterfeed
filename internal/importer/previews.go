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

// previewsLeft is the number of media files still waiting for a thumbnail,
// out of previewsTotal in the current round.
var previewsLeft, previewsTotal atomic.Int64

// PreviewsLeft returns how many thumbnails are still being prepared.
func (m *Manager) PreviewsLeft() int64 { return previewsLeft.Load() }

// PreviewProgress returns how many thumbnails are left of how many, while
// they are being prepared.
func (m *Manager) PreviewProgress() (left, total int64) {
	return previewsLeft.Load(), previewsTotal.Load()
}

// PrepareNew reads the text in a memory made in the web app right away, so
// search finds it soon, and then makes any missing thumbnails.
func (m *Manager) PrepareNew(memoryID string) {
	ctx := m.ctx
	if ctx == nil || ctx.Err() != nil {
		return
	}
	var rel string
	if m.st.R.QueryRowContext(ctx, `SELECT path FROM memories WHERE id = ?`, memoryID).Scan(&rel) != nil {
		m.kickPreviews()
		return
	}
	m.running.Add(1)
	go func() {
		defer m.running.Done()
		if m.media.CanReadText() {
			var failures atomic.Int64
			if _, err := m.readMemoryText(ctx, textJob{memoryID: memoryID, rel: rel}, &failures); err != nil && ctx.Err() == nil {
				slog.Warn("read text", "memory", rel, "err", err)
			}
		}
		m.kickPreviews()
	}()
}

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
		// Then the text in photos, for search.
		if !m.importRunning() && !m.previewsWaiting() {
			if err := m.readTexts(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("read text in photos", "err", err)
			}
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
	// What Today shows comes first: the days around today in every year,
	// then the newest memories.
	now := time.Now().In(m.lib.Location(""))
	days := make([]any, 0, 7)
	for d := -3; d <= 3; d++ {
		days = append(days, now.AddDate(0, 0, d).Format("01-02"))
	}
	rows, err := m.st.R.QueryContext(ctx, `SELECT md.id, md.kind, md.mime, md.file, md.overlay, mm.path
		FROM media md JOIN memories mm ON mm.id = md.memory_id
		WHERE md.kind IN ('photo', 'video')
		ORDER BY CASE WHEN mm.month_day IN (?, ?, ?, ?, ?, ?, ?) THEN 0 ELSE 1 END, mm.taken_at DESC`, days...)
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
	previewsTotal.Store(int64(len(jobs)))
	defer previewsLeft.Store(0)
	began := time.Now()
	var busyMu sync.Mutex
	busy := map[string]time.Time{}
	stopLog := make(chan struct{})
	defer close(stopLog)
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		last, still := int64(-1), 0
		for {
			select {
			case <-stopLog:
				return
			case <-t.C:
				left := previewsLeft.Load()
				slog.Info("preparing previews", "left", left, "of", len(jobs))
				if left == last {
					still++
				} else {
					still = 0
				}
				last = left
				if still >= 4 {
					// Two minutes without progress: say which files it is on.
					busyMu.Lock()
					for p, since := range busy {
						slog.Warn("preview has not finished", "file", p, "for", time.Since(since).Round(time.Second))
					}
					busyMu.Unlock()
					still = 0
				}
			}
		}
	}()
	defer func() {
		if ctx.Err() == nil && previewsLeft.Load() == 0 {
			slog.Info("previews ready", "count", len(jobs), "took", time.Since(began).Round(time.Second))
		}
	}()

	ch := make(chan previewJob)
	var wg sync.WaitGroup
	for range m.media.BackgroundWorkers() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				busyMu.Lock()
				busy[j.src.Path] = time.Now()
				busyMu.Unlock()
				if err := m.media.Prepare(ctx, j.src, media.Thumb); err != nil && ctx.Err() == nil {
					slog.Debug("thumbnail", "id", j.src.ID, "err", err)
				}
				busyMu.Lock()
				delete(busy, j.src.Path)
				busyMu.Unlock()
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
