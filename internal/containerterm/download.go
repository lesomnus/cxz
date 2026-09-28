package containerterm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
)

type DownloadClient interface {
	Download(context.Context, string, string, io.Writer) error
}

// DownloadSizeWriter receives the source size before any file bytes.
type DownloadSizeWriter interface{ SetDownloadSize(int64) error }

func ReportDownloadSize(dst io.Writer, size int64) error {
	if w, ok := dst.(DownloadSizeWriter); ok {
		return w.SetDownloadSize(size)
	}
	return nil
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
stat -Lc %s -- "$file"
exec cat -- "$file"
`

// No PTY: stdout carries a size header followed by unmodified file bytes.
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "exec", "--user", p.RemoteUser, "--workdir", p.RemoteWorkspace, c.ID, "sh", "-c", downloadScript, "cxz-download", path)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	err = receiveDownload(pipe, dst)
	if err != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if err != nil {
		return err
	}
	if waitErr != nil {
		return fmt.Errorf("download failed: %w", waitErr)
	}
	return nil
}

func receiveDownload(src io.Reader, dst io.Writer) error {
	reader := bufio.NewReaderSize(src, 128)
	header, err := reader.ReadSlice('\n')
	if err != nil {
		return fmt.Errorf("download failed: source must be a readable regular file: %w", err)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(header)), 10, 64)
	if err != nil || size < 0 {
		return fmt.Errorf("invalid download size")
	}
	if err = ReportDownloadSize(dst, size); err != nil {
		return err
	}
	n, err := io.Copy(dst, reader)
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("source size changed during download: expected %d bytes, received %d", size, n)
	}
	return nil
}
