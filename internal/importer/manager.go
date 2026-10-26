package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
	"github.com/rudyh0/afterfeed/internal/store"
)

// Import statuses.
const (
	StatusQueued     = "queued"
	StatusRunning    = "running"
	StatusNeedsParts = "needs_parts"
	StatusDone       = "done"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

// Import phases, shown while an import runs.
const (
	PhaseOpening  = "opening"
	PhaseReading  = "reading"
	PhaseCopying  = "copying"
	PhaseIndexing = "indexing"
)

// FileRef is one part of an import: an upload or a file in the import folder.
type FileRef struct {
	Kind     string `json:"kind"` // "upload" or "folder"
	ID       string `json:"id,omitempty"`
	Path     string `json:"path,omitempty"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Platform string `json:"platform,omitempty"`
}

// Status is the state of an import as shown in the web app.
type Status struct {
	ID       int64   `json:"id"`
	Platform string  `json:"platform"`
	Status   string  `json:"status"`
	Phase    string  `json:"phase"`
	Progress float64 `json:"progress"`
	// Done and Total count the items of the current phase while it runs.
	Done       int       `json:"done,omitempty"`
	Total      int       `json:"total,omitempty"`
	Files      []FileRef `json:"files"`
	Check      *Check    `json:"check,omitempty"`
	Report     *Report   `json:"report,omitempty"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  string    `json:"created_at"`
	FinishedAt string    `json:"finished_at,omitempty"`
}

// Problem is a file that could not be read.
type Problem struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Check is what the first read of an export found, before anything is stored.
type Check struct {
	Memories        int         `json:"memories"`
	Photos          int         `json:"photos"`
	Videos          int         `json:"videos"`
	Conversations   int         `json:"conversations"`
	Messages        int         `json:"messages"`
	Files           int         `json:"files"`
	Missing         int         `json:"missing"`
	MissingExamples []string    `json:"missing_examples,omitempty"`
	First           string      `json:"first,omitempty"`
	Last            string      `json:"last,omitempty"`
	Years           []YearCount `json:"years,omitempty"`
	Problems        int         `json:"problems"`
	ProblemExamples []Problem   `json:"problem_examples,omitempty"`
}

// YearCount is the number of memories in a year.
type YearCount struct {
	Year     int `json:"year"`
	Memories int `json:"memories"`
}

// Report is the result of a finished import.
type Report struct {
	New              int          `json:"new"`
	Existing         int          `json:"existing"`
	Updated          int          `json:"updated"`
	Photos           int          `json:"photos"`
	Videos           int          `json:"videos"`
	Conversations    int          `json:"conversations"`
	NewConversations int          `json:"new_conversations"`
	Messages         int          `json:"messages"`
	Years            []YearReport `json:"years"`
	First            string       `json:"first,omitempty"`
	Last             string       `json:"last,omitempty"`
	Missing          int          `json:"missing"`
	MissingExamples  []string     `json:"missing_examples,omitempty"`
	Problems         int          `json:"problems"`
	ProblemExamples  []Problem    `json:"problem_examples,omitempty"`
}

// YearReport counts what came in for one year.
type YearReport struct {
	Year     int `json:"year"`
	New      int `json:"new"`
	Existing int `json:"existing"`
	Photos   int `json:"photos"`
	Videos   int `json:"videos"`
}

const maxExamples = 50

// maxLoggedProblems is how many unreadable files an import writes to the log.
const maxLoggedProblems = 20

// Error codes shown to the user.
var (
	ErrEmpty           = errors.New("import.empty")
	ErrHTMLFormat      = errors.New("import.html_format")
	ErrNotZip          = errors.New("import.not_zip")
	ErrFileGone        = errors.New("import.file_gone")
	ErrUnknownPlatform = errors.New("import.unknown_platform")
)

// Manager runs imports one at a time, in the background.
type Manager struct {
	cfg       *config.Config
	st        *store.Store
	lib       *library.Library
	media     *media.Service
	importers map[string]Importer
	order     []Importer

	wake chan struct{}

	mu      sync.Mutex
	lives   map[int64]*liveState
	cancels map[int64]context.CancelFunc

	previewWake chan struct{}
	detectCache sync.Map
	running     sync.WaitGroup
	// ctx is the context passed to Start, for background work started later.
	ctx context.Context
}

// NewManager creates the import manager.
func NewManager(cfg *config.Config, st *store.Store, lib *library.Library, ms *media.Service, importers []Importer) *Manager {
	m := &Manager{
		cfg: cfg, st: st, lib: lib, media: ms,
		importers:   map[string]Importer{},
		order:       importers,
		wake:        make(chan struct{}, 1),
		lives:       map[int64]*liveState{},
		cancels:     map[int64]context.CancelFunc{},
		previewWake: make(chan struct{}, 1),
	}
	for _, im := range importers {
		m.importers[im.Platform()] = im
	}
	return m
}

// Platforms returns the names of the supported platforms.
func (m *Manager) Platforms() []string {
	out := make([]string, 0, len(m.order))
	for _, im := range m.order {
		out = append(out, im.Platform())
	}
	return out
}

// Start runs the import worker and the preview worker until ctx is done.
// Imports that were running when Afterfeed stopped start again.
func (m *Manager) Start(ctx context.Context) {
	m.ctx = ctx
	if _, err := m.st.W.ExecContext(ctx, `UPDATE imports SET status = ?, phase = '', progress = 0 WHERE status = ?`,
		StatusQueued, StatusRunning); err != nil {
		slog.Warn("requeue imports", "err", err)
	}
	m.running.Add(2)
	go func() {
		defer m.running.Done()
		m.worker(ctx)
	}()
	go func() {
		defer m.running.Done()
		m.previewWorker(ctx)
	}()
	m.kick()
	m.kickPreviews()
}

// Wait blocks until the workers have stopped after the context passed to
// Start is done.
func (m *Manager) Wait() { m.running.Wait() }

func (m *Manager) kick() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) worker(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		for {
			id, err := m.nextQueued(ctx)
			if err != nil || id == 0 {
				break
			}
			m.run(ctx, id)
			if ctx.Err() != nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-t.C:
		}
	}
}

func (m *Manager) nextQueued(ctx context.Context) (int64, error) {
	var id int64
	err := m.st.R.QueryRowContext(ctx, `SELECT id FROM imports WHERE status = ? ORDER BY id LIMIT 1`, StatusQueued).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// Create queues a new import of the given files.
func (m *Manager) Create(ctx context.Context, platform string, files []FileRef) (int64, error) {
	if _, ok := m.importers[platform]; !ok {
		return 0, ErrUnknownPlatform
	}
	files, err := m.resolveRefs(ctx, files)
	if err != nil {
		return 0, err
	}
	data, _ := json.Marshal(files)
	now := store.Now()
	res, err := m.st.W.ExecContext(ctx, `INSERT INTO imports (platform, status, files_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, platform, StatusQueued, string(data), now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	m.attachUploads(ctx, id, files)
	m.kick()
	return id, nil
}

// AddFiles adds parts to an import that is waiting for them and checks again.
func (m *Manager) AddFiles(ctx context.Context, id int64, files []FileRef) error {
	st, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	if st.Status != StatusNeedsParts && st.Status != StatusFailed {
		return errors.New("import.not_waiting")
	}
	files, err = m.resolveRefs(ctx, files)
	if err != nil {
		return err
	}
	all := st.Files
	for _, f := range files {
		dup := false
		for _, e := range all {
			if e.Kind == f.Kind && e.ID == f.ID && e.Path == f.Path {
				dup = true
			}
		}
		if !dup {
			all = append(all, f)
		}
	}
	data, _ := json.Marshal(all)
	_, err = m.st.W.ExecContext(ctx, `UPDATE imports SET files_json = ?, status = ?, phase = '', progress = 0,
		error = '', check_json = NULL, updated_at = ? WHERE id = ?`, string(data), StatusQueued, store.Now(), id)
	if err == nil {
		m.attachUploads(ctx, id, files)
		m.kick()
	}
	return err
}

// Continue starts an import that is waiting for parts without them.
func (m *Manager) Continue(ctx context.Context, id int64) error {
	res, err := m.st.W.ExecContext(ctx, `UPDATE imports SET force = 1, status = ?, phase = '', progress = 0,
		updated_at = ? WHERE id = ? AND status = ?`, StatusQueued, store.Now(), id, StatusNeedsParts)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("import.not_waiting")
	}
	m.kick()
	return nil
}

// Retry queues a failed or cancelled import again.
func (m *Manager) Retry(ctx context.Context, id int64) error {
	res, err := m.st.W.ExecContext(ctx, `UPDATE imports SET status = ?, phase = '', progress = 0, error = '',
		updated_at = ? WHERE id = ? AND status IN (?, ?)`, StatusQueued, store.Now(), id, StatusFailed, StatusCancelled)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("import.not_retryable")
	}
	m.kick()
	return nil
}

// Cancel stops an import. What was stored so far stays; importing the same
// files again continues where it stopped.
func (m *Manager) Cancel(ctx context.Context, id int64) error {
	m.mu.Lock()
	cancel := m.cancels[id]
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	_, err := m.st.W.ExecContext(ctx, `UPDATE imports SET status = ?, finished_at = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?, ?)`, StatusCancelled, store.Now(), store.Now(), id,
		StatusQueued, StatusRunning, StatusNeedsParts)
	if err != nil {
		return err
	}
	// The uploads can be used for a new import.
	_, err = m.st.W.ExecContext(ctx, `UPDATE uploads SET import_id = NULL WHERE import_id = ?`, id)
	return err
}

// Get returns the status of an import.
func (m *Manager) Get(ctx context.Context, id int64) (*Status, error) {
	rows, err := m.st.R.QueryContext(ctx, statusQuery+` WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	list, err := m.scanStatus(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	return list[0], nil
}

// List returns the most recent imports.
func (m *Manager) List(ctx context.Context, limit int) ([]*Status, error) {
	rows, err := m.st.R.QueryContext(ctx, statusQuery+` ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return m.scanStatus(rows)
}

const statusQuery = `SELECT id, platform, status, phase, progress, files_json, check_json, report_json,
	error, created_at, finished_at FROM imports`

func (m *Manager) scanStatus(rows *sql.Rows) ([]*Status, error) {
	defer rows.Close()
	var out []*Status
	for rows.Next() {
		var s Status
		var files string
		var check, report, finished sql.NullString
		if err := rows.Scan(&s.ID, &s.Platform, &s.Status, &s.Phase, &s.Progress, &files, &check, &report,
			&s.Error, &s.CreatedAt, &finished); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(files), &s.Files)
		if check.Valid {
			s.Check = &Check{}
			json.Unmarshal([]byte(check.String), s.Check)
		}
		if report.Valid {
			s.Report = &Report{}
			json.Unmarshal([]byte(report.String), s.Report)
		}
		s.FinishedAt = finished.String
		if l := m.liveSnapshot(s.ID); l != nil && s.Status == StatusRunning {
			s.Phase, s.Progress, s.Done, s.Total = l.phase, l.progress, l.done, l.total
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// run performs one import.
func (m *Manager) run(parent context.Context, id int64) {
	st, err := m.Get(parent, id)
	if err != nil || st.Status != StatusQueued {
		return
	}
	var force bool
	m.st.R.QueryRowContext(parent, `SELECT force FROM imports WHERE id = ?`, id).Scan(&force)

	ctx, cancel := context.WithCancel(parent)
	m.mu.Lock()
	m.cancels[id] = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.cancels, id)
		delete(m.lives, id)
		m.mu.Unlock()
	}()

	now := store.Now()
	m.st.W.ExecContext(ctx, `UPDATE imports SET status = ?, phase = ?, progress = 0, error = '', started_at = ?, updated_at = ?
		WHERE id = ?`, StatusRunning, PhaseOpening, now, now, id)
	m.setLive(id, PhaseOpening, 0)

	// Save progress every second so a reload shows it, and log it.
	stopReport := make(chan struct{})
	go m.report(ctx, id, stopReport)

	var size int64
	names := make([]string, 0, len(st.Files))
	for _, f := range st.Files {
		size += f.Size
		names = append(names, f.Name)
	}
	slog.Info("import started", "id", id, "platform", st.Platform, "files", len(st.Files), "size", humanBytes(size),
		"names", strings.Join(names, ", "))
	began := time.Now()
	status, check, report, runErr := m.execute(ctx, st, force)
	close(stopReport)

	finish := func(status, errMsg string) {
		var checkJSON, reportJSON any
		if check != nil {
			b, _ := json.Marshal(check)
			checkJSON = string(b)
		}
		if report != nil {
			b, _ := json.Marshal(report)
			reportJSON = string(b)
		}
		var finished any
		if status != StatusNeedsParts && status != StatusQueued {
			finished = store.Now()
		}
		phase := ""
		progress := 0.0
		if status == StatusDone {
			progress = 1
		}
		_, err := m.st.W.ExecContext(context.WithoutCancel(ctx), `UPDATE imports SET status = ?, phase = ?, progress = ?, error = ?,
			check_json = COALESCE(?, check_json), report_json = COALESCE(?, report_json), finished_at = ?, updated_at = ?
			WHERE id = ?`, status, phase, progress, errMsg, checkJSON, reportJSON, finished, store.Now(), id)
		if err != nil {
			slog.Error("save import status", "id", id, "err", err)
		}
	}

	switch {
	case runErr == nil:
		finish(status, "")
		if status == StatusDone {
			slog.Info("import done", "id", id, "new", report.New, "existing", report.Existing, "updated", report.Updated,
				"conversations", report.Conversations, "messages", report.Messages, "problems", report.Problems,
				"took", roundDuration(time.Since(began)))
			m.removeUploads(context.WithoutCancel(ctx), st.Files)
			m.kickPreviews()
		}
	case parent.Err() != nil:
		// Afterfeed is stopping: start again next time.
		slog.Info("import interrupted; it continues when Afterfeed starts again", "id", id)
		finish(StatusQueued, "")
	case ctx.Err() != nil:
		slog.Info("import cancelled", "id", id)
		finish(StatusCancelled, "")
	default:
		slog.Warn("import failed", "id", id, "err", runErr, "took", roundDuration(time.Since(began)))
		finish(StatusFailed, errorCode(runErr))
		// The uploads show up again on the upload screen, so a broken part
		// can be replaced without uploading the others again.
		m.st.W.ExecContext(context.WithoutCancel(ctx), `UPDATE uploads SET import_id = NULL WHERE import_id = ?`, id)
	}
}

// errorCode is what the web app gets for a failed import: a code it knows,
// followed by the details, such as the name of the file.
func errorCode(err error) string {
	for _, e := range []error{ErrEmpty, ErrHTMLFormat, ErrNotZip, ErrFileGone, ErrUnknownPlatform} {
		if errors.Is(err, e) {
			if msg := err.Error(); strings.HasPrefix(msg, e.Error()) {
				return msg
			}
			return e.Error() + ": " + err.Error()
		}
	}
	return "import.failed: " + err.Error()
}

// execute opens the files, checks the export and stores it.
func (m *Manager) execute(ctx context.Context, st *Status, force bool) (string, *Check, *Report, error) {
	im := m.importers[st.Platform]
	if im == nil {
		return "", nil, nil, ErrUnknownPlatform
	}
	paths := make([]string, 0, len(st.Files))
	for _, f := range st.Files {
		p, err := m.refPath(f)
		if err != nil {
			return "", nil, nil, err
		}
		if _, err := os.Stat(p); err != nil {
			return "", nil, nil, fmt.Errorf("%w: %s", ErrFileGone, f.Name)
		}
		paths = append(paths, p)
	}
	tmp := filepath.Join(m.cfg.StagingDir(), fmt.Sprintf("import-%d", st.ID))
	defer os.RemoveAll(tmp)
	began := time.Now()
	tree, err := archive.Open(paths, tmp)
	if err != nil {
		if errors.Is(err, archive.ErrNotZip) {
			return "", nil, nil, fmt.Errorf("%w: %v", ErrNotZip, err)
		}
		return "", nil, nil, err
	}
	defer tree.Close()
	slog.Info("import files opened", "id", st.ID, "parts", len(tree.Parts), "entries", len(tree.Files()),
		"took", roundDuration(time.Since(began)))

	// First pass: read everything without storing, to find missing files.
	began = time.Now()
	m.setLive(st.ID, PhaseReading, 0)
	cs := newCheckSink(m, st.Platform, func(done, total int) { m.setCounts(st.ID, done, total) })
	if err := im.Read(ctx, tree, cs); err != nil {
		return "", nil, nil, err
	}
	check := cs.result()
	slog.Info("import export read", "id", st.ID, "memories", check.Memories, "photos", check.Photos, "videos", check.Videos,
		"conversations", check.Conversations, "messages", check.Messages, "missing", check.Missing,
		"problems", check.Problems, "took", roundDuration(time.Since(began)))
	if check.Memories == 0 && check.Conversations == 0 {
		if htmlOnly(tree) {
			return "", check, nil, ErrHTMLFormat
		}
		return "", check, nil, ErrEmpty
	}
	if check.Missing > 0 && !force {
		slog.Info("import waiting for the remaining files", "id", st.ID, "missing", check.Missing,
			"examples", strings.Join(check.MissingExamples[:min(3, len(check.MissingExamples))], ", "))
		return StatusNeedsParts, check, nil, nil
	}

	// Second pass: store.
	began = time.Now()
	total := check.Memories + check.Conversations
	m.setLive(st.ID, PhaseCopying, 0)
	m.setCounts(st.ID, 0, total)
	slog.Info("import copying", "id", st.ID, "items", total)
	ss := newStoreSink(ctx, m, st.ID, st.Platform, total, func(done, total int) { m.setCounts(st.ID, done, total) })
	readErr := im.Read(ss.ctx, tree, ss)
	report, storeErr := ss.finish()
	if storeErr != nil {
		return "", check, nil, storeErr
	}
	if readErr != nil {
		return "", check, nil, readErr
	}
	report.Missing, report.MissingExamples = check.Missing, check.MissingExamples

	slog.Info("import copied", "id", st.ID, "took", roundDuration(time.Since(began)))
	m.setLive(st.ID, PhaseIndexing, 0.5)
	if err := m.lib.RefreshMonthStats(ctx); err != nil {
		return "", check, nil, err
	}
	return StatusDone, check, report, nil
}

// htmlOnly reports whether an export is in HTML instead of JSON format.
func htmlOnly(tree *archive.Tree) bool {
	html, js := 0, 0
	for _, f := range tree.Files() {
		switch f.Ext() {
		case ".html", ".htm":
			html++
		case ".json", ".js":
			js++
		}
	}
	return html > 5 && js < html/10
}

// --- the first pass ---

type checkSink struct {
	m        *Manager
	platform string
	progress func(done, total int)

	mu    sync.Mutex
	c     Check
	years map[int]int
	first time.Time
	last  time.Time
}

func newCheckSink(m *Manager, platform string, progress func(done, total int)) *checkSink {
	return &checkSink{m: m, platform: platform, progress: progress, years: map[int]int{}}
}

func (s *checkSink) Memory(mem *Memory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.c.Memories++
	for i := range mem.Media {
		md := &mem.Media[i]
		if md.Missing() {
			s.missing(md.Ref)
			continue
		}
		if md.File == nil {
			continue
		}
		switch kindFromName(md) {
		case media.Video:
			s.c.Videos++
		default:
			s.c.Photos++
		}
	}
	if !mem.TakenAt.IsZero() {
		local := mem.TakenAt.In(s.m.lib.Location(mem.TZ))
		s.years[local.Year()]++
		if s.first.IsZero() || local.Before(s.first) {
			s.first = local
		}
		if local.After(s.last) {
			s.last = local
		}
	}
}

// kindFromName guesses photo or video from the file name, which is good
// enough for counting before files are opened.
func kindFromName(md *Media) media.Kind {
	if md.Kind != "" {
		return md.Kind
	}
	switch md.File.Ext() {
	case ".mp4", ".mov", ".m4v", ".webm", ".3gp", ".avi", ".mkv":
		return media.Video
	}
	return media.Photo
}

func (s *checkSink) Conversation(c *Conversation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.c.Conversations++
	s.c.Messages += len(c.Messages)
	for i := range c.Messages {
		for j := range c.Messages[i].Files {
			f := &c.Messages[i].Files[j]
			if f.Missing() {
				s.missing(f.Ref)
			} else if f.File != nil {
				s.c.Files++
			}
		}
	}
}

func (s *checkSink) missing(ref string) {
	s.c.Missing++
	if len(s.c.MissingExamples) < maxExamples {
		s.c.MissingExamples = append(s.c.MissingExamples, ref)
	}
}

func (s *checkSink) Problem(path string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.c.Problems++
	if len(s.c.ProblemExamples) < maxExamples {
		s.c.ProblemExamples = append(s.c.ProblemExamples, Problem{Path: path, Error: err.Error()})
	}
	if s.c.Problems <= maxLoggedProblems {
		slog.Warn("import problem", "path", path, "err", err)
	}
}

func (s *checkSink) Progress(done, total int) {
	if total > 0 && s.progress != nil {
		s.progress(done, total)
	}
}

func (s *checkSink) result() *Check {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.c
	for y, n := range s.years {
		c.Years = append(c.Years, YearCount{Year: y, Memories: n})
	}
	sort.Slice(c.Years, func(i, j int) bool { return c.Years[i].Year < c.Years[j].Year })
	if !s.first.IsZero() {
		c.First, c.Last = s.first.Format("2006-01-02"), s.last.Format("2006-01-02")
	}
	return &c
}

// --- files ---

// refPath returns the location on disk of a part.
func (m *Manager) refPath(f FileRef) (string, error) {
	switch f.Kind {
	case "upload":
		if !validID(f.ID) {
			return "", errors.New("invalid upload")
		}
		return m.uploadPath(f.ID), nil
	case "folder":
		return m.folderPath(f.Path)
	}
	return "", errors.New("invalid file kind")
}

// folderPath resolves a path inside the import folder, refusing anything
// that points outside it.
func (m *Manager) folderPath(rel string) (string, error) {
	rel = filepath.Clean("/" + filepath.FromSlash(rel))
	if rel == "/" || strings.Contains(rel, "..") {
		return "", errors.New("invalid path")
	}
	return filepath.Join(m.cfg.ImportDir, rel), nil
}

func (m *Manager) resolveRefs(ctx context.Context, files []FileRef) ([]FileRef, error) {
	if len(files) == 0 {
		return nil, errors.New("import.no_files")
	}
	out := make([]FileRef, 0, len(files))
	for _, f := range files {
		switch f.Kind {
		case "upload":
			u, err := m.GetUpload(ctx, f.ID)
			if err != nil {
				return nil, fmt.Errorf("%w: upload %s", ErrFileGone, f.ID)
			}
			if u.Status == UploadInvalid {
				return nil, fmt.Errorf("%w: %s", ErrNotZip, u.Name)
			}
			if u.Status != UploadComplete {
				return nil, errors.New("import.upload_incomplete")
			}
			out = append(out, FileRef{Kind: "upload", ID: u.ID, Name: u.Name, Size: u.Size, Platform: u.Detected})
		case "folder":
			p, err := m.folderPath(f.Path)
			if err != nil {
				return nil, err
			}
			fi, err := os.Stat(p)
			if err != nil {
				return nil, fmt.Errorf("%w: %s", ErrFileGone, f.Path)
			}
			size := fi.Size()
			if fi.IsDir() {
				size = dirSize(p)
			}
			out = append(out, FileRef{Kind: "folder", Path: filepath.ToSlash(filepath.Clean(f.Path)), Name: fi.Name(), Size: size,
				Platform: m.detectCached(p, fi)})
		default:
			return nil, errors.New("invalid file kind")
		}
	}
	return out, nil
}

func (m *Manager) attachUploads(ctx context.Context, id int64, files []FileRef) {
	for _, f := range files {
		if f.Kind == "upload" {
			m.st.W.ExecContext(ctx, `UPDATE uploads SET import_id = ? WHERE id = ?`, id, f.ID)
		}
	}
}

// removeUploads deletes browser uploads after a successful import. Files in
// the import folder are left alone.
func (m *Manager) removeUploads(ctx context.Context, files []FileRef) {
	for _, f := range files {
		if f.Kind == "upload" {
			if err := m.DeleteUpload(ctx, f.ID); err != nil {
				slog.Warn("remove upload", "id", f.ID, "err", err)
			}
		}
	}
}
