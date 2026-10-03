package webui

import (
	"embed"
	"io/fs"
)

// Assets are checked in so source-based updates and Go-only releases include
// the exact browser build reviewed in the PR. CI verifies reproducibility.
//
//go:embed assets
var embedded embed.FS

func Assets() fs.FS {
	f, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err)
	}
	return f
}
