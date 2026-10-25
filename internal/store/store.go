// Package store opens the SQLite database and applies migrations.
//
// The database is an index: the source of truth for memories and messages is
// the data volume. It uses WAL mode with one writer connection and a pool of
// reader connections, which keeps reads fast while an import is writing.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // registers the "sqlite3" driver
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// TimeFormat is the format of every timestamp stored in the database. It sorts
// lexicographically in chronological order.
const TimeFormat = "2006-01-02T15:04:05Z"

// Store holds the database connections.
type Store struct {
	// W is the single writer connection. Use it for every statement that changes data.
	W *sql.DB
	// R is a pool of read-only connections.
	R *sql.DB
}

// Open opens (and creates, if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	params := url.Values{}
	params.Set("_journal_mode", "WAL")
	params.Set("_synchronous", "NORMAL")
	params.Set("_foreign_keys", "on")
	params.Set("_busy_timeout", "10000")
	params.Set("_txlock", "immediate")
	params.Set("_cache_size", "-65536")

	w, err := sql.Open("sqlite3", "file:"+path+"?"+params.Encode())
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	w.SetConnMaxLifetime(0)

	rparams := url.Values{}
	for k, v := range params {
		rparams[k] = v
	}
	rparams.Del("_txlock")
	rparams.Del("_journal_mode")
	rparams.Set("mode", "ro")
	r, err := sql.Open("sqlite3", "file:"+path+"?"+rparams.Encode())
	if err != nil {
		w.Close()
		return nil, err
	}
	r.SetMaxOpenConns(max(4, runtime.NumCPU()))

	s := &Store{W: w, R: r}
	if err := s.migrate(context.Background()); err != nil {
		s.Close()
		return nil, err
	}
	for _, db := range []*sql.DB{w, r} {
		if _, err := db.Exec("PRAGMA temp_store = MEMORY; PRAGMA mmap_size = 268435456;"); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

// Close closes all connections.
func (s *Store) Close() error {
	return errors.Join(s.R.Close(), s.W.Close())
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.W.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		var exists int
		if err := s.W.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.W.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, version, Now()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Now returns the current time in the database time format.
func Now() string { return time.Now().UTC().Format(TimeFormat) }

// FormatTime formats t in the database time format.
func FormatTime(t time.Time) string { return t.UTC().Format(TimeFormat) }

// ParseTime parses a timestamp in the database time format.
func ParseTime(s string) (time.Time, error) { return time.Parse(TimeFormat, s) }

// GetSetting reads a setting into v. It returns false when the setting is not set.
func (s *Store) GetSetting(ctx context.Context, key string, v any) (bool, error) {
	var raw string
	err := s.R.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

// SetSetting stores v as the value of a setting.
func (s *Store) SetSetting(ctx context.Context, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.W.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, string(raw))
	return err
}
