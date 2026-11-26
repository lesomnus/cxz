package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A session whose supervisor is gone is stopped, and says so: the socket it
// would have listened on is an implementation detail.
func TestCommandToStoppedSession(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE sessions(id TEXT PRIMARY KEY, manifest BLOB)"); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 24)
	b, _ := json.Marshal(core.Session{ID: id, ProjectID: "project", Workspace: "/work"})
	if _, err = db.Exec("INSERT INTO sessions VALUES(?,?)", id, b); err != nil {
		t.Fatal(err)
	}
	s := &Server{root: t.TempDir(), db: db}
	_, err = s.Send(ctx, &api.Input{SessionId: id, Text: "hello"})
	if status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), "is not running; resume it first") || strings.Contains(err.Error(), "dial unix") {
		t.Fatal(err)
	}
}
