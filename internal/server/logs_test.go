package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/payday/config"
)

func TestLogsScopeAndUnavailableSources(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE sessions(id TEXT PRIMARY KEY, manifest BLOB)"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	s := &Server{root: root, db: db}
	ids := []string{strings.Repeat("a", 24), strings.Repeat("b", 24), strings.Repeat("c", 24)}
	for i, id := range ids {
		project := "project"
		if i == 2 {
			project = "foreign"
		}
		m := core.Session{ID: id, ProjectID: project, Workspace: "/work"}
		b, _ := json.Marshal(m)
		if _, err = db.Exec("INSERT INTO sessions VALUES(?,?)", id, b); err != nil {
			t.Fatal(err)
		}
		dir := core.Dir(root, id)
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "supervisor.log"), []byte("output "+id), 0600)
	}
	for _, project := range []bool{false, true} {
		r, err := s.Logs(ctx, &api.LogsRequest{SessionId: ids[0], Project: project})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(r.Text, ids[0]) || strings.Contains(r.Text, ids[2]) || strings.Contains(r.Text, ids[1]) != project || !strings.Contains(r.Text, "Unavailable:") {
			t.Fatal(r.Text)
		}
	}
	if _, err := s.Logs(ctx, &api.LogsRequest{SessionId: "../../private", Project: true}); err == nil {
		t.Fatal("unresolved session read logs")
	}
}
