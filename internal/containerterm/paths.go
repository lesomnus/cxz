package containerterm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/wisp"
)

type PathEntry = wisp.Entry
type PathListing struct {
	Entries   []PathEntry
	Truncated bool
}

// PathClient streams cumulative snapshots for a project on the selected daemon.
// The daemon resolves container identity and user; callers supply only a project ID.
type PathClient interface {
	Paths(context.Context, string, string, func(PathListing)) (PathListing, error)
}

// Arguments are passed as argv, never interpolated into shell source. Only
// immediate entry names/types are read; no file contents or recursive walk.
const listPathsScript = `
set -eu
dir=$1
case "$dir" in
  '~'|'~/'*)
    home=${HOME:-}
    if command -v getent >/dev/null 2>&1; then
      found=$(getent passwd "$(id -u)" | cut -d: -f6)
      if [ -n "$found" ]; then home=$found; fi
    fi
    if [ -z "$home" ]; then exit 2; fi
    dir="$home${dir#\~}"
    ;;
  /*) ;;
  *) exit 2;;
esac
cd "$dir" 2>/dev/null || exit 3
[ -r . ] || exit 3
n=0
for entry in ./* ./.[!.]* ./..?*; do
  if [ ! -e "$entry" ] && [ ! -L "$entry" ]; then continue; fi
  if [ "$n" -ge 2048 ]; then printf 'limit\000\000\000'; break; fi
  kind=f
  target=
  if [ -x "$entry" ]; then kind=x; fi
  if [ -d "$entry" ]; then kind=d; fi
  if [ -L "$entry" ]; then
    if [ "$kind" = d ]; then kind=ld; else kind=l; fi
    target=$(readlink "$entry" 2>/dev/null || :)
  fi
  printf '%s\000%s\000%s\000' "$kind" "${entry#./}" "$target"
  n=$((n+1))
done
`

func ListPaths(ctx context.Context, p *api.Project, dir string) (PathListing, error) {
	return StreamPaths(ctx, p, dir, nil)
}

// StreamPaths emits cumulative, immutable snapshots before the lookup finishes.
func StreamPaths(ctx context.Context, p *api.Project, dir string, emit func(PathListing)) (PathListing, error) {
	if err := transport.LocalOnly(ctx, "workspace path lookup"); err != nil {
		return PathListing{}, err
	}
	if p == nil || p.Id == "" || p.ContainerId == "" || p.RemoteUser == "" {
		return PathListing{}, fmt.Errorf("project container unavailable")
	}
	if len(dir) > 4096 || strings.IndexByte(dir, 0) >= 0 || !(strings.HasPrefix(dir, "/") || dir == "~" || strings.HasPrefix(dir, "~/")) {
		return PathListing{}, fmt.Errorf("absolute or home-relative directory required")
	}
	c, err := dockerx.Inspect(ctx, p.ContainerId)
	if err != nil {
		return PathListing{}, err
	}
	if !c.State.Running || c.Config.Labels["cxz.project"] != p.Id || c.Config.Labels["cxz.owner"] == "" {
		return PathListing{}, fmt.Errorf("refusing path lookup in unowned or stopped container")
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", "--user", p.RemoteUser, c.ID, "sh", "-c", listPathsScript, "cxz-paths", dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return PathListing{}, err
	}
	if err := cmd.Start(); err != nil {
		return PathListing{}, err
	}
	out, readErr := readPaths(stdout, emit)
	if readErr != nil {
		_ = cmd.Process.Kill()
	}
	err = cmd.Wait()
	if readErr != nil {
		return out, readErr
	}
	if err != nil {
		return out, fmt.Errorf("directory unavailable or permission denied")
	}
	return out, nil
}

func parsePaths(b []byte) (PathListing, error) {
	return readPaths(strings.NewReader(string(b)), nil)
}

func readPaths(src io.Reader, emit func(PathListing)) (PathListing, error) {
	var out PathListing
	r := bufio.NewReader(src)
	for n := 0; ; n++ {
		kind, err := r.ReadString(0)
		if err == io.EOF && kind == "" {
			break
		}
		if err != nil || n > 2048 {
			return out, fmt.Errorf("invalid directory response")
		}
		name, err := r.ReadString(0)
		if err != nil {
			return out, fmt.Errorf("invalid directory response")
		}
		target, err := r.ReadString(0)
		if err != nil {
			return out, fmt.Errorf("invalid directory response")
		}
		kind, name, target = strings.TrimSuffix(kind, "\x00"), strings.TrimSuffix(name, "\x00"), strings.TrimSuffix(target, "\x00")
		if kind == "limit" {
			out.Truncated = true
			continue
		}
		if (kind != "d" && kind != "f" && kind != "x" && kind != "l" && kind != "ld") || name == "" || strings.ContainsRune(name, '/') {
			return out, fmt.Errorf("invalid directory entry")
		}
		if !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			continue
		}
		if !utf8.ValidString(target) || strings.IndexFunc(target, unicode.IsControl) >= 0 {
			target = "(unprintable target)"
		}
		out.Entries = append(out.Entries, PathEntry{Name: name, Directory: kind == "d" || kind == "ld", Executable: kind == "x", Symlink: kind == "l" || kind == "ld", LinkTarget: target})
		if emit != nil && (len(out.Entries) == 1 || len(out.Entries)%16 == 0) {
			emit(PathListing{Entries: append([]PathEntry(nil), out.Entries...), Truncated: out.Truncated})
		}
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.Directory != b.Directory {
			return a.Directory
		}
		return a.Name < b.Name
	})
	return out, nil
}
