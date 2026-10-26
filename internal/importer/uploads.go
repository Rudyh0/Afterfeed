package importer

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rudyh0/afterfeed/internal/store"
)

// ChunkSize is the size of the parts the web app uploads.
const ChunkSize = 16 << 20

// Upload statuses.
const (
	UploadPartial  = "uploading"
	UploadComplete = "complete"
	// UploadInvalid is a complete upload that is not a readable zip file,
	// for example because the download from the platform was incomplete.
	UploadInvalid = "invalid"
)

// Upload is a file uploaded through the browser.
type Upload struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Received int64  `json:"received"`
	Platform string `json:"platform"`
	Detected string `json:"detected"`
	Status   string `json:"status"`
	ImportID *int64 `json:"import_id,omitempty"`
}

// Upload errors.
var (
	ErrOffset       = errors.New("upload.offset")
	ErrNoSpace      = errors.New("upload.no_space")
	ErrUploadTooBig = errors.New("upload.too_large")
)

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (m *Manager) uploadPath(id string) string {
	return filepath.Join(m.cfg.StagingDir(), id+".upload")
}

// CreateUpload starts an upload, or returns the unfinished upload of the
// same file so it can continue where it stopped.
func (m *Manager) CreateUpload(ctx context.Context, name string, size, modified int64, platform string) (*Upload, error) {
	if size <= 0 || name == "" {
		return nil, errors.New("upload.invalid")
	}
	var id string
	// Uploads of an import that failed or was stopped can be used again, so
	// adding the same file does not upload it a second time.
	err := m.st.R.QueryRowContext(ctx, `SELECT id FROM uploads WHERE name = ? AND size = ? AND modified = ?
		AND (import_id IS NULL OR import_id IN (SELECT id FROM imports WHERE status IN (?, ?)))
		AND status != ? ORDER BY created_at DESC LIMIT 1`,
		name, size, modified, StatusFailed, StatusCancelled, UploadInvalid).Scan(&id)
	if err == nil {
		u, err := m.GetUpload(ctx, id)
		if err == nil && u.Status == UploadComplete {
			if zerr := checkZip(m.uploadPath(id)); zerr != nil {
				// Uploaded before files were checked: start a new upload.
				slog.Warn("uploaded file is not a readable zip file", "name", u.Name, "size", u.Size, "err", zerr)
				m.st.W.ExecContext(ctx, `UPDATE uploads SET status = ?, import_id = NULL WHERE id = ?`, UploadInvalid, id)
				err = sql.ErrNoRows
			}
		}
		if err == nil && u.ImportID != nil {
			m.st.W.ExecContext(ctx, `UPDATE uploads SET import_id = NULL WHERE id = ?`, id)
			u.ImportID = nil
		}
		if err == nil {
			// The file on disk is what counts: it may be shorter after a crash.
			if fi, err := os.Stat(m.uploadPath(id)); err == nil && fi.Size() < u.Received {
				u.Received = fi.Size()
				m.st.W.ExecContext(ctx, `UPDATE uploads SET received = ?, status = ? WHERE id = ?`, u.Received, UploadPartial, id)
				u.Status = UploadPartial
			}
			if platform != "" && u.Platform != platform {
				m.st.W.ExecContext(ctx, `UPDATE uploads SET platform = ? WHERE id = ?`, platform, id)
				u.Platform = platform
			}
			return u, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := m.checkSpace(size); err != nil {
		return nil, err
	}
	b := make([]byte, 16)
	rand.Read(b)
	id = hex.EncodeToString(b)
	f, err := os.OpenFile(m.uploadPath(id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	f.Close()
	now := store.Now()
	if _, err := m.st.W.ExecContext(ctx, `INSERT INTO uploads (id, name, size, modified, received, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?)`, id, name, size, modified, platform, UploadPartial, now, now); err != nil {
		os.Remove(m.uploadPath(id))
		return nil, err
	}
	return m.GetUpload(ctx, id)
}

// checkSpace refuses an upload that does not fit on the data volume, with
// some room to spare for the import itself.
func (m *Manager) checkSpace(need int64) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(m.cfg.StagingDir(), &st); err != nil {
		return nil
	}
	free := int64(st.Bavail) * int64(st.Bsize)
	if free < need+256<<20 {
		return ErrNoSpace
	}
	return nil
}

// WriteUpload appends a chunk at offset, which must be where the upload
// stopped.
func (m *Manager) WriteUpload(ctx context.Context, id string, offset int64, r io.Reader) (*Upload, error) {
	u, err := m.GetUpload(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.Status == UploadComplete || u.Status == UploadInvalid {
		return u, nil
	}
	if offset != u.Received {
		return u, ErrOffset
	}
	f, err := os.OpenFile(m.uploadPath(id), os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	limit := min(u.Size-offset, ChunkSize+1)
	n, copyErr := io.Copy(f, io.LimitReader(r, limit))
	if offset+n > u.Size || n > ChunkSize {
		f.Truncate(offset)
		return u, ErrUploadTooBig
	}
	if err := f.Sync(); err != nil {
		return nil, err
	}
	u.Received = offset + n
	status := UploadPartial
	detected := ""
	if u.Received == u.Size {
		status = UploadComplete
		f.Close()
		// Check the file now, so a broken part shows up in the list instead
		// of when the import starts.
		if err := checkZip(m.uploadPath(id)); err != nil {
			status = UploadInvalid
			slog.Warn("uploaded file is not a readable zip file", "name", u.Name, "size", u.Size, "err", err)
		} else {
			detected = m.Detect(m.uploadPath(id), u.Name)
		}
	}
	if _, err := m.st.W.ExecContext(ctx, `UPDATE uploads SET received = ?, status = ?, detected = ?, updated_at = ? WHERE id = ?`,
		u.Received, status, detected, store.Now(), id); err != nil {
		return nil, err
	}
	u.Status, u.Detected = status, detected
	if copyErr != nil {
		// The connection dropped mid-chunk; what arrived is kept.
		return u, copyErr
	}
	return u, nil
}

// GetUpload returns an upload.
func (m *Manager) GetUpload(ctx context.Context, id string) (*Upload, error) {
	if !validID(id) {
		return nil, sql.ErrNoRows
	}
	var u Upload
	var imp sql.NullInt64
	err := m.st.R.QueryRowContext(ctx, `SELECT id, name, size, modified, received, platform, detected, status, import_id
		FROM uploads WHERE id = ?`, id).Scan(&u.ID, &u.Name, &u.Size, &u.Modified, &u.Received, &u.Platform, &u.Detected, &u.Status, &imp)
	if err != nil {
		return nil, err
	}
	if imp.Valid {
		u.ImportID = &imp.Int64
	}
	return &u, nil
}

// ListUploads returns uploads that are not part of an import yet.
func (m *Manager) ListUploads(ctx context.Context, platform string) ([]Upload, error) {
	rows, err := m.st.R.QueryContext(ctx, `SELECT id, name, size, modified, received, platform, detected, status
		FROM uploads WHERE import_id IS NULL AND (? = '' OR platform = ?) ORDER BY created_at`, platform, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Upload{}
	for rows.Next() {
		var u Upload
		if err := rows.Scan(&u.ID, &u.Name, &u.Size, &u.Modified, &u.Received, &u.Platform, &u.Detected, &u.Status); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteUpload removes an upload and its file.
func (m *Manager) DeleteUpload(ctx context.Context, id string) error {
	if !validID(id) {
		return sql.ErrNoRows
	}
	if err := os.Remove(m.uploadPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err := m.st.W.ExecContext(ctx, `DELETE FROM uploads WHERE id = ?`, id)
	return err
}

// CleanupUploads removes unfinished uploads that have not changed for 30
// days, and upload files without a database row.
func (m *Manager) CleanupUploads(ctx context.Context) error {
	cutoff := store.FormatTime(time.Now().Add(-30 * 24 * time.Hour))
	rows, err := m.st.R.QueryContext(ctx, `SELECT id FROM uploads WHERE status = ? AND import_id IS NULL AND updated_at < ?`,
		UploadPartial, cutoff)
	if err != nil {
		return err
	}
	var old []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			old = append(old, id)
		}
	}
	rows.Close()
	for _, id := range old {
		m.DeleteUpload(ctx, id)
	}
	entries, err := os.ReadDir(m.cfg.StagingDir())
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".upload" {
			continue
		}
		id := name[:len(name)-len(".upload")]
		if _, err := m.GetUpload(ctx, id); errors.Is(err, sql.ErrNoRows) {
			os.Remove(filepath.Join(m.cfg.StagingDir(), name))
		}
	}
	return nil
}

// checkZip reports whether a file is a zip file that can be read.
func checkZip(p string) error {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return err
	}
	return zr.Close()
}
