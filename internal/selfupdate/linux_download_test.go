package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func linuxArchive(t *testing.T, name string, kind byte, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	h := &tar.Header{Name: name, Mode: 0755, Size: int64(len(data)), Typeflag: kind}
	if kind != tar.TypeReg {
		h.Size = 0
	}
	if e := tw.WriteHeader(h); e != nil {
		t.Fatal(e)
	}
	if h.Size > 0 {
		tw.Write(data)
	}
	tw.Close()
	gz.Close()
	return b.Bytes()
}
func TestLinuxReleaseDownload(t *testing.T) {
	binary := cleanExecutableFixture(t, "linux")
	data := linuxArchive(t, "cxz", tar.TypeReg, binary)
	name := "cxz-v0.1.0-linux-" + runtime.GOARCH + ".tar.gz"
	sum := sha256.Sum256(data)
	sums := []byte(fmt.Sprintf("%x  %s\n", sum, name))
	release := releaseInfo{Tag: "v0.1.0", Assets: []releaseAsset{{Name: name, URL: releaseDownloads + "v0.1.0/" + name}, {Name: "SHA256SUMS", URL: releaseDownloads + "v0.1.0/SHA256SUMS"}}}
	a, e := downloadPlatform(context.Background(), fixtureClient(t, release, sums, data), t.TempDir(), "v0.1.0", "linux", runtime.GOARCH, io.Discard)
	if e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(a.Path)
	if !bytes.Equal(got, binary) || a.Version != "v0.1.0" {
		t.Fatal("release identity changed")
	}
}
func TestLinuxArchiveRejectsUnsafeMembers(t *testing.T) {
	for _, v := range []struct {
		name string
		kind byte
	}{{"../cxz", tar.TypeReg}, {"cxz", tar.TypeSymlink}, {"cxz/", tar.TypeDir}} {
		if e := unpackLinuxArchive(linuxArchive(t, v.name, v.kind, []byte("x")), filepath.Join(t.TempDir(), "cxz")); e == nil {
			t.Fatal(v)
		}
	}
}
