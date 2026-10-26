package library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Removed tells what RemovePlatform deleted.
type Removed struct {
	Memories      int
	Conversations int
	// FileIDs are the ids of the media and message files, whose cached
	// previews can be removed too.
	FileIDs []string
}

// RemovePlatform deletes everything imported from a platform: its memories
// and conversations, their folders with the photos, videos and files, and
// their entries in the index. Own memories cannot be removed this way.
//
// The folders go first and the index last: when this stops halfway, the
// index still lists what is left, and removing again finishes the job.
func (l *Library) RemovePlatform(ctx context.Context, platform string) (Removed, error) {
	var out Removed
	if platform == "" || platform == "afterfeed" {
		return out, fmt.Errorf("library: cannot remove platform %q", platform)
	}
	memRels, err := l.column(ctx, `SELECT path FROM memories WHERE platform = ?`, platform)
	if err != nil {
		return out, err
	}
	convRels, err := l.column(ctx, `SELECT path FROM conversations WHERE platform = ?`, platform)
	if err != nil {
		return out, err
	}
	mediaIDs, err := l.column(ctx, `SELECT md.id FROM media md JOIN memories m ON m.id = md.memory_id WHERE m.platform = ?`, platform)
	if err != nil {
		return out, err
	}
	fileIDs, err := l.column(ctx, `SELECT f.id FROM message_files f JOIN conversations c ON c.id = f.conversation_id
		WHERE c.platform = ?`, platform)
	if err != nil {
		return out, err
	}
	out.Memories, out.Conversations = len(memRels), len(convRels)
	out.FileIDs = append(mediaIDs, fileIDs...)

	for _, rel := range memRels {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		dir, ok := inside(l.memories, rel)
		if !ok {
			continue
		}
		unlock := l.LockMemory(rel)
		err := os.RemoveAll(dir)
		unlock()
		if err != nil {
			return out, err
		}
		removeIfEmpty(filepath.Dir(dir), l.memories)
	}
	for _, rel := range convRels {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		dir, ok := inside(l.messages, rel)
		if !ok {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return out, err
		}
		removeIfEmpty(filepath.Dir(dir), l.messages)
	}

	tx, err := l.st.W.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`UPDATE memories SET duplicate_of = NULL
			WHERE duplicate_of IN (SELECT id FROM memories WHERE platform = ?)`,
		// Media, messages and message files go with them.
		`DELETE FROM memories WHERE platform = ?`,
		`DELETE FROM conversations WHERE platform = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, platform); err != nil {
			return out, err
		}
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	return out, l.RefreshMonthStats(ctx)
}

func (l *Library) column(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := l.st.R.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s.String)
	}
	return out, rows.Err()
}

// inside returns the folder of rel under root, and false when rel is empty
// or would point at root itself or outside it.
func inside(root, rel string) (string, bool) {
	if strings.TrimSpace(rel) == "" {
		return "", false
	}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	r, err := filepath.Rel(root, dir)
	if err != nil || r == "." || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return dir, true
}

// removeIfEmpty removes dir when it is empty and below root, such as a year
// folder whose last memory was removed.
func removeIfEmpty(dir, root string) {
	if _, ok := inside(root, mustRel(root, dir)); !ok {
		return
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		os.Remove(dir)
	}
}

func mustRel(root, dir string) string {
	r, err := filepath.Rel(root, dir)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(r)
}
