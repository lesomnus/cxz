package maintenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

func TestRotationKeepsInheritedAppendDescriptorAndBoundedArchives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for range 6 {
		if _, err = f.WriteString(strings.Repeat("x", 300)); err != nil {
			t.Fatal(err)
		}
		if ok, err := Rotate(path, 100, 4); err != nil || !ok {
			t.Fatal(ok, err)
		}
	}
	if _, err = f.WriteString("still connected"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "still connected" {
		t.Fatalf("writer stranded: %q %v", b, err)
	}
	for i := 1; i < 4; i++ {
		st, err := os.Stat(path + "." + string(rune('0'+i)))
		if err != nil || st.Size() != 100 {
			t.Fatal(st, err)
		}
	}
	if _, err = os.Stat(path + ".4"); !os.IsNotExist(err) {
		t.Fatal("unbounded archives")
	}
}

func TestProfilesProtectRunningAgentsCredentialsAndSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "session-profiles", "session", "accounts", "work")
	for _, rel := range []string{"tmp/work", "home/.cache/go-build", "config"} {
		if err := os.MkdirAll(filepath.Join(profile, rel), 0700); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(profile, "config", "auth.json")
	os.WriteFile(secret, []byte("fixture credential"), 0600)
	guard, err := core.Lock(filepath.Join(profile, "login.lock"))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := ProfileCandidates(root, Default(), time.Now(), true)
	if err != nil || len(cs) != 0 {
		t.Fatal(cs, err)
	}
	guard.Close()
	cs, err = ProfileCandidates(root, Default(), time.Now(), false)
	if err != nil || len(cs) != 0 {
		t.Fatal("fresh stopped profile lost grace", cs, err)
	}
	cs, err = ProfileCandidates(root, Default(), time.Now(), true)
	if err != nil || len(cs) != 2 {
		t.Fatal(cs, err)
	}
	guard, err = core.Lock(filepath.Join(profile, "login.lock"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if ok, err := c.Remove(t.Context()); err != nil || ok {
			t.Fatal("profile resumed after collection", ok, err)
		}
	}
	guard.Close()
	// A cache symlink must never turn a disposable path into credential deletion.
	cache := filepath.Join(profile, "home", ".cache", "go-build")
	os.RemoveAll(cache)
	os.Symlink(filepath.Join(profile, "config"), cache)
	for _, c := range cs {
		if _, err = c.Remove(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = os.Stat(secret); err != nil {
		t.Fatal("credentials removed", err)
	}
}
