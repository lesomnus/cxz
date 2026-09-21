package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func response(req *http.Request, status int, data []byte) *http.Response {
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: req}
}

func zipFixture(t *testing.T, name string, mode os.FileMode, contents []byte) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	f, err := w.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func releaseFixture(t *testing.T, tag, arch string, data []byte) (releaseInfo, []byte) {
	t.Helper()
	name := "cxz-" + tag + "-windows-" + arch + ".zip"
	sum := sha256.Sum256(data)
	sums := []byte(fmt.Sprintf("%x  %s\n", sum, name))
	sumDigest := sha256.Sum256(sums)
	return releaseInfo{Tag: tag, Assets: []releaseAsset{
		{Name: name, URL: releaseDownloads + tag + "/" + name, Digest: "sha256:" + hex.EncodeToString(sum[:])},
		{Name: "SHA256SUMS", URL: releaseDownloads + tag + "/SHA256SUMS", Digest: "sha256:" + hex.EncodeToString(sumDigest[:])},
	}}, sums
}

func fixtureClient(t *testing.T, release releaseInfo, sums, archive []byte) *http.Client {
	t.Helper()
	metadata, err := json.Marshal(release)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		if req.Header.Get("Authorization") != "" {
			t.Error("public updater sent credentials")
		}
		switch req.URL.String() {
		case releaseAPI + release.Tag:
			return response(req, http.StatusOK, metadata), nil
		case releaseDownloads + release.Tag + "/SHA256SUMS":
			return response(req, http.StatusOK, sums), nil
		case releaseDownloads + release.Tag + "/" + release.Assets[0].Name:
			return response(req, http.StatusOK, archive), nil
		default:
			t.Errorf("unexpected request %s", req.URL)
			return response(req, http.StatusNotFound, nil), nil
		}
	})}
}

// Build a tiny cxz-shaped fixture from a clean Git checkout. This exercises real
// Go build metadata without depending on the main working tree being clean.
func cleanExecutableFixture(t *testing.T) []byte {
	t.Helper()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "cmd", "cxz"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"go.mod":          "module github.com/lesomnus/cxz\n\ngo 1.27.0\n",
		"cmd/cxz/main.go": "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"cxz edge fixture\") }\n",
	} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "fixture.exe")
	for _, args := range [][]string{
		{"git", "init", "--quiet"}, {"git", "add", "."},
		{"git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Fixture"},
		{"go", "build", "-buildvcs=true", "-o", path, "./cmd/cxz"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestWindowsDownloadAndReplacementWithoutBuildTools(t *testing.T) {
	binary := cleanExecutableFixture(t)
	// Preparation used Go and Git, but the updater itself must use neither.
	t.Setenv("PATH", t.TempDir())
	archive := zipFixture(t, "cxz.exe", 0755, binary)
	for _, ref := range []string{"main", "v1.2.3-rc.1"} {
		t.Run(ref, func(t *testing.T) {
			tag := ref
			if ref == "main" {
				tag = "edge"
			}
			release, sums := releaseFixture(t, tag, runtime.GOARCH, archive)
			a, err := downloadWindows(context.Background(), fixtureClient(t, release, sums, archive), t.TempDir(), ref, runtime.GOARCH, io.Discard)
			if err != nil || !validRevision.MatchString(a.Revision) {
				t.Fatal(a, err)
			}
			wantVersion := tag
			if ref == "main" {
				wantVersion = "source-" + a.Revision[:12]
			}
			if a.Version != wantVersion {
				t.Fatal(a)
			}
			if out, err := exec.Command(a.Path, "version").CombinedOutput(); err != nil || string(out) != "cxz edge fixture\n" {
				t.Fatal(string(out), err)
			}
			target := filepath.Join(t.TempDir(), "cxz.exe")
			writeTestFile(t, target, "previous executable")
			r, err := Prepare(target)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if err := r.Stage(a.Path); err != nil {
				t.Fatal(err)
			}
			if changed, err := r.Apply(); err != nil || !changed {
				t.Fatal(changed, err)
			}
			requireContents(t, r.Previous, "previous executable")
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, binary) {
				t.Fatal("installed image differs", err)
			}
		})
	}
}

func TestWindowsDownloadRejectsIncompleteChangedOrInvalidReleases(t *testing.T) {
	for _, kind := range []string{"missing asset", "duplicate asset", "draft", "foreign URL", "checksum", "missing checksum", "duplicate checksum", "asset changed", "sums changed", "invalid executable", "invalid archive", "traversal", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			data := zipFixture(t, "cxz.exe", 0755, []byte("invalid executable"))
			switch kind {
			case "invalid archive":
				data = []byte("not a ZIP")
			case "traversal":
				data = zipFixture(t, "../cxz.exe", 0755, []byte("bad"))
			case "symlink":
				data = zipFixture(t, "cxz.exe", os.ModeSymlink|0777, []byte("bad"))
			}
			release, sums := releaseFixture(t, "edge", runtime.GOARCH, data)
			want := "read downloaded executable"
			switch kind {
			case "missing asset":
				release.Assets = release.Assets[1:]
				want = "has no"
			case "duplicate asset":
				release.Assets = append(release.Assets, release.Assets[0])
				want = "duplicate release asset"
			case "draft":
				release.Draft = true
				want = "does not match"
			case "foreign URL":
				release.Assets[0].URL = "https://elsewhere.invalid/cxz.zip"
				want = "unexpected download location"
			case "checksum":
				sums = bytes.ReplaceAll(sums, sums[:64], bytes.Repeat([]byte("0"), 64))
				release.Assets[1].Digest = ""
				want = "checksum differs"
			case "missing checksum":
				sums = []byte("empty\n")
				release.Assets[1].Digest = ""
				want = "missing checksum"
			case "duplicate checksum":
				sums = append(sums, sums...)
				release.Assets[1].Digest = ""
				want = "duplicate checksum"
			case "asset changed":
				release.Assets[0].Digest = "sha256:" + strings.Repeat("0", 64)
				want = "asset changed"
			case "sums changed":
				release.Assets[1].Digest = "sha256:" + strings.Repeat("0", 64)
				want = "asset changed"
			case "invalid archive":
				want = "invalid Windows archive"
			case "traversal", "symlink":
				want = "must contain only"
			}
			_, err := downloadWindows(context.Background(), fixtureClient(t, release, sums, data), t.TempDir(), "main", runtime.GOARCH, io.Discard)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want %q, got %v", want, err)
			}
		})
	}
}

func TestReleaseFetchLimitsStatusCancellationAndRedirects(t *testing.T) {
	for _, kind := range []string{"missing", "rate limit", "oversized", "stream oversized", "cancelled", "insecure redirect"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancelled" {
				cancel()
			}
			client := &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
				if err := req.Context().Err(); err != nil {
					return nil, err
				}
				res := response(req, http.StatusOK, []byte("12345"))
				switch kind {
				case "missing":
					res.StatusCode, res.Status = http.StatusNotFound, "404 Not Found"
				case "rate limit":
					res.StatusCode, res.Status = http.StatusForbidden, "403 Forbidden"
				case "stream oversized":
					res.ContentLength = -1
				case "insecure redirect":
					res.StatusCode = http.StatusFound
					res.Header.Set("Location", "http://example.invalid/cxz")
				}
				return res, nil
			})}
			if _, err := fetchReleaseFile(ctx, client, releaseAPI+"edge", 4); err == nil {
				t.Fatal("bad response accepted")
			}
		})
	}
}

func TestDownloadRefRequiresPublishedBuild(t *testing.T) {
	for _, ref := range []string{"topic/test", strings.Repeat("a", 40), "edge", "", "../main"} {
		if err := ValidateDownloadRef(ref); err == nil {
			t.Fatal("unpublished ref accepted", ref)
		}
	}
	for _, ref := range []string{"main", "v1.2.3", "v1.2.3-rc.1"} {
		if err := ValidateDownloadRef(ref); err != nil {
			t.Fatal(ref, err)
		}
	}
}
