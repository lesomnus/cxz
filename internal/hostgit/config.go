// Package hostgit snapshots global Git configuration without borrowing session HOME.
package hostgit

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/dockerx"
)

const Directory = "/cxz/state/data/host-git"
const SnapshotFile = "host-git.json"
const maxSize = 1 << 20

type Bundle struct {
	ID    string
	Files map[string][]byte
}

func quote(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\t", "\\t", "\b", "\\b")
	return "\"" + r.Replace(s) + "\""
}

// Keep includeIf predicates, ordering, repeated values and valueless booleans.
// Include paths point to immutable copied files, not paths on the host.
func Snapshot(ctx context.Context) (Bundle, error) {
	b := Bundle{ID: strings.Repeat("0", 24), Files: map[string][]byte{}}
	home, err := os.UserHomeDir()
	if err != nil {
		return b, err
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	paths := []string{filepath.Join(xdg, "git", "config"), filepath.Join(home, ".gitconfig")}
	if file := os.Getenv("GIT_CONFIG_GLOBAL"); file != "" {
		paths = []string{file}
	}
	active := map[string]bool{}
	seen := map[string]string{}
	total := 0
	var snapshot func(string) (string, error)
	snapshot = func(path string) (string, error) {
		path, err = filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if active[path] {
			return "", fmt.Errorf("cyclic Git configuration include")
		}
		if name, ok := seen[path]; ok {
			return name, nil
		}
		if len(seen) >= 64 {
			return "", fmt.Errorf("Git configuration exceeds 64 included files")
		}
		raw, e := os.ReadFile(path)
		if os.IsNotExist(e) {
			return "", nil
		}
		if e != nil {
			return "", fmt.Errorf("cannot read host Git configuration")
		}
		total += len(raw)
		if total > maxSize {
			return "", fmt.Errorf("host Git configuration exceeds 1 MiB")
		}
		name := fmt.Sprintf("config-%d", len(seen))
		seen[path] = name
		active[path] = true
		defer delete(active, path)
		cmd := exec.CommandContext(ctx, "git", "config", "--file", path, "--no-includes", "--null", "--list")
		data, e := cmd.Output()
		if e != nil {
			return "", fmt.Errorf("cannot parse host Git configuration")
		}
		var out strings.Builder
		for _, entry := range bytes.Split(data, []byte{0}) {
			if len(entry) == 0 {
				continue
			}
			key, value, hasValue := strings.Cut(string(entry), "\n")
			first, last := strings.IndexByte(key, '.'), strings.LastIndexByte(key, '.')
			if first < 1 {
				return "", fmt.Errorf("invalid Git configuration key")
			}
			section, option := key[:first], key[last+1:]
			if strings.EqualFold(section, "include") && option == "path" || strings.EqualFold(section, "includeif") && option == "path" {
				target := value
				if strings.HasPrefix(target, "~/") {
					target = filepath.Join(home, target[2:])
				} else if strings.HasPrefix(target, "~") || strings.HasPrefix(target, "%(prefix)") {
					return "", fmt.Errorf("unsupported Git include path; use an absolute path or ~/")
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(path), target)
				}
				child, e := snapshot(target)
				if e != nil {
					return "", e
				}
				if child == "" {
					continue
				}
				value = Directory + "/" + b.ID + "/" + child
			}
			out.WriteString("[" + section)
			if last > first {
				out.WriteString(" " + quote(key[first+1:last]))
			}
			out.WriteString("]\n\t" + option)
			if hasValue {
				out.WriteString(" = " + quote(value))
			}
			out.WriteByte('\n')
		}
		b.Files[name] = []byte(out.String())
		return name, nil
	}
	var root strings.Builder
	for _, path := range paths {
		name, e := snapshot(path)
		if e != nil {
			return b, e
		}
		if name != "" {
			root.WriteString("[include]\n\tpath = " + quote(Directory+"/"+b.ID+"/"+name) + "\n")
		}
	}
	b.Files["config"] = []byte(root.String())
	canonical, _ := json.Marshal(b.Files)
	hash := sha256.Sum256(canonical)
	id := hex.EncodeToString(hash[:12])
	for name, data := range b.Files {
		b.Files[name] = bytes.ReplaceAll(data, []byte(Directory+"/"+b.ID+"/"), []byte(Directory+"/"+id+"/"))
	}
	b.ID = id
	return b, nil
}

func (b Bundle) archive() ([]byte, error) {
	if len(b.ID) != 24 || strings.Trim(b.ID, "0123456789abcdef") != "" {
		return nil, fmt.Errorf("invalid Git snapshot identity")
	}
	if _, ok := b.Files["config"]; !ok {
		return nil, fmt.Errorf("missing Git snapshot root")
	}
	if len(b.Files) > 65 {
		return nil, fmt.Errorf("too many Git snapshot files")
	}
	var out bytes.Buffer
	w := tar.NewWriter(&out)
	size := 0
	for name, data := range b.Files {
		if name != "config" && (!strings.HasPrefix(name, "config-") || strings.Trim(strings.TrimPrefix(name, "config-"), "0123456789") != "") {
			return nil, fmt.Errorf("invalid Git snapshot file")
		}
		size += len(data)
		if size > 4*maxSize {
			return nil, fmt.Errorf("Git snapshot too large")
		}
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Inject writes a new immutable generation before atomically publishing it.
// Existing repository/global configuration remains higher precedence than this
// managed system include. Already-running agents read it on their next Git call.
func Inject(ctx context.Context, container, user string, b Bundle) error {
	data, err := b.archive()
	if err != nil {
		return err
	}
	if user == "" {
		user = "root"
	}
	script := `set -eu
umask 077
root=/cxz/state/data/host-git
test -d /cxz/state/data
test ! -L /cxz/state/data
test ! -L "$root"
test ! -L "$root/$1"
test ! -L "$root/active.config"
mkdir -p "$root"
chmod 700 "$root"
if test ! -d "$root/$1"; then
  staging=$(mktemp -d "$root/.stage.XXXXXX")
  trap 'rm -rf "$staging"' EXIT
  tar -xf - -C "$staging"
  chown -R -- "$2" "$staging"
  mv "$staging" "$root/$1"
else
  cat >/dev/null
fi
tmp=$(mktemp "$root/.active.XXXXXX")
printf '[include]\n\tpath = "%s/%s/config"\n' "$root" "$1" > "$tmp"
chown -- "$2" "$root" "$tmp"
mv -f "$tmp" "$root/active.config"
test ! -L /etc/gitconfig
if ! grep -Fxq '# cxz-host-gitconfig' /etc/gitconfig 2>/dev/null; then
  printf '\n# cxz-host-gitconfig\n[include]\n\tpath = "%s/active.config"\n' "$root" >> /etc/gitconfig
  chmod 644 /etc/gitconfig
fi`
	if err = dockerx.Input(ctx, bytes.NewReader(data), "exec", "-i", "--user", "root", container, "sh", "-c", script, "cxz-gitconfig", b.ID, user); err != nil {
		return fmt.Errorf("could not apply host Git configuration to project")
	}
	return nil
}

func Read(r io.Reader) (Bundle, error) {
	var b Bundle
	err := json.NewDecoder(io.LimitReader(r, 8*maxSize)).Decode(&b)
	if err != nil {
		return b, err
	}
	_, err = b.archive()
	return b, err
}
