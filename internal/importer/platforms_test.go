package importer_test

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/importer/instagram"
	"github.com/rudyh0/afterfeed/internal/importer/snapchat"
	"github.com/rudyh0/afterfeed/internal/importer/x"
)

type recorder struct {
	mu       sync.Mutex
	memories []*importer.Memory
	convs    []*importer.Conversation
	problems []string
}

func (r *recorder) Memory(m *importer.Memory) {
	r.mu.Lock()
	r.memories = append(r.memories, m)
	r.mu.Unlock()
}
func (r *recorder) Conversation(c *importer.Conversation) {
	r.mu.Lock()
	r.convs = append(r.convs, c)
	r.mu.Unlock()
}
func (r *recorder) Problem(p string, err error) {
	r.mu.Lock()
	r.problems = append(r.problems, p+": "+err.Error())
	r.mu.Unlock()
}
func (r *recorder) Progress(int, int) {}

func (r *recorder) memory(key string) *importer.Memory {
	for _, m := range r.memories {
		if m.Key == key {
			return m
		}
	}
	return nil
}

// writeTree writes files into a folder and opens it as an export.
func writeTree(t *testing.T, files map[string]string) *archive.Tree {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := archive.Open([]string{dir}, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tree.Close() })
	return tree
}

func read(t *testing.T, im importer.Importer, tree *archive.Tree) *recorder {
	t.Helper()
	r := &recorder{}
	if err := im.Read(context.Background(), tree, r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestInstagram(t *testing.T) {
	tree := writeTree(t, map[string]string{
		"your_instagram_activity/media/posts_1.json": js([]any{
			// One photo: date and caption on the photo.
			map[string]any{"media": []any{map[string]any{"uri": "media/posts/201910/111.jpg", "creation_timestamp": 1571932020, "title": mojibake("Zon ☀️")}}},
			// A carousel: date and caption on the post.
			map[string]any{"title": "Trip", "creation_timestamp": 1600000000, "media": []any{
				map[string]any{"uri": "media/posts/202009/222.jpg", "creation_timestamp": 1600000001, "title": ""},
				map[string]any{"uri": "media/posts/202009/223.mp4", "creation_timestamp": 1600000002, "title": ""},
			}},
		}),
		"your_instagram_activity/media/stories.json": js(map[string]any{"ig_stories": []any{
			map[string]any{"uri": "media/stories/202101/333.mp4", "creation_timestamp": 1610000000, "title": ""},
		}}),
		"your_instagram_activity/media/reels.json": js(map[string]any{"ig_reels_media": []any{
			map[string]any{"media": []any{map[string]any{"uri": "media/reels/202201/444.mp4", "creation_timestamp": 1640000000, "title": "Reel"}}},
		}}),
		"media/posts/201910/111.jpg":   jpeg,
		"media/posts/202009/222.jpg":   jpeg,
		"media/posts/202009/223.mp4":   mp4,
		"media/stories/202101/333.mp4": mp4,
		"your_instagram_activity/messages/inbox/anna_123/message_1.json": js(map[string]any{
			"participants": []any{map[string]any{"name": "Anna"}, map[string]any{"name": "Rudy"}},
			"messages":     []any{map[string]any{"sender_name": "Anna", "timestamp_ms": 1600000000000, "content": "Hi"}},
			"title":        "Anna",
		}),
	})
	im := instagram.New()
	if s := im.Detect([]string{"your_instagram_activity/media/posts_1.json"}, "instagram-rudy.zip"); s < 0.9 {
		t.Errorf("detect = %v", s)
	}
	r := read(t, im, tree)
	if len(r.memories) != 4 {
		t.Fatalf("memories = %d, want 4", len(r.memories))
	}
	one := r.memory("post:111.jpg")
	if one == nil || one.Text != "Zon ☀️" || one.TakenAt.Unix() != 1571932020 {
		t.Errorf("single photo post = %+v", one)
	}
	carousel := r.memory("post:222.jpg")
	if carousel == nil || len(carousel.Media) != 2 || carousel.Text != "Trip" || carousel.TakenAt.Unix() != 1600000000 {
		t.Errorf("carousel = %+v", carousel)
	}
	if s := r.memory("story:333.mp4"); s == nil || s.Kind != "story" {
		t.Errorf("story = %+v", s)
	}
	if reel := r.memory("reel:444.mp4"); reel == nil || reel.Media[0].File != nil || !reel.Media[0].Missing() {
		t.Errorf("reel without its file should be missing: %+v", reel)
	}
	if len(r.convs) != 1 || r.convs[0].Messages[0].Text != "Hi" {
		t.Errorf("conversations = %+v", r.convs)
	}
}

func TestX(t *testing.T) {
	tree := writeTree(t, map[string]string{
		"data/manifest.js": `window.__THAR_CONFIG = {"userInfo":{"accountId":"1"}}`,
		"data/account.js":  `window.YTD.account.part0 = [{"account":{"accountId":"1","username":"rudy"}}]`,
		"data/tweets.js": `window.YTD.tweets.part0 = ` + js([]any{
			map[string]any{"tweet": map[string]any{
				"id_str": "100", "created_at": "Fri Oct 25 19:47:00 +0000 2019",
				"full_text": "Look at this &amp; that https://t.co/abc https://t.co/media",
				"entities": map[string]any{
					"urls":          []any{map[string]any{"url": "https://t.co/abc", "expanded_url": "https://example.com/page"}},
					"user_mentions": []any{map[string]any{"screen_name": "anna", "id_str": "2"}},
				},
				"extended_entities": map[string]any{"media": []any{
					map[string]any{"url": "https://t.co/media", "media_url_https": "https://pbs.twimg.com/media/AbC.jpg", "type": "photo"},
					map[string]any{"url": "https://t.co/media", "media_url_https": "https://pbs.twimg.com/ext_tw_video_thumb/9/pu/img/XyZ.jpg", "type": "video"},
				}},
				"geo": map[string]any{"coordinates": []any{"52.37", "4.89"}},
			}},
			map[string]any{"tweet": map[string]any{"id_str": "101", "created_at": "Fri Oct 25 20:00:00 +0000 2019", "full_text": "RT @someone: not mine"}},
			map[string]any{"tweet": map[string]any{"id_str": "102", "created_at": "Fri Oct 25 21:00:00 +0000 2019", "full_text": "@anna yes", "in_reply_to_user_id_str": "2"}},
			map[string]any{"tweet": map[string]any{"id_str": "103", "created_at": "Sat Oct 26 10:00:00 +0000 2019", "full_text": "A long post that was cut off because it was longer than the limit…"}},
		}),
		"data/note-tweet.js": `window.YTD.note_tweet.part0 = ` + js([]any{map[string]any{"noteTweet": map[string]any{
			"createdAt": "2019-10-26T10:00:00.000Z",
			"core":      map[string]any{"text": "A long post that was cut off because it was longer than the limit, and here is the rest."},
		}}}),
		"data/tweets_media/100-AbC.jpg":      jpeg,
		"data/tweets_media/100-video123.mp4": mp4,
		"data/direct-messages.js": `window.YTD.direct_messages.part0 = ` + js([]any{map[string]any{"dmConversation": map[string]any{
			"conversationId": "1-2",
			"messages": []any{
				map[string]any{"messageCreate": map[string]any{"id": "500", "senderId": "2", "recipientId": "1", "text": "Photo https://t.co/m",
					"createdAt": "2022-01-27T15:58:52.744Z", "mediaUrls": []any{"https://ton.twitter.com/dm/500/1/x.jpg"},
					"urls": []any{map[string]any{"url": "https://t.co/m", "expanded": "https://twitter.com/messages/media/500"}}}},
				map[string]any{"messageCreate": map[string]any{"id": "499", "senderId": "1", "recipientId": "2", "text": "Hi Anna",
					"createdAt": "2022-01-27T15:58:00.000Z", "reactions": []any{map[string]any{"senderId": "2", "reactionKey": "like"}}}},
			},
		}}}),
		"data/direct_messages_media/500-x.jpg": jpeg,
	})
	r := read(t, x.New(), tree)
	if len(r.memories) != 3 {
		t.Fatalf("memories = %d, want 3 (repost skipped)", len(r.memories))
	}
	p := r.memory("post:100")
	if p == nil || p.Text != "Look at this & that https://example.com/page" || len(p.Media) != 2 || p.Media[0].File == nil || p.Media[1].File == nil {
		t.Fatalf("post = %+v", p)
	}
	if p.Place == nil || *p.Place.Lat != 52.37 {
		t.Errorf("place = %+v", p.Place)
	}
	if reply := r.memory("post:102"); reply == nil || reply.Kind != "reply" {
		t.Errorf("reply = %+v", reply)
	}
	if long := r.memory("post:103"); long == nil || !strings.HasSuffix(long.Text, "here is the rest.") {
		t.Errorf("long post = %+v", long)
	}
	if len(r.convs) != 1 {
		t.Fatalf("conversations = %d", len(r.convs))
	}
	c := r.convs[0]
	if c.Title != "@anna" || len(c.Messages) != 2 || !c.Messages[0].Self || c.Messages[0].Reactions[0].Reaction != "❤️" {
		t.Errorf("conversation = %+v", c)
	}
	if c.Messages[1].Text != "Photo" || len(c.Messages[1].Files) != 1 || c.Messages[1].Files[0].File == nil {
		t.Errorf("media message = %+v", c.Messages[1])
	}
}

func TestSnapchat(t *testing.T) {
	tree := writeTree(t, map[string]string{
		"json/memories_history.json": js(map[string]any{"Saved Media": []any{
			map[string]any{"Date": "2019-06-01 18:03:48 UTC", "Media Type": "Image", "Location": "Latitude, Longitude: 37.97, 23.72",
				"Download Link": "https://app.snapchat.com/dmd/memories?uid=x&sid=9D8EB6C5-0000-4000-8000-000000000001&mid=9D8EB6C5-0000-4000-8000-000000000001"},
			map[string]any{"Date": "2019-06-02 08:00:00 UTC", "Media Type": "Video", "Location": "Latitude, Longitude: 0.0, 0.0",
				"Download Link": "https://app.snapchat.com/dmd/memories?mid=AAAAAAAA-0000-4000-8000-000000000002"},
			map[string]any{"Date": "2019-06-03 08:00:00 UTC", "Media Type": "Image", "Location": "",
				"Download Link": "https://app.snapchat.com/dmd/memories?mid=BBBBBBBB-0000-4000-8000-000000000003"},
		}}),
		"memories/2019-06-01_9D8EB6C5-0000-4000-8000-000000000001-main.jpg":    jpeg,
		"memories/2019-06-01_9D8EB6C5-0000-4000-8000-000000000001-overlay.png": "\x89PNG\r\n\x1a\noverlay",
		"memories/2019-06-02_AAAAAAAA-0000-4000-8000-000000000002-main.mp4":    mp4,
		"json/chat_history.json": js(map[string]any{
			"anna": []any{
				map[string]any{"From": "rudy", "Media Type": "TEXT", "Created": "2024-03-01 10:00:00 UTC", "Content": "Hey", "IsSender": true, "Created(microseconds)": 1709287200000000},
				map[string]any{"From": "anna", "Media Type": "NOTE", "Created": "2024-03-01 10:01:00 UTC", "Content": nil, "IsSender": false, "Media IDs": "b~voice1"},
				map[string]any{"From": "anna", "Media Type": "MEDIA", "Created": "2024-03-01 10:02:00 UTC", "Content": nil, "IsSender": false, "Media IDs": "b~gone"},
			},
		}),
		"chat_media/2024-03-01_b~voice1.mp4": "\x00\x00\x00\x18ftypM4A \x00\x00\x00\x00voice",
	})
	im := snapchat.New()
	r := read(t, im, tree)
	if len(r.memories) != 3 {
		t.Fatalf("memories = %d, want 3", len(r.memories))
	}
	var withOverlay, missing int
	for _, m := range r.memories {
		if m.Media[0].Overlay != nil {
			withOverlay++
			if m.Place == nil || m.TakenAt.Format("2006-01-02 15:04") != "2019-06-01 18:03" {
				t.Errorf("memory with overlay = %+v", m)
			}
		}
		if m.Media[0].Missing() {
			missing++
		}
		if m.TakenAt.Format("2006-01-02") == "2019-06-02" && m.Place != nil {
			t.Error("0,0 is not a location")
		}
	}
	if withOverlay != 1 || missing != 1 {
		t.Errorf("overlay = %d, missing = %d", withOverlay, missing)
	}
	if len(r.convs) != 1 {
		t.Fatalf("conversations = %d", len(r.convs))
	}
	c := r.convs[0]
	if len(c.Messages) != 2 || c.Self != "rudy" || !c.Messages[0].Self || c.Messages[1].Kind != "audio" {
		t.Errorf("conversation = %+v", c)
	}

	// An export without the Memories themselves says so instead of reporting missing parts.
	tree = writeTree(t, map[string]string{"json/memories_history.json": js(map[string]any{"Saved Media": []any{
		map[string]any{"Date": "2019-06-01 18:03:48 UTC", "Media Type": "Image"},
	}})})
	r = read(t, im, tree)
	if len(r.memories) != 0 || len(r.problems) != 1 {
		t.Errorf("without memories: %d memories, problems %v", len(r.memories), r.problems)
	}
}

// Newer Snapchat exports list Memories without ids. Files are then paired by
// the time in the zip file, or per day when the numbers agree.
func TestSnapchatWithoutIDs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mydata~1.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	add := func(name string, mod time.Time, body string) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: mod})
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	at := func(s string) time.Time { v, _ := time.Parse("2006-01-02 15:04:05", s); return v.UTC() }
	export := at("2026-10-20 08:00:00")
	add("json/memories_history.json", export, js(map[string]any{"Saved Media": []any{
		map[string]any{"Date": "2025-07-30 10:00:00 UTC", "Media Type": "Image", "Location": "Latitude, Longitude: 52.1, 4.3"},
		map[string]any{"Date": "2025-07-30 14:00:00 UTC", "Media Type": "Image", "Location": "Latitude, Longitude: 52.2, 4.4"},
		map[string]any{"Date": "2025-07-30 20:00:00 UTC", "Media Type": "Image", "Location": "Latitude, Longitude: 52.3, 4.5"},
		map[string]any{"Date": "2025-07-28 09:00:00 UTC", "Media Type": "Image", "Location": ""},
		map[string]any{"Date": "2025-07-28 09:30:00 UTC", "Media Type": "Video", "Location": ""},
	}}))
	// Zip times that are the capture times.
	add("memories/2025-07-30_aaaa-main.jpg", at("2025-07-30 20:00:10"), jpeg)
	add("memories/2025-07-30_bbbb-main.jpg", at("2025-07-30 14:00:30"), jpeg)
	// Zip times of the export, and a video listed as a photo.
	add("memories/2025-07-28_cccc-main.jpg", export, jpeg)
	add("memories/2025-07-28_dddd-main.jpg", export, jpeg)
	zw.Close()
	f.Close()
	tree, err := archive.Open([]string{p}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	r := read(t, snapchat.New(), tree)
	byFile := map[string]*importer.Memory{}
	missing := 0
	for _, m := range r.memories {
		if m.Media[0].File == nil {
			missing++
			continue
		}
		byFile[m.Media[0].File.Name()] = m
	}
	if missing != 1 || len(byFile) != 4 {
		t.Fatalf("missing = %d, files = %d, want 1 and 4", missing, len(byFile))
	}
	if got := byFile["2025-07-30_aaaa-main.jpg"].TakenAt.Format("15:04"); got != "20:00" {
		t.Errorf("aaaa taken at %s, want 20:00", got)
	}
	if m := byFile["2025-07-30_bbbb-main.jpg"]; m.TakenAt.Format("15:04") != "14:00" || m.Place == nil {
		t.Errorf("bbbb = %v %v, want 14:00 with a place", m.TakenAt, m.Place)
	}
	for _, name := range []string{"2025-07-28_cccc-main.jpg", "2025-07-28_dddd-main.jpg"} {
		if m := byFile[name]; m.TakenAt.Format("2006-01-02") != "2025-07-28" || m.TakenAt.Hour() != 9 {
			t.Errorf("%s taken at %v, want an entry of that morning", name, m.TakenAt)
		}
	}
}
