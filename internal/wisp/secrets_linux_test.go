//go:build linux

package wisp

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHostTmpfsDoesNotRequireHardeningFlags(t *testing.T) {
	for _, flags := range []int64{0, unix.ST_NOSUID, unix.ST_NODEV | unix.ST_NOSUID, unix.ST_NODEV | unix.ST_NOSUID | unix.ST_NOEXEC} {
		fs := unix.Statfs_t{Type: unix.TMPFS_MAGIC, Flags: flags}
		if err := validateSecretFilesystem(&fs); err != nil {
			t.Fatal(flags, err)
		}
	}
	if err := validateSecretFilesystem(&unix.Statfs_t{Type: unix.EXT4_SUPER_MAGIC}); err == nil {
		t.Fatal("disk accepted")
	}
	if err := validateSecretFilesystem(&unix.Statfs_t{Type: unix.TMPFS_MAGIC, Flags: unix.ST_RDONLY}); err == nil {
		t.Fatal("read-only accepted")
	}
}

func TestSecretRootRejectsDiskAndSymlinks(t *testing.T) {
	root := t.TempDir()
	if fd, err := checkedSecretRoot(root); err == nil {
		unix.Close(fd)
		t.Fatal("accepted disk")
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
func TestSecretSweepIdleAndClosePersistence(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old := now.Add(-9 * time.Hour)
	create := func(name string, at, mt time.Time) string {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "value")
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, mt); err != nil {
			t.Fatal(err)
		}
		return path
	}
	expired := create("12345678", old, old)
	read := create("12345679", now, old)
	written := create("1234567a", old, now)
	unrelated := create("unrelated", old, old)
	rootFD, _ := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	dirFD, _ := unix.Open(filepath.Dir(read), unix.O_RDONLY|unix.O_DIRECTORY, 0)
	store := &secretStore{}
	store.track(read, &secretFile{session: "s", name: "12345679", root: rootFD, dir: dirFD})
	store.clear("")
	if _, err := os.Stat(read); err != nil {
		t.Fatal("disconnect deleted file")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = sweepSecretFD(fd, now); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(expired); !os.IsNotExist(err) {
		t.Fatal("old file retained")
	}
	for _, path := range []string{read, written, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("unexpected deletion", path)
		}
	}
	fd, _ = unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err = sweepSecretFD(fd, now.Add(9*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(read); !os.IsNotExist(err) {
		t.Fatal("restart sweep did not expire file")
	}
}
