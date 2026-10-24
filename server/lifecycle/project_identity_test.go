package lifecycle

import (
	"bytes"
	"context"
	"testing"

	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/proto"
)

func TestLegacyProjectReferencesResolveToSameUUIDOverRPC(t *testing.T) {
	f, stack, _ := taskStack(t)
	conn := taskConn(t, stack)
	client := resource.NewProjectServiceClient(conn)
	legacy := resource.ProjectRef_builder{RuntimeId: proto.String(f.p.Id)}.Build()
	canonical := projectid.Ref(f.p.Id)
	for _, ref := range []*resource.ProjectRef{legacy, canonical} {
		p, err := client.Get(t.Context(), resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build())
		if err != nil || !bytes.Equal(p.GetId(), canonical.GetId()) {
			t.Fatal(p, err)
		}
		list, err := resource.NewSessionServiceClient(conn).List(t.Context(), resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Project: ref}.Build()}}.Build())
		if err != nil || len(list.GetItems()) != len(f.sessions) {
			t.Fatal(list, err)
		}
	}
	// A project reference nested in Task scope traverses the same translator.
	v, err := resource.NewTaskServiceClient(conn).Add(t.Context(), resource.TaskAddRequest_builder{Project: legacy, ClientId: "legacy-reference", Name: "Retained work"}.Build())
	if err != nil || !bytes.Equal(v.GetProject().GetId(), canonical.GetId()) {
		t.Fatal(v, err)
	}
}

func TestProjectSchemaUpgradePreservesResourceEdges(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := context.Background()
	p, err := stack.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: projectRef(f.p.Id), Select: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	v, err := stack.Task().Add(ctx, resource.TaskAddRequest_builder{Project: projectRef(f.p.Id), ClientId: "before-upgrade", Name: "Existing work"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the removed column and index as they existed on old installs.
	for _, q := range []string{"ALTER TABLE project ADD COLUMN runtime_id TEXT NOT NULL DEFAULT ''", "UPDATE project SET runtime_id=id", "CREATE UNIQUE INDEX project_runtime_id ON project(runtime_id) WHERE date_erased IS NULL"} {
		if _, err = db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		stack, err = Build(ctx, db, f, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := stack.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: projectRef(f.p.Id), Select: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build())
		if err != nil || !bytes.Equal(got.GetId(), p.GetId()) {
			t.Fatal(got, err)
		}
		work, err := stack.Task().Get(ctx, resource.TaskGetRequest_builder{Ref: resource.TaskRef_builder{Id: v.GetId(), Project: projectRef(f.p.Id)}.Build()}.Build())
		if err != nil || work.GetName() != v.GetName() {
			t.Fatal(work, err)
		}
		if _, err = stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(f.s.Id), Select: resource.SessionSelect_builder{All: proto.Bool(true)}.Build()}.Build()); err != nil {
			t.Fatal(err)
		}
	}
}
