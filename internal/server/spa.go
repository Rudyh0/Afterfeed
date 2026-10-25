package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/rudyh0/afterfeed/internal/web"
)

// spaHandler serves the built web app. Files under /assets/ have content
// hashes in their names and are cached forever; every other path that is not a
// file gets index.html, so the app's own router can handle it.
func spaHandler() http.Handler {
	files := web.FS()
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		index = []byte(notBuiltPage)
	}
	fileServer := http.FileServer(http.FS(files))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" && p != "index.html" {
			if fi, err := fs.Stat(files, p); err == nil && !fi.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
			// A missing file with an extension is a real 404, not an app route.
			if path.Ext(p) != "" {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}

const notBuiltPage = `<!doctype html><meta charset="utf-8"><title>Afterfeed</title>
<p style="font-family:system-ui;margin:3rem">The web app has not been built. Run <code>make web</code> and rebuild the server.</p>`
