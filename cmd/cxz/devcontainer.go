//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/internal/settings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func syncCurrentDevcontainer(ctx context.Context, client api.SessionsClient, root string) (*api.Receipt, error) {
	lock, err := core.Lock(filepath.Join(root, "settings.lock"))
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	cfg, err := settings.Load(root)
	if err != nil {
		return nil, err
	}
	return syncDevcontainer(ctx, client, root, cfg)
}

func syncDevcontainer(ctx context.Context, client api.SessionsClient, root string, cfg settings.Config) (*api.Receipt, error) {
	spec, err := cfg.Devcontainer.Snapshot(root)
	if err != nil {
		return nil, err
	}
	return publishDevcontainerSpec(ctx, client, spec)
}

func publishDevcontainerSpec(ctx context.Context, client api.SessionsClient, spec projectconfig.Spec) (*api.Receipt, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := client.Devcontainer(ctx, &api.DevcontainerInput{Spec: data})
	if status.Code(err) == codes.Unimplemented {
		if len(spec.Compose) == 0 && spec.Template == nil {
			return &api.Receipt{}, nil // No override to lose on older managers.
		}
		return nil, fmt.Errorf("devcontainer overrides require an updated manager; run cxz install --recreate, then retry cxz up")
	}
	if err == nil && spec.Template != nil && (out == nil || out.Status != projectconfig.TemplateReceipt) {
		return nil, fmt.Errorf("default devcontainer templates require an updated manager; run cxz install --recreate, then retry cxz up")
	}
	return out, err
}

func publishDevcontainer(ctx context.Context, root string) (*api.Receipt, error) {
	conn, err := server.Dial(root)
	if err != nil {
		return nil, fmt.Errorf("settings saved locally; retry cxz up after connecting to the manager: %w", err)
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	defer client.Close()
	return syncCurrentDevcontainer(ctx, client, root)
}
