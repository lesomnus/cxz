package selfupdate

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

const installImage = "python:3.13-alpine"

//go:embed install.py
var installHelper string

// PrepareWithDocker falls back only on host permission errors. A root helper
// proves that the bind is the client's actual directory before acquiring the
// same installation lock as a direct writer. It holds that lock through build.
func PrepareWithDocker(ctx context.Context, target, work string, out io.Writer) (*Replacement, error) {
	r, err := Prepare(target)
	if err == nil || (!errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.EROFS)) {
		return r, err
	}
	fmt.Fprintf(out, "Using Docker root installer for %s\n", target)
	return prepareDocker(ctx, target, work, out)
}

type installReply struct {
	Stage   string `json:"stage"`
	Changed bool   `json:"changed"`
	Error   string `json:"error"`
}

type dockerInstaller struct {
	input  io.WriteCloser
	output *os.File
	reader *json.Decoder
	cancel context.CancelFunc
	done   chan error
	name   string
	once   sync.Once
}

func prepareDocker(ctx context.Context, target, work string, out io.Writer) (*Replacement, error) {
	r, err := resolveReplacement(target)
	if err != nil {
		return nil, err
	}
	old, err := os.Open(r.Target)
	if err != nil {
		return nil, err
	}
	sum, err := checksum(old)
	old.Close()
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(r.Target)
	engineParent, err := dockerx.EnginePath(parent)
	if err != nil {
		// The challenge below, not Docker endpoint syntax, establishes whether
		// an untranslatable path denotes the client's directory.
		engineParent = parent
	}
	id, token := core.ID(), core.ID()
	marker := ".cxz-install-probe-" + id
	st := r.original.Sys().(*syscall.Stat_t)
	helperCtx, cancel := context.WithCancel(ctx)
	d := &dockerInstaller{name: "cxz-self-update-" + id, cancel: cancel, done: make(chan error, 1)}
	cmd := exec.CommandContext(helperCtx, "docker", "run", "--rm", "-i", "--name", d.name,
		"--label", "cxz.role=self-update-installer", "--network", "none", "--read-only",
		"--user", "0:0", "--userns", "host", "--cap-drop", "ALL",
		"--cap-add", "DAC_OVERRIDE", "--cap-add", "CHOWN", "--cap-add", "FOWNER",
		"--security-opt", "no-new-privileges", "--mount", bindDirectory(engineParent),
		installImage, "python3", "-u", "-c", installHelper, "/target", filepath.Base(r.Target),
		marker, token, sum, strconv.FormatUint(uint64(st.Uid), 10), strconv.FormatUint(uint64(st.Gid), 10),
		strconv.FormatUint(uint64(r.original.Mode().Perm()), 10))
	cmd.Stderr = out
	cmd.WaitDelay = 5 * time.Second
	d.input, err = cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		d.input.Close()
		cancel()
		return nil, err
	}
	cmd.Stdout = writer
	d.output, d.reader = reader, json.NewDecoder(reader)
	if err := cmd.Start(); err != nil {
		d.input.Close()
		reader.Close()
		writer.Close()
		cancel()
		return nil, err
	}
	writer.Close()
	// A remote Docker transport can leave inherited pipe descriptors open after
	// cancellation. Unblock protocol reads even if that transport has not exited.
	context.AfterFunc(helperCtx, func() { _ = reader.Close() })
	go func() { d.done <- cmd.Wait() }()
	ok := false
	defer func() {
		if !ok {
			d.close()
		}
	}()
	if _, err := d.receive("probe"); err != nil {
		return nil, err
	}
	if err := verifyInstallProbe(parent, marker, token); err != nil {
		return nil, err
	}
	if err := json.NewEncoder(d.input).Encode(map[string]bool{"continue": true}); err != nil {
		return nil, err
	}
	if _, err := d.receive("ready"); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(work, ".cxz-update-*.exe")
	if err != nil {
		return nil, err
	}
	r.Candidate = f.Name()
	if err := f.Close(); err != nil {
		os.Remove(r.Candidate)
		return nil, err
	}
	r.apply = func() (bool, error) {
		changed, err := d.apply(r.Candidate)
		if err != nil {
			// An interrupted reply can follow a successful rename. Stop the
			// helper before inspecting the local file to report that accurately.
			d.close()
			if !changed {
				changed = replacementVisible(r)
			}
		}
		return changed, err
	}
	r.close = d.close
	ok = true
	return r, nil
}

func verifyInstallProbe(parent, marker, token string) error {
	proof, err := os.ReadFile(filepath.Join(parent, marker))
	if err != nil || string(proof) != token {
		return fmt.Errorf("Docker does not expose this client's installation directory %s; refusing to update a different filesystem", parent)
	}
	return nil
}

func replacementVisible(r *Replacement) bool {
	current, err := os.Open(r.Target)
	if err != nil {
		return false
	}
	defer current.Close()
	st, err := current.Stat()
	if err != nil || os.SameFile(st, r.original) {
		return false
	}
	candidate, err := os.Open(r.Candidate)
	if err != nil {
		return false
	}
	defer candidate.Close()
	actual, err := checksum(current)
	if err != nil {
		return false
	}
	expected, err := checksum(candidate)
	return err == nil && actual == expected
}

func bindDirectory(path string) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"type=bind", "source=" + path, "target=/target"})
	w.Flush()
	return strings.TrimSuffix(b.String(), "\n")
}

func checksum(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	_, err := f.Seek(0, io.SeekStart)
	return hex.EncodeToString(h.Sum(nil)), err
}

func (d *dockerInstaller) receive(stage string) (installReply, error) {
	var reply installReply
	if err := d.reader.Decode(&reply); err != nil {
		return reply, fmt.Errorf("Docker installation helper disconnected: %w", err)
	}
	if reply.Error != "" {
		return reply, fmt.Errorf("Docker installation helper: %s", reply.Error)
	}
	if reply.Stage != stage {
		return reply, fmt.Errorf("unexpected Docker installation stage %q", reply.Stage)
	}
	return reply, nil
}

func (d *dockerInstaller) apply(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	sum, err := checksum(f)
	if err != nil {
		return false, err
	}
	if err := json.NewEncoder(d.input).Encode(map[string]any{"size": st.Size(), "sha256": sum}); err != nil {
		return false, err
	}
	if _, err := io.CopyN(d.input, f, st.Size()); err != nil {
		return false, fmt.Errorf("send executable to Docker installer: %w", err)
	}
	reply, err := d.receive("done")
	return reply.Changed, err
}

func (d *dockerInstaller) close() {
	d.once.Do(d.stop)
}

func (d *dockerInstaller) stop() {
	d.input.Close()
	select {
	case <-d.done:
	case <-time.After(2 * time.Second):
		d.cancel()
		<-d.done
	}
	d.cancel()
	d.output.Close()
	// A detached Docker client can leave its container alive. Remove only this
	// invocation's random, named helper; successful --rm exits need no deletion.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// SIGTERM lets the helper remove a partially received file even when the
	// original docker client was interrupted. Force removal is the fallback.
	_ = exec.CommandContext(ctx, "docker", "stop", "--timeout", "2", d.name).Run()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", d.name).Run()
}
