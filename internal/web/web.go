// Package web embeds the built frontend.
//
// The frontend is built into internal/web/dist by `npm run build` in web/.
// When the folder is empty (for example in a backend-only development build),
// the server shows a short notice instead of the app.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built frontend files.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
