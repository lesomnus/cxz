package distribution

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const ClaudeVersion = "2.1.267"
const CodexVersion = "0.154.0"

func get(ctx context.Context, url, path string) error {
	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return e
	}
	r, e := (&http.Client{Timeout: 15 * time.Minute}).Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("download %s: %s", url, r.Status)
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(f, io.LimitReader(r.Body, 2<<30))
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func verify(path, want string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want) {
		return fmt.Errorf("checksum mismatch for %s", filepath.Base(path))
	}
	return nil
}

// Ensure publishes only checksum-verified immutable releases. Shared tools are
// read-only in projects; credentials never enter this cache.
func Ensure(ctx context.Context, root, kind, arch string, musl bool) (string, error) {
	version := ClaudeVersion
	if kind == "codex" {
		version = CodexVersion
	}
	return EnsureVersion(ctx, root, kind, arch, musl, SelectedVersion(root, kind, version))
}

func EnsureVersion(ctx context.Context, root, kind, arch string, musl bool, version string) (string, error) {
	if !ValidVersion(version) {
		return "", fmt.Errorf("invalid release version")
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	platform := "linux-x64"
	target := "x86_64-unknown-linux-musl"
	if arch == "arm64" || arch == "aarch64" {
		platform = "linux-arm64"
		target = "aarch64-unknown-linux-musl"
	} else if arch != "amd64" && arch != "x86_64" {
		return "", fmt.Errorf("unsupported architecture %s", arch)
	}
	if musl {
		platform += "-musl"
	}
	key := platform
	if kind == "codex" {
		key = target
	} else if kind != "claude" {
		return "", fmt.Errorf("unknown agent %s", kind)
	}
	dir := filepath.Join(root, kind, version, key)
	bin := filepath.Join(dir, "claude")
	if kind == "codex" {
		bin = filepath.Join(dir, "bin", "codex")
	}
	if _, e := os.Stat(bin); e == nil {
		return bin, nil
	}
	if e := os.MkdirAll(filepath.Dir(dir), 0755); e != nil {
		return "", e
	}
	lock, e := core.Lock(dir + ".lock")
	if e != nil {
		return "", e
	}
	defer lock.Close()
	if _, e = os.Stat(bin); e == nil {
		return bin, nil
	}
	tmp, e := os.MkdirTemp(filepath.Dir(dir), ".download-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(tmp)
	if kind == "claude" {
		base := "https://downloads.claude.ai/claude-code-releases/" + version
		manifest := filepath.Join(tmp, "manifest.json")
		if e = get(ctx, base+"/manifest.json", manifest); e != nil {
			return "", e
		}
		b, e := os.ReadFile(manifest)
		if e != nil {
			return "", e
		}
		var m struct {
			Platforms map[string]struct {
				Checksum string `json:"checksum"`
			} `json:"platforms"`
		}
		if e = json.Unmarshal(b, &m); e != nil {
			return "", e
		}
		sum := m.Platforms[platform].Checksum
		if sum == "" {
			return "", fmt.Errorf("platform missing from Claude manifest")
		}
		p := filepath.Join(tmp, "claude")
		if e = get(ctx, base+"/"+platform+"/claude", p); e != nil {
			return "", e
		}
		if e = verify(p, sum); e != nil {
			return "", e
		}
		if e = os.Chmod(p, 0755); e != nil {
			return "", e
		}
	} else {
		base := "https://github.com/openai/codex/releases/download/rust-v" + version
		name := "codex-package-" + target + ".tar.gz"
		sums := filepath.Join(tmp, "SHA256SUMS")
		if e = get(ctx, base+"/codex-package_SHA256SUMS", sums); e != nil {
			return "", e
		}
		b, e := os.ReadFile(sums)
		if e != nil {
			return "", e
		}
		sum := ""
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
				sum = f[0]
			}
		}
		if sum == "" {
			return "", fmt.Errorf("Codex package checksum missing")
		}
		archive := filepath.Join(tmp, "package.tgz")
		if e = get(ctx, base+"/"+name, archive); e != nil {
			return "", e
		}
		if e = verify(archive, sum); e != nil {
			return "", e
		}
		if e = unpack(archive, tmp); e != nil {
			return "", e
		}
		if _, e = os.Stat(filepath.Join(tmp, "bin", "codex")); e != nil {
			return "", fmt.Errorf("unsupported Codex package layout: %w", e)
		}
		os.Remove(archive)
	}
	if e = os.Chmod(tmp, 0755); e != nil {
		return "", e
	}
	if e = os.Rename(tmp, dir); e != nil {
		return "", e
	}
	return bin, core.SyncDir(filepath.Dir(dir))
}
func unpack(path, dest string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, e := tr.Next()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		name := filepath.Clean(h.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("unsafe archive path")
		}
		p := filepath.Join(dest, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if e = os.MkdirAll(p, 0755); e != nil {
				return e
			}
		case tar.TypeReg:
			if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
				return e
			}
			out, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode)&0755)
			if e != nil {
				return e
			}
			_, e = io.CopyN(out, tr, h.Size)
			out.Close()
			if e != nil {
				return e
			}
		default:
			return fmt.Errorf("unsupported archive entry %s", h.Name)
		}
	}
}
