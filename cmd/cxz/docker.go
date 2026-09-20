package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/xli"
	"time"
)

func dockerCommand() *xli.Command {
	c := commandGroup("docker", "Manage the shared Docker engine for project containers")
	for _, op := range []string{"up", "down", "status", "sync"} {
		c.Commands = append(c.Commands, &xli.Command{Name: op, Brief: map[string]string{"up": "Start/apply engine configuration (changes restart Docker workloads)", "down": "Remove engine, retaining its images and volumes", "status": "Show managed engine endpoint", "sync": "Save engine configuration without restarting it"}[op], Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			action := c.Name
			if action == "sync" {
				action = "save"
			}
			out, err := publishDocker(ctx, stateFrom(ctx), action)
			if err != nil {
				return err
			}
			return writeOutput(c, out)
		})})
	}
	return c
}
func publishDocker(ctx context.Context, root, action string) (*api.Receipt, error) {
	r := &api.DockerInput{Action: action}
	if action == "up" || action == "save" {
		cfg, err := settings.Load(root)
		if err != nil {
			return nil, err
		}
		spec, err := cfg.Docker.Snapshot(root)
		if err != nil {
			return nil, err
		}
		r.Spec, err = json.Marshal(spec)
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	conn, err := server.Dial(root)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	out, err := resourceclient.New(conn).Docker(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("managed Docker: %w (saved local settings can be retried with cxz docker sync/up)", err)
	}
	return out, nil
}
