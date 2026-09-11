package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/workspace"
	"os"
	"path/filepath"
)

func (s *Server) RegisterProject(ctx context.Context, path, config string) (*api.Project, error) {
	if s.manager != nil {
		return s.manager.Register(ctx, path, config)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("workspace must be a directory")
	}
	id := ""
	if os.Getenv("CXZ_PROJECT_ID") != "" {
		r, err := workspace.LoadRuntime(s.root)
		if err != nil {
			return nil, err
		}
		if path != r.Workspace {
			return nil, fmt.Errorf("workspace outside project scope")
		}
		id = r.ProjectID
	}
	if id == "" {
		h := sha256.Sum256([]byte(path))
		id = hex.EncodeToString(h[:12])
	}
	return &api.Project{Id: id, Workspace: path, Name: filepath.Base(path), Config: config, State: "running", RemoteWorkspace: path}, nil
}

func (s *Server) ResourceSnapshot(ctx context.Context) (*api.ProjectList, *api.SessionList, error) {
	list, err := s.List(ctx, &api.Empty{})
	if err != nil {
		return nil, nil, err
	}
	if s.manager != nil {
		p, err := s.Projects(ctx, &api.Empty{})
		return p, list, err
	}
	projects := &api.ProjectList{}
	seen := map[string]bool{}
	for _, session := range list.Sessions {
		p, err := s.RegisterProject(ctx, session.Workspace, "")
		if err != nil {
			return nil, nil, err
		}
		if !seen[p.Id] {
			projects.Projects = append(projects.Projects, p)
			seen[p.Id] = true
		}
		session.ProjectId = p.Id
	}
	if os.Getenv("CXZ_PROJECT_ID") != "" && len(projects.Projects) == 0 {
		r, err := workspace.LoadRuntime(s.root)
		if err != nil {
			return nil, nil, err
		}
		p, err := s.RegisterProject(ctx, r.Workspace, "")
		if err != nil {
			return nil, nil, err
		}
		projects.Projects = append(projects.Projects, p)
	}
	return projects, list, nil
}
