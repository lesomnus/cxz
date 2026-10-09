//go:build !windows

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
)

func dockerCommand() *xli.Command {
	c := commandGroup("docker", "Manage the shared Docker engine for project containers")
	for _, op := range []string{"up", "down", "status", "sync"} {
		c.Commands = append(c.Commands, &xli.Command{Name: op, Brief: map[string]string{"up": "Start/apply engine configuration (changes restart Docker workloads)", "down": "Remove engine, retaining its images and volumes", "status": "Show managed engine endpoint", "sync": "Save engine configuration without restarting it"}[op], Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			out, err := publishDocker(ctx, stateFrom(ctx), c.Name)
			if err != nil {
				return err
			}
			return writeOutput(c, out)
		})})
	}
	return c
}

// Each subcommand is its own call now. sync and up are the two that publish
// local settings, which is why they are the two that read them: the rest had a
// spec field they could not mean anything by.
func publishDocker(ctx context.Context, root, op string) (*api.EngineReply, error) {
	var spec *api.EngineSpec
	if op == "up" || op == "sync" {
		cfg, err := settings.Load(root)
		if err != nil {
			return nil, err
		}
		v, err := cfg.Docker.Snapshot(root)
		if err != nil {
			return nil, err
		}
		spec = &api.EngineSpec{Mode: v.Mode, Image: v.Image, Override: v.Override}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	conn, err := server.Dial(root)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	var out *api.EngineReply
	switch op {
	case "sync":
		out, err = client.SaveEngine(ctx, &api.SaveEngineInput{Spec: spec})
	case "up":
		out, err = client.StartEngine(ctx, &api.StartEngineInput{Spec: spec})
	case "down":
		out, err = client.StopEngine(ctx, &api.Empty{})
	case "status":
		out, err = client.EngineStatus(ctx, &api.Empty{})
	default:
		return nil, fmt.Errorf("unknown docker command: %s", op)
	}
	if err != nil {
		return nil, fmt.Errorf("managed Docker: %w (saved local settings can be retried with cxz docker sync/up)", err)
	}
	return out, nil
}
