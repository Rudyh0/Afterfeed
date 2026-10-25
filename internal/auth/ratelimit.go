package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rudyh0/afterfeed/internal/store"
)

// Failed attempts are counted per key. After freeAttempts failures the key is
// locked for an increasing time: 30 seconds, then 1, 2, 4 minutes and so on,
// up to maxLock.
const (
	freeAttempts = 5
	firstLock    = 30 * time.Second
	maxLock      = 15 * time.Minute
)

// CheckAttempts returns a RateLimitedError when key is locked.
func (s *Service) CheckAttempts(ctx context.Context, key string) error {
	var locked sql.NullString
	err := s.st.R.QueryRowContext(ctx, `SELECT locked_until FROM attempts WHERE key = ?`, key).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) || !locked.Valid {
		return nil
	}
	if err != nil {
		return err
	}
	until, err := store.ParseTime(locked.String)
	if err != nil {
		return nil
	}
	if d := until.Sub(s.now()); d > 0 {
		return &RateLimitedError{RetryAfter: d}
	}
	return nil
}

// FailAttempt records a failure for key and returns a RateLimitedError when
// the key is now locked.
func (s *Service) FailAttempt(ctx context.Context, key string) error {
	now := s.now().UTC()
	tx, err := s.st.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var failures int
	err = tx.QueryRowContext(ctx, `SELECT failures FROM attempts WHERE key = ?`, key).Scan(&failures)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	failures++
	var lockedUntil any
	var lock time.Duration
	if failures >= freeAttempts {
		lock = firstLock << (failures - freeAttempts)
		if lock > maxLock || lock <= 0 {
			lock = maxLock
		}
		lockedUntil = store.FormatTime(now.Add(lock))
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO attempts(key, failures, locked_until, last_failure_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET failures = excluded.failures, locked_until = excluded.locked_until, last_failure_at = excluded.last_failure_at`,
		key, failures, lockedUntil, store.FormatTime(now)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if lock > 0 {
		return &RateLimitedError{RetryAfter: lock}
	}
	return nil
}

// ResetAttempts clears the failures for a key, or for every key with the given
// prefix when it ends with a colon.
func (s *Service) ResetAttempts(ctx context.Context, key string) {
	if len(key) > 0 && key[len(key)-1] == ':' {
		s.st.W.ExecContext(ctx, `DELETE FROM attempts WHERE key LIKE ? ESCAPE '\'`, escapeLike(key)+"%")
		return
	}
	s.st.W.ExecContext(ctx, `DELETE FROM attempts WHERE key = ?`, key)
}

func escapeLike(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '%', '_', '\\':
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	return string(out)
}
