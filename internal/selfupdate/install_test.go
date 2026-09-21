package selfupdate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestInstallLocalExecutableAndUpdateInPlace(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.exe")
	target := filepath.Join(t.TempDir(), "Programs", "cxz", "cxz.exe")
	writeTestFile(t, source, "first build")
	if changed, err := Install(source, target); err != nil || !changed {
		t.Fatal(changed, err)
	}
	requireContents(t, source, "first build")
	requireContents(t, target, "first build")
	if changed, err := Install(target, target); err != nil || changed {
		t.Fatal("installation from the permanent executable was not a no-op", changed, err)
	}
	writeTestFile(t, source, "second build")
	if changed, err := Install(source, target); err != nil || !changed {
		t.Fatal(changed, err)
	}
	requireContents(t, target, "second build")
	requireContents(t, filepath.Join(filepath.Dir(target), "cxz.previous.exe"), "first build")
	if changed, err := Install(filepath.Join(t.TempDir(), "missing.exe"), target); err == nil || changed {
		t.Fatal("missing source was accepted")
	}
	requireContents(t, target, "second build")
}

func TestInstallCoordinatesWithSelfUpdate(t *testing.T) {
	for _, exists := range []bool{false, true} {
		source, target := filepath.Join(t.TempDir(), "source.exe"), filepath.Join(t.TempDir(), "cxz.exe")
		writeTestFile(t, source, "new build")
		if exists {
			writeTestFile(t, target, "installed build")
		}
		lock, err := core.Lock(filepath.Join(filepath.Dir(target), ".cxz.exe.update.lock"))
		if err != nil {
			t.Fatal(err)
		}
		changed, installErr := Install(source, target)
		lock.Close()
		if installErr == nil || changed {
			t.Fatal("concurrent installation or self-update was allowed")
		}
		if exists {
			requireContents(t, target, "installed build")
		} else if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("failed installation published an executable", err)
		}
	}
}
