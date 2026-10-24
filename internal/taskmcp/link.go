package taskmcp

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type linkInput struct {
	Ref       string `json:"ref" jsonschema:"The blocked task: ID or alias in this project"`
	BlockedBy string `json:"blocked_by" jsonschema:"The task that must finish first: ID or alias in this project"`
}

type linkView struct {
	Ref    string `json:"ref"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// refsPerRead is how many refs one List may carry, so reading a set of tasks by
// ID is a few calls and not one per task. The task filters are capped at 32.
const refsPerRead = 32

func statusOf(v *resource.Task) string {
	return strings.ToLower(strings.TrimPrefix(v.GetStatus().String(), "TASK_STATUS_"))
}

func refOf(v *resource.Task) string {
	if v.GetAlias() != "" {
		return v.GetAlias()
	}
	id, _ := uuid.FromBytes(v.GetId())
	return id.String()
}

func open(v *resource.Task) bool {
	switch v.GetStatus() {
	case resource.TaskStatus_TASK_STATUS_DONE, resource.TaskStatus_TASK_STATUS_CANCELED:
		return false
	}
	return true
}

// resolve reads tasks by ID, in batches, and answers them by ID.
func (t tools) resolve(ctx context.Context, ids [][]byte) (map[string]*resource.Task, error) {
	out := map[string]*resource.Task{}
	for len(ids) > 0 {
		batch := ids
		if len(batch) > refsPerRead {
			batch = batch[:refsPerRead]
		}
		ids = ids[len(batch):]
		filters := make([]*resource.TaskFilter, 0, len(batch))
		for _, id := range batch {
			filters = append(filters, resource.TaskFilter_builder{
				Ref: resource.TaskRef_builder{Id: id, Project: t.project()}.Build(),
			}.Build())
		}
		page, err := t.tasks.List(ctx, resource.TaskListRequest_builder{
			Project: t.project(), Filters: filters, Size: int32(len(batch)),
		}.Build())
		if err != nil {
			return nil, err
		}
		for _, v := range page.GetItems() {
			out[string(v.GetId())] = v
		}
	}
	return out, nil
}

// links reads this project's links as one page. The layer caps a project at one
// page for the same reason, so a second page means something is already wrong
// rather than that paging is needed here.
func (t tools) links(ctx context.Context, filters []*resource.TaskLinkFilter) ([]*resource.TaskLink, error) {
	page, err := t.linkClient.List(ctx, resource.TaskLinkListRequest_builder{
		Project: t.project(), Filters: filters, Size: 500,
	}.Build())
	if err != nil {
		return nil, err
	}
	return page.GetItems(), nil
}

// blocked answers which of these tasks something still blocks. One link read
// for the whole page, then one read per batch of blockers -- not one per task.
func (t tools) blocked(ctx context.Context, items []*resource.Task) (map[string]bool, error) {
	if len(items) == 0 {
		return nil, nil
	}
	links, err := t.links(ctx, nil)
	if err != nil {
		return nil, err
	}
	page := map[string]bool{}
	for _, v := range items {
		page[string(v.GetId())] = true
	}
	var wanted [][]byte
	seen := map[string]bool{}
	for _, link := range links {
		if link.GetKind() != resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS || !page[string(link.GetTarget().GetId())] {
			continue
		}
		id := link.GetSource().GetId()
		if !seen[string(id)] {
			seen[string(id)] = true
			wanted = append(wanted, id)
		}
	}
	blockers, err := t.resolve(ctx, wanted)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, link := range links {
		if link.GetKind() != resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS || !page[string(link.GetTarget().GetId())] {
			continue
		}
		if source, ok := blockers[string(link.GetSource().GetId())]; ok && open(source) {
			out[string(link.GetTarget().GetId())] = true
		}
	}
	return out, nil
}

// relations answers what blocks this task and what it blocks, in one link read.
// Finished blockers stay in the answer with their status: why a task was held up
// is a record, and the one that still holds it is the one that is not done.
func (t tools) relations(ctx context.Context, v *resource.Task) (blockedBy, blocks []linkView, err error) {
	links, err := t.links(ctx, []*resource.TaskLinkFilter{
		resource.TaskLinkFilter_builder{Target: resource.TaskRef_builder{Id: v.GetId(), Project: t.project()}.Build()}.Build(),
		resource.TaskLinkFilter_builder{Source: resource.TaskRef_builder{Id: v.GetId(), Project: t.project()}.Build()}.Build(),
	})
	if err != nil {
		return nil, nil, err
	}
	var wanted [][]byte
	seen := map[string]bool{}
	other := func(link *resource.TaskLink) []byte {
		if string(link.GetTarget().GetId()) == string(v.GetId()) {
			return link.GetSource().GetId()
		}
		return link.GetTarget().GetId()
	}
	for _, link := range links {
		id := other(link)
		if !seen[string(id)] {
			seen[string(id)] = true
			wanted = append(wanted, id)
		}
	}
	tasks, err := t.resolve(ctx, wanted)
	if err != nil {
		return nil, nil, err
	}
	for _, link := range links {
		if link.GetKind() != resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS {
			continue
		}
		end, ok := tasks[string(other(link))]
		if !ok {
			continue
		}
		item := linkView{Ref: refOf(end), Name: end.GetName(), Status: statusOf(end)}
		if string(link.GetTarget().GetId()) == string(v.GetId()) {
			blockedBy = append(blockedBy, item)
		} else {
			blocks = append(blocks, item)
		}
	}
	return blockedBy, blocks, nil
}

func (t tools) link(ctx context.Context, _ *mcp.CallToolRequest, in linkInput) (*mcp.CallToolResult, taskView, error) {
	blocked, err := t.ref(in.Ref)
	if err != nil {
		return nil, taskView{}, err
	}
	blocker, err := t.ref(in.BlockedBy)
	if err != nil {
		return nil, taskView{}, status.Error(codes.InvalidArgument, "blocked_by is required")
	}
	if err := t.active(ctx); err != nil {
		return nil, taskView{}, err
	}
	if _, err := t.linkClient.Add(ctx, resource.TaskLinkAddRequest_builder{
		Project: t.project(), Source: blocker, Target: blocked,
		Kind: resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS,
	}.Build()); err != nil {
		return nil, taskView{}, err
	}
	return t.get(ctx, nil, getInput{Ref: in.Ref})
}

func (t tools) unlink(ctx context.Context, _ *mcp.CallToolRequest, in linkInput) (*mcp.CallToolResult, taskView, error) {
	blockedRef, err := t.ref(in.Ref)
	if err != nil {
		return nil, taskView{}, err
	}
	blocked, err := t.read(ctx, blockedRef)
	if err != nil {
		return nil, taskView{}, err
	}
	blockerRef, err := t.ref(in.BlockedBy)
	if err != nil {
		return nil, taskView{}, status.Error(codes.InvalidArgument, "blocked_by is required")
	}
	blocker, err := t.read(ctx, blockerRef)
	if err != nil {
		return nil, taskView{}, err
	}
	if err := t.active(ctx); err != nil {
		return nil, taskView{}, err
	}
	links, err := t.links(ctx, []*resource.TaskLinkFilter{resource.TaskLinkFilter_builder{
		Source: resource.TaskRef_builder{Id: blocker.GetId(), Project: t.project()}.Build(),
		Target: resource.TaskRef_builder{Id: blocked.GetId(), Project: t.project()}.Build(),
	}.Build()})
	if err != nil {
		return nil, taskView{}, err
	}
	for _, link := range links {
		if link.GetKind() != resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS {
			continue
		}
		if _, err := t.linkClient.Erase(ctx, resource.TaskLinkRef_builder{Id: link.GetId(), Project: t.project()}.Build()); err != nil {
			return nil, taskView{}, err
		}
	}
	return t.get(ctx, nil, getInput{Ref: in.Ref})
}
