package resource_test

import (
	"strings"
	"testing"

	_ "github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/pdpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestApplicationEntitiesRespectPaydayHeaderSlots(t *testing.T) {
	reserved := map[protoreflect.FieldNumber]protoreflect.Name{
		1: "id", 2: "tenant", 4: "alias", 5: "name", 6: "desc", 7: "labels",
		13: "date_updated", 14: "date_erased", 15: "date_created",
	}
	checked := 0
	protoregistry.GlobalFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		if !strings.HasPrefix(file.Path(), "cxz/") || strings.HasPrefix(file.Path(), "cxz/payday/") {
			return true
		}
		for i := 0; i < file.Messages().Len(); i++ {
			md := file.Messages().Get(i)
			if !proto.HasExtension(md.Options(), pdpb.E_Entity) {
				continue
			}
			t.Run(string(md.Name()), func(t *testing.T) {
				for i := 0; i < md.Fields().Len(); i++ {
					field := md.Fields().Get(i)
					if name, ok := reserved[field.Number()]; ok && field.Name() != name {
						t.Errorf("%s occupies payday slot %d for %s", field.FullName(), field.Number(), name)
					}
					for number, name := range reserved {
						if field.Name() == name && field.Number() != number {
							t.Errorf("%s uses %d, expected payday slot %d", field.FullName(), field.Number(), number)
						}
					}
					switch field.Name() {
					case "alias", "name", "desc":
						if field.Kind() != protoreflect.StringKind || field.IsList() {
							t.Errorf("%s must be a string", field.FullName())
						}
					case "labels":
						if !field.IsMap() || field.MapKey().Kind() != protoreflect.StringKind || field.MapValue().Kind() != protoreflect.StringKind {
							t.Errorf("%s must be map<string,string>", field.FullName())
						}
					case "date_updated", "date_erased", "date_created":
						if field.Kind() != protoreflect.MessageKind || field.Message().FullName() != "google.protobuf.Timestamp" || field.IsList() {
							t.Errorf("%s must be a Timestamp", field.FullName())
						}
					}
				}
			})
			checked++
		}
		return true
	})
	if checked == 0 {
		t.Fatal("no application entities were checked")
	}
}
