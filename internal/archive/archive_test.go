package archive

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, p string, files map[string]string, method uint16) {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestTreeAcrossParts(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "a.zip"), map[string]string{
		"your_facebook_activity/posts/your_posts_1.json": `[{"timestamp":1}]`,
		"__MACOSX/your_facebook_activity/._x":            "junk",
		"your_facebook_activity/.DS_Store":               "junk",
	}, zip.Deflate)
	// The second part has an extra wrapper folder.
	writeZip(t, filepath.Join(dir, "b.zip"), map[string]string{
		"facebook-user/your_facebook_activity/posts/media/Album_1/123.jpg": "jpeg",
		"facebook-user/other/media/Album_2/123.jpg":                        "other",
	}, zip.Store)

	tree, err := Open([]string{filepath.Join(dir, "a.zip"), filepath.Join(dir, "b.zip")}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	if got := len(tree.Files()); got != 3 {
		t.Fatalf("files = %d, want 3 (junk dropped)", got)
	}
	f := tree.Resolve("your_facebook_activity/posts/media/Album_1/123.jpg")
	if f == nil || f.Part != 1 || f.Path != "facebook-user/your_facebook_activity/posts/media/Album_1/123.jpg" {
		t.Fatalf("resolve across parts = %+v", f)
	}
	if tree.Resolve("https://example.com/123.jpg") != nil {
		t.Fatal("URLs must not resolve")
	}
	if tree.Resolve("missing/999.jpg") != nil {
		t.Fatal("missing file resolved")
	}
	data, err := ReadAll(f, 100)
	if err != nil || string(data) != "jpeg" {
		t.Fatalf("read = %q, %v", data, err)
	}
}

func TestNestedAndFolderParts(t *testing.T) {
	dir := t.TempDir()
	// A large stored zip inside an outer zip.
	var inner bytes.Buffer
	zw := zip.NewWriter(&inner)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "json/memories_history.json", Method: zip.Store})
	w.Write(bytes.Repeat([]byte(" "), 2<<20))
	w, _ = zw.CreateHeader(&zip.FileHeader{Name: "memories/2020-01-01_x-main.jpg", Method: zip.Store})
	w.Write([]byte("photo"))
	zw.Close()

	writeZip(t, filepath.Join(dir, "outer.zip"), map[string]string{"mydata~1.zip": inner.String()}, zip.Store)
	writeZip(t, filepath.Join(dir, "outer-deflated.zip"), map[string]string{"mydata~2.zip": inner.String()}, zip.Deflate)

	folder := filepath.Join(dir, "extracted")
	os.MkdirAll(filepath.Join(folder, "media"), 0o755)
	os.WriteFile(filepath.Join(folder, "media", "a.jpeg"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(folder, "Sanne_1.json"), []byte(`{}`), 0o644)

	tree, err := Open([]string{filepath.Join(dir, "outer.zip"), filepath.Join(dir, "outer-deflated.zip"), folder}, filepath.Join(dir, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()

	if f := tree.Lookup("memories/2020-01-01_x-main.jpg"); f == nil {
		t.Fatal("nested file not found")
	} else if b, _ := ReadAll(f, 10); string(b) != "photo" {
		t.Fatalf("nested read = %q", b)
	}
	if f := tree.ResolveFrom(".", "./media/a.jpeg"); f == nil {
		t.Fatal("folder file not resolved")
	}
	if n := len(tree.Parts); n != 5 {
		t.Fatalf("parts = %d, want 5", n)
	}
}

func TestReadJS(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	os.WriteFile(filepath.Join(dir, "data", "tweets.js"), []byte("window.YTD.tweets.part0 = [{\"tweet\":{\"id_str\":\"1\"}}]"), 0o644)
	tree, err := Open([]string{dir}, "")
	if err != nil {
		t.Fatal(err)
	}
	var v []map[string]map[string]string
	if err := ReadJS(tree.Lookup("data/tweets.js"), &v); err != nil || v[0]["tweet"]["id_str"] != "1" {
		t.Fatalf("ReadJS = %v, %v", v, err)
	}
}
