package workspace

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/dockerx"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/logview"
)

func (m *Manager) Logs(ctx context.Context, r *api.LogsRequest) (*api.LogsReply, error) {
	all, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		found := false
		for _, s := range p.Sessions {
			if s.Id == r.SessionId {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		var report logview.Report
		if r.Project {
			report.File("project provisioning", filepath.Join(m.Root, "projects", p.ID, "provision.log"))
			output := &logview.Buffer{}
			work, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, inspectErr := dockerx.Owned(work, p.ContainerID, m.Owner, p.ID)
			if inspectErr == nil {
				cmd := exec.CommandContext(work, "docker", "logs", "--timestamps", "--tail", "200", p.ContainerID)
				cmd.Stdout = output
				cmd.Stderr = output
				inspectErr = cmd.Run()
			}
			cancel()
			if inspectErr != nil {
				fmt.Fprintf(output, "\nContainer logs unavailable: %v", inspectErr)
			}
			report.Add("project container stdout/stderr · latest 200 lines", output.String())
		}
		conn, client, err := m.client(ctx, p)
		if err == nil {
			defer conn.Close()
			var result *api.LogsReply
			result, err = client.Logs(ctx, r)
			if err == nil {
				report.Add("runtime report", result.Text)
			}
		}
		if err != nil {
			report.Add("runtime logs unavailable", err.Error())
		}
		return &api.LogsReply{Text: report.String()}, nil
	}
	return nil, fmt.Errorf("session not found: %s", r.SessionId)
}
