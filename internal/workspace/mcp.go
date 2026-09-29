package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"strings"
)

func (m *Manager) syncMCP(ctx context.Context, client api.SessionsClient, project string) error {
	c, e := mcpconfig.Load(m.Root)
	if e != nil {
		return e
	}
	b, _ := json.Marshal(c.Resolve(project))
	_, e = client.Docker(ctx, &api.DockerInput{Action: "mcp-sync", Spec: b})
	return e
}
func (m *Manager) mcp(ctx context.Context, spec []byte) (*api.Receipt, error) {
	var r mcpconfig.Request
	if e := json.Unmarshal(spec, &r); e != nil {
		return nil, e
	}
	if r.Project != "" {
		p, e := m.resolve(ctx, r.Project)
		if e != nil {
			return nil, e
		}
		r.Project = p.ID
	}
	if r.Action == "logs" || r.Action == "restart" {
		if r.Project == "" {
			return nil, fmt.Errorf("MCP logs/restart requires a project and session")
		}
		p, e := m.resolve(ctx, r.Project)
		if e != nil {
			return nil, e
		}
		conn, client, e := m.client(ctx, p)
		if e != nil {
			return nil, e
		}
		defer conn.Close()
		b, _ := json.Marshal(r)
		return client.Docker(ctx, &api.DockerInput{Action: "mcp-control", Spec: b})
	}
	out, e := mcpconfig.Apply(m.Root, r)
	if e != nil {
		return nil, e
	}
	if r.Action != "list" && r.Action != "" {
		projects, e := m.all(ctx)
		if e != nil {
			return nil, e
		}
		var failed []string
		for _, p := range projects {
			if r.Project != "" && r.Project != p.ID || p.ContainerID == "" {
				continue
			}
			v, e := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
			if e == nil && !v.State.Running {
				continue
			}
			if e == nil {
				conn, client, err := m.client(ctx, p)
				e = err
				if e == nil {
					e = m.syncMCP(ctx, client, p.ID)
					conn.Close()
				}
			}
			if e != nil {
				failed = append(failed, p.Name)
			}
		}
		if len(failed) > 0 {
			out.Message += " Runtime sync pending (retry the setting change or launch through the manager): " + strings.Join(failed, ", ")
		}
	}
	if r.Project != "" {
		p, err := m.resolve(ctx, r.Project)
		if err == nil && p.ContainerID != "" {
			conn, client, err := m.client(ctx, p)
			if err == nil {
				reply, err := client.Docker(ctx, &api.DockerInput{Action: "mcp-state"})
				conn.Close()
				if err == nil {
					_ = json.Unmarshal([]byte(reply.Status), &out.Sessions)
					cfg, e := mcpconfig.Load(m.Root)
					if e != nil {
						return nil, e
					}
					desired := cfg.Resolve(r.Project).Digest()
					for i := range out.Sessions {
						out.Sessions[i].Pending = out.Sessions[i].LaunchDigest != desired
					}
				} else {
					out.Message += " Runtime status unavailable."
				}
			}
		}
	}
	b, e := json.Marshal(out)
	return &api.Receipt{Status: string(b)}, e
}
