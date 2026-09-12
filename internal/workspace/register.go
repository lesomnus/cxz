package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"os"
	"path/filepath"
	"strings"
)

// Register records workspace intent only. Docker operations belong to Up.
func (m *Manager) Register(ctx context.Context, path, config string) (*api.Project, error) {
	if p, err := m.resolve(ctx, path); err == nil {
		return projectView(p), nil
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
	rel, err := filepath.Rel(m.WorkspaceRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, fmt.Errorf("workspace outside installed root")
	}
	sum := sha256.Sum256([]byte(path))
	id := hex.EncodeToString(sum[:12])
	lock := m.projectLock(id)
	lock.Lock()
	defer lock.Unlock()
	if p, err := m.resolve(ctx, id); err == nil {
		return projectView(p), nil
	}
	if len(m.Owner) < 12 {
		return nil, fmt.Errorf("invalid manager owner")
	}
	p := &Project{ID: id, Workspace: path, Name: filepath.Base(path), Config: config, Network: "cxz-" + m.Owner[:12] + "-" + id, Volume: "cxz-" + m.Owner[:12] + "-" + id + "-state", Token: core.ID() + core.ID()}
	if err = m.save(ctx, p); err != nil {
		return nil, err
	}
	return projectView(p), nil
}
func projectView(p *Project) *api.Project {
	return &api.Project{Id: p.ID, Workspace: p.Workspace, Name: p.Name, Config: p.Config, ContainerId: p.ContainerID, RemoteUser: p.RemoteUser, RemoteWorkspace: p.RemoteWorkspace, Error: p.Error, ProvisionState: p.Job.State, ProvisionStep: p.Job.Step, ProvisionAttempt: p.Job.Attempt}
}

// CreateSession operates only on a prepared project. Provisioning is Project.Up,
// not an implicit part of Session.Add, so its checkpoint/retry count is stable.
func (m *Manager) CreateSession(ctx context.Context, r *api.CreateRequest) (*api.Session, error) {
	if r.Account == "" {
		return nil, fmt.Errorf("account required; use --account")
	}
	p, err := m.resolve(ctx, r.Workspace)
	if err != nil {
		return nil, err
	}
	lock := m.projectLock(p.ID)
	lock.Lock()
	defer lock.Unlock()
	p, err = m.resolve(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	conn, client, err := m.client(ctx, p)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	// Never replace credentials underneath a live agent, even on a create retry.
	live, err := client.List(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	active := false
	for _, v := range live.Sessions {
		if v.State == "starting" || v.State == "idle" || v.State == "working" || v.State == "waiting_input" {
			active = true
		}
	}
	if !active {
		if err = m.prepareAccount(ctx, p, r.Account, r.Agent); err != nil {
			return nil, err
		}
	}
	if err = resourceclient.New(conn).EnsureAccount(ctx, r.Account, r.Agent); err != nil {
		return nil, err
	}
	v, err := client.Create(ctx, &api.CreateRequest{Workspace: p.RemoteWorkspace, Title: r.Title, Agent: r.Agent, Model: r.Model, ClientId: r.ClientId, Account: r.Account})
	if err != nil {
		return nil, err
	}
	list, err := client.List(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	p.Sessions = list.Sessions
	for _, s := range p.Sessions {
		s.ProjectId = p.ID
	}
	v.ProjectId = p.ID
	return v, m.save(ctx, p)
}
