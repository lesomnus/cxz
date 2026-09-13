package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/githubauth"
)

func (m *Manager) installGitHub(ctx context.Context, p *Project, arch string) error {
	if err := m.checkpoint(ctx, p, "github-cli"); err != nil {
		return err
	}
	bin, err := distribution.EnsureGitHub(ctx, "/cxz/tools", arch)
	if err != nil {
		return err
	}
	// Preserve an image-provided gh. The injected wrapper also works in old
	// containers whose original Docker environment predates GH_CONFIG_DIR.
	script := `set -eu
if ! command -v gh >/dev/null 2>&1; then
  mkdir -p /usr/local/bin
  printf '#!/bin/sh\n# cxz-managed-gh\nexport GH_CONFIG_DIR="${GH_CONFIG_DIR:-/cxz/state/data/gh}"\nexec /cxz/state/gh-bin "$@"\n' > /usr/local/bin/gh
  chmod 755 /usr/local/bin/gh
fi`
	if _, err = dockerx.Run(ctx, "exec", "--user", "root", p.ContainerID, "sh", "-c", script, "cxz-gh", bin); err != nil {
		return err
	}
	// A custom gh is intentionally left untouched.
	if _, e := dockerx.Run(ctx, "exec", p.ContainerID, "grep", "-q", "cxz-managed-gh", "/usr/local/bin/gh"); e == nil {
		if err = m.replaceGitHub(ctx, p, bin); err != nil {
			return err
		}
	}
	b, err := os.ReadFile(filepath.Join(m.Root, "host-gh", "hosts.yml"))
	if os.IsNotExist(err) {
		b = []byte("{}\n")
	} else if err != nil {
		return err
	}
	return githubauth.Inject(ctx, p.ContainerID, p.RemoteUser, b)
}

func (m *Manager) replaceGitHub(ctx context.Context, p *Project, bin string) error {
	if _, err := dockerx.Run(ctx, "exec", p.ContainerID, bin, "--version"); err != nil {
		return fmt.Errorf("new gh failed validation")
	}
	script := `set -eu
grep -q 'cxz-managed-gh' /usr/local/bin/gh || exit 2
test -x "$1"
tmp=/cxz/state/gh-bin-"$2"
ln -s "$1" "$tmp"
mv -Tf "$tmp" /cxz/state/gh-bin`
	_, err := dockerx.Run(ctx, "exec", "--user", "root", p.ContainerID, "sh", "-c", script, "cxz-gh", bin, core.ID())
	if err != nil {
		return fmt.Errorf("gh update skipped: custom/legacy gh wrapper or filesystem unavailable")
	}
	return nil
}
