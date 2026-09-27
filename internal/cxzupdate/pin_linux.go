package cxzupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var mappedDigest = sync.OnceValues(func() (string, error) {
	f, e := os.Open("/proc/self/exe")
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
})

// PinSelf preserves the actual mapped image even after its original pathname was
// atomically replaced. Rollback must not resolve a mutable tools symlink.
func PinSelf(root string) (string, error) {
	b := Current()
	if !b.Managed() {
		return os.Executable()
	}
	sum, e := mappedDigest()
	if e != nil {
		return "", e
	}
	path := filepath.Join(root, "runtime-binaries", b.Revision, sum, "cxz")
	if st, e := os.Stat(path); e == nil && st.Mode().IsRegular() {
		return path, nil
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return "", e
	}
	in, e := os.Open("/proc/self/exe")
	if e != nil {
		return "", e
	}
	defer in.Close()
	f, e := os.CreateTemp(filepath.Dir(path), ".pin-")
	if e != nil {
		return "", e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, e = io.Copy(f, in); e != nil {
		return "", e
	}
	if e = f.Chmod(0700); e != nil {
		return "", e
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	if e = os.Link(f.Name(), path); e != nil && !os.IsExist(e) {
		return "", e
	}
	return path, core.SyncDir(filepath.Dir(path))
}
func ValidSessionBinary(root, path string) bool {
	if ValidBinary(path) {
		return true
	}
	prefix := filepath.Join(root, "runtime-binaries") + string(os.PathSeparator)
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	p := strings.Split(strings.TrimPrefix(path, prefix), "/")
	return (len(p) == 2 || len(p) == 3) && revisionPattern.MatchString(p[0]) && p[len(p)-1] == "cxz" && (len(p) == 2 || hashPattern.MatchString(p[1])) && filepath.Clean(path) == path
}
func CheckPinned(root, path string) error {
	if !ValidSessionBinary(root, path) {
		return fmt.Errorf("supervisor must be an immutable cxz release")
	}
	st, e := os.Stat(path)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
		return fmt.Errorf("supervisor binary unavailable")
	}
	return nil
}

// PublishTools changes only the pathname used for future connections. Existing
// processes and rollback binaries keep their original mapped images.
func PublishTools() error {
	if !Current().Managed() {
		return fmt.Errorf("unmanaged tools build")
	}
	lock, e := core.Lock("/cxz/tools/.cxz.update.lock")
	if e != nil {
		return e
	}
	defer lock.Close()
	in, e := os.Open("/proc/self/exe")
	if e != nil {
		return e
	}
	defer in.Close()
	f, e := os.CreateTemp("/cxz/tools", ".cxz-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, e = io.Copy(f, in); e != nil {
		return e
	}
	if e = f.Chmod(0755); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), "/cxz/tools/cxz"); e != nil {
		return e
	}
	return core.SyncDir("/cxz/tools")
}

func CheckSupervisor(ctx context.Context, root, path string) error {
	if e := CheckPinned(root, path); e != nil {
		return e
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, e := exec.CommandContext(check, path, "_build-info").Output()
	if e != nil {
		return e
	}
	var build Build
	if json.Unmarshal(b, &build) != nil || !build.Managed() || build.Platform != Current().Platform || build.Protocol != Protocol || build.Schema != Schema {
		return fmt.Errorf("supervisor build is incompatible")
	}
	expected := strings.Split(strings.TrimPrefix(path, filepath.Join(root, "runtime-binaries")+"/"), "/")[0]
	if ValidBinary(path) {
		expected = strings.Split(strings.TrimPrefix(path, "/cxz/tools/cxz-builds/releases/"), "/")[0]
	}
	if build.Revision != expected {
		return fmt.Errorf("supervisor revision does not match immutable path")
	}
	return nil
}
