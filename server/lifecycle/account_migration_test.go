package lifecycle

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/payday/config"
)

// A column added to a table SQLite already created is the one migration shape a
// fresh database never exercises, and SQLite cannot add a NOT NULL column with
// no default at all. The installation restart-looped on exactly that, so the
// table is reproduced here as it stood before login_identity existed.
func TestAccountMigrationAddsColumnsToAnExistingTable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		"CREATE TABLE `account` (`id` uuid NOT NULL, `alias` text NOT NULL, `name` text NOT NULL, `desc` text NOT NULL, `agent` text NOT NULL, `auth_backend` text NOT NULL, `date_updated` datetime NOT NULL, `date_erased` datetime NULL, `date_created` datetime NULL, PRIMARY KEY (`id`))",
		"CREATE UNIQUE INDEX `account_alias` ON `account` (`alias`) WHERE date_erased IS NULL",
		"INSERT INTO account VALUES('7c6eb3bd-bcc0-8728-9c09-1fdfc70e499d','work1','work1','','claude','project-local-oauth',1789861987646468039,NULL,1789861987646468510)",
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{p: &api.Project{Id: "project", Name: "workspace", Workspace: "/workspace", State: "running"}}
	if _, err := Build(ctx, db, f, nil, Options{Manager: true}); err != nil {
		t.Fatal("migration refused a database that predates the column:", err)
	}
	// An account that existed before the column is pinned to nothing, so its
	// next login is accepted and pins whatever it reaches.
	var identity *string
	if err := db.QueryRowContext(ctx, "SELECT login_identity FROM account WHERE alias='work1'").Scan(&identity); err != nil {
		t.Fatal("column not usable after migration:", err)
	}
	if identity != nil && *identity != "" {
		t.Fatal("an account that predates the pin came out pinned", *identity)
	}
}
