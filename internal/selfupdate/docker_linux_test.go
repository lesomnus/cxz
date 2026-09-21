package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

type helperTest struct {
	input  io.WriteCloser
	reader *json.Decoder
	cmd    *exec.Cmd
	dir    string
	marker string
	token  string
	closed bool
}

func startHelperTest(t *testing.T) *helperTest {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required to test the embedded installer")
	}
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "cxz"), "old")
	st, err := os.Stat(filepath.Join(dir, "cxz"))
	if err != nil {
		t.Fatal(err)
	}
	sys := st.Sys().(*syscall.Stat_t)
	sum := sha256.Sum256([]byte("old"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	h := &helperTest{dir: dir, marker: ".cxz-probe-test", token: core.ID()}
	h.cmd = exec.CommandContext(ctx, python, "-u", "-c", installHelper, dir, "cxz", h.marker, h.token,
		hex.EncodeToString(sum[:]), strconv.FormatUint(uint64(sys.Uid), 10), strconv.FormatUint(uint64(sys.Gid), 10), "489") // 0751
	h.input, err = h.cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	output, err := h.cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	h.reader = json.NewDecoder(output)
	h.cmd.Stderr = os.Stderr
	if err := h.cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		h.input.Close()
		if !h.closed {
			_ = h.cmd.Wait()
		}
		cancel()
	})
	h.receive(t, "probe")
	return h
}

func (h *helperTest) receive(t *testing.T, stage string) installReply {
	t.Helper()
	var reply installReply
	if err := h.reader.Decode(&reply); err != nil {
		t.Fatal("installer response", err)
	}
	if reply.Stage != stage {
		t.Fatalf("want %s, got %+v", stage, reply)
	}
	return reply
}

func (h *helperTest) confirm(t *testing.T) {
	t.Helper()
	if err := verifyInstallProbe(h.dir, h.marker, h.token); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(h.input).Encode(map[string]bool{"continue": true}); err != nil {
		t.Fatal(err)
	}
}

func (h *helperTest) send(t *testing.T, content, declared string) {
	t.Helper()
	sum := sha256.Sum256([]byte(declared))
	if err := json.NewEncoder(h.input).Encode(map[string]any{"size": len(declared), "sha256": hex.EncodeToString(sum[:])}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(h.input, content); err != nil {
		t.Fatal(err)
	}
}

func (h *helperTest) finish(t *testing.T) {
	t.Helper()
	h.input.Close()
	if err := h.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	h.closed = true
}

func TestDockerInstallerProtocolPreservesFileAndCoordinatesLocks(t *testing.T) {
	h := startHelperTest(t)
	h.confirm(t)
	h.receive(t, "ready")
	if lock, err := core.Lock(filepath.Join(h.dir, ".cxz.update.lock")); err == nil {
		lock.Close()
		t.Fatal("installer does not coordinate with direct updates")
	}
	h.send(t, "new", "new")
	if reply := h.receive(t, "done"); !reply.Changed {
		t.Fatal("successful replacement not reported")
	}
	h.finish(t)
	requireContents(t, filepath.Join(h.dir, "cxz"), "new")
	requireContents(t, filepath.Join(h.dir, "cxz.previous"), "old")
	for _, name := range []string{"cxz", "cxz.previous"} {
		st, err := os.Stat(filepath.Join(h.dir, name))
		if err != nil || st.Mode().Perm() != 0751 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || st.Sys().(*syscall.Stat_t).Gid != uint32(os.Getgid()) {
			t.Fatal("ownership or permissions changed", name, err)
		}
	}
}

func TestDockerInstallerRejectsOtherFilesystemWithIdenticalExecutable(t *testing.T) {
	h := startHelperTest(t)
	other := t.TempDir()
	writeTestFile(t, filepath.Join(other, "cxz"), "old")
	if err := verifyInstallProbe(other, h.marker, h.token); err == nil {
		t.Fatal("same binary on a different filesystem passed identity proof")
	}
	h.finish(t) // Disconnect before confirming the root write operation.
	requireContents(t, filepath.Join(h.dir, "cxz"), "old")
	entries, err := os.ReadDir(h.dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("unconfirmed probe left files behind", entries, err)
	}
}

func TestDockerInstallerRejectsInterruptedCorruptOrConcurrentUpdates(t *testing.T) {
	for _, failure := range []string{"checksum", "interrupted", "changed"} {
		t.Run(failure, func(t *testing.T) {
			h := startHelperTest(t)
			h.confirm(t)
			h.receive(t, "ready")
			writeTestFile(t, filepath.Join(h.dir, "cxz.previous"), "existing backup")
			want := "old"
			switch failure {
			case "checksum":
				h.send(t, "bad", "new")
			case "interrupted":
				h.send(t, "n", "new")
				h.input.Close()
			case "changed":
				want = "another update"
				writeTestFile(t, filepath.Join(h.dir, "cxz"), want)
				h.send(t, "new", "new")
			}
			if reply := h.receive(t, "error"); reply.Changed || reply.Error == "" {
				t.Fatal("failure response lost", reply)
			}
			h.finish(t)
			requireContents(t, filepath.Join(h.dir, "cxz"), want)
			requireContents(t, filepath.Join(h.dir, "cxz.previous"), "existing backup")
			files, _ := filepath.Glob(filepath.Join(h.dir, ".cxz-*"))
			if len(files) != 0 {
				t.Fatal("failed transfer left temporary files", files)
			}
		})
	}
}

func TestDockerInstallerRefusesLockedExecutable(t *testing.T) {
	h := startHelperTest(t)
	lock, err := core.Lock(filepath.Join(h.dir, ".cxz.update.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	h.confirm(t)
	if reply := h.receive(t, "error"); reply.Changed || reply.Error == "" {
		t.Fatal(reply)
	}
	h.finish(t)
	requireContents(t, filepath.Join(h.dir, "cxz"), "old")
}

func TestDockerInstallerCancellationCleansPartialTransfer(t *testing.T) {
	h := startHelperTest(t)
	h.confirm(t)
	h.receive(t, "ready")
	h.send(t, "n", "new")
	deadline := time.Now().Add(2 * time.Second)
	for {
		files, _ := filepath.Glob(filepath.Join(h.dir, ".cxz-update-*"))
		if len(files) != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("installer did not begin receiving the executable")
		}
		time.Sleep(time.Millisecond)
	}
	if err := h.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if reply := h.receive(t, "error"); reply.Changed || !strings.Contains(reply.Error, "cancelled") {
		t.Fatal(reply)
	}
	h.finish(t)
	requireContents(t, filepath.Join(h.dir, "cxz"), "old")
	files, _ := filepath.Glob(filepath.Join(h.dir, ".cxz-*"))
	if len(files) != 0 {
		t.Fatal("cancelled transfer left temporary files", files)
	}
}

func TestBindDirectoryKeepsCSVCharactersLiteral(t *testing.T) {
	path := "/directory with , and \"quotes\"/한글"
	fields, err := csv.NewReader(strings.NewReader(bindDirectory(path))).Read()
	if err != nil || len(fields) != 3 || fields[1] != "source="+path || fields[2] != "target=/target" {
		t.Fatal(fields, err)
	}
}

func TestPrepareWithDockerDoesNotElevateForOtherErrors(t *testing.T) {
	if r, err := PrepareWithDocker(context.Background(), filepath.Join(t.TempDir(), "missing"), t.TempDir(), io.Discard); err == nil || !os.IsNotExist(err) || r != nil {
		t.Fatal("missing executable should fail without Docker", r, err)
	}
	target := filepath.Join(t.TempDir(), "cxz")
	writeTestFile(t, target, "old")
	r, err := PrepareWithDocker(context.Background(), target, t.TempDir(), io.Discard)
	if err != nil || r.UsesDocker() {
		t.Fatal("writable path unnecessarily used Docker", err)
	}
	defer r.Close()
	if other, err := PrepareWithDocker(context.Background(), target, t.TempDir(), io.Discard); err == nil {
		other.Close()
		t.Fatal("lock contention escalated instead of failing")
	}
}
