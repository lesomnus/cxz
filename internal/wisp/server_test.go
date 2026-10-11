package wisp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestServeMultipleLookupsAndMetadata(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"한글 파일", "$(touch injected)", "bad\nname"} {
		if err := os.WriteFile(filepath.Join(root, n), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "run"), nil, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing target", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	var in, out bytes.Buffer
	enc := json.NewEncoder(&in)
	for _, p := range []string{root, "relative", filepath.Join(root, "dir")} {
		_ = enc.Encode(Request{Path: p})
	}
	if err := Serve(&in, &out); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(&out)
	var hello Response
	if err := dec.Decode(&hello); err != nil || hello.Version != Version {
		t.Fatal(hello, err)
	}
	entries := map[string]Entry{}
	done, errors := 0, 0
	for {
		var r Response
		err := dec.Decode(&r)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if r.Done {
			done++
		}
		if r.Error != "" {
			errors++
		}
		for _, e := range r.Entries {
			entries[e.Name] = e
		}
	}
	if done != 3 || errors != 1 || len(entries) != 5 || !entries["dir"].Directory || !entries["run"].Executable || entries["link"].LinkTarget != "missing target" || entries["link"].Kind != "symlink" || entries["dir"].Kind != "directory" || entries["run"].Kind != "file" {
		t.Fatal(done, errors, entries)
	}
}

func TestFileKind(t *testing.T) {
	for _, tc := range []struct {
		mode os.FileMode
		want string
	}{
		{0600, "file"}, {0700, "file"}, {os.ModeDir, "directory"}, {os.ModeSymlink, "symlink"},
		{os.ModeNamedPipe, "fifo"}, {os.ModeSocket, "socket"}, {os.ModeDevice, "block"},
		{os.ModeDevice | os.ModeCharDevice, "character"}, {os.ModeIrregular, "unknown"},
	} {
		if got := fileKind(tc.mode); got != tc.want {
			t.Fatalf("%v: %s, want %s", tc.mode, got, tc.want)
		}
	}
}
