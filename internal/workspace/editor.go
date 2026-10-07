package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/editor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Manager) editorProject(ctx context.Context, project string) (*Project, error) {
	p, err := m.resolve(ctx, project)
	if err != nil {
		return nil, err
	}
	if p.ContainerID == "" || p.RemoteUser == "" || p.RemoteWorkspace == "" {
		return nil, status.Error(codes.FailedPrecondition, "start the project before connecting its editor")
	}
	c, err := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
	if err != nil {
		return nil, err
	}
	if !c.State.Running {
		return nil, status.Error(codes.FailedPrecondition, "project is stopped; start it before connecting")
	}
	return p, nil
}

func (m *Manager) Editor(ctx context.Context, project string) (editor.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	p, err := m.editorProject(ctx, project)
	if err != nil {
		return editor.Result{}, err
	}
	args := []string{"exec", "--user", p.RemoteUser, "--workdir", p.RemoteWorkspace, p.ContainerID, "/cxz/tools/cxz"}
	if _, err = dockerx.Run(ctx, append(args, "_editor-installed")...); err != nil {
		archBytes, e := dockerx.Run(ctx, "exec", p.ContainerID, "uname", "-m")
		if e != nil {
			return editor.Result{}, e
		}
		arch := map[string]string{"x86_64": "amd64", "aarch64": "arm64"}[strings.TrimSpace(string(archBytes))]
		archive, e := m.editorArchive(ctx, arch)
		if e != nil {
			return editor.Result{}, e
		}
		defer archive.Close()
		installArgs := append([]string{"exec", "-i"}, args[1:]...)
		if e = dockerx.Input(ctx, archive, append(installArgs, "_editor-install")...); e != nil {
			return editor.Result{}, e
		}
	}
	out, err := dockerx.Run(ctx, append(args, "_editor-start", p.ID, p.RemoteWorkspace)...)
	if err != nil {
		return editor.Result{}, err
	}
	var result editor.Result
	err = json.Unmarshal(out, &result)
	if err == nil && (len(result.Token) != 64 || result.Workspace != p.RemoteWorkspace) {
		err = fmt.Errorf("invalid editor startup reply")
	}
	return result, err
}

func (m *Manager) editorArchive(ctx context.Context, arch string) (*os.File, error) {
	digest, err := editor.Digest(arch)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(m.Root, "editor-cache")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, editor.Version+"-"+arch+".tar.gz")
	lock := flock.New(path + ".lock")
	ok, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil || !ok {
		return nil, fmt.Errorf("editor download lock: %w", err)
	}
	defer lock.Unlock()
	verify := func() (*os.File, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(f, editor.MaxArchive+1))
		if err != nil || n > editor.MaxArchive || hex.EncodeToString(h.Sum(nil)) != digest {
			f.Close()
			return nil, fmt.Errorf("editor archive digest mismatch")
		}
		_, err = f.Seek(0, 0)
		if err != nil {
			f.Close()
			return nil, err
		}
		return f, nil
	}
	if f, err := verify(); err == nil {
		return f, nil
	}
	address, _ := editor.ArchiveURL(arch)
	req, _ := http.NewRequestWithContext(ctx, "GET", address, nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("editor release download: HTTP %d", response.StatusCode)
	}
	tmp, err := os.CreateTemp(dir, "download-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(response.Body, editor.MaxArchive+1))
	closeErr := tmp.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if n > editor.MaxArchive {
		return nil, fmt.Errorf("editor archive too large")
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return nil, err
	}
	return verify()
}

type editorPipe struct {
	io.Reader
	io.Writer
	cancel context.CancelFunc
	cmd    *exec.Cmd
	once   sync.Once
}

func (p *editorPipe) Close() error { p.once.Do(func() { p.cancel(); p.cmd.Wait() }); return nil }

func (m *Manager) OpenEditorTunnel(ctx context.Context, project string) (io.ReadWriteCloser, error) {
	p, err := m.editorProject(ctx, project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", "--user", p.RemoteUser, p.ContainerID, "/cxz/tools/cxz", "_editor-tunnel")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		cancel()
		return nil, err
	}
	return &editorPipe{Reader: out, Writer: in, cancel: cancel, cmd: cmd}, nil
}
