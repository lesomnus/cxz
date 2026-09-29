package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/skillconfig"
)

// syncSkills delivers only what this project is meant to see. Resolving before
// the push is what keeps a library the installation shares from arriving
// everywhere: an unactivated skill costs the project nothing, not even the
// bytes.
func (m *Manager) syncSkills(ctx context.Context, client api.SessionsClient, project string) error {
	c, err := skillconfig.Load(m.Root)
	if err != nil {
		return err
	}
	bundle, err := filemap.Snapshot(m.Root, c.Mappings(project))
	if err != nil {
		return err
	}
	b, err := json.Marshal(bundle)
	if err != nil {
		return err
	}
	_, err = client.Docker(ctx, &api.DockerInput{Action: "skills-sync", Spec: b})
	return err
}

func (m *Manager) skills(ctx context.Context, spec []byte) (*api.Receipt, error) {
	var r skillconfig.Request
	if err := json.Unmarshal(spec, &r); err != nil {
		return nil, err
	}
	if r.Project != "" {
		p, err := m.resolve(ctx, r.Project)
		if err != nil {
			return nil, err
		}
		r.Project = p.ID
	}
	reply, err := skillconfig.Apply(m.Root, r)
	if err != nil {
		return nil, err
	}
	if r.Action != "list" {
		// A change decides what a project may see, so push it now rather than
		// leaving it until something else happens to sync.
		if failed := m.pushSkills(ctx, r.Project); len(failed) > 0 {
			reply.Message = strings.TrimSuffix(reply.Message+"; ", "; ") +
				fmt.Sprintf("projects needing a retry: %s", strings.Join(failed, ", "))
		}
	}
	b, err := json.Marshal(reply)
	return &api.Receipt{Status: string(b)}, err
}

// pushSkills refreshes running projects, or just one when the change was
// scoped to it. Names that could not be reached are reported rather than
// failing the change, which is already saved.
func (m *Manager) pushSkills(ctx context.Context, project string) []string {
	projects, err := m.all(ctx)
	if err != nil {
		return []string{err.Error()}
	}
	var failed []string
	for _, p := range projects {
		if p.ContainerID == "" || (project != "" && p.ID != project) {
			continue
		}
		v, err := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.ID)
		if err == nil && !v.State.Running {
			continue
		}
		if err == nil {
			conn, client, e := m.client(ctx, p)
			err = e
			if err == nil {
				err = m.syncSkills(ctx, client, p.ID)
				conn.Close()
			}
		}
		if err != nil {
			failed = append(failed, p.Name)
		}
	}
	return failed
}
