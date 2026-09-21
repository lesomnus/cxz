package selfupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestBuildUsesClientExportAndIsolatedSource(t *testing.T) {
	work := t.TempDir()
	failure := errors.New("build stopped")
	_, err := build(context.Background(), work, "topic/update-v2", io.Discard, func(cmd *exec.Cmd) error {
		if cmd.Dir != work || !slices.Contains(cmd.Args, "type=local,dest=out") ||
			!slices.Contains(cmd.Args, "CXZ_REF=topic/update-v2") ||
			!slices.Contains(cmd.Args, "CXZ_GOOS="+runtime.GOOS) ||
			!slices.Contains(cmd.Args, "CXZ_GOARCH="+runtime.GOARCH) {
			t.Fatal("incorrect build/export contract", cmd.Args)
		}
		for _, arg := range cmd.Args {
			if strings.Contains(arg, "privileged") || arg == "--mount" || arg == "--volume" {
				t.Fatal("source build acquired host mounts or privileges", cmd.Args)
			}
		}
		input, err := io.ReadAll(cmd.Stdin)
		if err != nil || !strings.Contains(string(input), "https://github.com/lesomnus/cxz.git") || !strings.Contains(string(input), "git checkout --detach FETCH_HEAD") {
			t.Fatal("missing isolated source checkout", err)
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal("build error lost", err)
	}
}

func TestInvalidRefDoesNotStartBuild(t *testing.T) {
	for _, ref := range []string{"", "--upload-pack=other", "main; touch file", "main\n", "$(command)", strings.Repeat("a", 201)} {
		_, err := build(context.Background(), t.TempDir(), ref, io.Discard, func(*exec.Cmd) error {
			t.Fatal("invalid ref started a build", ref)
			return nil
		})
		if err == nil {
			t.Fatal("invalid ref accepted", ref)
		}
	}
}

func TestBuildRejectsInvalidArtifacts(t *testing.T) {
	for _, revision := range []string{"wrong", strings.Repeat("a", 40)} {
		work := t.TempDir()
		_, err := build(context.Background(), work, "main", io.Discard, func(*exec.Cmd) error {
			out := filepath.Join(work, "out")
			if err := os.Mkdir(out, 0700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(out, "revision"), []byte(revision), 0600); err != nil {
				return err
			}
			name := "cxz"
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			return os.WriteFile(filepath.Join(out, name), []byte("not a Go executable"), 0700)
		})
		if err == nil {
			t.Fatal("invalid artifact accepted", revision)
		}
	}
}
