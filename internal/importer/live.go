package importer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rudyh0/afterfeed/internal/store"
)

// liveState is the progress of a running import, kept in memory and saved
// to the database every second.
type liveState struct {
	phase    string
	progress float64
	// done and total count items in the current phase, when known.
	done, total int
	phaseStart  time.Time
	// changed is when the progress last moved.
	changed time.Time
	// inflight holds the files being copied right now, with their start time.
	inflight map[string]time.Time
}

const (
	// progressLogEvery is how often a running import writes a progress line
	// to the log.
	progressLogEvery = 15 * time.Second
	// stallAfter is how long an import may go without progress before the
	// log says what it is waiting for.
	stallAfter = 2 * time.Minute
)

func (m *Manager) live(id int64) *liveState {
	l := m.lives[id]
	if l == nil {
		l = &liveState{inflight: map[string]time.Time{}}
		m.lives[id] = l
	}
	return l
}

// setLive changes the phase and progress, between 0 and 1.
func (m *Manager) setLive(id int64, phase string, progress float64) {
	if progress != progress { // NaN
		progress = 0
	}
	now := time.Now()
	m.mu.Lock()
	l := m.live(id)
	if l.phase != phase {
		l.phaseStart = now
		l.done, l.total = 0, 0
	}
	if l.phase != phase || l.progress != progress {
		l.changed = now
	}
	l.phase, l.progress = phase, min(max(progress, 0), 1)
	m.mu.Unlock()
}

// setCounts records how many items of the current phase are done.
func (m *Manager) setCounts(id int64, done, total int) {
	m.mu.Lock()
	l := m.live(id)
	if l.done != done || l.total != total {
		l.changed = time.Now()
	}
	l.done, l.total = done, total
	if total > 0 {
		l.progress = min(max(float64(done)/float64(total), 0), 1)
	}
	m.mu.Unlock()
}

// startFile and endFile track the files being copied, so a stalled import
// can say which file it is waiting for.
func (m *Manager) startFile(id int64, path string) {
	m.mu.Lock()
	m.live(id).inflight[path] = time.Now()
	m.mu.Unlock()
}

func (m *Manager) endFile(id int64, path string) {
	m.mu.Lock()
	delete(m.live(id).inflight, path)
	m.mu.Unlock()
}

// liveSnapshot returns a copy of the live state, or nil.
func (m *Manager) liveSnapshot(id int64) *liveState {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.lives[id]
	if l == nil {
		return nil
	}
	c := *l
	c.inflight = nil
	return &c
}

// report saves the progress every second and writes progress lines to the
// log, until ctx is done or stop is closed.
func (m *Manager) report(ctx context.Context, id int64, stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	lastLog := time.Now()
	lastStallLog := time.Time{}
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case now := <-t.C:
			m.persistLive(ctx, id)
			m.mu.Lock()
			l := m.lives[id]
			if l == nil {
				m.mu.Unlock()
				continue
			}
			phase, progress, done, total := l.phase, l.progress, l.done, l.total
			phaseStart, changed := l.phaseStart, l.changed
			oldest, oldestSince := "", time.Time{}
			for p, since := range l.inflight {
				if oldest == "" || since.Before(oldestSince) {
					oldest, oldestSince = p, since
				}
			}
			m.mu.Unlock()

			if now.Sub(lastLog) >= progressLogEvery {
				lastLog = now
				args := []any{"id", id, "phase", phase, "percent", int(progress * 100)}
				if total > 0 {
					args = append(args, "done", done, "total", total)
					if elapsed := now.Sub(phaseStart); done > 0 && elapsed > 5*time.Second {
						rate := float64(done) / elapsed.Minutes()
						left := time.Duration(float64(total-done) / rate * float64(time.Minute))
						args = append(args, "per_minute", int(rate), "left", roundDuration(left))
					}
				}
				slog.Info("import progress", args...)
			}
			if !changed.IsZero() && now.Sub(changed) >= stallAfter && now.Sub(lastStallLog) >= stallAfter {
				lastStallLog = now
				args := []any{"id", id, "phase", phase, "for", roundDuration(now.Sub(changed))}
				if oldest != "" {
					args = append(args, "file", oldest, "file_for", roundDuration(now.Sub(oldestSince)))
				}
				slog.Warn("import has not moved", args...)
			}
		}
	}
}

func (m *Manager) persistLive(ctx context.Context, id int64) {
	l := m.liveSnapshot(id)
	if l == nil {
		return
	}
	m.st.W.ExecContext(ctx, `UPDATE imports SET phase = ?, progress = ?, updated_at = ? WHERE id = ?`,
		l.phase, l.progress, store.Now(), id)
}

// roundDuration makes a duration short enough to read in a log line.
func roundDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return d.Round(time.Second).String()
	case d < time.Hour:
		return d.Round(10 * time.Second).String()
	default:
		return d.Round(time.Minute).String()
	}
}

// humanBytes formats a size for log lines.
func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
