package importer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

// Text in a photo ends up in memory.json and in the search index.
func TestReadTextInPhotos(t *testing.T) {
	if !media.New(t.TempDir()).CanReadText() {
		t.Skip("tesseract or its language models are not installed")
	}
	photo, err := os.ReadFile("../media/testdata/caption.jpg")
	if err != nil {
		t.Skip("no test photo")
	}
	e := newEnv(t)
	ctx := context.Background()

	now := time.Date(2021, 7, 14, 15, 0, 0, 0, time.UTC)
	mem := &library.Memory{
		ID: library.MemoryID("test:texel"), Platform: "afterfeed", Kind: "memory",
		Fields:    library.Fields{TakenAt: now, Text: "Een dag op het strand"},
		Source:    library.Source{Key: "test:texel"},
		CreatedAt: now, UpdatedAt: now,
	}
	rel, err := e.lib.CreateMemoryDir(mem)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.lib.MemoryDir(rel), "01.jpg"), photo, 0o640); err != nil {
		t.Fatal(err)
	}
	mem.Media = []library.Media{{ID: library.MediaID(mem.ID, "01"), Key: "01", File: "01.jpg", Kind: "photo", Mime: "image/jpeg"}}
	if err := e.lib.SaveMemory(rel, mem); err != nil {
		t.Fatal(err)
	}
	tx, err := e.st.W.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.lib.IndexMemory(ctx, tx, rel, mem, 0); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	e.m.PrepareNew(mem.ID)

	deadline := time.Now().Add(60 * time.Second)
	for e.count(t, `SELECT COUNT(*) FROM media WHERE ocr_text IS NULL OR ocr_version < 2`) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("the text was not read")
		}
		time.Sleep(100 * time.Millisecond)
	}
	saved, err := e.lib.LoadMemory(rel)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Media[0].OCR == nil || *saved.Media[0].OCR == "" {
		t.Fatalf("memory.json has no text: %+v", saved.Media[0].OCR)
	}
	if saved.Text != "Een dag op het strand" {
		t.Errorf("text = %q", saved.Text)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH '"strandpaviljoen"*'`); n != 1 {
		t.Errorf("search for a word in the photo found %d memories, want 1", n)
	}

	if saved.Media[0].OCRVersion != media.TextVersion {
		t.Errorf("reader version = %d", saved.Media[0].OCRVersion)
	}

	// Text read by an older reader is read again; its text stays
	// searchable until then.
	old := "oude tekst"
	saved.Media[0].OCR, saved.Media[0].OCRVersion = &old, 0
	if err := e.lib.SaveMemory(rel, saved); err != nil {
		t.Fatal(err)
	}
	tx, _ = e.st.W.BeginTx(ctx, nil)
	if err := e.lib.IndexMemory(ctx, tx, rel, saved, 0); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	if n := e.count(t, `SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH '"oude"*'`); n != 1 {
		t.Errorf("old text not searchable: %d", n)
	}
	e.m.PrepareNew(mem.ID)
	deadline = time.Now().Add(60 * time.Second)
	for e.count(t, `SELECT COUNT(*) FROM media WHERE ocr_version < 2`) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("the text was not read again")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH '"strandpaviljoen"*'`); n != 1 {
		t.Errorf("after reading again, search found %d memories, want 1", n)
	}

	// A rebuild of the index keeps the text without reading it again.
	if _, err := e.lib.Rebuild(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH '"texel"*'`); n != 1 {
		t.Errorf("after a rebuild, search found %d memories, want 1", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM media WHERE ocr_text IS NULL`); n != 0 {
		t.Errorf("after a rebuild, %d media are waiting to be read again", n)
	}
}
