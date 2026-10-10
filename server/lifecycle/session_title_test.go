package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/sessiontitle"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type titleFixture struct {
	fixture
	sessions   []*api.Session
	titleCalls int
	titleErr   error
}

func (f *titleFixture) ResourceSnapshot(context.Context) (*api.ProjectList, *api.SessionList, error) {
	return &api.ProjectList{Projects: []*api.Project{f.p}}, &api.SessionList{Sessions: f.sessions}, nil
}

func (f *titleFixture) SetSessionTitle(_ context.Context, id, text string) (string, error) {
	f.titleCalls++
	if f.titleErr != nil {
		return "", f.titleErr
	}
	for _, v := range f.sessions {
		if v.Id == id {
			v.Title = sessiontitle.Normalize(text)
			return v.Title, nil
		}
	}
	return "", status.Error(codes.NotFound, "session not found")
}

func TestSessionPatchManualTitleAndAlias(t *testing.T) {
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &titleFixture{fixture: fixture{p: &api.Project{Id: "project", Workspace: "/workspace", Name: "test"}}, sessions: []*api.Session{
		{Id: "first", ProjectId: "project", Title: "Old title", CreateId: "first-create", CreatedAt: time.Now().UnixMilli(), Agent: "codex", RunId: "stable-run", State: "working", LastSeq: 9},
		{Id: "second", ProjectId: "project", Title: "Other title", CreateId: "second-create", CreatedAt: time.Now().UnixMilli(), Agent: "codex"},
	}}
	for _, v := range f.sessions {
		v.Account = "work"
		v.AuthBackend = accounts.ProjectLocalOAuth
		v.AuthBinding = accounts.BindingID(v.ProjectId, v.Account, v.AuthBackend)
	}
	stack, err := Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	get := func(id string) *resource.Session {
		t.Helper()
		v, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(id), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	before, second := get("first"), get("second")
	patch := func(name, alias *string) error {
		_, err := stack.Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef("first"), Name: name, Alias: alias}.Build())
		return err
	}
	for _, r := range []struct {
		name, alias *string
		code        codes.Code
	}{
		{ptr("New title"), ptr(second.GetAlias()), codes.AlreadyExists},
		{ptr("New title"), ptr("Bad_Alias"), codes.InvalidArgument},
		{ptr(" ` \n ` "), ptr("oak-tree"), codes.InvalidArgument},
		{ptr(strings.Repeat("x", sessiontitle.MaxInputBytes+1)), nil, codes.InvalidArgument},
		{nil, nil, codes.InvalidArgument},
	} {
		if err := patch(r.name, r.alias); status.Code(err) != r.code {
			t.Fatal("invalid metadata edit", err)
		}
	}
	if _, err := stack.Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef("first"), Name: ptr("New title"), Listed: ptr(false)}.Build()); status.Code(err) != codes.PermissionDenied {
		t.Fatal("metadata field bypass", err)
	}
	if f.titleCalls != 0 || !proto.Equal(before, get("first")) {
		t.Fatal("failed validation changed title or alias")
	}
	f.titleErr = status.Error(codes.Unavailable, "title persistence failed")
	if err := patch(ptr("New title"), ptr("oak-tree")); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if !proto.Equal(before, get("first")) {
		t.Fatal("failed title save changed resource metadata")
	}
	f.titleErr = nil
	if err := patch(ptr(" `New\n title` "), nil); err != nil {
		t.Fatal("name-only patch", err)
	}
	if get("first").GetName() != "New title" || get("first").GetAlias() != before.GetAlias() {
		t.Fatal("name-only patch changed alias")
	}
	if err := patch(ptr("Final title"), ptr("oak-tree")); err != nil {
		t.Fatal("combined metadata patch", err)
	}
	// Reconciliation and rebuilding the resource projection must preserve the
	// runtime-owned title, stable identity, run and transcript cursor.
	layer, _ := resource.Find[Layer](stack)
	if err := layer.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	stack, err = Build(ctx, db, f)
	if err != nil {
		t.Fatal(err)
	}
	after := get("first")
	if after.GetName() != "Final title" || after.GetAlias() != "oak-tree" || after.GetRuntimeId() != before.GetRuntimeId() || string(after.GetId()) != string(before.GetId()) || !proto.Equal(after.GetStatus(), before.GetStatus()) {
		t.Fatal("metadata edit lost or changed runtime state", after)
	}
}
