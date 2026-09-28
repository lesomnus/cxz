package containerterm

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
)

type DownloadClient interface {
	Download(context.Context, string, string, io.Writer) error
}

func ValidDownloadPath(path string) bool {
	return path != "" && len(path) <= 4096 && !strings.ContainsAny(path, "\x00\r\n")
}

const downloadScript = `
set -eu
file=$1
case "$file" in
 '~'|'~/'*)
  home=${HOME:-}
  if command -v getent >/dev/null 2>&1; then
   found=$(getent passwd "$(id -u)" | cut -d: -f6)
   if [ -n "$found" ]; then home=$found; fi
  fi
  [ -n "$home" ] || exit 2
  file="$home${file#\~}"
  ;;
esac
[ -f "$file" ] && [ -r "$file" ] || exit 3
exec cat -- "$file"
`

// No PTY: stdout is the original byte stream, and the path is a separate argv.
func DownloadFile(ctx context.Context, p *api.Project, path string, dst io.Writer) error {
	if err := transport.LocalOnly(ctx, "container file download"); err != nil {
		return err
	}
	if !ValidDownloadPath(path) {
		return fmt.Errorf("a file path is required")
	}
	if p == nil || p.Id == "" || p.ContainerId == "" || p.RemoteUser == "" || p.RemoteWorkspace == "" {
		return fmt.Errorf("project container unavailable")
	}
	c, err := dockerx.Inspect(ctx, p.ContainerId)
	if err != nil {
		return err
	}
	if !c.State.Running || c.Config.Labels["cxz.project"] != p.Id || c.Config.Labels["cxz.owner"] == "" {
		return fmt.Errorf("refusing download from unowned or stopped container")
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", "--user", p.RemoteUser, "--workdir", p.RemoteWorkspace, c.ID, "sh", "-c", downloadScript, "cxz-download", path)
	cmd.Stdout = dst
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("download failed: source must be a readable regular file: %w", err)
	}
	return nil
}
