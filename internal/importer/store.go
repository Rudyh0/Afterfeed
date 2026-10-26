package importer

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

// storeSink is the second pass: it copies media into the library and
// indexes everything. Memories are prepared by a few workers in parallel and
// written to the database in batches.
type storeSink struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	m        *Manager
	importID int64
	platform string
	total    int
	progress func(done, total int)

	memCh chan *Memory
	idxCh chan prepared
	work  sync.WaitGroup
	index sync.WaitGroup
	done  atomic.Int64

	mu     sync.Mutex
	seen   map[string]bool
	convs  map[string]bool
	report Report
	years  map[int]*YearReport
	first  time.Time
	last   time.Time
}

type prepared struct {
	rel  string
	mem  *library.Memory
	year int
}

func newStoreSink(parent context.Context, m *Manager, importID int64, platform string, total int, progress func(done, total int)) *storeSink {
	ctx, cancel := context.WithCancelCause(parent)
	s := &storeSink{
		ctx: ctx, cancel: cancel, m: m, importID: importID, platform: platform,
		total: max(total, 1), progress: progress,
		memCh: make(chan *Memory, 32),
		idxCh: make(chan prepared, 64),
		seen:  map[string]bool{},
		convs: map[string]bool{},
		years: map[int]*YearReport{},
	}
	workers := min(4, max(2, runtime.NumCPU()/2))
	for range workers {
		s.work.Add(1)
		go s.worker()
	}
	s.index.Add(1)
	go s.indexer()
	return s
}

func (s *storeSink) fail(err error) {
	if err != nil && s.ctx.Err() == nil {
		s.cancel(err)
	}
}

func (s *storeSink) tick() {
	n := s.done.Add(1)
	if s.progress != nil {
		s.progress(int(n), s.total)
	}
}

// Memory queues a memory for storing.
func (s *storeSink) Memory(mem *Memory) {
	if s.ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	key := s.platform + ":" + mem.Key
	dup := s.seen[key]
	s.seen[key] = true
	s.mu.Unlock()
	if dup {
		s.tick()
		return
	}
	select {
	case s.memCh <- mem:
	case <-s.ctx.Done():
	}
}

func (s *storeSink) worker() {
	defer s.work.Done()
	for mem := range s.memCh {
		if s.ctx.Err() != nil {
			continue
		}
		if err := s.storeMemory(mem); err != nil {
			if errors.Is(err, errSkip) {
				s.tick()
				continue
			}
			var pe *problemError
			if errors.As(err, &pe) {
				s.Problem(pe.path, pe.err)
				s.tick()
				continue
			}
			s.fail(err)
		}
	}
}

var errSkip = errors.New("skip")

type problemError struct {
	path string
	err  error
}

func (e *problemError) Error() string { return e.path + ": " + e.err.Error() }

func (s *storeSink) storeMemory(im *Memory) error {
	key := s.platform + ":" + im.Key
	_, rel, err := s.m.lib.MemoryBySourceKey(s.ctx, key)
	if err != nil {
		return err
	}
	if rel != "" {
		return s.mergeMemory(rel, im, false)
	}

	takenAt := im.TakenAt
	if takenAt.IsZero() {
		for _, md := range im.Media {
			if !md.TakenAt.IsZero() {
				takenAt = md.TakenAt
				break
			}
		}
	}
	if takenAt.IsZero() {
		for _, md := range im.Media {
			if md.File != nil && !md.File.Modified.IsZero() {
				takenAt = md.File.Modified
				break
			}
		}
	}
	if takenAt.IsZero() {
		return &problemError{im.Source, fmt.Errorf("no date for %s", im.Key)}
	}

	now := time.Now().UTC()
	mem := &library.Memory{
		ID:       library.MemoryID(key),
		Platform: s.platform,
		Kind:     im.Kind,
		Fields: library.Fields{
			TakenAt: takenAt.UTC().Truncate(time.Second),
			TZ:      im.TZ,
			Text:    im.Text,
			Link:    im.Link,
			Place:   im.Place,
		},
		Album:     im.Album,
		Source:    library.Source{Key: key, Path: im.Source, Import: s.importID},
		CreatedAt: now,
		UpdatedAt: now,
	}
	hasMedia := false
	for _, md := range im.Media {
		if md.File != nil {
			hasMedia = true
		}
	}
	// Without its photo or video, a memory that had nothing else but a place
	// is left out: it would be an empty card.
	if !hasMedia && strings.TrimSpace(mem.Text) == "" && mem.Link == "" && (mem.Place == nil || len(im.Media) > 0) {
		return errSkip
	}

	// An import that was interrupted may have saved this memory without
	// indexing it. Continue from that folder instead of copying again.
	if existing := s.m.lib.MemoryRel(mem); fileExists(filepath.Join(s.m.lib.MemoryDir(existing), "memory.json")) {
		return s.mergeMemory(existing, im, true)
	}

	// A folder without memory.json is what an interrupted copy left behind.
	os.RemoveAll(s.m.lib.MemoryDir(s.m.lib.MemoryRel(mem)))
	rel, err = s.m.lib.CreateMemoryDir(mem)
	if err != nil {
		return err
	}
	dir := s.m.lib.MemoryDir(rel)
	// Every medium of a memory needs its own key. The same file listed
	// twice is copied once; another file with the same name gets a number.
	used := map[string]string{}
	for _, md := range im.Media {
		if md.File == nil {
			continue
		}
		key := md.key()
		if path, ok := used[key]; ok {
			if path == md.File.Path {
				continue
			}
			for n := 2; ; n++ {
				if _, ok := used[fmt.Sprintf("%s#%d", key, n)]; !ok {
					key = fmt.Sprintf("%s#%d", key, n)
					break
				}
			}
			md.Key = key
		}
		used[key] = md.File.Path
		lm, err := s.copyMedia(dir, len(mem.Media)+1, mem.ID, &md)
		if err != nil {
			if s.ctx.Err() != nil {
				return s.ctx.Err()
			}
			s.Problem(md.File.Path, err)
			continue
		}
		mem.Media = append(mem.Media, lm)
	}
	if len(mem.Media) == 0 && strings.TrimSpace(mem.Text) == "" && mem.Link == "" && mem.Place == nil {
		os.RemoveAll(dir)
		return errSkip
	}
	library.PlaceFromMedia(mem)
	s.m.lib.FillPlace(mem.Place)
	if err := s.m.lib.SaveMemory(rel, mem); err != nil {
		return err
	}
	year := mem.TakenAt.In(s.m.lib.Location(mem.TZ)).Year()
	s.mu.Lock()
	yr := s.year(year)
	yr.New++
	s.report.New++
	for _, md := range mem.Media {
		if md.Kind == string(media.Video) {
			yr.Videos++
			s.report.Videos++
		} else {
			yr.Photos++
			s.report.Photos++
		}
	}
	local := mem.TakenAt.In(s.m.lib.Location(mem.TZ))
	if s.first.IsZero() || local.Before(s.first) {
		s.first = local
	}
	if local.After(s.last) {
		s.last = local
	}
	s.mu.Unlock()

	select {
	case s.idxCh <- prepared{rel: rel, mem: mem, year: year}:
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	return nil
}

// mergeMemory adds media that an earlier import did not have, for example
// from a part that was missing then. Everything else, including the user's
// edits, stays as it is.
//
// index is set for a memory that is on disk but not yet in the index.
func (s *storeSink) mergeMemory(rel string, im *Memory, index bool) error {
	defer s.m.lib.LockMemory(rel)()
	mem, err := s.m.lib.LoadMemory(rel)
	if err != nil {
		return &problemError{rel, err}
	}
	dir := s.m.lib.MemoryDir(rel)
	// Before 0.1.7 a memory could get the same medium twice, which stopped
	// the import. The copy is removed.
	have := map[string]bool{}
	added := 0
	cleaned := false
	kept := mem.Media[:0]
	for _, md := range mem.Media {
		if have[md.Key] {
			os.Remove(filepath.Join(dir, md.File))
			cleaned = true
			continue
		}
		have[md.Key] = true
		kept = append(kept, md)
	}
	mem.Media = kept
	for _, md := range im.Media {
		if md.File == nil || have[md.key()] {
			continue
		}
		lm, err := s.copyMedia(dir, len(mem.Media)+1, mem.ID, &md)
		if err != nil {
			s.Problem(md.File.Path, err)
			continue
		}
		mem.Media = append(mem.Media, lm)
		have[lm.Key] = true
		added++
	}
	year := mem.TakenAt.In(s.m.lib.Location(mem.TZ)).Year()
	s.mu.Lock()
	if index {
		// Left by an interrupted import: new to the archive.
		yr := s.year(year)
		yr.New++
		s.report.New++
		for _, md := range mem.Media {
			if md.Kind == string(media.Video) {
				yr.Videos++
				s.report.Videos++
			} else {
				yr.Photos++
				s.report.Photos++
			}
		}
	} else {
		s.report.Existing++
		s.year(year).Existing++
		if added > 0 {
			s.report.Updated++
		}
	}
	s.mu.Unlock()
	if added == 0 && !index && !cleaned {
		s.tick()
		return nil
	}
	if added > 0 || cleaned {
		mem.UpdatedAt = time.Now().UTC()
		if err := s.m.lib.SaveMemory(rel, mem); err != nil {
			return err
		}
	}
	select {
	case s.idxCh <- prepared{rel: rel, mem: mem, year: year}:
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	return nil
}

func (s *storeSink) year(y int) *YearReport {
	yr := s.years[y]
	if yr == nil {
		yr = &YearReport{Year: y}
		s.years[y] = yr
	}
	return yr
}

// copyMedia copies a file from the export into a memory folder and reads
// its properties.
func (s *storeSink) copyMedia(dir string, n int, memoryID string, md *Media) (library.Media, error) {
	key := md.key()
	s.m.startFile(s.importID, md.File.Path)
	defer s.m.endFile(s.importID, md.File.Path)
	t, name, size, sum, err := copyFile(s.ctx, md.File, dir, fmt.Sprintf("%02d", n))
	if err != nil {
		return library.Media{}, err
	}
	if md.Kind != "" {
		t.Kind = md.Kind
	}
	info := s.m.media.Probe(s.ctx, filepath.Join(dir, name), t)
	lm := library.Media{
		ID:       library.MediaID(memoryID, key),
		Key:      key,
		File:     name,
		Kind:     string(info.Kind),
		Mime:     info.Mime,
		Size:     size,
		SHA256:   sum,
		Width:    info.Width,
		Height:   info.Height,
		Duration: info.Duration,
		Text:     md.Text,
		Source:   md.File.Path,
	}
	switch {
	case !md.TakenAt.IsZero():
		t := md.TakenAt.UTC().Truncate(time.Second)
		lm.TakenAt = &t
	case !info.TakenAt.IsZero():
		t := info.TakenAt.UTC()
		lm.TakenAt = &t
	}
	if md.Lat != nil && md.Lon != nil {
		lm.Lat, lm.Lon = md.Lat, md.Lon
	} else {
		lm.Lat, lm.Lon = info.Lat, info.Lon
	}
	if md.Overlay != nil {
		if _, oname, _, _, err := copyFile(s.ctx, md.Overlay, dir, fmt.Sprintf("%02d.overlay", n)); err == nil {
			lm.Overlay = oname
		} else {
			s.Problem(md.Overlay.Path, err)
		}
	}
	return lm, nil
}

// copyFile copies f into dir as base plus the extension that matches its
// content, and returns the detected type, file name, size and SHA-256.
func copyFile(ctx context.Context, f *archive.File, dir, base string) (media.Type, string, int64, string, error) {
	rc, err := f.Open()
	if err != nil {
		return media.Type{}, "", 0, "", err
	}
	defer rc.Close()
	br := bufio.NewReaderSize(rc, 256<<10)
	head, _ := br.Peek(media.SniffLen)
	t := media.Sniff(head, f.Name())
	ext := t.Ext
	if ext == "" {
		ext = strings.ToLower(path.Ext(f.Name()))
	}
	if ext == "" || len(ext) > 8 {
		ext = ".bin"
	}
	name := base + ext
	dst := filepath.Join(dir, name)
	for i := 2; fileExists(dst); i++ {
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
		dst = filepath.Join(dir, name)
	}
	tmp := dst + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return t, "", 0, "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), &ctxReader{ctx: ctx, r: br})
	if err == nil {
		err = out.Close()
	} else {
		out.Close()
	}
	if err != nil {
		os.Remove(tmp)
		return t, "", 0, "", err
	}
	if !f.Modified.IsZero() {
		os.Chtimes(tmp, f.Modified, f.Modified)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return t, "", 0, "", err
	}
	return t, name, n, hex.EncodeToString(h.Sum(nil)), nil
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// ctxReader stops a copy when the context is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// indexer writes prepared memories to the database in batches.
func (s *storeSink) indexer() {
	defer s.index.Done()
	var pending []prepared
	flush := func() {
		if len(pending) == 0 {
			return
		}
		err := s.withTx(func(tx *sql.Tx) error {
			for _, p := range pending {
				if err := s.m.lib.IndexMemory(s.ctx, tx, p.rel, p.mem, s.importID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			s.fail(err)
		}
		for range pending {
			s.tick()
		}
		pending = pending[:0]
	}
	timer := time.NewTicker(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case p, ok := <-s.idxCh:
			if !ok {
				flush()
				return
			}
			pending = append(pending, p)
			if len(pending) >= 100 {
				flush()
			}
		case <-timer.C:
			flush()
		}
	}
}

func (s *storeSink) withTx(fn func(*sql.Tx) error) error {
	tx, err := s.m.st.W.BeginTx(s.ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Conversation stores a conversation right away: new messages are added to
// its folder and the index.
func (s *storeSink) Conversation(c *Conversation) {
	if s.ctx.Err() != nil {
		return
	}
	if err := s.storeConversation(c); err != nil {
		if s.ctx.Err() != nil {
			return
		}
		var pe *problemError
		if errors.As(err, &pe) {
			s.Problem(pe.path, pe.err)
		} else {
			s.fail(err)
		}
	}
	s.tick()
}

func (s *storeSink) storeConversation(c *Conversation) error {
	key := s.platform + ":" + c.Key
	rel, err := s.m.lib.FindConversation(s.ctx, s.platform, key, c.MergeKey)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var conv *library.Conversation
	isNew := rel == ""
	if isNew {
		conv = &library.Conversation{
			ID: library.ConversationID(key), Platform: s.platform, Key: key, MergeKey: c.MergeKey,
			Title: c.Title, Participants: c.Participants, IsGroup: c.IsGroup, Self: c.Self,
			CreatedAt: now, UpdatedAt: now,
		}
		if conv.Title == "" {
			conv.Title = strings.Join(others(c.Participants, c.Self), ", ")
		}
	} else {
		conv, err = s.m.lib.LoadConversation(rel)
		if err != nil {
			return &problemError{rel, err}
		}
		conv.Participants = union(conv.Participants, c.Participants)
		if conv.Self == "" {
			conv.Self = c.Self
		}
		conv.IsGroup = conv.IsGroup || c.IsGroup
	}

	have := make(map[string]bool, len(conv.Messages))
	for _, m := range conv.Messages {
		have[m.Key] = true
	}
	var dir string
	if !isNew {
		dir = s.m.lib.ConversationDir(rel)
	}
	used := map[string]int{}
	added := 0
	for i := range c.Messages {
		msg := &c.Messages[i]
		k := messageKey(s.platform, msg)
		used[k]++
		if n := used[k]; n > 1 {
			k = fmt.Sprintf("%s#%d", k, n)
		}
		if have[k] {
			continue
		}
		lm := library.Message{
			Key: k, At: msg.At.UnixMilli(), Sender: msg.Sender, Self: msg.Self, Kind: msg.Kind,
			Text: msg.Text, Link: msg.Link, Reactions: msg.Reactions,
		}
		for j := range msg.Files {
			f := &msg.Files[j]
			if f.File == nil {
				continue
			}
			if dir == "" {
				if rel, err = s.m.lib.CreateConversationDir(conv); err != nil {
					return err
				}
				dir = s.m.lib.ConversationDir(rel)
			}
			mf, err := s.copyMessageFile(dir, conv.ID, k, f)
			if err != nil {
				if s.ctx.Err() != nil {
					return s.ctx.Err()
				}
				s.Problem(f.File.Path, err)
				continue
			}
			lm.Files = append(lm.Files, mf)
		}
		if lm.Kind == "" {
			lm.Kind = MessageText
		}
		conv.Messages = append(conv.Messages, lm)
		have[k] = true
		added++
	}
	s.mu.Lock()
	// Two exports of one chat (such as Messenger's encrypted download) count once.
	if !s.convs[conv.ID] {
		s.convs[conv.ID] = true
		s.report.Conversations++
		if isNew {
			s.report.NewConversations++
		}
	}
	s.report.Messages += added
	s.mu.Unlock()
	if added == 0 && !isNew {
		return nil
	}
	if len(conv.Messages) == 0 {
		return nil
	}
	if rel == "" {
		if rel, err = s.m.lib.CreateConversationDir(conv); err != nil {
			return err
		}
	}
	conv.UpdatedAt = now
	if err := s.m.lib.SaveConversation(rel, conv); err != nil {
		return err
	}
	return s.withTx(func(tx *sql.Tx) error { return s.m.lib.IndexConversation(s.ctx, tx, rel, conv) })
}

func (s *storeSink) copyMessageFile(dir, convID, msgKey string, f *Media) (library.MessageFile, error) {
	name := f.Name
	if name == "" {
		name = f.File.Name()
	}
	base := safeName(strings.TrimSuffix(f.File.Name(), path.Ext(f.File.Name())))
	s.m.startFile(s.importID, f.File.Path)
	defer s.m.endFile(s.importID, f.File.Path)
	t, fname, size, _, err := copyFile(s.ctx, f.File, filepath.Join(dir, "files"), base)
	if err != nil {
		return library.MessageFile{}, err
	}
	if f.Kind != "" {
		t.Kind = f.Kind
	}
	mf := library.MessageFile{
		ID:     library.MessageFileID(convID, msgKey, f.key()),
		File:   "files/" + fname,
		Kind:   string(t.Kind),
		Name:   name,
		Mime:   t.Mime,
		Size:   size,
		Source: f.File.Path,
	}
	if t.Kind == media.Photo || t.Kind == media.Video || t.Kind == media.Audio {
		info := s.m.media.Probe(s.ctx, filepath.Join(dir, "files", fname), t)
		mf.Kind, mf.Mime = string(info.Kind), info.Mime
		mf.Width, mf.Height, mf.Duration = info.Width, info.Height, info.Duration
		if f.Kind == media.Audio {
			mf.Kind = string(media.Audio)
		}
	}
	return mf, nil
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteByte('_')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
		if b.Len() > 100 {
			break
		}
	}
	out := strings.Trim(b.String(), ". ")
	if out == "" {
		return "file"
	}
	return out
}

func others(participants []string, self string) []string {
	var out []string
	for _, p := range participants {
		if p != self {
			out = append(out, p)
		}
	}
	return out
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{a, b} {
		for _, p := range list {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func (s *storeSink) Problem(path string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.report.Problems++
	if len(s.report.ProblemExamples) < maxExamples {
		s.report.ProblemExamples = append(s.report.ProblemExamples, Problem{Path: path, Error: err.Error()})
	}
	// The first problems were already logged in the first pass, but files can
	// also fail while copying.
	if s.report.Problems <= maxLoggedProblems {
		slog.Warn("import problem", "path", path, "err", err)
	}
}

// Progress is ignored in the second pass; progress follows stored items.
func (s *storeSink) Progress(done, total int) {}

// finish waits for all work and returns the report.
func (s *storeSink) finish() (*Report, error) {
	close(s.memCh)
	s.work.Wait()
	close(s.idxCh)
	s.index.Wait()
	defer s.cancel(nil)
	if cause := context.Cause(s.ctx); cause != nil && s.ctx.Err() != nil {
		return nil, cause
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.report
	for _, y := range s.years {
		r.Years = append(r.Years, *y)
	}
	sort.Slice(r.Years, func(i, j int) bool { return r.Years[i].Year < r.Years[j].Year })
	if !s.first.IsZero() {
		r.First, r.Last = s.first.Format("2006-01-02"), s.last.Format("2006-01-02")
	}
	return &r, nil
}
