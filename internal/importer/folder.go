package importer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rudyh0/afterfeed/internal/archive"
)

// FolderEntry is a zip file or folder in the import folder.
type FolderEntry struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Dir      bool   `json:"dir"`
	Modified string `json:"modified"`
	Platform string `json:"platform"`
}

// ImportFolder lists the zip files and folders in the import folder.
func (m *Manager) ImportFolder(ctx context.Context) ([]FolderEntry, error) {
	entries, err := os.ReadDir(m.cfg.ImportDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []FolderEntry{}, nil
		}
		return nil, err
	}
	out := []FolderEntry{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "#") {
			continue
		}
		p := filepath.Join(m.cfg.ImportDir, name)
		fi, err := os.Stat(p) // follows symlinks
		if err != nil {
			continue
		}
		entry := FolderEntry{Path: name, Name: name, Dir: fi.IsDir(), Modified: fi.ModTime().UTC().Format("2006-01-02T15:04:05Z")}
		switch {
		case fi.IsDir():
			entry.Size = dirSize(p)
		case strings.EqualFold(filepath.Ext(name), ".zip"):
			entry.Size = fi.Size()
		default:
			continue
		}
		entry.Platform = m.detectCached(p, fi)
		out = append(out, entry)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func dirSize(p string) int64 {
	var total int64
	n := 0
	filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		n++
		if n > 500000 {
			return filepath.SkipAll
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func (m *Manager) detectCached(p string, fi os.FileInfo) string {
	key := fmt.Sprintf("%s|%d|%d", p, fi.Size(), fi.ModTime().UnixNano())
	if v, ok := m.detectCache.Load(key); ok {
		return v.(string)
	}
	platform := m.Detect(p, fi.Name())
	m.detectCache.Store(key, platform)
	return platform
}

// Detect returns the platform a zip file or folder most likely belongs to,
// or an empty string.
func (m *Manager) Detect(p, fileName string) string {
	names, err := archive.Names(p, 20000)
	if err != nil {
		return ""
	}
	best, bestScore := "", 0.0
	for _, im := range m.order {
		if score := im.Detect(names, fileName); score > bestScore {
			best, bestScore = im.Platform(), score
		}
	}
	if bestScore < 0.5 {
		return ""
	}
	return best
}
