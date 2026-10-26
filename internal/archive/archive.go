// Package archive presents the parts of an export as one file tree.
//
// Platforms split large exports into several zip files, and a JSON file in
// one part can refer to a photo stored in another. A Tree combines every part,
// whether it is a zip file or a folder the user already extracted, so the
// importers can look files up without knowing which part holds them.
package archive

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxJSONSize is the largest JSON file ReadJSON accepts.
const MaxJSONSize = 1 << 30

// File is one file in the combined tree.
type File struct {
	// Path is the slash-separated path inside its part, without junk prefixes.
	Path     string
	Size     int64
	Modified time.Time
	// Part is the index of the part the file comes from.
	Part int

	zf   *zip.File
	disk string
}

// Name returns the base name of the file.
func (f *File) Name() string { return path.Base(f.Path) }

// Dir returns the directory of the file inside the tree.
func (f *File) Dir() string { return path.Dir(f.Path) }

// Ext returns the lower-case extension including the dot.
func (f *File) Ext() string { return strings.ToLower(path.Ext(f.Path)) }

// Open opens the file for reading.
func (f *File) Open() (io.ReadCloser, error) {
	if f.zf != nil {
		return f.zf.Open()
	}
	return os.Open(f.disk)
}

// Head returns up to n bytes from the start of the file.
func (f *File) Head(n int) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	buf := make([]byte, n)
	m, err := io.ReadFull(rc, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:m], nil
}

// Part is one zip file or folder of an export.
type Part struct {
	// Path is the location on disk.
	Path string
	Name string
	Size int64
	// Dir is true when the part is an extracted folder.
	Dir   bool
	Files int

	zr      *zip.Reader
	closer  io.Closer
	tempDir string
}

// Tree is the combined file tree of all parts.
type Tree struct {
	Parts []*Part

	files  []*File
	byPath map[string]*File
	byName map[string][]*File
	temp   string
}

// Open opens every path (zip files or folders) as one tree. Zip files inside
// a part are opened as extra parts. tempDir is used for nested zip files
// that are compressed; it may be empty when there are none.
func Open(paths []string, tempDir string) (*Tree, error) {
	t := &Tree{byPath: map[string]*File{}, byName: map[string][]*File{}, temp: tempDir}
	for _, p := range paths {
		if err := t.add(p, filepath.Base(p)); err != nil {
			t.Close()
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
	}
	sort.SliceStable(t.files, func(i, j int) bool { return t.files[i].Path < t.files[j].Path })
	return t, nil
}

func (t *Tree) add(p, name string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return t.addDir(p, name)
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		f.Close()
		return ErrNotZip
	}
	return t.addZip(zr, f, f, p, name, st.Size())
}

// ErrNotZip is returned for a file that is neither a zip file nor a folder.
var ErrNotZip = errors.New("not a zip file")

func (t *Tree) addZip(zr *zip.Reader, ra io.ReaderAt, closer io.Closer, p, name string, size int64) error {
	part := &Part{Path: p, Name: name, Size: size, zr: zr, closer: closer}
	idx := len(t.Parts)
	t.Parts = append(t.Parts, part)
	var nested []*zip.File
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		clean, ok := cleanPath(zf.Name)
		if !ok {
			continue
		}
		if strings.EqualFold(path.Ext(clean), ".zip") && looksNested(zf) {
			nested = append(nested, zf)
			continue
		}
		t.insert(&File{Path: clean, Size: int64(zf.UncompressedSize64), Modified: zf.Modified, Part: idx, zf: zf})
		part.Files++
	}
	for _, zf := range nested {
		if err := t.addNested(zf, ra, name); err != nil {
			return err
		}
	}
	return nil
}

// looksNested reports whether a .zip entry should be opened as a part. Small
// zip files are kept as normal files (they can be message attachments).
func looksNested(zf *zip.File) bool { return zf.UncompressedSize64 > 1<<20 }

func (t *Tree) addNested(zf *zip.File, outer io.ReaderAt, parent string) error {
	name := parent + " › " + path.Base(zf.Name)
	if zf.Method == zip.Store {
		// A stored zip can be read in place through the outer file.
		if off, err := zf.DataOffset(); err == nil {
			sr := io.NewSectionReader(outer, off, int64(zf.CompressedSize64))
			if zr, err := zip.NewReader(sr, int64(zf.CompressedSize64)); err == nil {
				return t.addZip(zr, sr, nopCloser{}, "", name, int64(zf.CompressedSize64))
			}
		}
	}
	if t.temp == "" {
		return errors.New("nested zip file needs a temporary folder")
	}
	if err := os.MkdirAll(t.temp, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(t.temp, "nested-*.zip")
	if err != nil {
		return err
	}
	// Unpacking a large inner zip file can take a while; say so in the log.
	began := time.Now()
	slog.Info("unpacking zip file inside the export", "name", name, "size", zf.UncompressedSize64)
	defer func() { slog.Info("unpacked zip file", "name", name, "took", time.Since(began).Round(time.Second)) }()
	rc, err := zf.Open()
	if err != nil {
		tmp.Close()
		return err
	}
	_, err = io.Copy(tmp, rc)
	rc.Close()
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	st, _ := tmp.Stat()
	zr, err := zip.NewReader(tmp, st.Size())
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil // not a zip after all; ignore
	}
	if err := t.addZip(zr, tmp, tmp, "", name, st.Size()); err != nil {
		return err
	}
	t.Parts[len(t.Parts)-1].tempDir = tmp.Name()
	return nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func (t *Tree) addDir(root, name string) error {
	part := &Part{Path: root, Name: name, Dir: true}
	idx := len(t.Parts)
	t.Parts = append(t.Parts, part)
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable folders are skipped
		}
		if d.IsDir() {
			if p != root && isJunkDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		clean, ok := cleanPath(filepath.ToSlash(rel))
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if strings.EqualFold(path.Ext(clean), ".zip") && info.Size() > 1<<20 {
			// An export copied to the import folder with its zip files inside a folder.
			return t.add(p, name+" › "+d.Name())
		}
		t.insert(&File{Path: clean, Size: info.Size(), Modified: info.ModTime(), Part: idx, disk: p})
		part.Files++
		part.Size += info.Size()
		return nil
	})
}

func (t *Tree) insert(f *File) {
	if _, dup := t.byPath[f.Path]; dup {
		// The same file in two parts: the export repeats it, keep the first.
		return
	}
	t.byPath[f.Path] = f
	key := strings.ToLower(f.Name())
	t.byName[key] = append(t.byName[key], f)
	t.files = append(t.files, f)
}

// Close closes every part and removes temporary files.
func (t *Tree) Close() error {
	var errs []error
	for _, p := range t.Parts {
		if p.closer != nil {
			errs = append(errs, p.closer.Close())
		}
		if p.tempDir != "" {
			os.Remove(p.tempDir)
		}
	}
	return errors.Join(errs...)
}

// Files returns every file, sorted by path.
func (t *Tree) Files() []*File { return t.files }

// Lookup returns the file at exactly this path.
func (t *Tree) Lookup(p string) *File { return t.byPath[p] }

// ByName returns the files with this base name, compared without case.
func (t *Tree) ByName(name string) []*File { return t.byName[strings.ToLower(name)] }

// Match returns the files for which fn returns true.
func (t *Tree) Match(fn func(*File) bool) []*File {
	var out []*File
	for _, f := range t.files {
		if fn(f) {
			out = append(out, f)
		}
	}
	return out
}

// Resolve finds the file a reference in the export points to. References are
// relative to the root of the export, but parts can have an extra folder
// around the export, so the reference is matched as a path suffix. When
// several files match, the one with the longest common suffix wins.
func (t *Tree) Resolve(ref string) *File {
	ref = normalizeRef(ref)
	if ref == "" {
		return nil
	}
	if f := t.byPath[ref]; f != nil {
		return f
	}
	candidates := t.byName[strings.ToLower(path.Base(ref))]
	switch len(candidates) {
	case 0:
		return nil
	case 1:
		return candidates[0]
	}
	var best *File
	bestScore := -1
	for _, c := range candidates {
		score := commonSuffix(c.Path, ref)
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best
}

// ResolveFrom resolves a reference relative to dir first, then like Resolve.
func (t *Tree) ResolveFrom(dir, ref string) *File {
	r := normalizeRef(ref)
	if r == "" {
		return nil
	}
	if f := t.byPath[path.Clean(path.Join(dir, r))]; f != nil {
		return f
	}
	return t.Resolve(r)
}

func normalizeRef(ref string) string {
	ref = strings.TrimSpace(strings.ReplaceAll(ref, "\\", "/"))
	if i := strings.IndexAny(ref, "?#"); i >= 0 && strings.Contains(ref, "://") {
		ref = ref[:i]
	}
	if strings.Contains(ref, "://") {
		return ""
	}
	ref = strings.TrimLeft(ref, "/")
	for strings.HasPrefix(ref, "./") {
		ref = ref[2:]
	}
	if ref == "" || ref == "." {
		return ""
	}
	return path.Clean(ref)
}

// commonSuffix counts the path segments two paths share at the end.
func commonSuffix(a, b string) int {
	as := strings.Split(strings.ToLower(a), "/")
	bs := strings.Split(strings.ToLower(b), "/")
	n := 0
	for i, j := len(as)-1, len(bs)-1; i >= 0 && j >= 0 && as[i] == bs[j]; i, j = i-1, j-1 {
		n++
	}
	return n
}

// cleanPath normalises a path from a zip file and drops operating system junk.
func cleanPath(p string) (string, bool) {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimLeft(p, "/")
	p = path.Clean(p)
	if p == "." || p == "" || strings.HasPrefix(p, "../") || p == ".." {
		return "", false
	}
	segs := strings.Split(p, "/")
	for _, s := range segs[:len(segs)-1] {
		if isJunkDir(s) {
			return "", false
		}
	}
	base := segs[len(segs)-1]
	if strings.HasPrefix(base, "._") || base == ".DS_Store" || strings.EqualFold(base, "Thumbs.db") || strings.EqualFold(base, "desktop.ini") {
		return "", false
	}
	return p, true
}

func isJunkDir(name string) bool {
	return name == "__MACOSX" || name == ".AppleDouble" || name == "@eaDir" || name == "#recycle" || name == ".Trashes"
}

// ReadAll reads a whole file up to limit bytes.
func ReadAll(f *File, limit int64) ([]byte, error) {
	if f.Size > limit {
		return nil, fmt.Errorf("%s is too large (%d bytes)", f.Path, f.Size)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit))
}

// ReadJSON decodes a JSON file into v.
func ReadJSON(f *File, v any) error {
	data, err := ReadAll(f, MaxJSONSize)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// ReadJS decodes a JavaScript data file of the form `window.X = <json>`, as
// used by the X archive.
func ReadJS(f *File, v any) error {
	data, err := ReadAll(f, MaxJSONSize)
	if err != nil {
		return err
	}
	i := strings.IndexAny(string(data[:min(len(data), 4096)]), "=")
	if i < 0 {
		return json.Unmarshal(data, v)
	}
	return json.Unmarshal(data[i+1:], v)
}

// Names lists the file paths in a zip file or folder without opening the
// whole tree, for recognising which platform an export belongs to.
func Names(p string, limit int) ([]string, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	var out []string
	if st.IsDir() {
		err := filepath.WalkDir(p, func(fp string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if len(out) >= limit {
				return filepath.SkipAll
			}
			if d.IsDir() {
				if fp != p && isJunkDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(p, fp)
			if err == nil {
				if c, ok := cleanPath(filepath.ToSlash(rel)); ok {
					out = append(out, c)
				}
			}
			return nil
		})
		return out, err
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, ErrNotZip
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if len(out) >= limit {
			break
		}
		if zf.FileInfo().IsDir() {
			continue
		}
		if c, ok := cleanPath(zf.Name); ok {
			out = append(out, c)
		}
	}
	return out, nil
}
