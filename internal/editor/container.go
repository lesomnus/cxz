package editor

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

func directory() (string, error) {
	account, err := user.Current()
	if err != nil {
		return "", err
	}
	home := account.HomeDir
	dir := filepath.Join(home, ".cache", "cxz-editor")
	err = os.MkdirAll(dir, 0700)
	return dir, err
}

func Installed() bool {
	dir, err := directory()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, Version, "bin", "openvscode-server"))
	return err == nil
}

// Install reads the exact release archive supplied by the Manager. Extraction
// is staged, bounded and restricted to regular files/directories/safe symlinks.
func Install(ctx context.Context, src io.Reader) error {
	dir, err := directory()
	if err != nil {
		return err
	}
	lock := flock.New(filepath.Join(dir, "install.lock"))
	ok, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil || !ok {
		return fmt.Errorf("editor install lock: %w", err)
	}
	defer lock.Unlock()
	if Installed() {
		_, err = io.Copy(io.Discard, src)
		return err
	}
	archive, err := os.CreateTemp(dir, "archive-")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(src, MaxArchive+1))
	if err != nil {
		return err
	}
	if n > MaxArchive {
		return fmt.Errorf("editor archive too large")
	}
	digest, err := Digest(runtime.GOARCH)
	if err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != digest {
		return fmt.Errorf("editor release digest mismatch")
	}
	if _, err = archive.Seek(0, 0); err != nil {
		return err
	}
	zipped, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	defer zipped.Close()
	stage, err := os.MkdirTemp(dir, "install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	reader := tar.NewReader(zipped)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		parts := strings.SplitN(h.Name, "/", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}
		rel := filepath.Clean(parts[1])
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("invalid archive path")
		}
		dest := filepath.Join(stage, rel)
		// A symlink may never become a parent of a later archive entry.
		for p := filepath.Dir(dest); p != stage; p = filepath.Dir(p) {
			if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("archive symlink parent")
			}
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(dest, 0755)
		case tar.TypeReg, tar.TypeRegA:
			total += h.Size
			if h.Size < 0 || total > 512<<20 {
				return fmt.Errorf("expanded editor too large")
			}
			if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode)&0755)
			if e != nil {
				return e
			}
			_, err = io.Copy(f, reader)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(h.Linkname) || !filepath.IsLocal(filepath.Join(filepath.Dir(rel), h.Linkname)) {
				return fmt.Errorf("invalid archive symlink")
			}
			if err = os.MkdirAll(filepath.Dir(dest), 0755); err == nil {
				err = os.Symlink(h.Linkname, dest)
			}
		default:
			return fmt.Errorf("unsupported editor archive entry")
		}
		if err != nil {
			return err
		}
	}
	if _, err = os.Stat(filepath.Join(stage, "bin", "openvscode-server")); err != nil {
		return err
	}
	return os.Rename(stage, filepath.Join(dir, Version))
}

func Start(ctx context.Context, project, workspace string, out io.Writer) error {
	base, err := BasePath(project)
	if err != nil {
		return err
	}
	dir, err := directory()
	if err != nil {
		return err
	}
	lock := flock.New(filepath.Join(dir, "start.lock"))
	ok, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil || !ok {
		return fmt.Errorf("editor start lock: %w", err)
	}
	defer lock.Unlock()
	tokenPath := filepath.Join(dir, "token")
	token, err := os.ReadFile(tokenPath)
	if os.IsNotExist(err) {
		raw := make([]byte, 32)
		if _, err = rand.Read(raw); err != nil {
			return err
		}
		token = []byte(hex.EncodeToString(raw))
		err = os.WriteFile(tokenPath, token, 0600)
	}
	if err != nil {
		return err
	}
	ready := func() bool {
		check, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(check, "GET", "http://"+Address+base+"/", nil)
		req.AddCookie(&http.Cookie{Name: "vscode-tkn", Value: string(token)})
		res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
		if err != nil {
			return false
		}
		defer res.Body.Close()
		io.Copy(io.Discard, res.Body)
		return res.StatusCode == http.StatusOK
	}
	if !ready() {
		if !Installed() {
			return fmt.Errorf("browser editor release is not installed")
		}
		log, err := os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer log.Close()
		cmd := exec.Command(filepath.Join(dir, Version, "bin", "openvscode-server"), "--host", "127.0.0.1", "--port", "7351", "--server-base-path", base, "--connection-token-file", tokenPath, "--disable-telemetry", "--user-data-dir", filepath.Join(dir, "user-data"), "--server-data-dir", filepath.Join(dir, "server-data"))
		cmd.Dir = workspace
		cmd.Stdout = log
		cmd.Stderr = log
		if err = cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		for !ready() {
			select {
			case <-ctx.Done():
				return fmt.Errorf("editor startup failed (requires glibc Linux); inspect %s: %w", log.Name(), ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return json.NewEncoder(out).Encode(Result{Workspace: workspace, Token: string(token)})
}
