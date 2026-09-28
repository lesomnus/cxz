package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/releasechannel"
	"github.com/lesomnus/cxz/internal/versionpin"
)

type channelRetryTransport func(*http.Request) (*http.Response, error)

func (f channelRetryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUseChannelRefreshAndResumeCache(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "cmd", "cxz"), 0700)
	os.WriteFile(filepath.Join(src, "go.mod"), []byte("module github.com/lesomnus/cxz\n\ngo 1.27.0\n"), 0600)
	os.WriteFile(filepath.Join(src, "cmd", "cxz", "main.go"), []byte("package main; func main() {}\n"), 0600)
	exe := filepath.Join(t.TempDir(), "fixture.exe")
	for _, args := range [][]string{{"git", "init", "--quiet"}, {"git", "add", "."}, {"git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture"}, {"go", "build", "-buildvcs=true", "-o", exe, "./cmd/cxz"}} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	cmd := exec.Command("git", "-C", src, "rev-parse", "HEAD")
	rev, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	release := func(rev string, sequence int64) releasechannel.Release {
		r := releasechannel.Release{Revision: rev, Tag: "edge", Version: "source-" + rev[:12], Sequence: sequence, Protocol: 1, Schema: releasechannel.StateSchema, Image: "ghcr.io/lesomnus/cxz@sha256:" + strings.Repeat("d", 64), Assets: map[string]releasechannel.Asset{}}
		for _, p := range []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
			name := "cxz-" + r.Revision + "-" + strings.ReplaceAll(p, "/", "-")
			if strings.HasPrefix(p, "windows/") {
				name += ".exe"
			}
			r.Assets[p] = releasechannel.Asset{Name: name, SHA256: hex.EncodeToString(sum[:])}
		}
		return r
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("supported platforms only")
	}
	stale, latest := release(strings.Repeat("a", 40), 1), release(strings.TrimSpace(string(rev)), 2)
	plan := usePlan{Requested: "@edge", Release: &stale, Pin: versionpin.Pin{Channel: "edge", Version: stale.Version, Revision: stale.Revision, Image: stale.Image}}
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	count := 0
	http.DefaultTransport = channelRetryTransport(func(req *http.Request) (*http.Response, error) {
		count++
		body, status := binary, 200
		if strings.Contains(req.URL.Path, stale.Revision) {
			body = nil
			status = 404
		} else if strings.HasSuffix(req.URL.Path, "cxz-update.json") {
			body, _ = json.Marshal(latest)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})
	root := t.TempDir()
	a, next, err := downloadUseChannel(context.Background(), root, plan, io.Discard)
	if err != nil || a.Revision != latest.Revision || next.Pin.Image != latest.Image || next.Pin.Revision != latest.Revision || count != 3 {
		t.Fatal(a, next, count, err)
	}
	next.Pin.Generation = "in-progress"
	if _, _, err := downloadUseChannel(context.Background(), root, next, io.Discard); err != nil || count != 3 {
		t.Fatal("resume did not reuse verified cache", count, err)
	}
	plan.Pin.Generation = "in-progress"
	if _, _, err := downloadUseChannel(context.Background(), root, plan, io.Discard); err == nil || count != 4 {
		t.Fatal("changed a captured transaction", count, err)
	}
}
