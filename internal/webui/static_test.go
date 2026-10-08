package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPAFileNavigation(t *testing.T) {
	h := spaFiles(fstest.MapFS{"index.html": {Data: []byte("app shell")}, "assets/app.js": {Data: []byte("script")}, "third-party-notices.txt": {Data: []byte("licenses")}})
	for _, tc := range []struct {
		method, path, accept, body string
		status                     int
	}{
		{"GET", "/sessions", "text/html", "app shell", 200},
		{"GET", "/sessions/session-1?tab=details", "text/html", "app shell", 200},
		{"GET", "/sessions/alias.with.dots", "text/html", "app shell", 200},
		{"GET", "/settings/editor", "text/html", "app shell", 200},
		{"GET", "/projects", "text/html", "app shell", 200},
		{"GET", "/unknown/page", "text/html", "app shell", 200},
		{"HEAD", "/sessions/session-1", "text/html", "", 200},
		{"GET", "/assets/app.js", "*/*", "script", 200},
		{"GET", "/third-party-notices.txt", "text/html", "licenses", 200},
		{"GET", "/assets/missing.js", "text/html", "", 404},
		{"GET", "/missing.js", "text/html", "", 404},
		{"GET", "/auth/missing", "text/html", "", 404},
		{"GET", "/editor/missing", "text/html", "", 404},
		{"GET", "/terminal/missing", "text/html", "", 404},
		{"GET", "/cxz.SessionService/Missing", "text/html", "", 404},
		{"GET", "/sessions/session-1", "application/json", "", 404},
		{"POST", "/sessions/session-1", "text/html", "", 404},
	} {
		t.Run(tc.method+" "+tc.path+" "+tc.accept, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body)
			}
			if tc.status == http.StatusOK && w.Body.String() != tc.body {
				t.Fatalf("body %q, want %q", w.Body, tc.body)
			}
		})
	}
}
