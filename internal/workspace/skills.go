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
	_, err = client.SyncSkills(ctx, &api.SyncSkillsInput{Bundle: b})
	return err
}

func (m *Manager) GetSkills(ctx context.Context, r *api.SkillsInput) (*api.SkillsReply, error) {
	project, err := m.skillScope(ctx, r.Project)
	if err != nil {
		return nil, err
	}
	listing, err := skillconfig.List(m.Root, project)
	if err != nil {
		return nil, err
	}
	return skillsReply(listing), nil
}

func (m *Manager) AddSkill(ctx context.Context, r *api.SkillInput) (*api.SkillsReply, error) {
	return m.changeSkills(ctx, r.Project, func(project string) (skillconfig.Listing, error) {
		return skillconfig.Add(m.Root, project, r.Name)
	})
}

func (m *Manager) RemoveSkill(ctx context.Context, r *api.SkillInput) (*api.SkillsReply, error) {
	return m.changeSkills(ctx, r.Project, func(project string) (skillconfig.Listing, error) {
		return skillconfig.Remove(m.Root, project, r.Name)
	})
}

func (m *Manager) SetSkillDefault(ctx context.Context, r *api.SkillDefaultInput) (*api.SkillsReply, error) {
	return m.changeSkills(ctx, "", func(string) (skillconfig.Listing, error) {
		return skillconfig.SetDefault(m.Root, r.Name, r.Enabled)
	})
}

func (m *Manager) SetProjectSkill(ctx context.Context, r *api.ProjectSkillInput) (*api.SkillsReply, error) {
	return m.changeSkills(ctx, r.Project, func(project string) (skillconfig.Listing, error) {
		return skillconfig.SetProject(m.Root, project, r.Name, r.Enabled)
	})
}

func (m *Manager) ClearProjectSkill(ctx context.Context, r *api.ClearProjectSkillInput) (*api.SkillsReply, error) {
	return m.changeSkills(ctx, r.Project, func(project string) (skillconfig.Listing, error) {
		return skillconfig.ClearProject(m.Root, project, r.Name)
	})
}

// SyncSkills is answered by a project container, not by a manager: the manager
// is the side that calls it.
func (m *Manager) SyncSkills(context.Context, *api.SyncSkillsInput) (*api.Receipt, error) {
	return nil, fmt.Errorf("skills are delivered to a project, not to a manager")
}

// changeSkills saves the change and then pushes it, because the change decides
// what a project may see; leaving it until something else happens to sync
// would mean a skill is switched on and not there.
func (m *Manager) changeSkills(ctx context.Context, handle string, change func(string) (skillconfig.Listing, error)) (*api.SkillsReply, error) {
	project, err := m.skillScope(ctx, handle)
	if err != nil {
		return nil, err
	}
	listing, err := change(project)
	if err != nil {
		return nil, err
	}
	out := skillsReply(listing)
	if failed := m.pushSkills(ctx, project); len(failed) > 0 {
		out.Message = strings.TrimSuffix(out.Message+"; ", "; ") +
			fmt.Sprintf("projects needing a retry: %s", strings.Join(failed, ", "))
	}
	return out, nil
}

// skillScope turns the handle a caller used into the project id the config is
// keyed by. An empty handle is the installation itself, and stays empty.
func (m *Manager) skillScope(ctx context.Context, handle string) (string, error) {
	if handle == "" {
		return "", nil
	}
	p, err := m.resolve(ctx, handle)
	if err != nil {
		return "", err
	}
	return p.ID, nil
}

func skillsReply(v skillconfig.Listing) *api.SkillsReply {
	out := &api.SkillsReply{Message: v.Message}
	for _, e := range v.Entries {
		out.Entries = append(out.Entries, &api.SkillEntry{
			Name: e.Name, Description: e.Description, Override: e.Override, Effective: e.Effective,
		})
	}
	return out
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
