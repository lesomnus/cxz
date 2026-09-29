package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/cxz/internal/skillconfig"
	"github.com/lesomnus/cxz/internal/wisp"
)

// RemoveProject never follows workspace or user-configured mount paths. Only
// cxz-owned Docker resources and paths beneath managed state are removed.
func (m *Manager) RemoveProject(ctx context.Context, id string, sessions []string) error {
	return m.removeProject(ctx, id, sessions, dockerx.Run)
}

func (m *Manager) removeProject(ctx context.Context, id string, sessions []string, run removalDocker) error {
	if !assets.ValidID(id) || m.Owner == "" {
		return fmt.Errorf("invalid project removal scope")
	}
	lock := m.projectLock(id)
	lock.Lock()
	defer lock.Unlock()
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	projects, err := m.all(ctx)
	if err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, session := range sessions {
		ids[session] = true
	}
	for _, p := range projects {
		if p.ID == id {
			if p.ContainerID != "" {
				containers, err := run(ctx, "container", "ls", "-a", "-q", "--no-trunc")
				if err != nil {
					return err
				}
				for _, container := range strings.Fields(string(containers)) {
					if container == p.ContainerID {
						if _, err := inspectRemoval(ctx, run, "container", container, m.Owner, id); err != nil {
							return err
						}
					}
				}
			}
			if p.Volume != "" {
				volumes, err := run(ctx, "volume", "ls", "-q")
				if err != nil {
					return err
				}
				for _, volume := range strings.Fields(string(volumes)) {
					if volume == p.Volume {
						if _, err := inspectRemoval(ctx, run, "volume", volume, m.Owner, id); err != nil {
							return err
						}
					}
				}
			}
			for _, s := range p.Sessions {
				ids[s.Id] = true
			}
		}
	}
	for session := range ids {
		if !assets.ValidID(session) {
			return fmt.Errorf("invalid session removal scope")
		}
	}
	if err := removeProjectDocker(ctx, run, m.Owner, id, m.Container); err != nil {
		return err
	}
	m.historyMu.Lock()
	if c := m.historyClients[id]; c != nil {
		c.conn.Close()
		delete(m.historyClients, id)
	}
	m.historyMu.Unlock()
	// Serialize with cached stream writes and block late frames from recreating
	// history or derived summaries after removal. These guards contain no content.
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if m.removedSessions == nil {
		m.removedSessions = map[string]bool{}
	}
	for session := range ids {
		m.removedSessions[session] = true
	}
	aux, err := m.auxiliaryController()
	if err != nil {
		return err
	}
	for session := range ids {
		if err := aux.Forget(session); err != nil {
			return err
		}
	}
	if err := accounts.RevokeProjectGrants(m.Root, id); err != nil {
		return err
	}
	if err := mcpconfig.ForgetProject(m.Root, id); err != nil {
		return err
	}
	if err := skillconfig.ForgetProject(m.Root, id); err != nil {
		return err
	}
	root, err := os.OpenRoot(m.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	for session := range ids {
		if err := assets.RemoveSession(ctx, filepath.Join(m.Root, "assets"), id, session); err != nil {
			return err
		}
	}
	if err := root.RemoveAll(filepath.Join("assets", "exports", id)); err != nil {
		return err
	}
	if _, err := os.Lstat(wisp.HostSecretsMount); err == nil {
		if err := wisp.CheckSecretRoot(wisp.HostSecretsMount); err != nil {
			return err
		}
		secrets, err := os.OpenRoot(wisp.HostSecretsMount)
		if err != nil {
			return err
		}
		err = secrets.RemoveAll(id)
		secrets.Close()
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// The manifest is removed before the database row, so startup cannot restore
	// deleted intent. Registry rows remain until all cleanup succeeds for retries.
	if err := root.RemoveAll(filepath.Join("projects", id)); err != nil {
		return err
	}
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for session := range ids {
		for _, table := range []string{"events", "history_floors"} {
			var exists int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&exists); err != nil {
				return err
			}
			if exists > 0 {
				if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE session_id=?", session); err != nil {
					return err
				}
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM projects WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

type removalDocker func(context.Context, ...string) ([]byte, error)
type removalResource struct {
	ID         string `json:"Id"`
	Name       string
	Labels     map[string]string
	Config     struct{ Labels map[string]string }
	Containers map[string]struct{ Name string }
}

func inspectRemoval(ctx context.Context, run removalDocker, kind, id, owner, project string) (removalResource, error) {
	var v []removalResource
	b, err := run(ctx, kind, "inspect", id)
	if err != nil {
		return removalResource{}, err
	}
	if err := json.Unmarshal(b, &v); err != nil || len(v) != 1 {
		return removalResource{}, fmt.Errorf("invalid %s inventory", kind)
	}
	labels := v[0].Labels
	if kind == "container" {
		labels = v[0].Config.Labels
	}
	if labels["cxz.owner"] != owner || labels["cxz.project"] != project {
		return removalResource{}, fmt.Errorf("refusing unowned %s %s", kind, id)
	}
	return v[0], nil
}

func removeProjectDocker(ctx context.Context, run removalDocker, owner, project, manager string) error {
	inventory := map[string][]string{}
	for _, kind := range []string{"container", "volume", "network"} {
		args := []string{kind, "ls", "-q", "--filter", "label=cxz.owner=" + owner, "--filter", "label=cxz.project=" + project}
		if kind == "container" {
			args = append(args, "-a")
		}
		b, err := run(ctx, args...)
		if err != nil {
			return err
		}
		inventory[kind] = strings.Fields(string(b))
		for _, id := range inventory[kind] {
			if _, err := inspectRemoval(ctx, run, kind, id, owner, project); err != nil {
				return err
			}
		}
	}
	for _, kind := range []string{"container", "volume", "network"} {
		for _, id := range inventory[kind] {
			// Recheck ownership immediately before each destructive operation.
			v, err := inspectRemoval(ctx, run, kind, id, owner, project)
			if err != nil {
				return err
			}
			if kind == "network" {
				for endpoint, c := range v.Containers {
					// Legacy dedicated networks may still connect the manager. Never detach
					// foreign/user endpoints or the installation's shared workspace network.
					if c.Name != manager && endpoint != manager && c.Name != "cxz-"+owner+"-docker" {
						return fmt.Errorf("project network %s still has endpoint %s; disconnect it before retrying", id, c.Name)
					}
					if _, err := inspectRemoval(ctx, run, "container", endpoint, owner, ""); err != nil {
						return err
					}
					if _, err := run(ctx, "network", "disconnect", id, endpoint); err != nil {
						return err
					}
				}
			}
			args := []string{kind, "rm"}
			if kind == "container" {
				args = append(args, "-f")
			}
			args = append(args, id)
			if _, err := run(ctx, args...); err != nil {
				return fmt.Errorf("remove project %s %s: %w", kind, id, err)
			}
		}
	}
	return nil
}
