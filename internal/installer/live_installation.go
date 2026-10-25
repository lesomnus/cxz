package installer

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
)

// Host metadata records the last host-side install. Channel rollouts replace
// the manager without writing into that user's directory, so its live image is
// authoritative for sibling services and their drift reports.
func liveInstallation(ctx context.Context, root string) (transport.Installation, error) {
	v, err := transport.Load(root)
	if err != nil {
		return v, err
	}
	manager, err := dockerx.Owned(ctx, v.Container, v.Owner, "")
	if err != nil {
		return v, err
	}
	if manager.Config.Labels["cxz.role"] != "daemon" || manager.Config.Image == "" {
		return v, fmt.Errorf("invalid manager identity")
	}
	v.Image = manager.Config.Image
	return v, nil
}
