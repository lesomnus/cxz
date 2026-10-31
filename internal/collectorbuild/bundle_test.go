package collectorbuild

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
)

func TestStandaloneBuildContextIsCompleteAndDeterministic(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"Dockerfile", "builder.yaml", "collector.lock.mod", "collector.lock.sum", "components/go.mod", "components/identityprocessor/processor.go", "identitylib/internal/telemetryauth/identity.go", "identitylib/internal/resourceid/id.go"} {
		if len(files[file]) == 0 {
			t.Fatal("missing build source", file)
		}
	}
	var first, second bytes.Buffer
	for _, out := range []*bytes.Buffer{&first, &second} {
		w := tar.NewWriter(out)
		if err = Archive(w, "collector-src/"); err != nil {
			t.Fatal(err)
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("nondeterministic build context")
	}
	r := tar.NewReader(&first)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Mode != 0644 {
			t.Fatal(h)
		}
	}
}
