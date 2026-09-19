package server

import (
	"context"
	"path/filepath"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/logview"
)

func (s *Server) Logs(ctx context.Context, r *api.LogsRequest) (*api.LogsReply, error) {
	if s.manager != nil {
		return s.manager.Logs(ctx, r)
	}
	selected, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	sessions := []core.Session{selected}
	var report logview.Report
	if r.Project {
		report.File("project runtime", filepath.Join(s.root, "runtime.log"))
		all, err := s.list(ctx)
		if err != nil {
			return nil, err
		}
		for _, v := range all {
			if v.ID != selected.ID && v.ProjectID == selected.ProjectID && (v.ProjectID != "" || v.Workspace == selected.Workspace) {
				sessions = append(sessions, v)
			}
		}
	}
	for _, v := range sessions {
		if err := ctx.Err(); err != nil {
			report.Add("collection", err.Error())
			break
		}
		if report.Len() >= logview.Limit {
			break
		}
		dir := core.Dir(s.root, v.ID)
		name := "session " + v.ID
		report.Journal(name+" · diagnostics", filepath.Join(dir, "events.jsonl"))
		report.File(name+" · supervisor", filepath.Join(dir, "supervisor.log"))
		report.File(name+" · agent stderr", filepath.Join(dir, "agent.stderr.log"))
	}
	return &api.LogsReply{Text: report.String()}, nil
}
