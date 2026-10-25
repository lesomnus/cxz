package taskapi

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// LinkService is the policy for links between tasks: what may be linked, what
// may not, and what a link can never become. Storage owns the rows; this owns
// the refusals.
//
// It runs where the rows are. A manager proxies the whole service to the
// project runtime, so the cycle check reads the graph it is about to change
// rather than a copy of it. See server/lifecycle/task_link.go.
type LinkService struct {
	resource.TaskLinkServiceServer
	Project func(context.Context, *resource.ProjectRef) (*resource.Project, error)
	Task    func(context.Context, *resource.TaskRef) (*resource.Task, error)
	// Guard serializes Add's read-check-write. Without it two sessions adding
	// opposite edges at the same time both read a graph without the other's edge
	// and both pass the cycle check. One mutex is enough because every task write
	// reaches one process -- the runtime that owns the project's resources.db.
	Guard sync.Locker
}

const (
	// LinksPerProject is both the cap and the reason there is one: it is a single
	// page of the generated List, which is what the cycle check reads. A graph
	// that does not fit in one page cannot be walked without paging, and a
	// partial graph is worse than a refusal -- it hides the cycle it was read to
	// find.
	LinksPerProject = 500
	// LinksPerTask bounds one task's links, following the label cap of 32.
	LinksPerTask = 32
)

func (s LinkService) scope(ctx context.Context, ref *resource.ProjectRef) (*resource.Project, error) {
	if ref == nil {
		return nil, invalid("project is required")
	}
	return s.Project(ctx, ref)
}

// endpoint reads one end of a link and refuses a task outside the named
// project. A ref carrying another project is not resolved against that project
// and then compared: it is resolved within this scope, so a cross-project link
// cannot be made by naming the other side correctly.
func (s LinkService) endpoint(ctx context.Context, p *resource.Project, ref *resource.TaskRef, side string) (*resource.Task, error) {
	if ref == nil {
		return nil, invalid(side + " is required")
	}
	if ref.GetProject() != nil && !sameProject(ctx, s.Project, ref.GetProject(), p) {
		return nil, invalid("a link joins two tasks of one project; " + side + " is in another")
	}
	key := proto.Clone(ref).(*resource.TaskRef)
	key.SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
	v, err := s.Task(ctx, key)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(v.GetProject().GetId(), p.GetId()) {
		return nil, status.Error(codes.NotFound, side+" not found in project")
	}
	return v, nil
}

func sameProject(ctx context.Context, read func(context.Context, *resource.ProjectRef) (*resource.Project, error), ref *resource.ProjectRef, p *resource.Project) bool {
	if len(ref.GetId()) > 0 {
		return bytes.Equal(ref.GetId(), p.GetId())
	}
	other, err := read(ctx, ref)
	return err == nil && bytes.Equal(other.GetId(), p.GetId())
}

// clientId is what makes adding the same link twice a retry. The unique index
// behind it covers the rows that are not erased, so an unlinked pair is free to
// be linked again and the erased row keeps its own key.
func clientId(kind resource.TaskLinkKind, source, target []byte) string {
	return fmt.Sprintf("%s:%d:%s", hex.EncodeToString(source), int32(kind), hex.EncodeToString(target))
}

func name(v *resource.Task) string {
	if v.GetAlias() != "" {
		return v.GetAlias()
	}
	return hex.EncodeToString(v.GetId())
}

func (s LinkService) Add(ctx context.Context, r *resource.TaskLinkAddRequest) (*resource.TaskLink, error) {
	switch r.GetKind() {
	case resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS:
	case resource.TaskLinkKind_TASK_LINK_KIND_RELATES:
		// Defined in the schema so the field never has to change, and not served
		// until the surfaces that make it useful exist. Nothing reads a relation
		// yet, and a link nothing reads is a row that can only be wrong.
		return nil, status.Error(codes.Unimplemented, "only blocking links are supported")
	default:
		return nil, invalid("link kind is required")
	}
	p, err := s.scope(ctx, r.GetProject())
	if err != nil {
		return nil, err
	}
	source, err := s.endpoint(ctx, p, r.GetSource(), "source")
	if err != nil {
		return nil, err
	}
	target, err := s.endpoint(ctx, p, r.GetTarget(), "target")
	if err != nil {
		return nil, err
	}
	if bytes.Equal(source.GetId(), target.GetId()) {
		return nil, invalid("a task cannot block itself")
	}
	if s.Guard != nil {
		s.Guard.Lock()
		defer s.Guard.Unlock()
	}
	key := clientId(r.GetKind(), source.GetId(), target.GetId())
	switch existing, err := s.TaskLinkServiceServer.Get(ctx, resource.TaskLinkGetRequest_builder{
		Ref:    resource.TaskLinkRef_builder{ClientId: proto.String(key), Project: resource.ProjectRef_builder{Id: p.GetId()}.Build()}.Build(),
		Select: resource.TaskLinkSelect_builder{All: proto.Bool(true)}.Build(),
	}.Build()); status.Code(err) {
	case codes.OK:
		return existing, nil
	case codes.NotFound:
	default:
		return nil, err
	}
	links, err := s.graph(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(links) >= LinksPerProject {
		return nil, status.Errorf(codes.FailedPrecondition, "this project already holds %d links; remove the ones that are no longer a record", len(links))
	}
	if n := count(links, source.GetId()); n >= LinksPerTask {
		return nil, status.Errorf(codes.FailedPrecondition, "%s already has %d links", name(source), n)
	}
	if n := count(links, target.GetId()); n >= LinksPerTask {
		return nil, status.Errorf(codes.FailedPrecondition, "%s already has %d links", name(target), n)
	}
	// A cycle is what the new edge would close: target already blocks source,
	// directly or through others. A finished blocker's edge is walked like any
	// other -- it stops blocking, but it is still an edge, and a graph with a
	// loop in it is the thing being refused.
	if path := reaches(links, target.GetId(), source.GetId()); path != nil {
		return nil, invalid(fmt.Sprintf("%s already blocks %s (through %d link(s)); linking these would close a cycle", name(target), name(source), len(path)))
	}
	request := proto.Clone(r).(*resource.TaskLinkAddRequest)
	request.SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
	request.SetSource(resource.TaskRef_builder{Id: source.GetId()}.Build())
	request.SetTarget(resource.TaskRef_builder{Id: target.GetId()}.Build())
	request.SetClientId(key)
	return s.TaskLinkServiceServer.Add(ctx, request)
}

// graph is every blocking link of the project, as one page. More than a page is
// refused rather than paged: see LinksPerProject.
func (s LinkService) graph(ctx context.Context, p *resource.Project) ([]*resource.TaskLink, error) {
	page, err := s.TaskLinkServiceServer.List(ctx, resource.TaskLinkListRequest_builder{
		Project: resource.ProjectRef_builder{Id: p.GetId()}.Build(),
		Filters: []*resource.TaskLinkFilter{resource.TaskLinkFilter_builder{
			Project: resource.ProjectRef_builder{Id: p.GetId()}.Build(),
		}.Build()},
		Size: LinksPerProject,
	}.Build())
	if err != nil {
		return nil, err
	}
	if page.GetNext() != "" {
		return nil, status.Errorf(codes.FailedPrecondition, "this project holds more than %d links, which is more than one read; remove some before adding another", LinksPerProject)
	}
	links := make([]*resource.TaskLink, 0, len(page.GetItems()))
	for _, v := range page.GetItems() {
		if v.GetKind() == resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS {
			links = append(links, v)
		}
	}
	return links, nil
}

func count(links []*resource.TaskLink, id []byte) int {
	n := 0
	for _, v := range links {
		if bytes.Equal(v.GetSource().GetId(), id) || bytes.Equal(v.GetTarget().GetId(), id) {
			n++
		}
	}
	return n
}

// reaches walks source-blocks-target edges from `from` and answers the path to
// `to`, or nil. The walk is bounded by the number of links, which is bounded by
// LinksPerProject.
func reaches(links []*resource.TaskLink, from, to []byte) [][]byte {
	next := map[string][][]byte{}
	for _, v := range links {
		key := string(v.GetSource().GetId())
		next[key] = append(next[key], v.GetTarget().GetId())
	}
	seen := map[string]bool{string(from): true}
	queue := [][]byte{from}
	var path [][]byte
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		path = append(path, at)
		for _, to2 := range next[string(at)] {
			if bytes.Equal(to2, to) {
				return append(path, to2)
			}
			if !seen[string(to2)] {
				seen[string(to2)] = true
				queue = append(queue, to2)
			}
		}
	}
	return nil
}

// Blockers answers the tasks that still block this one: the sources of its
// blocking links that are neither done nor canceled. It is derived on every
// read and never stored, because a stored copy is a second truth that has to be
// kept -- and because the eight trackers surveyed in
// docs/plans/task-relations.md all derive it too.
func (s LinkService) Blockers(ctx context.Context, v *resource.Task) ([]*resource.Task, error) {
	page, err := s.TaskLinkServiceServer.List(ctx, resource.TaskLinkListRequest_builder{
		Project: resource.ProjectRef_builder{Id: v.GetProject().GetId()}.Build(),
		Filters: []*resource.TaskLinkFilter{resource.TaskLinkFilter_builder{
			Project: resource.ProjectRef_builder{Id: v.GetProject().GetId()}.Build(),
			Target:  resource.TaskRef_builder{Id: v.GetId()}.Build(),
		}.Build()},
		Size: LinksPerTask,
	}.Build())
	if err != nil {
		return nil, err
	}
	var open []*resource.Task
	for _, link := range page.GetItems() {
		if link.GetKind() != resource.TaskLinkKind_TASK_LINK_KIND_BLOCKS {
			continue
		}
		source, err := s.Task(ctx, resource.TaskRef_builder{
			Id: link.GetSource().GetId(), Project: resource.ProjectRef_builder{Id: v.GetProject().GetId()}.Build(),
		}.Build())
		if err != nil {
			return nil, err
		}
		switch source.GetStatus() {
		case resource.TaskStatus_TASK_STATUS_DONE, resource.TaskStatus_TASK_STATUS_CANCELED:
		default:
			open = append(open, source)
		}
	}
	return open, nil
}

// Blocked is the sentence a refusal says, and empty when nothing blocks.
func Blocked(blockers []*resource.Task) string {
	if len(blockers) == 0 {
		return ""
	}
	names := make([]string, 0, len(blockers))
	for _, v := range blockers {
		names = append(names, name(v))
	}
	return "task is blocked by " + strings.Join(names, ", ")
}

func (s LinkService) Erase(ctx context.Context, r *resource.TaskLinkRef) (*resource.TaskLinkEraseResponse, error) {
	p, err := s.scope(ctx, r.GetProject())
	if err != nil {
		return nil, err
	}
	key := proto.Clone(r).(*resource.TaskLinkRef)
	key.SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
	v, err := s.TaskLinkServiceServer.Get(ctx, resource.TaskLinkGetRequest_builder{
		Ref: key, Select: resource.TaskLinkSelect_builder{All: proto.Bool(true)}.Build(),
	}.Build())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// The same answer the generated Erase gives for a row that was already
			// gone, for the same reason: a caller unlinking twice meant no harm.
			return resource.TaskLinkEraseResponse_builder{Erased: proto.Bool(false)}.Build(), nil
		}
		return nil, err
	}
	if !bytes.Equal(v.GetProject().GetId(), p.GetId()) {
		return resource.TaskLinkEraseResponse_builder{Erased: proto.Bool(false)}.Build(), nil
	}
	return s.TaskLinkServiceServer.Erase(ctx, resource.TaskLinkRef_builder{Id: v.GetId()}.Build())
}

// Patch and Apply have nothing to write: every field of a link is immutable, so
// the generated PatchRequest carries only the version. A link is added and
// removed.
func (s LinkService) Patch(context.Context, *resource.TaskLinkPatchRequest) (*resource.TaskLink, error) {
	return nil, status.Error(codes.PermissionDenied, "a link is added and removed, not edited")
}
func (s LinkService) Apply(context.Context, *resource.TaskLinkApplyRequest) (*resource.TaskLink, error) {
	return nil, status.Error(codes.PermissionDenied, "a link is added and removed, not edited")
}

func (s LinkService) Get(ctx context.Context, r *resource.TaskLinkGetRequest) (*resource.TaskLink, error) {
	p, err := s.scope(ctx, r.GetRef().GetProject())
	if err != nil {
		return nil, err
	}
	request := proto.Clone(r).(*resource.TaskLinkGetRequest)
	request.GetRef().SetProject(resource.ProjectRef_builder{Id: p.GetId()}.Build())
	v, err := s.TaskLinkServiceServer.Get(ctx, request)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(v.GetProject().GetId(), p.GetId()) {
		return nil, status.Error(codes.NotFound, "link not found in project")
	}
	return v, nil
}

// NormalizeFilters resolves every project constraint before routing a request,
// the way the task filters are resolved: the outer scope wins, and a filter that
// names another project is refused rather than quietly answered from this one.
func (s LinkService) NormalizeFilters(ctx context.Context, project *resource.ProjectRef, filters []*resource.TaskLinkFilter) ([]*resource.TaskLinkFilter, error) {
	p, err := s.scope(ctx, project)
	if err != nil {
		return nil, err
	}
	if len(filters) > 32 {
		return nil, invalid("too many link filters")
	}
	if len(filters) == 0 {
		filters = []*resource.TaskLinkFilter{resource.TaskLinkFilter_builder{}.Build()}
	}
	scope := func() *resource.ProjectRef { return resource.ProjectRef_builder{Id: p.GetId()}.Build() }
	result := make([]*resource.TaskLinkFilter, 0, len(filters))
	for _, f := range filters {
		if f == nil {
			return nil, invalid("empty link filter")
		}
		if f.HasProject() && !sameProject(ctx, s.Project, f.GetProject(), p) {
			return nil, invalid("filter is outside project scope")
		}
		f = proto.Clone(f).(*resource.TaskLinkFilter)
		if f.HasRef() {
			f.GetRef().SetProject(scope())
		}
		if f.HasSource() {
			f.GetSource().SetProject(scope())
		}
		if f.HasTarget() {
			f.GetTarget().SetProject(scope())
		}
		f.SetProject(scope())
		result = append(result, f)
	}
	return result, nil
}

func (s LinkService) List(ctx context.Context, r *resource.TaskLinkListRequest) (*resource.TaskLinkListResponse, error) {
	filters, err := s.NormalizeFilters(ctx, r.GetProject(), r.GetFilters())
	if err != nil {
		return nil, err
	}
	request := proto.Clone(r).(*resource.TaskLinkListRequest)
	request.SetFilters(filters)
	return s.TaskLinkServiceServer.List(ctx, request)
}

func (s LinkService) Watch(r *resource.TaskLinkWatchRequest, out grpc.ServerStreamingServer[resource.TaskLinkWatchResponse]) error {
	filters, err := s.NormalizeFilters(out.Context(), r.GetProject(), r.GetFilters())
	if err != nil {
		return err
	}
	request := proto.Clone(r).(*resource.TaskLinkWatchRequest)
	request.SetFilters(filters)
	return s.TaskLinkServiceServer.Watch(request, out)
}
