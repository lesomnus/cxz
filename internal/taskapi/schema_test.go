package taskapi_test

import (
	"testing"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestTaskRespectsPaydayHeaderSlots(t *testing.T) {
	// This is the payday entity numbering contract, not a convention inferred
	// from other application resources. Slot 3 is the optional app set edge.
	reserved := map[protoreflect.FieldNumber]protoreflect.Name{
		1: "id", 2: "tenant", 4: "alias", 5: "name", 6: "desc", 7: "labels",
		13: "date_updated", 14: "date_erased", 15: "date_created",
	}
	md := (&resource.Task{}).ProtoReflect().Descriptor()
	for i := 0; i < md.Fields().Len(); i++ {
		f := md.Fields().Get(i)
		if name, ok := reserved[f.Number()]; ok && f.Name() != name {
			t.Errorf("Task.%s occupies payday slot %d for %s", f.Name(), f.Number(), name)
		}
	}
	for _, name := range []protoreflect.Name{"alias", "name", "desc", "labels", "date_updated", "date_erased", "date_created"} {
		f := md.Fields().ByName(name)
		if f == nil || reserved[f.Number()] != name {
			t.Errorf("Task.%s is outside its payday slot", name)
		}
	}
	labels := md.Fields().ByName("labels")
	if !labels.IsMap() || labels.MapKey().Kind() != protoreflect.StringKind || labels.MapValue().Kind() != protoreflect.StringKind {
		t.Fatal("payday labels must be map<string,string>")
	}
	for _, name := range []protoreflect.Name{"project", "status", "priority", "assignee", "assignee_run", "client_id", "source_session", "source_run", "source_seq"} {
		n := md.Fields().ByName(name).Number()
		if !(n >= 8 && n <= 12 || n >= 16) {
			t.Errorf("app field %s uses payday slot %d", name, n)
		}
	}
}
