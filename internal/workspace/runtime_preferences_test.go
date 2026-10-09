package workspace

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type runtimeClient struct {
	api.SessionsClient
	err     error
	actions []string
}

func (c *runtimeClient) FileMappings(_ context.Context, _ *api.FileMappingsInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	return &api.Receipt{}, nil
}

// Each push is its own call now, so a runtime that does not know one answers
// Unimplemented rather than routing an action it never had -- which is the
// other shape this test has to survive.
func (c *runtimeClient) SyncSkills(_ context.Context, _ *api.SyncSkillsInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.actions = append(c.actions, "SyncSkills")
	return &api.Receipt{}, c.err
}
func (c *runtimeClient) SyncMcpServers(_ context.Context, _ *api.SyncMcpServersInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.actions = append(c.actions, "SyncMcpServers")
	return &api.Receipt{}, c.err
}

// Preferences are pushed to a project before a session is resumed or created,
// and they take effect at the next agent start. A runtime older than the
// manager cannot take a push it does not know: it routes the action to the
// manager path it never has and answers with a failed precondition. Letting
// that decide the outcome took the project down entirely, and a stopped
// session cannot bring about the quiet the runtime needs to update.
func TestPreferencePushDoesNotDecideWhetherAProjectWorks(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(root, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}

	client := &runtimeClient{err: status.Error(codes.FailedPrecondition, "managed Docker requires an installed host manager")}
	if err := m.syncRuntimePreferences(t.Context(), client, "project"); err != nil {
		t.Fatal("an older project runtime blocked the session:", err)
	}
	if len(client.actions) == 0 {
		t.Fatal("nothing was pushed")
	}

	// One older still does not serve the call.
	client = &runtimeClient{err: status.Error(codes.Unimplemented, "unknown method")}
	if err := m.syncRuntimePreferences(t.Context(), client, "project"); err != nil {
		t.Fatal("a runtime without the call blocked the session:", err)
	}

	// Anything else is a real failure and still stops the caller.
	client = &runtimeClient{err: errors.New("connection reset")}
	if err := m.syncRuntimePreferences(t.Context(), client, "project"); err == nil {
		t.Fatal("a failed push was ignored")
	}
}
