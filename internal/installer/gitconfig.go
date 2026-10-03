package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/hostgit"
	"github.com/lesomnus/cxz/internal/transport"
)

// SyncGitConfig publishes host settings, then applies them without restarting agents.
func SyncGitConfig(ctx context.Context, root string, out io.Writer, projects ...*api.Project) error {
	v, err := transport.Load(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	b, err := hostgit.Snapshot(ctx)
	if err != nil {
		return err
	}
	c, err := dockerx.Inspect(ctx, v.Container)
	if err != nil {
		return err
	}
	if c.Config.Labels["cxz.owner"] != v.Owner || c.Config.Labels["cxz.role"] != "daemon" {
		return fmt.Errorf("refusing Git config sync to unowned manager")
	}
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	script := `set -eu
umask 077
tmp=$(mktemp /var/lib/cxz/.host-git.XXXXXX)
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
mv -f "$tmp" /var/lib/cxz/host-git.json`
	if err = dockerx.Input(ctx, bytes.NewReader(data), "exec", "-i", v.Container, "sh", "-c", script); err != nil {
		return fmt.Errorf("could not sync host Git configuration to manager")
	}
	var failures []error
	for _, p := range projects {
		if _, err = dockerx.Owned(ctx, p.ContainerId, v.Owner, p.Id); err == nil {
			err = hostgit.Inject(ctx, p.ContainerId, p.RemoteUser, b)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("project %s: %w", p.Id, err))
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	if out != nil {
		fmt.Fprintln(out, "cxz: host Git configuration synced (project provisioning applies it)")
	}
	return nil
}

func SyncHostConfig(ctx context.Context, root string, out io.Writer, projects ...*api.Project) error {
	if err := SyncGitHub(ctx, root, out, projects...); err != nil {
		return err
	}
	return SyncGitConfig(ctx, root, out, projects...)
}
