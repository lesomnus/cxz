//go:build !windows

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/configeditor"
)

func TestWebConfigurationProxyForwardsOnlyToHostSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "cxz-config-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "host.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != configeditor.Prefix+"compose" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("host editor received unexpected path or browser credentials")
		}
		w.WriteHeader(http.StatusNoContent)
	})}
	go server.Serve(listener)
	defer server.Close()

	t.Setenv(configeditor.SocketEnv, socket)
	proxy := configurationHandler("/var/lib/cxz", true)
	r := httptest.NewRequest(http.MethodGet, configeditor.Prefix+"compose", nil)
	r.Header.Set("Cookie", "example=example")
	r.Header.Set("Authorization", "example")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("proxy response: %d %s", w.Code, w.Body.String())
	}
}
