package convindex

import (
	"context"
	"github.com/lesomnus/cxz/internal/projectid"
)

func canonicalProject(id string) string {
	if projectid.Legacy(id) {
		return projectid.Canonical(id)
	}
	return id
}

func (i *Index) migrateProjectIDs(ctx context.Context) error {
	rows, err := i.db.QueryContext(ctx, "SELECT DISTINCT project FROM sessions WHERE length(project)=24")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if projectid.Legacy(id) {
			ids = append(ids, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = i.db.ExecContext(ctx, "UPDATE sessions SET project=? WHERE project=?", projectid.Canonical(id), id); err != nil {
			return err
		}
	}
	return nil
}
