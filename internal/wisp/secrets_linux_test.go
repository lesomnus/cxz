//go:build linux

package wisp

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSecretRootRejectsDiskAndSymlinks(t *testing.T) {
	root := t.TempDir()
	if fd, err := checkedSecretRoot(root); err == nil {
		unix.Close(fd)
		t.Fatal("accepted ordinary directory")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if fd, err := checkedSecretRoot(link); err == nil {
		unix.Close(fd)
		t.Fatal("accepted symlink")
	}
}
func TestSecretExpiryAndScopedCleanup(t *testing.T) {
	store := &secretStore{}
	defer store.clear("")
	root := t.TempDir()
	create := func(name, session string, ttl time.Duration) string {
		t.Helper()
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "value")
		if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
		rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		if err != nil {
			t.Fatal(err)
		}
		dirFD, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		if err != nil {
			t.Fatal(err)
		}
		store.track(path, &secretFile{session: session, name: name, root: rootFD, dir: dirFD}, ttl)
		return path
	}
	first := create("first", "s1", time.Hour)
	second := create("second", "s2", time.Hour)
	store.clear("s1")
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("session secret retained")
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatal("other session removed")
	}
	unrelated := filepath.Join(root, "unrelated")
	if err := os.WriteFile(unrelated, nil, 0600); err != nil {
		t.Fatal(err)
	}
	store.drop(unrelated)
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("untracked path removed")
	}
	expires := create("expires", "s3", 10*time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(expires); os.IsNotExist(err) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("TTL did not remove file")
}
