package conversation

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
)

// RegistryFromResources uses the Manager's canonical UUIDs and aliases, never
// the independently generated aliases in a project runtime's resource database.
func RegistryFromResources(ctx context.Context, project string, list func(context.Context, *resource.SessionListRequest) (*resource.SessionListResponse, error)) ([]Session, error) {
	var out []Session
	after := ""
	yes := true
	for {
		page, err := list(ctx, resource.SessionListRequest_builder{Size: 200, After: after, Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Listed: &yes, Project: projectid.Ref(project)}.Build()}}.Build())
		if err != nil {
			return nil, err
		}
		for _, v := range page.GetItems() {
			// List expands project relations; check even if a backend ignores filters.
			if !projectid.Same(projectid.String(v.GetProject().GetId()), project) || !v.GetListed() {
				continue
			}
			id, err := uuid.FromBytes(v.GetId())
			if err != nil {
				return nil, fmt.Errorf("invalid session UUID")
			}
			out = append(out, Session{ID: id.String(), Alias: v.GetAlias(), Agent: v.GetAgent(), State: v.GetStatus().GetState(), RuntimeID: v.GetRuntimeId(), ProjectID: projectid.Canonical(project)})
			if len(out) > 10000 {
				return nil, fmt.Errorf("too many project sessions")
			}
		}
		if page.GetNext() == "" {
			return out, nil
		}
		if after == page.GetNext() {
			return nil, fmt.Errorf("registry cursor did not advance")
		}
		after = page.GetNext()
	}
}
