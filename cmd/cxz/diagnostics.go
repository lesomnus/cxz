package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"os"
	"os/exec"
	"regexp"
	"time"
)

func doctorCommand() *xli.Command {
	return &xli.Command{Name: "doctor", Brief: "Read-only installation and project health checks (JSON)", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		type check struct {
			Name   string `json:"name"`
			OK     bool   `json:"ok"`
			Detail string `json:"detail"`
		}
		checks := []check{}
		healthy := true
		add := func(name string, err error, detail string) {
			if err != nil {
				healthy = false
				detail = err.Error()
			}
			checks = append(checks, check{name, err == nil, detail})
		}
		_, err := settings.Load(stateFrom(ctx))
		add("settings", err, "valid non-secret preferences")
		_, err = dockerx.Run(ctx, "info", "--format", "{{.ServerVersion}}")
		add("docker", err, "engine reachable")
		v, err := transport.Load(stateFrom(ctx))
		add("installation", err, "locator available")
		if err == nil {
			_, err = dockerx.Owned(ctx, v.Container, v.Owner, "")
			add("manager", err, v.Container)
			if err == nil {
				conn, e := transport.Dial(stateFrom(ctx))
				add("transport", e, "Docker exec bridge")
				if e == nil {
					defer conn.Close()
					client := resourceclient.New(conn)
					projects, e := client.Projects(ctx, &api.Empty{})
					add("projects", e, "inventory reconciled")
					if e == nil {
						for _, p := range projects.Projects {
							if p.State == "foreign" {
								continue
							}
							var failure error
							if p.ProvisionState == "failed" || p.ProvisionState == "interrupted" {
								failure = fmt.Errorf("%s at %s: %s; inspect cxz logs %s", p.ProvisionState, p.ProvisionStep, p.Error, p.Id)
							}
							add("project:"+p.Id, failure, p.State+" / "+p.ProvisionStep)
						}
					}
					_, e = client.List(ctx, &api.Empty{})
					add("sessions", e, "session registry reachable; vendor authentication is not tested")
				}
			}
		}
		if err = json.NewEncoder(c.Writer).Encode(checks); err != nil {
			return err
		}
		if !healthy {
			return fmt.Errorf("health checks failed; see JSON details (no credentials were read)")
		}
		return nil
	})}
}

func logsCommand() *xli.Command {
	return &xli.Command{Name: "logs", Brief: "Show manager logs or project provisioning logs; may contain private hook output", Args: arg.Args{stringArg("PROJECT", true)}, Flags: flg.Flags{&flg.Uint64{Name: "tail", Default: ptr(uint64(100)), Brief: "Lines, 1–10000"}}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		n := flg.MustGet[uint64](c, "tail")
		if n < 1 || n > 10000 {
			return fmt.Errorf("tail must be 1–10000")
		}
		v, err := transport.Load(stateFrom(ctx))
		if err != nil {
			return fmt.Errorf("host installation required: %w", err)
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if _, err = dockerx.Owned(ctx, v.Container, v.Owner, ""); err != nil {
			return err
		}
		args := []string{"logs", "--tail", fmt.Sprint(n), v.Container}
		name := arg.MustGet[string](c, "PROJECT")
		if name != "" {
			conn, err := transport.Dial(stateFrom(ctx))
			if err != nil {
				return err
			}
			defer conn.Close()
			list, err := resourceclient.New(conn).Projects(ctx, &api.Empty{})
			if err != nil {
				return err
			}
			id := ""
			for _, p := range list.Projects {
				if p.Id == name || p.Name == name || p.Workspace == name {
					if id != "" {
						return fmt.Errorf("ambiguous project")
					}
					id = p.Id
				}
			}
			if !regexp.MustCompile(`^[a-f0-9]{24}$`).MatchString(id) {
				return fmt.Errorf("owned project not found; use cxz projects")
			}
			args = []string{"exec", v.Container, "tail", "-n", fmt.Sprint(n), "--", "/var/lib/cxz/projects/" + id + "/provision.log"}
		}
		cmd := exec.CommandContext(ctx, "docker", args...)
		// Docker logs uses stderr for the container's stderr. Keep streams intact.
		cmd.Stdout = c.Writer
		cmd.Stderr = c.ErrWriter
		cmd.Env = os.Environ()
		return cmd.Run()
	})}
}
