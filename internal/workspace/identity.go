package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/internal/skillconfig"
)

func (p *Project) storageID() string {
	if p.StorageID != "" {
		return p.StorageID
	}
	return p.ID
}

func (m *Manager) LegacyProjectID(ctx context.Context, id string) (string, error) {
	p, err := m.resolve(ctx, id)
	if err != nil {
		return "", err
	}
	return p.StorageID, nil
}

// The manifest is the recovery record. Write the explicit mapping there before
// replacing registry keys, so interruption at either write is safely retried.
// State directories, mounts, running containers and volumes are never moved.
func (m *Manager) migrateProjectIdentities(ctx context.Context) error {
	projects, err := m.all(ctx)
	if err != nil {
		return err
	}
	byStorage := map[string]*Project{}
	for _, p := range projects {
		byStorage[p.storageID()] = p
	}
	files, err := filepath.Glob(filepath.Join(m.Root, "projects", "*", "project.json"))
	if err != nil {
		return err
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var p Project
		if err = json.Unmarshal(b, &p); err != nil {
			return fmt.Errorf("invalid project manifest %s: %w", file, err)
		}
		if p.storageID() != filepath.Base(filepath.Dir(file)) {
			return fmt.Errorf("project manifest identity mismatch")
		}
		byStorage[p.storageID()] = &p
	}
	for storage, p := range byStorage {
		if projectid.Legacy(p.ID) {
			p.StorageID = p.ID
			p.ID = projectid.Canonical(p.ID)
		}
		if p.StorageID == "" {
			continue
		}
		if !projectid.Same(p.ID, p.StorageID) {
			return fmt.Errorf("invalid legacy project mapping for %s", p.ID)
		}
		if err = mcpconfig.MigrateProject(m.Root, storage, p.ID); err != nil {
			return err
		}
		if err = skillconfig.MigrateProject(m.Root, storage, p.ID); err != nil {
			return err
		}
		// Existing containers bind this physical export directory. Publish through
		// a relative UUID alias so their mounts keep seeing subsequent uploads.
		exports := filepath.Join(m.Root, "assets", "exports")
		if err = os.MkdirAll(filepath.Join(exports, storage), 0755); err != nil { return err }
		alias := filepath.Join(exports,p.ID)
		if target,e := os.Readlink(alias); e == nil {
			if target!=storage { return fmt.Errorf("project export mapping mismatch") }
		} else if os.IsNotExist(e) {
			if err=os.Symlink(storage,alias);err!=nil{return err}
		} else { return fmt.Errorf("project export mapping conflict: %w",e) }
		for _, session := range p.Sessions {
			session.ProjectId = p.ID
		}
		if err = os.MkdirAll(filepath.Join(m.Root, "projects", storage), 0700); err != nil {
			return err
		}
		if err = core.WriteJSON(filepath.Join(m.Root, "projects", storage, "project.json"), p); err != nil {
			return err
		}
		tx, err := m.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		b, err := json.Marshal(p)
		if err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO projects VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", p.ID, b)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM projects WHERE id=?", storage)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
