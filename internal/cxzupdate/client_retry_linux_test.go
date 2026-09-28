//go:build linux

package cxzupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClientCandidateRefreshesPrunedEdgeAsset(t *testing.T) {
	oldRevision := Revision
	Revision = strings.Repeat("c", 40)
	t.Cleanup(func() { Revision = oldRevision })
	t.Setenv("CXZ_TEST_RUNTIME_CHILD", "1")
	exe, _ := os.Executable()
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	latest := testRelease(binary)
	latest.Ancestors = []string{Revision}
	latest.Sequence = 3
	stale := latest
	stale.Revision = strings.Repeat("a", 40)
	stale.Sequence = 2
	stale.Assets = map[string]Asset{}
	for platform, a := range latest.Assets {
		a.Name = strings.ReplaceAll(a.Name, latest.Revision, stale.Revision)
		stale.Assets[platform] = a
	}
	root := t.TempDir()
	if err := Save(root, State{Release: &stale, CheckedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	requests := []string{}
	http.DefaultTransport = roundTrip(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.Path)
		body, status := binary, 200
		if strings.HasSuffix(r.URL.Path, "cxz-update.json") {
			body, _ = json.Marshal(latest)
		} else if strings.Contains(r.URL.Path, stale.Revision) {
			body = nil
			status = 404
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})
	state, path, err := ClientCandidate(context.Background(), Client{Root: root})
	if err != nil || state.Release.Revision != latest.Revision || state.State != "staged" || !strings.Contains(path, latest.Revision) || len(requests) != 3 {
		t.Fatal(state, path, requests, err)
	}
}

func TestClientCandidateBoundsMissingAssetRetry(t *testing.T) {
	oldRevision := Revision
	Revision = strings.Repeat("c", 40)
	t.Cleanup(func() { Revision = oldRevision })
	r := testRelease(nil)
	r.Ancestors = []string{Revision}
	root := t.TempDir()
	if err := Save(root, State{Release: &r, CheckedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	count := 0
	http.DefaultTransport = roundTrip(func(req *http.Request) (*http.Response, error) {
		count++
		status := 404
		var body []byte
		if strings.HasSuffix(req.URL.Path, "cxz-update.json") {
			status = 200
			body, _ = json.Marshal(r)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})
	if _, _, err := ClientCandidate(context.Background(), Client{Root: root}); err == nil || count != 2 {
		t.Fatal("retry was not bounded", count, err)
	}
}
