// Package webui embeds the compiled React front-end so the daemon ships as a
// single self-contained binary. The front-end is built into ./dist by the
// Makefile (web build -> outDir internal/webui/dist) before the Go build.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// FS returns the built front-end as a filesystem rooted at the dist directory.
// If the front-end has not been built, only the placeholder index.html is served.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		// embed guarantees dist exists at compile time; this is unreachable.
		panic(err)
	}
	return sub
}
