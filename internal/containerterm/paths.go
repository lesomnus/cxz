package containerterm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
)

type PathEntry struct {
	Name      string
	Directory bool
}
type PathListing struct {
	Entries   []PathEntry
	Truncated bool
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
  if [ "$n" -ge 2048 ]; then printf 'limit\000\000'; break; fi
  kind=f
  if [ -d "$entry" ]; then kind=d; fi
  printf '%s\000%s\000' "$kind" "${entry#./}"
  n=$((n+1))
done
`

func ListPaths(ctx context.Context, p *api.Project, dir string) (PathListing, error) {
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
	b, err := dockerx.Run(ctx, "exec", "--user", p.RemoteUser, c.ID, "sh", "-c", listPathsScript, "cxz-paths", dir)
	if err != nil {
		return PathListing{}, fmt.Errorf("directory unavailable or permission denied")
	}
	return parsePaths(b)
}

func parsePaths(b []byte) (PathListing, error) {
	var out PathListing
	parts := strings.Split(string(b), "\x00")
	if len(parts)%2 != 1 || parts[len(parts)-1] != "" {
		return out, fmt.Errorf("invalid directory response")
	}
	for i := 0; i+1 < len(parts); i += 2 {
		if parts[i] == "limit" {
			out.Truncated = true
			continue
		}
		name := parts[i+1]
		if (parts[i] != "d" && parts[i] != "f") || name == "" || strings.ContainsRune(name, '/') {
			return out, fmt.Errorf("invalid directory entry")
		}
		if !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			continue
		}
		out.Entries = append(out.Entries, PathEntry{Name: name, Directory: parts[i] == "d"})
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
