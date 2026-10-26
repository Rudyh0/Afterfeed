package server

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
)

// addMemory stores a memory with one photo for a platform.
func addMemory(t *testing.T, lib *library.Library, platform, key string, vault bool) string {
	t.Helper()
	now := time.Date(2021, 7, 14, 15, 0, 0, 0, time.UTC)
	mem := &library.Memory{
		ID: library.MemoryID(key), Platform: platform, Kind: "post",
		Fields: library.Fields{TakenAt: now, Text: "tekst " + key}, Source: library.Source{Key: key},
		Vault: vault, CreatedAt: now, UpdatedAt: now,
	}
	rel, err := lib.CreateMemoryDir(mem)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(lib.MemoryDir(rel)+"/01.jpg", []byte("photo"), 0o640)
	mem.Media = []library.Media{{ID: library.MediaID(mem.ID, "1"), Key: "1", File: "01.jpg", Kind: "photo", Mime: "image/jpeg"}}
	if err := lib.SaveMemory(rel, mem); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, _ := lib.Store().W.BeginTx(ctx, nil)
	if err := lib.IndexMemory(ctx, tx, rel, mem, 0); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	return lib.MemoryDir(rel)
}

func TestRemoveSource(t *testing.T) {
	c, deps := newTestServer(t)
	const password = "a long enough password"
	if code := c.do("POST", "/api/setup", map[string]string{"username": "rudy", "password": password}, nil); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	lib := deps.Library
	fb1 := addMemory(t, lib, "facebook", "fb:1", false)
	fb2 := addMemory(t, lib, "facebook", "fb:2", true)
	snap := addMemory(t, lib, "snapchat", "snap:1", false)
	own := addMemory(t, lib, "afterfeed", "own:1", false)
	addConversation(t, lib, "fb-thread", 4)

	var list struct {
		Sources []sourceInfo `json:"sources"`
	}
	c.do("GET", "/api/sources", nil, &list)
	var fb *sourceInfo
	for i, s := range list.Sources {
		if s.Platform == "facebook" {
			fb = &list.Sources[i]
		}
		if s.Platform == "afterfeed" && s.Removable {
			t.Error("own memories are offered for removal")
		}
	}
	if fb == nil || fb.Memories != 2 || fb.Conversations != 1 || !fb.Removable {
		t.Fatalf("facebook source = %+v", fb)
	}

	if code := c.do("POST", "/api/sources/facebook/remove", map[string]any{"password": "wrong password!"}, nil); code != 400 {
		t.Fatalf("wrong password: %d", code)
	}
	if code := c.do("POST", "/api/sources/afterfeed/remove", map[string]any{"password": password}, nil); code != 400 {
		t.Fatalf("own memories: %d", code)
	}
	var res struct{ Memories, Conversations int }
	if code := c.do("POST", "/api/sources/facebook/remove", map[string]any{"password": password}, &res); code != 200 {
		t.Fatalf("remove: %d", code)
	}
	if res.Memories != 2 || res.Conversations != 1 {
		t.Errorf("removed %+v", res)
	}
	for _, dir := range []string{fb1, fb2} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s still exists", dir)
		}
	}
	for _, dir := range []string{snap, own} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s was removed: %v", dir, err)
		}
	}
	count := func(q string) int {
		var n int
		lib.Store().R.QueryRow(q).Scan(&n)
		return n
	}
	if n := count(`SELECT COUNT(*) FROM memories WHERE platform = 'facebook'`); n != 0 {
		t.Errorf("%d facebook memories left", n)
	}
	if n := count(`SELECT COUNT(*) FROM media`); n != 2 {
		t.Errorf("%d media left, want 2", n)
	}
	if n := count(`SELECT COUNT(*) FROM conversations`) + count(`SELECT COUNT(*) FROM messages`); n != 0 {
		t.Errorf("%d conversations and messages left", n)
	}
	if n := count(`SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH '"fb"*'`); n != 0 {
		t.Errorf("search still finds %d removed memories", n)
	}
	c.do("GET", "/api/sources", nil, &list)
	for _, s := range list.Sources {
		if s.Platform == "facebook" {
			t.Error("facebook is still listed")
		}
	}
}
