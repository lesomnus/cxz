package webui

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Browser navigation may name a client route instead of a bundled file.
// Missing assets and service endpoints retain their normal HTTP errors.
func spaFiles(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.Contains(r.Header.Get("Accept"), "text/html") && clientNavigation(r.URL.Path) {
			name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
			if _, err := fs.Stat(assets, name); errors.Is(err, fs.ErrNotExist) {
				r = r.Clone(r.Context())
				r.URL.Path = "/"
				r.URL.RawPath = ""
			}
		}
		files.ServeHTTP(w, r)
	})
}

func clientNavigation(urlPath string) bool {
	for _, prefix := range []string{"/assets", "/auth", "/editor", "/terminal", "/attachments", "/cxz."} {
		if urlPath == prefix || strings.HasPrefix(urlPath, prefix+"/") || (prefix == "/cxz." && strings.HasPrefix(urlPath, prefix)) {
			return false
		}
	}
	// Session identifiers can contain punctuation. Other missing file paths stay 404.
	for _, prefix := range []string{"/sessions", "/projects", "/settings"} {
		if urlPath == prefix || strings.HasPrefix(urlPath, prefix+"/") {
			return true
		}
	}
	return path.Ext(urlPath) == ""
}
