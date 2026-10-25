package cxzupdate

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/dockerx"
)

// ContainerHealth uses the private maintenance socket as the container user.
// Docker ownership is checked by callers against their installation/project.
func ContainerHealth(ctx context.Context, container, user, root, action, tx string) (Health, error) {
	var h Health
	args := []string{"exec"}
	if user != "" {
		args = append(args, "--user", user)
	}
	args = append(args, container, "/cxz/tools/cxz", "_maintenance", root, action, tx)
	b, e := dockerx.Run(ctx, args...)
	if e != nil {
		return h, e
	}
	e = json.Unmarshal(b, &h)
	return h, e
}
func dockerSocket() (string, error) { return dockerx.Socket() }

// CloneManager preserves the manager's configuration while switching its image.
func CloneManager(ctx context.Context, oldID, name, owner, image string) (string, error) {
	return dockerx.CloneOwned(ctx, oldID, name, owner, "daemon", image)
}
