package projectid

import "github.com/lesomnus/cxz/resource"

func Ref(id string) *resource.ProjectRef {
	return resource.ProjectRef_builder{Id: Bytes(id)}.Build()
}

func RefString(ref *resource.ProjectRef) string {
	if len(ref.GetId()) > 0 {
		return String(ref.GetId())
	}
	return Canonical(ref.GetRuntimeId())
}
