package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// A store the manager wrote has to be openable by the runtime that owns the
// session, which is not root. SQLite writes `-shm` even for a read-only
// connection, so a store left owned by root made every resume of a converted
// session fail with "unable to open database file". See cxz#280.
func TestAConvertedStoreIsHandedToTheSessionsOwner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conversation.db")
	for _, p := range []string{path, path + "-wal"} { // no -shm: it may not exist
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}

	type handover struct {
		path     string
		uid, gid int
	}
	var handed []handover
	defer func(e func() int, h func(string, int, int) error) { effectiveUID, handOver = e, h }(effectiveUID, handOver)
	effectiveUID = func() int { return 0 }
	handOver = func(p string, uid, gid int) error {
		handed = append(handed, handover{p, uid, gid})
		return nil
	}

	if err = ownLikeDir(dir, storeFiles(path)...); err != nil {
		t.Fatal(err)
	}
	if len(handed) != 3 {
		t.Fatalf("handed over %d files, not the store and both of SQLite's: %+v", len(handed), handed)
	}
	var sawShm bool
	for _, h := range handed {
		if h.path == path+"-shm" {
			sawShm = true
		}
		if h.uid != int(ownerUID(t, info)) {
			t.Fatal("handed to the wrong owner:", h)
		}
	}
	if !sawShm {
		t.Fatal("the -shm was not handed over, which is the file a reader writes")
	}

	// Not root: nothing is touched, so a frontend running as a person does not
	// try to chown anything.
	handed = nil
	effectiveUID = func() int { return 1000 }
	if err = ownLikeDir(dir, storeFiles(path)...); err != nil {
		t.Fatal(err)
	}
	if len(handed) != 0 {
		t.Fatal("a non-root manager handed files over anyway:", handed)
	}
}
