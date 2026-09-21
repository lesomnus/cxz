package selfupdate

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0751); err != nil {
		t.Fatal(err)
	}
}

func requireContents(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || string(b) != want {
		t.Fatalf("%s: want %q, got %q: %v", path, want, b, err)
	}
}

func TestReplacementBacksUpAndLocksExecutable(t *testing.T) {
	dir := t.TempDir()
	target, source := filepath.Join(dir, "cxz.exe"), filepath.Join(dir, "source")
	writeTestFile(t, target, "old")
	writeTestFile(t, source, "new")
	r, err := Prepare(target)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if other, err := Prepare(target); err == nil {
		other.Close()
		t.Fatal("concurrent executable update permitted")
	}
	if err := r.Stage(source); err != nil {
		t.Fatal(err)
	}
	requireContents(t, target, "old")
	changed, err := r.Apply()
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	requireContents(t, target, "new")
	requireContents(t, r.Previous, "old")
	if runtime.GOOS != "windows" {
		st, err := os.Stat(target)
		if err != nil || st.Mode().Perm() != 0751 {
			t.Fatal("executable permissions changed", err)
		}
	}
}

func TestReplacementFailureKeepsOriginal(t *testing.T) {
	for _, failure := range []string{"changed target", "missing candidate"} {
		t.Run(failure, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "cxz.exe")
			writeTestFile(t, target, "old")
			r, err := Prepare(target)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			want := "old"
			if failure == "changed target" {
				want = "another update"
				writeTestFile(t, target, want)
			} else if err := os.Remove(r.Candidate); err != nil {
				t.Fatal(err)
			}
			if changed, err := r.Apply(); err == nil || changed {
				t.Fatal("failed replacement reported success", changed, err)
			}
			requireContents(t, target, want)
		})
	}
}

func TestReplacementPreservesSymlink(t *testing.T) {
	dir := t.TempDir()
	target, link, source := filepath.Join(dir, "real-cxz"), filepath.Join(dir, "cxz"), filepath.Join(dir, "source")
	writeTestFile(t, target, "old")
	writeTestFile(t, source, "new")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	r, err := Prepare(link)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Stage(source); err != nil {
		t.Fatal(err)
	}
	if changed, err := r.Apply(); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatal("PATH symlink replaced", err)
	}
	requireContents(t, link, "new")
}

// Exercise the real OS rule for replacing a loaded executable, especially on
// Windows where a rename-to-backup is required before installing the new file.
func TestReplacementOfRunningExecutable(t *testing.T) {
	if target := os.Getenv("CXZ_TEST_RUNNING_UPDATE"); target != "" {
		r, err := Prepare(target)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		if err := r.Stage(target + ".source"); err != nil {
			t.Fatal(err)
		}
		if changed, err := r.Apply(); err != nil || !changed {
			t.Fatal(changed, err)
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "cxz.exe")
	if err := os.WriteFile(target, original, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, target+".source", "new executable contents")
	cmd := exec.Command(target, "-test.run=^TestReplacementOfRunningExecutable$")
	cmd.Env = append(os.Environ(), "CXZ_TEST_RUNNING_UPDATE="+target)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("live replacement failed: %v\n%s", err, out)
	}
	requireContents(t, target, "new executable contents")
	backup, err := os.ReadFile(filepath.Join(filepath.Dir(target), "cxz.previous.exe"))
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatal("running executable backup differs", err)
	}
}
