package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"golang.org/x/sys/windows/registry"
)

func testEnvironmentKey(t *testing.T) registry.Key {
	t.Helper()
	path := `Software\cxz-test-` + core.ID()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		key.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	})
	return key
}

func TestWindowsInstallPreservesUserPath(t *testing.T) {
	for _, kind := range []uint32{registry.SZ, registry.EXPAND_SZ} {
		key := testEnvironmentKey(t)
		original := strings.Repeat(`C:\keep\한글;`, 200) + `%USERPROFILE%\my tools`
		var err error
		if kind == registry.SZ {
			err = key.SetStringValue("Path", original)
		} else {
			err = key.SetExpandStringValue("Path", original)
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", `C:\process-only-must-not-be-persisted`)
		dir := filepath.Join(t.TempDir(), "cxz 한글 with spaces")
		if changed, err := addUserPath(key, dir); err != nil || !changed {
			t.Fatal(changed, err)
		}
		if changed, err := addUserPath(key, strings.ToUpper(dir)+`\`); err != nil || changed {
			t.Fatal("case or trailing slash duplicated the PATH entry", changed, err)
		}
		value, gotKind, err := key.GetStringValue("Path")
		if err != nil || gotKind != kind || value != original+";"+dir {
			t.Fatal("user PATH was truncated, expanded, reordered, or merged with process PATH", err)
		}
	}
}

func TestWindowsInstallRecognizesExpandedPathAndMissingPath(t *testing.T) {
	key := testEnvironmentKey(t)
	dir := filepath.Join(t.TempDir(), "Programs", "cxz")
	t.Setenv("CXZ_TEST_PROGRAMS", filepath.Dir(dir))
	const original = `C:\keep;"%CXZ_TEST_PROGRAMS%\cxz\";C:\other`
	if err := key.SetExpandStringValue("Path", original); err != nil {
		t.Fatal(err)
	}
	if changed, err := addUserPath(key, dir); err != nil || changed {
		t.Fatal("environment reference was not recognized", changed, err)
	}
	if value, _, err := key.GetStringValue("Path"); err != nil || value != original {
		t.Fatal("an existing PATH entry was rewritten", err)
	}
	if err := key.DeleteValue("Path"); err != nil {
		t.Fatal(err)
	}
	if changed, err := addUserPath(key, dir); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if value, kind, err := key.GetStringValue("Path"); err != nil || kind != registry.EXPAND_SZ || value != dir {
		t.Fatal("missing user PATH was not initialized", err)
	}
}

func TestWindowsInstallRejectsInvalidPathWithoutChangingRegistry(t *testing.T) {
	key := testEnvironmentKey(t)
	if err := key.SetDWordValue("Path", 7); err != nil {
		t.Fatal(err)
	}
	if changed, err := addUserPath(key, `C:\Programs\cxz`); err == nil || changed {
		t.Fatal("unexpected registry value type was overwritten")
	}
	if err := key.SetStringValue("Path", `C:\keep`); err != nil {
		t.Fatal(err)
	}
	if changed, err := addUserPath(key, `C:\Programs;bad\cxz`); err == nil || changed {
		t.Fatal("a directory containing a PATH separator was registered")
	}
	if value, _, err := key.GetStringValue("Path"); err != nil || value != `C:\keep` {
		t.Fatal("invalid directory changed PATH", err)
	}
}
