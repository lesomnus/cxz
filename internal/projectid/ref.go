package projectid

import "github.com/lesomnus/cxz/resource"

func Ref(id string) *resource.ProjectRef {
	return resource.ProjectRef_builder{Id: Bytes(id)}.Build()
}
