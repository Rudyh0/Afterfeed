package importer_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/importer/facebook"
	"github.com/rudyh0/afterfeed/internal/importer/instagram"
	"github.com/rudyh0/afterfeed/internal/importer/snapchat"
	"github.com/rudyh0/afterfeed/internal/importer/x"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
	"github.com/rudyh0/afterfeed/internal/store"
)

// jpeg is enough of a JPEG file to be recognised as one.
const jpeg = "\xff\xd8\xff\xe0\x00\x10JFIF\x00fake image data"

// mp4 is enough of an MP4 file to be recognised as one.
const mp4 = "\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00isommp42fake video"

func writeZip(t *testing.T, p string, files map[string]string) {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// mojibake encodes text the way Meta exports do.
func mojibake(s string) string {
	var out []rune
	for _, b := range []byte(s) {
		out = append(out, rune(b))
	}
	return string(out)
}

type env struct {
	cfg *config.Config
	st  *store.Store
	lib *library.Library
	m   *importer.Manager
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		ConfigDir: filepath.Join(root, "config"),
		DataDir:   filepath.Join(root, "data"),
		CacheDir:  filepath.Join(root, "cache"),
		ImportDir: filepath.Join(root, "data", "import"),
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	lib := library.New(cfg.MemoriesDir(), cfg.MessagesDir(), st)
	lib.SetDefaultLocation(time.UTC)
	ms := media.New(cfg.CacheDir)
	m := importer.NewManager(cfg, st, lib, ms, []importer.Importer{facebook.New(), instagram.New(), x.New(), snapchat.New()})
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	t.Cleanup(func() {
		cancel()
		m.Wait()
	})
	return &env{cfg: cfg, st: st, lib: lib, m: m}
}

func (e *env) wait(t *testing.T, id int64, want string) *importer.Status {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		st, err := e.m.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		switch st.Status {
		case importer.StatusQueued, importer.StatusRunning:
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if st.Status != want {
			t.Fatalf("import %d: status %s (%s), want %s", id, st.Status, st.Error, want)
		}
		return st
	}
	t.Fatalf("import %d did not finish", id)
	return nil
}

func (e *env) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.st.R.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func facebookExport(t *testing.T, dir string) (part1, part2, e2ee string) {
	posts := []any{
		map[string]any{
			"timestamp": 1571932020,
			"attachments": []any{map[string]any{"data": []any{
				map[string]any{"media": map[string]any{
					"uri": "your_facebook_activity/posts/media/Greece_1/1001.jpg", "creation_timestamp": 1571932019,
					"media_metadata": map[string]any{"photo_metadata": map[string]any{"exif_data": []any{
						map[string]any{"taken_timestamp": 1571920000, "latitude": 37.97, "longitude": 23.72},
					}}},
				}},
				map[string]any{"place": map[string]any{"name": mojibake("Akropolis Αθήνα"), "coordinate": map[string]any{"latitude": 37.9715, "longitude": 23.7257}}},
			}}},
			"data":  []any{map[string]any{"post": mojibake("Eindelijk in Athene! Café ☕")}},
			"title": "Rudy shared a memory.",
		},
		map[string]any{"timestamp": 1500000000, "data": []any{map[string]any{"post": "Just text"}}},
		map[string]any{"timestamp": 1400000000, "attachments": []any{map[string]any{"data": []any{
			map[string]any{"place": map[string]any{"name": "Dam Square", "coordinate": map[string]any{"latitude": 52.373, "longitude": 4.893}}},
		}}}},
		map[string]any{"timestamp": 1300000000, "title": "Rudy is now friends with someone."},
	}
	album := map[string]any{
		"name": "Greece 2019",
		"photos": []any{
			map[string]any{"uri": "your_facebook_activity/posts/media/Greece_1/1001.jpg", "creation_timestamp": 1571932019},
			map[string]any{"uri": "your_facebook_activity/posts/media/Greece_1/1002.jpg", "creation_timestamp": 1571940000, "description": "Sunset"},
			map[string]any{"uri": "your_facebook_activity/posts/media/Greece_1/1003.jpg", "creation_timestamp": 1571940060},
		},
	}
	videos := map[string]any{"videos_v2": []any{
		map[string]any{"uri": "your_facebook_activity/posts/media/videos/2001.mp4", "creation_timestamp": 1600000000},
	}}
	thread := map[string]any{
		"participants": []any{map[string]any{"name": "Rudy H"}, map[string]any{"name": mojibake("Sanne Jänsen")}},
		"messages": []any{
			map[string]any{"sender_name": "Rudy H", "timestamp_ms": 1600000003000, "audio_files": []any{map[string]any{"uri": "your_facebook_activity/messages/inbox/sanne_10/audio/voice.mp4"}}},
			map[string]any{"sender_name": mojibake("Sanne Jänsen"), "timestamp_ms": 1600000002000, "photos": []any{map[string]any{"uri": "your_facebook_activity/messages/inbox/sanne_10/photos/3001.jpg"}}},
			map[string]any{"sender_name": "Rudy H", "timestamp_ms": 1600000001000, "content": mojibake("Hoi! 😀"), "reactions": []any{map[string]any{"reaction": mojibake("❤"), "actor": mojibake("Sanne Jänsen")}}},
		},
		"title": mojibake("Sanne Jänsen"),
	}
	group := map[string]any{
		"participants": []any{map[string]any{"name": "Rudy H"}, map[string]any{"name": "Piet"}, map[string]any{"name": "Klaas"}},
		"messages":     []any{map[string]any{"sender_name": "Piet", "timestamp_ms": 1600000100000, "content": "Group hello"}},
		"title":        "Friends",
	}
	part1 = filepath.Join(dir, "facebook-rudy-2026-10-25-a.zip")
	writeZip(t, part1, map[string]string{
		"your_facebook_activity/posts/your_posts__check_ins__photos_and_videos_1.json": js(posts),
		"your_facebook_activity/posts/album/0.json":                                    js(album),
		"your_facebook_activity/posts/your_videos.json":                                js(videos),
		"your_facebook_activity/messages/inbox/sanne_10/message_1.json":                js(thread),
		"your_facebook_activity/messages/inbox/friends_20/message_1.json":              js(group),
		"your_facebook_activity/posts/media/Greece_1/1001.jpg":                         jpeg,
	})
	part2 = filepath.Join(dir, "facebook-rudy-2026-10-25-b.zip")
	writeZip(t, part2, map[string]string{
		"your_facebook_activity/posts/media/Greece_1/1002.jpg":           jpeg + "2",
		"your_facebook_activity/posts/media/Greece_1/1003.jpg":           jpeg + "3",
		"your_facebook_activity/posts/media/videos/2001.mp4":             mp4,
		"your_facebook_activity/messages/inbox/sanne_10/photos/3001.jpg": jpeg + "msg",
		"your_facebook_activity/messages/inbox/sanne_10/audio/voice.mp4": "\x00\x00\x00\x18ftypM4A \x00\x00\x00\x00voice",
	})
	e2ee = filepath.Join(dir, "messages.zip")
	writeZip(t, e2ee, map[string]string{
		"Sanne Jänsen_1.json": js(map[string]any{
			"participants": []any{"Rudy H", "Sanne Jänsen"},
			"threadName":   "Sanne Jänsen_1",
			"messages": []any{
				map[string]any{"senderName": "Rudy H", "timestamp": 1600000001000, "text": "Hoi! 😀", "type": "text"},
				map[string]any{"senderName": "Sanne Jänsen", "timestamp": 1700000000000, "text": "Encrypted hello", "type": "text"},
				map[string]any{"senderName": "Sanne Jänsen", "timestamp": 1700000001000, "type": "media", "media": []any{map[string]any{"uri": "./media/abc.jpeg"}}},
			},
		}),
		"media/abc.jpeg": jpeg + "e2ee",
	})
	return
}

func TestFacebookImportWithParts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	part1, part2, e2ee := facebookExport(t, e.cfg.ImportDir)

	if p := e.m.Detect(part1, filepath.Base(part1)); p != "facebook" {
		t.Errorf("detect part 1 = %q", p)
	}
	if p := e.m.Detect(e2ee, "messages.zip"); p != "facebook" {
		t.Errorf("detect messages.zip = %q", p)
	}

	// With only the first part, photos are missing.
	id, err := e.m.Create(ctx, "facebook", []importer.FileRef{{Kind: "folder", Path: filepath.Base(part1)}})
	if err != nil {
		t.Fatal(err)
	}
	st := e.wait(t, id, importer.StatusNeedsParts)
	if st.Check.Missing != 5 {
		t.Errorf("missing = %d, want 5 (%v)", st.Check.Missing, st.Check.MissingExamples)
	}

	// Adding the other part and the encrypted chats completes it.
	if err := e.m.AddFiles(ctx, id, []importer.FileRef{
		{Kind: "folder", Path: filepath.Base(part2)},
		{Kind: "folder", Path: filepath.Base(e2ee)},
	}); err != nil {
		t.Fatal(err)
	}
	st = e.wait(t, id, importer.StatusDone)
	r := st.Report
	// Post with photo and place, text post, check-in, album group of two, video.
	if r.New != 5 {
		t.Errorf("new = %d, want 5", r.New)
	}
	if r.Photos != 3 || r.Videos != 1 {
		t.Errorf("photos/videos = %d/%d, want 3/1", r.Photos, r.Videos)
	}
	if r.Conversations != 3 || r.NewConversations != 2 {
		t.Errorf("conversations = %d new %d, want 3 and 2 (encrypted chat merged)", r.Conversations, r.NewConversations)
	}
	if r.Messages != 6 {
		t.Errorf("messages = %d, want 6 (one duplicate between exports)", r.Messages)
	}

	var text, place, kind, rel string
	if err := e.st.R.QueryRow(`SELECT text, place_name, kind, path FROM memories WHERE taken_at = '2019-10-24T15:47:00Z'`).Scan(&text, &place, &kind, &rel); err != nil {
		t.Fatal(err)
	}
	if text != "Eindelijk in Athene! Café ☕" || place != "Akropolis Αθήνα" || kind != "post" {
		t.Errorf("post = %q %q %q", text, place, kind)
	}
	mem, err := e.lib.LoadMemory(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(mem.Media) != 1 || mem.Media[0].File != "01.jpg" || mem.Media[0].TakenAt == nil || mem.Media[0].Lat == nil {
		t.Errorf("media = %+v", mem.Media)
	}
	if _, err := os.Stat(filepath.Join(e.lib.MemoryDir(rel), "01.jpg")); err != nil {
		t.Error(err)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM memories WHERE kind = 'checkin' AND place_name = 'Dam Square'`); n != 1 {
		t.Errorf("check-ins = %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM memories WHERE album = 'Greece 2019' AND media_count = 2`); n != 1 {
		t.Errorf("album group = %d", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH 'athene'`); n != 1 {
		t.Errorf("search = %d", n)
	}

	var convRel string
	if err := e.st.R.QueryRow(`SELECT path FROM conversations WHERE title = 'Sanne Jänsen'`).Scan(&convRel); err != nil {
		t.Fatal(err)
	}
	conv, err := e.lib.LoadConversation(convRel)
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Messages) != 5 || conv.Self != "Rudy H" {
		t.Fatalf("conversation = %d messages, self %q", len(conv.Messages), conv.Self)
	}
	if !conv.Messages[0].Self || conv.Messages[0].Text != "Hoi! 😀" || conv.Messages[0].Reactions[0].Reaction != "❤" {
		t.Errorf("first message = %+v", conv.Messages[0])
	}
	if conv.Messages[2].Kind != "audio" || conv.Messages[2].Files[0].Kind != "audio" {
		t.Errorf("voice message = %+v", conv.Messages[2])
	}
	if n := e.count(t, `SELECT COUNT(*) FROM message_files WHERE conversation_id = ?`, conv.ID); n != 3 {
		t.Errorf("message files = %d", n)
	}
	// Uploaded files would be removed; folder files stay.
	if _, err := os.Stat(part1); err != nil {
		t.Error("import folder file removed")
	}

	// Importing again adds nothing.
	id2, err := e.m.Create(ctx, "facebook", []importer.FileRef{
		{Kind: "folder", Path: filepath.Base(part1)}, {Kind: "folder", Path: filepath.Base(part2)}, {Kind: "folder", Path: filepath.Base(e2ee)},
	})
	if err != nil {
		t.Fatal(err)
	}
	st = e.wait(t, id2, importer.StatusDone)
	if st.Report.New != 0 || st.Report.Existing != 5 || st.Report.Messages != 0 {
		t.Errorf("re-import = %+v", st.Report)
	}

	// The index can be rebuilt from the files.
	stats, err := e.lib.Rebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Memories != 5 || stats.Conversations != 2 {
		t.Errorf("rebuild = %+v", stats)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM messages`); n != 6 {
		t.Errorf("messages after rebuild = %d", n)
	}
}

func TestUploadResume(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	part1, _, _ := facebookExport(t, t.TempDir())
	data, _ := os.ReadFile(part1)

	u, err := e.m.CreateUpload(ctx, "export.zip", int64(len(data)), 123, "facebook")
	if err != nil {
		t.Fatal(err)
	}
	half := len(data) / 2
	if _, err := e.m.WriteUpload(ctx, u.ID, 0, bytesReader(data[:half])); err != nil {
		t.Fatal(err)
	}
	// The same file again continues where it stopped.
	again, err := e.m.CreateUpload(ctx, "export.zip", int64(len(data)), 123, "facebook")
	if err != nil || again.ID != u.ID || again.Received != int64(half) {
		t.Fatalf("resume = %+v, %v", again, err)
	}
	if _, err := e.m.WriteUpload(ctx, u.ID, 0, bytesReader(data)); err != importer.ErrOffset {
		t.Errorf("wrong offset accepted: %v", err)
	}
	done, err := e.m.WriteUpload(ctx, u.ID, int64(half), bytesReader(data[half:]))
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != importer.UploadComplete || done.Detected != "facebook" {
		t.Errorf("upload = %+v", done)
	}
	id, err := e.m.Create(ctx, "facebook", []importer.FileRef{{Kind: "upload", ID: u.ID}})
	if err != nil {
		t.Fatal(err)
	}
	e.wait(t, id, importer.StatusNeedsParts)
	if err := e.m.Continue(ctx, id); err != nil {
		t.Fatal(err)
	}
	e.wait(t, id, importer.StatusDone)
	if _, err := e.m.GetUpload(ctx, u.ID); err == nil {
		t.Error("upload not removed after import")
	}
}
