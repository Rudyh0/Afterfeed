package library

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"path/filepath"
)

// RebuildStats counts what a rebuild indexed.
type RebuildStats struct {
	Memories      int
	Conversations int
	Skipped       []string
}

// Rebuild replaces the index of memories and conversations with what is on
// the data volume. Accounts and settings are not touched.
func (l *Library) Rebuild(ctx context.Context, progress func(RebuildStats)) (RebuildStats, error) {
	var stats RebuildStats
	if _, err := l.st.W.ExecContext(ctx, `DELETE FROM media; DELETE FROM memories; DELETE FROM message_files;
		DELETE FROM messages; DELETE FROM conversations; DELETE FROM month_stats;`); err != nil {
		return stats, err
	}

	batch := newBatch(ctx, l.st.W, 200)
	err := filepath.WalkDir(l.memories, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "memory.json" {
			return nil
		}
		rel, _ := filepath.Rel(l.memories, filepath.Dir(p))
		rel = filepath.ToSlash(rel)
		m, err := l.LoadMemory(rel)
		if err != nil || m.ID == "" {
			stats.Skipped = append(stats.Skipped, p)
			slog.Warn("skip memory", "path", p, "err", err)
			return nil
		}
		if err := batch.add(func(tx *sql.Tx) error { return l.IndexMemory(ctx, tx, rel, m, m.Source.Import) }); err != nil {
			return err
		}
		stats.Memories++
		if progress != nil && stats.Memories%500 == 0 {
			progress(stats)
		}
		return ctx.Err()
	})
	if err != nil {
		return stats, err
	}
	if err := batch.flush(); err != nil {
		return stats, err
	}

	err = filepath.WalkDir(l.messages, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "conversation.json" {
			return nil
		}
		rel, _ := filepath.Rel(l.messages, filepath.Dir(p))
		rel = filepath.ToSlash(rel)
		c, err := l.LoadConversation(rel)
		if err != nil || c.ID == "" {
			stats.Skipped = append(stats.Skipped, p)
			slog.Warn("skip conversation", "path", p, "err", err)
			return nil
		}
		// Conversations can be large; each gets its own transaction.
		tx, err := l.st.W.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := l.IndexConversation(ctx, tx, rel, c); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		stats.Conversations++
		if progress != nil {
			progress(stats)
		}
		return ctx.Err()
	})
	if err != nil {
		return stats, err
	}
	return stats, l.RefreshMonthStats(ctx)
}

// batch groups index writes into transactions.
type batch struct {
	ctx  context.Context
	db   *sql.DB
	size int
	fns  []func(*sql.Tx) error
}

func newBatch(ctx context.Context, db *sql.DB, size int) *batch {
	return &batch{ctx: ctx, db: db, size: size}
}

func (b *batch) add(fn func(*sql.Tx) error) error {
	b.fns = append(b.fns, fn)
	if len(b.fns) >= b.size {
		return b.flush()
	}
	return nil
}

func (b *batch) flush() error {
	if len(b.fns) == 0 {
		return nil
	}
	tx, err := b.db.BeginTx(b.ctx, nil)
	if err != nil {
		return err
	}
	for _, fn := range b.fns {
		if err := fn(tx); err != nil {
			tx.Rollback()
			return err
		}
	}
	b.fns = b.fns[:0]
	return tx.Commit()
}
