package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The manager is not the one to ask where a client is attached. Left to the
// store it would answer with an empty table, which reads as "you have no
// connections" rather than "I do not know". See docs/plans/web-connections.md.
func TestConnectionsAreNotThisServersToAnswer(t *testing.T) {
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stack, err := Build(ctx, db, &fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work", State: "running"},
		s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", State: "idle", RunId: "run", CreateId: "create", Agent: "claude"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stack.Connection().List(ctx, resource.ConnectionListRequest_builder{}.Build())
	if status.Code(err) != codes.Unimplemented {
		t.Fatal("expected Unimplemented, got", err)
	}
}
