//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/internal/settings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func syncDevcontainer(ctx context.Context, client api.SessionsClient, root string, cfg settings.Config) (*api.Receipt, error) {
	spec, err := cfg.Devcontainer.Snapshot(root)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := client.Devcontainer(ctx, &api.DevcontainerInput{Spec: data})
	if status.Code(err) == codes.Unimplemented {
		if len(spec.Compose) == 0 {
			return &api.Receipt{}, nil // No override to lose on older managers.
		}
		return nil, fmt.Errorf("devcontainer overrides require an updated manager; run cxz install --recreate, then retry cxz up")
	}
	return out, err
}

func publishDevcontainer(ctx context.Context, root string, cfg settings.Config) (*api.Receipt, error) {
	conn, err := server.Dial(root)
	if err != nil {
		return nil, fmt.Errorf("settings saved locally; retry cxz up after connecting to the manager: %w", err)
	}
	defer conn.Close()
	return syncDevcontainer(ctx, resourceclient.New(conn), root, cfg)
}
