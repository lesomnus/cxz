package cxzupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testRelease(data []byte) Release {
	sum := sha256.Sum256(data)
	r := Release{Revision: strings.Repeat("b", 40), Sequence: 2, Protocol: Protocol, Schema: Schema, Image: "ghcr.io/lesomnus/cxz@sha256:" + strings.Repeat("c", 64), Assets: map[string]Asset{}}
	for _, p := range []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
		name := "cxz-" + r.Revision + "-" + strings.ReplaceAll(p, "/", "-")
		if strings.HasPrefix(p, "windows/") {
			name += ".exe"
		}
		r.Assets[p] = Asset{name, hex.EncodeToString(sum[:])}
	}
	return r
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func useDownload(t *testing.T, body string) {
	t.Helper()
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
}
func TestReleaseRejectsUnpinnedAndIncompatibleArtifacts(t *testing.T) {
	for _, mutate := range []func(*Release){func(r *Release) { r.Image = "ghcr.io/lesomnus/cxz:edge" }, func(r *Release) { r.Protocol++ }, func(r *Release) { r.Schema++ }, func(r *Release) { r.Revision = "../../cxz" }, func(r *Release) { r.Sequence = 0 }, func(r *Release) { a := r.Assets["linux/amd64"]; a.Name = "../cxz"; r.Assets["linux/amd64"] = a }} {
		r := testRelease(nil)
		mutate(&r)
		if r.Validate() == nil {
			t.Fatalf("accepted invalid release: %+v", r)
		}
	}
	if e := testRelease(nil).Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestStageChecksumAndImmutableReuse(t *testing.T) {
	body := "verified binary"
	r := testRelease([]byte(body))
	root := t.TempDir()
	useDownload(t, body)
	p, e := Stage(context.Background(), root, r, "linux/amd64")
	if e != nil {
		t.Fatal(e)
	}
	first, e := os.Stat(p)
	if e != nil {
		t.Fatal(e)
	}
	p2, e := Stage(context.Background(), root, r, "linux/amd64")
	if e != nil {
		t.Fatal(e)
	}
	second, _ := os.Stat(p2)
	if !os.SameFile(first, second) {
		t.Fatal("immutable binary replaced")
	}
	if e = os.WriteFile(p, []byte("tampered"), 0755); e != nil {
		t.Fatal(e)
	}
	if _, e = Stage(context.Background(), root, r, "linux/amd64"); e == nil {
		t.Fatal("overwrote invalid existing release")
	}
	if _, e = Stage(context.Background(), t.TempDir(), testRelease([]byte("wrong checksum")), "linux/amd64"); e == nil {
		t.Fatal("accepted invalid checksum")
	}
}
func TestCheckNeverRegressesPublishedSequence(t *testing.T) {
	root := t.TempDir()
	r := testRelease(nil)
	r.Sequence = 8
	if e := Save(root, State{AppliedSequence: 8, Release: &r}); e != nil {
		t.Fatal(e)
	}
	older := r
	older.Sequence = 7
	older.Revision = strings.Repeat("d", 40)
	// Serialize a valid older manifest with the corresponding names.
	older = testRelease(nil)
	older.Sequence = 7
	b := mustJSON(t, older)
	useDownload(t, string(b))
	s, e := Check(context.Background(), root, true)
	if e != nil {
		t.Fatal(e)
	}
	if s.Release.Sequence != 8 || s.AppliedSequence != 8 {
		t.Fatal("downgraded")
	}
	s.FailedRevision = s.Release.Revision
	s.FailedAt = time.Now()
	if s.RetryAllowed() {
		t.Fatal("missing failure backoff")
	}
	s.FailedAt = time.Now().Add(-Interval)
	if !s.RetryAllowed() {
		t.Fatal("backoff never expires")
	}
}
func TestFrontendResumeIsScopedAndContainsOnlyNavigation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CXZ_FRONTEND_SLOT", "100")
	a := ResumePath(root)
	t.Setenv("CXZ_FRONTEND_SLOT", "101")
	b := ResumePath(root)
	if a == b || filepath.Dir(a) != root {
		t.Fatal("shared resume file")
	}
	t.Setenv("CXZ_FRONTEND_SLOT", "../../escape")
	if filepath.Dir(ResumePath(root)) != root {
		t.Fatal("path traversal")
	}
}
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestPublishedBuildCannotDowngradeUnknownLocalRevision(t *testing.T) {
	r := testRelease(nil)
	current := Build{Revision: strings.Repeat("a", 40)}
	if r.CanReplace(current) {
		t.Fatal("unknown local revision would be overwritten")
	}
	r.Ancestors = []string{current.Revision}
	if !r.CanReplace(current) {
		t.Fatal("main ancestor rejected")
	}
	if !r.CanReplace(Build{Revision: r.Revision}) {
		t.Fatal("current build rejected")
	}
}
