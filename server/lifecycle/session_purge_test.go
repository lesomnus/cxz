package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/sessionpurge"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type purgeFixture struct {
	deletionFixture
	purged []sessionpurge.Request
}

func (f *purgeFixture) Docker(_ context.Context, r *api.DockerInput) (*api.Receipt, error) {
	if r.Action != "session-purge" {
		return nil, status.Error(codes.Unimplemented, r.Action)
	}
	var q sessionpurge.Request
	if err := json.Unmarshal(r.Spec, &q); err != nil {
		return nil, err
	}
	f.purged = append(f.purged, q)
	reply := sessionpurge.Reply{Session: q.Session, DryRun: q.DryRun, Targets: []sessionpurge.Target{{Kind: "journal", Path: "/state/sessions/" + q.Session, Files: 3, Bytes: 4096}}}
	b, err := json.Marshal(reply)
	return &api.Receipt{Status: string(b)}, err
}

func purgeStack(t *testing.T) (*purgeFixture, resource.Server, *sql.DB, func()) {
	t.Helper()
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := &purgeFixture{deletionFixture: deletionFixture{fixture: fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work"},
		s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work", Title: "secrets in the title", Agent: "claude", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, State: "idle", RunId: "run", CreateId: "create"},
	}}}
	f.s.AuthBinding = accounts.BindingID("project", "work", accounts.ProjectLocalOAuth)
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stack.Session().List(ctx, &resource.SessionListRequest{}); err != nil {
		t.Fatal(err)
	}
	return f, stack, db, func() { db.Close() }
}

func purge(ctx context.Context, stack resource.Server, handle string, dry bool) (sessionpurge.Reply, error) {
	var reply sessionpurge.Reply
	spec, err := json.Marshal(sessionpurge.Request{Session: handle, DryRun: dry})
	if err != nil {
		return reply, err
	}
	out, err := stack.Project().Docker(ctx, resource.DockerRequest_builder{Action: ptr("session-purge"), Spec: spec}.Build())
	if err != nil {
		return reply, err
	}
	return reply, json.Unmarshal([]byte(out.GetStatus()), &reply)
}

func TestPurgeStopsTheAgentAndLeavesNoRecord(t *testing.T) {
	ctx := context.Background()
	f, stack, _, done := purgeStack(t)
	defer done()
	reply, err := purge(ctx, stack, "session", false)
	if err != nil {
		t.Fatal(err)
	}
	if f.stops != 1 {
		t.Fatal("purge did not stop the agent", f.stops)
	}
	if len(f.purged) != 1 || f.purged[0].Session != "session" || f.purged[0].DryRun {
		t.Fatal("runtime received", f.purged)
	}
	// The runtime's targets are reported alongside the record purge removed here.
	kinds := map[string]bool{}
	for _, target := range reply.Targets {
		kinds[target.Kind] = true
	}
	if !kinds["journal"] || !kinds["record"] {
		t.Fatal("reply hid part of the purge", reply.Targets)
	}
	// A purged session is not a tombstone: it is gone from every read.
	if _, err = stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build()); status.Code(err) != codes.NotFound {
		t.Fatal("purged session is still readable", err)
	}
	list, err := stack.Session().List(ctx, &resource.SessionListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range list.GetItems() {
		if v.GetRuntimeId() == "session" {
			t.Fatal("purged session came back through the projection")
		}
	}
}

// Erasing hides a row; it does not overwrite it. The title is the caller's own
// words, so purge has to replace it or it has not deleted what it claimed to.
func TestPurgeDoesNotLeaveTheTitleInTheDatabase(t *testing.T) {
	ctx := context.Background()
	_, stack, db, done := purgeStack(t)
	defer done()
	var rows int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM session WHERE name='secrets in the title'").Scan(&rows); err != nil || rows != 1 {
		t.Fatal("fixture title never reached the database", rows, err)
	}
	if _, err := purge(ctx, stack, "session", false); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM session WHERE name='secrets in the title' OR alias IS NOT NULL").Scan(&rows); err != nil || rows != 0 {
		t.Fatal("purge left the title or alias behind", rows, err)
	}
}

func TestPurgeDryRunTouchesNothing(t *testing.T) {
	ctx := context.Background()
	f, stack, _, done := purgeStack(t)
	defer done()
	reply, err := purge(ctx, stack, "session", true)
	if err != nil {
		t.Fatal(err)
	}
	if f.stops != 0 {
		t.Fatal("a dry run stopped the agent")
	}
	if len(f.purged) != 1 || !f.purged[0].DryRun {
		t.Fatal("runtime was not told this was a dry run", f.purged)
	}
	for _, target := range reply.Targets {
		if target.Kind == "record" {
			t.Fatal("a dry run reported erasing the record")
		}
	}
	v, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal("a dry run erased the session", err)
	}
	if !v.GetListed() || v.GetName() != "secrets in the title" {
		t.Fatal("a dry run changed the record", v.GetListed(), v.GetName())
	}
}

// A caller types the alias, not the runtime id. Resolving it before the runtime
// call is what stops purge from deleting by a handle the runtime never knew.
func TestPurgeResolvesTheAliasTheCallerTyped(t *testing.T) {
	ctx := context.Background()
	f, stack, _, done := purgeStack(t)
	defer done()
	v, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("session"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if v.GetAlias() == "" {
		t.Fatal("session has no alias to resolve")
	}
	reply, err := purge(ctx, stack, v.GetAlias(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.purged) != 1 || f.purged[0].Session != "session" {
		t.Fatal("runtime received an unresolved handle", f.purged)
	}
	if reply.Session != v.GetAlias() {
		t.Fatal("reply named the session by something the caller never typed", reply.Session)
	}
}

func TestPurgeRefusesAnUnknownOrUnnamedSession(t *testing.T) {
	ctx := context.Background()
	f, stack, _, done := purgeStack(t)
	defer done()
	for _, handle := range []string{"", "missing"} {
		if _, err := purge(ctx, stack, handle, false); err == nil {
			t.Fatal("purged", handle)
		}
	}
	if len(f.purged) != 0 || f.stops != 0 {
		t.Fatal("a refused purge still reached the runtime", f.purged, f.stops)
	}
}
