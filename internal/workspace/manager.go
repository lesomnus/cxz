package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/transport"
	"google.golang.org/grpc"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Project struct {
	ID, Workspace, Name, Config, ContainerID, Network, Volume, Token, RemoteWorkspace, RemoteUser, Error string
	Trusted                                                                                              bool
	Sessions                                                                                             []*api.Session
}
type Runtime struct{ ProjectID, Workspace, Token, Claude, Codex string }
type Manager struct {
	mu                                                        sync.Mutex
	DB                                                        *sql.DB
	Root, Owner, WorkspaceRoot, ToolsVolume, Image, Container string
}

func New(db *sql.DB, root string) (*Manager, error) {
	m := &Manager{DB: db, Root: root, Owner: os.Getenv("CXZ_OWNER"), WorkspaceRoot: os.Getenv("CXZ_WORKSPACE_ROOT"), ToolsVolume: os.Getenv("CXZ_TOOLS_VOLUME"), Image: os.Getenv("CXZ_MANAGER_IMAGE"), Container: os.Getenv("CXZ_MANAGER_CONTAINER")}
	_, e := db.Exec(`CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,data BLOB NOT NULL)`)
	return m, e
}
func (m *Manager) save(ctx context.Context, p *Project) error {
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Join(m.Root, "projects", p.ID), 0700); e != nil {
		return e
	}
	if e = core.WriteJSON(filepath.Join(m.Root, "projects", p.ID, "project.json"), p); e != nil {
		return e
	}
	_, e = m.DB.ExecContext(ctx, "INSERT INTO projects VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", p.ID, b)
	return e
}
func (m *Manager) all(ctx context.Context) ([]*Project, error) {
	rows, e := m.DB.QueryContext(ctx, "SELECT data FROM projects ORDER BY rowid DESC")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var p Project
		if e = json.Unmarshal(b, &p); e != nil {
			return nil, e
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}
func (m *Manager) resolve(ctx context.Context, path string) (*Project, error) {
	all, e := m.all(ctx)
	if e != nil {
		return nil, e
	}
	for _, p := range all {
		if p.ID == path || p.Workspace == path || p.Name == path {
			return p, nil
		}
	}
	return nil, fmt.Errorf("project not found: %s", path)
}
func (m *Manager) client(ctx context.Context, p *Project) (*grpc.ClientConn, api.SessionsClient, error) {
	v, e := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
	if e != nil {
		return nil, nil, e
	}
	if !v.State.Running {
		return nil, nil, fmt.Errorf("project is stopped; run cxz up")
	}
	ip := v.NetworkSettings.Networks[p.Network].IPAddress
	if ip == "" {
		return nil, nil, fmt.Errorf("project network is not attached")
	}
	conn, e := transport.Remote(ip+":7348", p.Token)
	if e != nil {
		return nil, nil, e
	}
	return conn, api.NewSessionsClient(conn), nil
}
func (m *Manager) ClientFor(ctx context.Context, id string) (*grpc.ClientConn, api.SessionsClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all, e := m.all(ctx)
	if e != nil {
		return nil, nil, e
	}
	for _, p := range all {
		for _, s := range p.Sessions {
			if s.Id == id {
				return m.client(ctx, p)
			}
		}
	}
	return nil, nil, fmt.Errorf("session not found: %s", id)
}
func (m *Manager) Sessions(ctx context.Context) (*api.SessionList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all, e := m.all(ctx)
	if e != nil {
		return nil, e
	}
	out := &api.SessionList{}
	for _, p := range all {
		c, client, e := m.client(ctx, p)
		var list *api.SessionList
		if e == nil {
			q, cancel := context.WithTimeout(ctx, 2*time.Second)
			list, e = client.List(q, &api.Empty{})
			cancel()
			c.Close()
		}
		if e == nil {
			for _, s := range list.Sessions {
				s.ProjectId = p.ID
			}
			before, _ := json.Marshal(p.Sessions)
			after, _ := json.Marshal(list.Sessions)
			p.Sessions = list.Sessions
			if string(before) != string(after) {
				if e = m.save(ctx, p); e != nil {
					return nil, e
				}
			}
		} else {
			for _, s := range p.Sessions {
				if s.State != "stopped" && s.State != "failed" {
					s.State = "interrupted"
				}
				s.Pending = nil
			}
		}
		out.Sessions = append(out.Sessions, p.Sessions...)
	}
	return out, nil
}
func (m *Manager) Get(ctx context.Context, id string) (*api.Session, error) {
	list, e := m.Sessions(ctx)
	if e != nil {
		return nil, e
	}
	for _, s := range list.Sessions {
		if s.Id == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("session not found: %s", id)
}
func (m *Manager) Projects(ctx context.Context) (*api.ProjectList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all, e := m.all(ctx)
	if e != nil {
		return nil, e
	}
	out := &api.ProjectList{}
	seen := map[string]bool{}
	for _, p := range all {
		state := "absent"
		if v, e := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID); e == nil {
			state = "stopped"
			if v.State.Running {
				state = "running"
			}
		}
		seen[p.ContainerID] = true
		out.Projects = append(out.Projects, &api.Project{Id: p.ID, Workspace: p.Workspace, Name: p.Name, Config: p.Config, ContainerId: p.ContainerID, State: state, Error: p.Error})
	}
	foreign, e := dockerx.List(ctx, "label=devcontainer.local_folder")
	if e != nil {
		return nil, e
	}
	for _, v := range foreign {
		if !seen[v.ID] && v.Config.Labels["cxz.ignore"] != "true" {
			out.Projects = append(out.Projects, &api.Project{Workspace: v.Config.Labels["devcontainer.local_folder"], ContainerId: v.ID, Name: strings.TrimPrefix(v.Name, "/"), State: "foreign"})
		}
	}
	return out, nil
}
func (m *Manager) Open(ctx context.Context, r *api.ProjectRequest) (*api.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := r.Workspace
	if path == "" {
		return nil, fmt.Errorf("workspace required")
	}
	existing, e := m.resolve(ctx, path)
	if e == nil {
		path = existing.Workspace
	} else {
		path, e = filepath.Abs(path)
		if e != nil {
			return nil, e
		}
		path, e = filepath.EvalSymlinks(path)
		if e != nil {
			return nil, e
		}
		rel, e := filepath.Rel(m.WorkspaceRoot, path)
		if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return nil, fmt.Errorf("workspace is outside installed workspace root %s", m.WorkspaceRoot)
		}
		existing, _ = m.resolve(ctx, path)
	}
	p := existing
	if p == nil {
		h := sha256.Sum256([]byte(path))
		id := hex.EncodeToString(h[:12])
		p = &Project{ID: id, Workspace: path, Name: filepath.Base(path), Network: "cxz-" + m.Owner[:12] + "-" + id, Volume: "cxz-" + m.Owner[:12] + "-" + id + "-state", Token: core.ID() + core.ID()}
	}
	if r.TrustConfig {
		p.Trusted = true
	}
	if r.Config != "" {
		p.Config = r.Config
	}
	if r.Agent != "" && r.Agent != "claude" && r.Agent != "codex" {
		return nil, fmt.Errorf("agent must be claude or codex")
	}
	containers, e := dockerx.List(ctx, "label=devcontainer.local_folder="+path)
	if e != nil {
		return nil, e
	}
	for _, v := range containers {
		if v.Config.Labels["cxz.owner"] == m.Owner && v.Config.Labels["cxz.project"] == p.ID {
			p.ContainerID = v.ID
			continue
		}
		if !r.Recreate || !r.Confirmed {
			return nil, fmt.Errorf("foreign container %s: no adoption; use recreate with confirmation (writable layer lost, editors disconnect; workspace and named volumes retained)", v.ID[:12])
		}
		if _, e = dockerx.Run(ctx, "rm", "-f", v.ID); e != nil {
			return nil, e
		}
	}
	if r.Recreate && p.ContainerID != "" {
		if !r.Confirmed {
			return nil, fmt.Errorf("recreate requires confirmation: writable layer lost and editors disconnect")
		}
		if e = m.down(ctx, p); e != nil {
			return nil, e
		}
		p.ContainerID = ""
	}
	if e = m.save(ctx, p); e != nil {
		return nil, e
	}
	kind := r.Agent
	if kind == "" && len(p.Sessions) > 0 {
		kind = p.Sessions[0].Agent
	}
	if kind == "" {
		kind = "claude"
	}
	if e = m.provision(ctx, p, kind); e != nil {
		p.Error = e.Error()
		_ = m.save(context.WithoutCancel(ctx), p)
		return nil, e
	}
	p.Error = ""
	if e = m.save(ctx, p); e != nil {
		return nil, e
	}
	conn, client, e := m.client(ctx, p)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	var list *api.SessionList
	for i := 0; i < 150; i++ {
		q, cancel := context.WithTimeout(ctx, time.Second)
		list, e = client.List(q, &api.Empty{})
		cancel()
		if e == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if e != nil {
		return nil, fmt.Errorf("project runtime not ready: %w", e)
	}
	var chosen *api.Session
	for _, s := range list.Sessions {
		live := s.State == "idle" || s.State == "working" || s.State == "waiting_input" || s.State == "starting"
		if live {
			if r.NewSession || s.Agent != kind {
				return nil, fmt.Errorf("workspace has an active %s session %s; stop it explicitly before starting another", s.Agent, s.Id)
			}
			chosen = s
			break
		}
		if !r.NewSession && chosen == nil && s.Agent == kind {
			chosen = s
		}
	}
	if chosen == nil {
		clientID := r.ClientId
		if clientID == "" {
			clientID = core.ID()
		}
		chosen, e = client.Create(ctx, &api.CreateRequest{Workspace: p.RemoteWorkspace, Agent: kind, ClientId: clientID})
	} else if chosen.State == "interrupted" || chosen.State == "stopped" || chosen.State == "failed" {
		chosen, e = client.Resume(ctx, &api.Control{SessionId: chosen.Id, RunId: chosen.RunId, ClientId: core.ID()})
	}
	if e != nil {
		return nil, e
	}
	list, e = client.List(ctx, &api.Empty{})
	if e != nil {
		return nil, e
	}
	p.Sessions = list.Sessions
	for _, s := range p.Sessions {
		s.ProjectId = p.ID
	}
	chosen.ProjectId = p.ID
	return chosen, m.save(ctx, p)
}
func (m *Manager) Down(ctx context.Context, r *api.ProjectRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, e := m.resolve(ctx, r.Workspace)
	if e != nil {
		return e
	}
	return m.down(ctx, p)
}
func (m *Manager) down(ctx context.Context, p *Project) error {
	containers, e := dockerx.List(ctx, "label=cxz.owner="+m.Owner, "label=cxz.project="+p.ID)
	if e != nil {
		return e
	}
	for _, v := range containers {
		if _, e = dockerx.Owned(ctx, v.ID, m.Owner, p.ID); e != nil {
			return e
		}
		if _, e = dockerx.Run(ctx, "rm", "-f", v.ID); e != nil {
			return e
		}
	}
	p.ContainerID = ""
	return m.save(ctx, p)
}
func Discover(path string) []string {
	var out []string
	for _, p := range []string{filepath.Join(path, ".devcontainer", "devcontainer.json"), filepath.Join(path, ".devcontainer.json")} {
		if st, e := os.Stat(p); e == nil && !st.IsDir() {
			out = append(out, p)
		}
	}
	extra, _ := filepath.Glob(filepath.Join(path, ".devcontainer", "*", "devcontainer.json"))
	sort.Strings(extra)
	return append(out, extra...)
}
