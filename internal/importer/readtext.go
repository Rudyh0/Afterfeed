package importer

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

// textLeft is the number of photos and videos still waiting to have their
// text read, out of textTotal in the current round.
var textLeft, textTotal atomic.Int64

// TextProgress returns how many photos and videos are left of how many,
// while the text in them is being read.
func (m *Manager) TextProgress() (left, total int64) {
	return textLeft.Load(), textTotal.Load()
}

// textPending selects the photos and videos whose text has not been read,
// or was read by an older reader. It takes the current version.
const textPending = `(ocr_text IS NULL OR ocr_version < ?) AND kind IN ('photo', 'video')`

// textJob is a memory with media whose text has not been read yet.
type textJob struct {
	memoryID, rel string
}

// maxTextFailures stops a round when reading fails this many times in a
// row: something is wrong with the tools, not with the photos.
const maxTextFailures = 25

// readTexts reads the text in photos and videos that have not been looked
// at yet, so search finds words on signs, screenshots and captions. It runs
// after the thumbnails, in the background.
func (m *Manager) readTexts(ctx context.Context) error {
	if !m.media.CanReadText() {
		return nil
	}
	var pending int64
	if err := m.st.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM media
		WHERE `+textPending, media.TextVersion).Scan(&pending); err != nil {
		return err
	}
	if pending == 0 {
		return nil
	}
	rows, err := m.st.R.QueryContext(ctx, `SELECT mm.id, mm.path FROM memories mm
		WHERE EXISTS (SELECT 1 FROM media WHERE media.memory_id = mm.id AND `+textPending+`)
		ORDER BY mm.taken_at DESC`, media.TextVersion)
	if err != nil {
		return err
	}
	var jobs []textJob
	for rows.Next() {
		var j textJob
		if err := rows.Scan(&j.memoryID, &j.rel); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	slog.Info("reading text in photos", "count", pending)
	textLeft.Store(pending)
	textTotal.Store(pending)
	defer textLeft.Store(0)
	began := time.Now()
	var found, failures atomic.Int64
	stopLog := make(chan struct{})
	defer close(stopLog)
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-stopLog:
				return
			case <-t.C:
				slog.Info("reading text in photos", "left", textLeft.Load(), "of", pending)
			}
		}
	}()

	wctx, stop := context.WithCancel(ctx)
	defer stop()
	ch := make(chan textJob)
	var wg sync.WaitGroup
	for range m.media.BackgroundWorkers() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				pending := countPending(wctx, m, j.memoryID)
				n, err := m.readMemoryText(wctx, j, &failures)
				textLeft.Add(-int64(pending))
				found.Add(int64(n))
				if err != nil && wctx.Err() == nil {
					slog.Warn("read text", "memory", j.rel, "err", err)
				}
				if failures.Load() >= maxTextFailures {
					stop()
				}
			}
		}()
	}
	complete := true
	for _, j := range jobs {
		if m.importRunning() || m.previewsWaiting() {
			// Thumbnails and imports go first; continue afterwards.
			m.kickPreviews()
			complete = false
			break
		}
		select {
		case ch <- j:
		case <-wctx.Done():
		}
		if wctx.Err() != nil {
			break
		}
	}
	close(ch)
	wg.Wait()
	if failures.Load() >= maxTextFailures && ctx.Err() == nil {
		slog.Warn("stopped reading text in photos after failing for every photo; it is tried again after the next restart or import")
		return nil
	}
	if ctx.Err() == nil && complete {
		slog.Info("text in photos read", "count", pending, "with_text", found.Load(),
			"took", time.Since(began).Round(time.Second))
	}
	return nil
}

// previewsWaiting reports whether a new round of thumbnails was asked for.
func (m *Manager) previewsWaiting() bool { return len(m.previewWake) > 0 }

// readMemoryText reads the text in the media of one memory and stores it in
// memory.json and in the index. It returns how many had text.
func (m *Manager) readMemoryText(ctx context.Context, j textJob, failures *atomic.Int64) (int, error) {
	mem, err := m.lib.LoadMemory(j.rel)
	if errors.Is(err, fs.ErrNotExist) {
		// Removed meanwhile, for example with its source.
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	dir := m.lib.MemoryDir(j.rel)
	texts := map[string]string{}
	withText := 0
	for _, md := range mem.Media {
		if (md.OCR != nil && md.OCRVersion >= media.TextVersion) || (md.Kind != string(media.Photo) && md.Kind != string(media.Video)) {
			continue
		}
		src := media.Source{ID: md.ID, Path: filepath.Join(dir, md.File), Kind: media.Kind(md.Kind), Mime: md.Mime}
		if md.Overlay != "" {
			src.Overlay = filepath.Join(dir, md.Overlay)
		}
		text, err := m.media.ReadText(ctx, src)
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			// Not stored: it is tried again in a later round. Text from an
			// older reader stays meanwhile.
			failures.Add(1)
			slog.Debug("read text", "file", src.Path, "err", err)
			continue
		}
		failures.Store(0)
		texts[md.ID] = text
		if text != "" {
			withText++
		}
	}
	if len(texts) == 0 {
		// Text that memory.json already has but the index does not, for
		// example after an edit raced with an earlier round.
		return 0, m.indexText(ctx, j.memoryID, mem)
	}
	return withText, m.saveText(ctx, j, texts)
}

// saveText stores the text read for media of a memory. The memory is
// loaded again under its lock, so changes made meanwhile are kept.
func (m *Manager) saveText(ctx context.Context, j textJob, texts map[string]string) error {
	defer m.lib.LockMemory(j.rel)()
	mem, err := m.lib.LoadMemory(j.rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for i := range mem.Media {
		if t, ok := texts[mem.Media[i].ID]; ok {
			t := t
			mem.Media[i].OCR = &t
			mem.Media[i].OCRVersion = media.TextVersion
		}
	}
	if err := m.lib.SaveMemory(j.rel, mem); err != nil {
		return err
	}
	return m.indexText(ctx, j.memoryID, mem)
}

// indexText copies the text of a memory's media to the index.
func (m *Manager) indexText(ctx context.Context, memoryID string, mem *library.Memory) error {
	tx, err := m.st.W.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, md := range mem.Media {
		if md.OCR == nil {
			continue
		}
		if _, err := tx.Exec(`UPDATE media SET ocr_text = ?, ocr_version = ? WHERE id = ?`, *md.OCR, md.OCRVersion, md.ID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE memories SET ocr_text = ? WHERE id = ? AND ocr_text != ?`,
		mem.OCRText(), memoryID, mem.OCRText()); err != nil {
		return err
	}
	return tx.Commit()
}

func countPending(ctx context.Context, m *Manager, memoryID string) int {
	var n int
	m.st.R.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE memory_id = ? AND `+textPending,
		memoryID, media.TextVersion).Scan(&n)
	return n
}
