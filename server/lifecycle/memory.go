package lifecycle

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/pmezard/go-difflib/difflib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const memoryBodyLimit = 256 * 1024

type MemoryServer struct {
	Layer
	resource.UnimplementedMemoryServiceServer
}

func (s Layer) Memory() resource.MemoryServiceServer { return MemoryServer{Layer: s} }
func memoryAll() *resource.MemorySelect {
	return resource.MemorySelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()
}
func memoryInvalid(text string) error { return status.Error(codes.InvalidArgument, text) }
func memoryExpired(m *resource.Memory) bool {
	return m.HasDateDeleted() && !m.GetDateDeleted().AsTime().After(time.Now())
}
func memoryFields(name, content string, labels map[string]string) error {
	if strings.TrimSpace(name) == "" || len(name) > 1024 {
		return memoryInvalid("name is required (maximum 1024 bytes)")
	}
	if len(content) > memoryBodyLimit || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return memoryInvalid("content must be UTF-8 Markdown, maximum 256 KiB")
	}
	if len(labels) > 64 {
		return memoryInvalid("maximum 64 labels")
	}
	for k, v := range labels {
		if strings.TrimSpace(k) == "" || len(k) > 128 || len(v) > 1024 {
			return memoryInvalid("invalid label")
		}
	}
	return nil
}
func (s MemoryServer) local(ctx context.Context, ref *resource.MemoryRef) (*resource.Memory, error) {
	if ref == nil || (!ref.HasId() && !ref.HasAlias() && !ref.HasClientId()) {
		return nil, memoryInvalid("memory ref is required")
	}
	v, err := s.Next().Memory().Get(ctx, resource.MemoryGetRequest_builder{Ref: ref, Select: memoryAll()}.Build())
	if err != nil {
		return nil, err
	}
	if ref.GetProject() != nil {
		p, err := s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: ref.GetProject()}.Build())
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(p.GetId(), v.GetProject().GetId()) {
			return nil, status.Error(codes.NotFound, "memory not found in project")
		}
	}
	if memoryExpired(v) {
		return nil, status.Error(codes.NotFound, "memory has expired")
	}
	return v, nil
}
func (s MemoryServer) Get(ctx context.Context, r *resource.MemoryGetRequest) (*resource.Memory, error) {
	out := &resource.Memory{}
	if ok, err := s.remote(ctx, r.GetRef().GetProject(), resource.MemoryService_Get_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	m, err := s.local(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	if r.GetSelect() == nil {
		return resource.Memory_builder{Id: m.GetId()}.Build(), nil
	}
	return s.Next().Memory().Get(ctx, resource.MemoryGetRequest_builder{Ref: m.Ref(), Select: r.GetSelect()}.Build())
}
func (s MemoryServer) Add(ctx context.Context, r *resource.MemoryAddRequest) (*resource.Memory, error) {
	out := &resource.Memory{}
	if ok, err := s.remote(ctx, r.GetProject(), resource.MemoryService_Add_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	if r.HasId() || r.HasDateCreated() || r.GetRevision() != 0 || r.HasDateDeleted() {
		return nil, memoryInvalid("identity, revision and deletion time are server controlled")
	}
	p, err := s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject()}.Build())
	if err != nil {
		return nil, err
	}
	if err = memoryFields(r.GetName(), r.GetContent(), r.GetLabels()); err != nil {
		return nil, err
	}
	if len(r.GetClientId()) > 256 {
		return nil, memoryInvalid("client_id maximum 1024 bytes")
	}
	request := proto.Clone(r).(*resource.MemoryAddRequest)
	request.SetProject(projectRef(projectid.String(p.GetId())))
	if request.GetOwnerSession() != "" {
		owner, err := s.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(request.GetOwnerSession()), Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
		if err != nil {
			if id, e := uuid.Parse(request.GetOwnerSession()); e == nil {
				owner, err = s.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Id: id[:]}.Build(), Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
			}
		}
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(owner.GetProject().GetId(), p.GetId()) {
			return nil, memoryInvalid("owner session belongs to another project")
		}
		request.SetOwnerSession(projectid.String(owner.GetId()))
	}
	if request.GetClientId() == "" {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		request.SetClientId(hex.EncodeToString(b[:]))
	}
	existing, err := s.Next().Memory().Get(ctx, resource.MemoryGetRequest_builder{Ref: resource.MemoryByClientId(request.GetClientId()), Select: memoryAll()}.Build())
	if err == nil {
		if !bytes.Equal(existing.GetProject().GetId(), p.GetId()) || existing.GetOwnerSession() != request.GetOwnerSession() {
			return nil, status.Error(codes.AlreadyExists, "client_id belongs to a different owner/project")
		}
		if memoryExpired(existing) {
			return nil, status.Error(codes.AlreadyExists, "client_id belongs to expired memory")
		}
		return existing, nil
	}
	if status.Code(err) != codes.NotFound {
		return nil, err
	}
	if request.GetAlias() == "" {
		hash := sha256.Sum256([]byte(request.GetClientId()))
		request.SetAlias("memory-" + hex.EncodeToString(hash[:8]))
	}
	request.SetRevision(1)
	return s.Next().Memory().Add(ctx, request)
}
func (s MemoryServer) Patch(ctx context.Context, r *resource.MemoryPatchRequest) (*resource.Memory, error) {
	out := &resource.Memory{}
	if ok, err := s.remote(ctx, r.GetRef().GetProject(), resource.MemoryService_Patch_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	m, err := s.local(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	if m.HasDateDeleted() {
		return nil, status.Error(codes.FailedPrecondition, "restore memory before editing")
	}
	if !r.HasDateUpdated() || r.GetDateUpdatedForce() || r.HasRevision() || r.HasDateDeleted() || r.GetDateDeletedNull() {
		return nil, memoryInvalid("date_updated from the last read is required; revision and deletion are server controlled")
	}
	request := proto.Clone(r).(*resource.MemoryPatchRequest)
	request.SetRef(m.Ref())
	name, content, labels := m.GetName(), m.GetContent(), m.GetLabels()
	if r.HasName() {
		name = r.GetName()
	}
	if r.HasContent() {
		content = r.GetContent()
		if !r.HasBaseRevision() || r.GetBaseRevision() != m.GetRevision() {
			return nil, status.Error(codes.Aborted, "memory revision changed; read latest before updating")
		}
		if content != m.GetContent() {
			request.SetRevision(m.GetRevision() + 1)
		}
	}
	if len(r.GetLabels()) > 0 {
		labels = r.GetLabels()
	}
	if r.GetClearLabels() {
		if len(labels) > 0 && len(r.GetLabels()) > 0 {
			return nil, memoryInvalid("labels and clear_labels cannot be combined")
		}
		labels = nil
		request.SetLabels(nil) /* an explicit map clear is compiled below */
	}
	if err = memoryFields(name, content, labels); err != nil {
		return nil, err
	}
	var saved *resource.Memory
	if r.GetClearLabels() {
		saved, err = s.patchClearLabels(ctx, m, request)
	} else {
		saved, err = s.Next().Memory().Patch(ctx, request)
	}
	if status.Code(err) == codes.FailedPrecondition {
		return nil, status.Error(codes.Aborted, "memory metadata version changed; read latest before updating")
	}
	return saved, err
}
func (s MemoryServer) Apply(context.Context, *resource.MemoryApplyRequest) (*resource.Memory, error) {
	return nil, status.Error(codes.PermissionDenied, "use versioned Memory.Patch")
}
func (s MemoryServer) Erase(ctx context.Context, r *resource.MemoryRef) (*resource.MemoryEraseResponse, error) {
	out := &resource.MemoryEraseResponse{}
	if ok, err := s.remote(ctx, r.GetProject(), resource.MemoryService_Erase_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	m, err := s.local(ctx, r)
	if status.Code(err) == codes.NotFound {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if m.HasDateDeleted() {
		return out, nil
	}
	_, err = s.Next().Memory().Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), DateUpdated: m.GetDateUpdated(), DateDeleted: timestamppb.New(time.Now().Add(30 * 24 * time.Hour))}.Build())
	if err != nil {
		return nil, err
	}
	out.SetErased(true)
	return out, nil
}
func (s MemoryServer) Restore(ctx context.Context, r *resource.MemoryRestoreRequest) (*resource.Memory, error) {
	out := &resource.Memory{}
	if ok, err := s.remote(ctx, r.GetRef().GetProject(), resource.MemoryService_Restore_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	m, err := s.local(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	if !m.HasDateDeleted() {
		return nil, status.Error(codes.FailedPrecondition, "memory is not in trash")
	}
	if r.GetDateUpdated() == nil {
		return nil, memoryInvalid("date_updated is required")
	}
	return s.Next().Memory().Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), DateUpdated: r.GetDateUpdated(), DateDeletedNull: ptr(true)}.Build())
}
func (s MemoryServer) Purge(ctx context.Context, r *resource.MemoryPurgeRequest) (*resource.MemoryEraseResponse, error) {
	out := &resource.MemoryEraseResponse{}
	if ok, err := s.remote(ctx, r.GetRef().GetProject(), resource.MemoryService_Purge_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	m, err := s.local(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	if !m.HasDateDeleted() {
		return nil, status.Error(codes.FailedPrecondition, "move memory to trash first")
	}
	if r.GetDateUpdated() == nil || !r.GetDateUpdated().AsTime().Equal(m.GetDateUpdated().AsTime()) {
		return nil, status.Error(codes.Aborted, "memory changed; refresh before purging")
	}
	return s.purgeVersioned(ctx, m)
}
func memoryScope(m *resource.Memory, scope resource.MemoryScope) bool {
	return !memoryExpired(m) && m.HasDateDeleted() == (scope == resource.MemoryScope_MEMORY_SCOPE_TRASH)
}
func (s MemoryServer) List(ctx context.Context, r *resource.MemoryListRequest) (*resource.MemoryListResponse, error) {
	out := &resource.MemoryListResponse{}
	if err := s.effect(); err != nil {
		return nil, err
	}
	if r.GetScope() < 0 || r.GetScope() > resource.MemoryScope_MEMORY_SCOPE_TRASH {
		return nil, memoryInvalid("unknown memory scope")
	}
	filters, err := s.memoryFilters(ctx, r)
	if err != nil {
		return nil, err
	}
	// Resolve aliases on this connection before partitioning requests by project.
	request := proto.Clone(r).(*resource.MemoryListRequest)
	request.SetFilters(filters)
	request.ClearSession()
	if ok, err := s.remote(ctx, r.GetProject(), resource.MemoryService_List_FullMethodName, request, out); ok || err != nil {
		return out, err
	}
	limit := int(r.GetSize())
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	cursor := r.GetAfter()
	items := []*resource.Memory{}
	for {
		page, err := s.Next().Memory().List(ctx, resource.MemoryListRequest_builder{Filters: filters, Size: 500, After: cursor, Reverse: r.GetReverse()}.Build())
		if err != nil {
			return nil, err
		}
		for _, m := range page.GetItems() {
			if !memoryScope(m, r.GetScope()) {
				continue
			}
			if r.GetQuery() != "" && !strings.Contains(strings.ToLower(m.GetName()+"\n"+m.GetContent()), strings.ToLower(r.GetQuery())) {
				continue
			}
			m.SetContent("")
			items = append(items, m)
		}
		if len(items) > limit {
			items = items[:limit]
			last := items[len(items)-1]
			return s.memoryListPage(ctx, r, filters, items, last)
		}
		if page.GetNext() == "" {
			out.SetItems(items)
			return out, nil
		}
		cursor = page.GetNext()
	}
}

// Keep EOF newline changes visible without synthesizing a blank content line.
func memoryDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	lines[len(lines)-1] += "\n\\ No newline at end of file\n"
	return lines
}

func (s MemoryServer) Read(ctx context.Context, r *resource.MemoryReadRequest) (*resource.MemoryReadReply, error) {
	out := &resource.MemoryReadReply{}
	if ok, err := s.remote(ctx, r.GetRef().GetProject(), resource.MemoryService_Read_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	m, err := s.local(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	revision := r.GetRevision()
	if revision == 0 {
		revision = m.GetRevision()
	}
	var content string
	if revision == m.GetRevision() {
		content = m.GetContent()
	} else if err = s.shared.db.QueryRowContext(ctx, `SELECT content FROM memory_version WHERE memory_id=? AND revision=?`, projectid.String(m.GetId()), revision).Scan(&content); err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Error(codes.NotFound, "memory version not retained")
		}
		return nil, err
	}
	m.SetContent("")
	out.SetMemory(m)
	out.SetRevision(revision)
	out.SetFormat(resource.MemoryReadFormat_MEMORY_READ_FORMAT_FULL)
	out.SetContent(content)
	format := r.GetFormat()
	if format < 0 || format > resource.MemoryReadFormat_MEMORY_READ_FORMAT_DIFF {
		return nil, memoryInvalid("unknown read format")
	}
	if format == resource.MemoryReadFormat_MEMORY_READ_FORMAT_FULL || format == resource.MemoryReadFormat_MEMORY_READ_FORMAT_UNSPECIFIED || r.GetBaseRevision() == 0 {
		return out, nil
	}
	var base string
	err = s.shared.db.QueryRowContext(ctx, `SELECT content FROM memory_version WHERE memory_id=? AND revision=?`, projectid.String(m.GetId()), r.GetBaseRevision()).Scan(&base)
	if err == sql.ErrNoRows {
		out.SetFallbackReason("base revision is not retained")
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: memoryDiffLines(base), B: memoryDiffLines(content), FromFile: fmt.Sprintf("%s@%d", m.GetName(), r.GetBaseRevision()), ToFile: fmt.Sprintf("%s@%d", m.GetName(), revision), Context: 5})
	if err != nil {
		return nil, err
	}
	if format == resource.MemoryReadFormat_MEMORY_READ_FORMAT_AUTO && len(diff) >= len(content) {
		out.SetFallbackReason("full text is no larger than the diff")
		return out, nil
	}
	out.SetBaseRevision(r.GetBaseRevision())
	out.SetFormat(resource.MemoryReadFormat_MEMORY_READ_FORMAT_DIFF)
	out.SetContent("")
	out.SetDiff(diff)
	return out, nil
}
func (s MemoryServer) Versions(ctx context.Context, r *resource.MemoryVersionsRequest) (*resource.MemoryVersionsReply, error) {
	out := &resource.MemoryVersionsReply{}
	if ok, err := s.remote(ctx, r.GetRef().GetProject(), resource.MemoryService_Versions_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	m, err := s.local(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	limit := int(r.GetSize())
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	before := r.GetBeforeRevision()
	if before == 0 {
		before = m.GetRevision() + 1
	}
	rows, err := s.shared.db.QueryContext(ctx, `SELECT revision,content_hash,date_created FROM memory_version WHERE memory_id=? AND revision<? ORDER BY revision DESC LIMIT ?`, projectid.String(m.GetId()), before, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []*resource.MemoryVersion
	for rows.Next() {
		var rev uint64
		var hash string
		var created time.Time
		if err = rows.Scan(&rev, &hash, &created); err != nil {
			return nil, err
		}
		versions = append(versions, resource.MemoryVersion_builder{MemoryId: m.GetId(), Revision: rev, ContentHash: hash, DateCreated: timestamppb.New(created)}.Build())
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(versions) > limit {
		versions = versions[:limit]
		out.SetNextRevision(versions[len(versions)-1].GetRevision())
	}
	out.SetVersions(versions)
	return out, nil
}

// The cursor is scoped to its project filter, never interpreted as an audit log.
type memoryCursor struct {
	Epoch    string `json:"epoch"`
	Project  string `json:"project"`
	Sequence uint64 `json:"sequence"`
	Baseline bool   `json:"baseline"`
	After    string `json:"after,omitempty"`
}

func (s MemoryServer) Changes(ctx context.Context, r *resource.MemoryChangesRequest) (*resource.MemoryChangesReply, error) {
	out := &resource.MemoryChangesReply{}
	if ok, err := s.remote(ctx, r.GetProject(), resource.MemoryService_Changes_FullMethodName, r, out); ok || err != nil {
		return out, err
	}
	project := ""
	if r.GetProject() != nil {
		p, err := s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject()}.Build())
		if err != nil {
			return nil, err
		}
		project = projectid.String(p.GetId())
	}
	c := memoryCursor{Project: project, Baseline: true}
	if r.GetCursor() != "" {
		b, e := base64.RawURLEncoding.DecodeString(r.GetCursor())
		if e != nil || len(b) > 4096 || json.Unmarshal(b, &c) != nil {
			return nil, memoryInvalid("invalid or out-of-scope memory cursor")
		}
	}
	var epoch string
	var floor, maxSeq uint64
	if err := s.shared.db.QueryRowContext(ctx, `SELECT value FROM memory_meta WHERE key='epoch'`).Scan(&epoch); err != nil {
		return nil, err
	}
	if err := s.shared.db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM memory_meta WHERE key='changes_floor'`).Scan(&floor); err != nil {
		return nil, err
	}
	if err := s.shared.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM memory_change`).Scan(&maxSeq); err != nil {
		return nil, err
	}
	if r.GetCursor() != "" {
		if c.Epoch != epoch || c.Sequence < floor || c.Sequence > maxSeq {
			out.SetResetRequired(true)
			return out, nil
		}
		if c.Project != project {
			return nil, memoryInvalid("memory cursor belongs to another project")
		}
	} else {
		c.Epoch = epoch
	}
	limit := int(r.GetSize())
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if r.GetCursor() == "" {
		if err := s.shared.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM memory_change`).Scan(&c.Sequence); err != nil {
			return nil, err
		}
	}
	out.SetBaseline(c.Baseline)
	if c.Baseline {
		// The baseline is a metadata list at the initial change watermark. Changes
		// after it are drained once pagination completes, so concurrent writes survive.
		filters := []*resource.MemoryFilter{}
		if project != "" {
			filters = append(filters, resource.MemoryFilter_builder{Project: projectRef(project)}.Build())
		}
		page, err := s.Next().Memory().List(ctx, resource.MemoryListRequest_builder{Filters: filters, After: c.After, Size: int32(limit)}.Build())
		if err != nil {
			return nil, err
		}
		for _, m := range page.GetItems() {
			if m.HasDateDeleted() {
				continue
			}
			out.SetChanges(append(out.GetChanges(), resource.MemoryChange_builder{Kind: "created", MemoryId: m.GetId(), ProjectId: m.GetProject().GetId(), Revision: m.GetRevision()}.Build()))
		}
		c.After = page.GetNext()
		out.SetHasMore(c.After != "")
		if c.After == "" {
			c.Baseline = false
			var seq uint64
			if err = s.shared.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM memory_change`).Scan(&seq); err != nil {
				return nil, err
			}
			out.SetHasMore(seq > c.Sequence)
		}
	} else {
		q := `SELECT sequence,memory_id,project_id,revision,kind FROM memory_change WHERE sequence>?`
		args := []any{c.Sequence}
		if project != "" {
			q += ` AND project_id=?`
			args = append(args, project)
		}
		q += ` ORDER BY sequence LIMIT ?`
		args = append(args, limit+1)
		rows, err := s.shared.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			if len(out.GetChanges()) == limit {
				out.SetHasMore(true)
				break
			}
			var seq, rev uint64
			var id, p, kind string
			if err = rows.Scan(&seq, &id, &p, &rev, &kind); err != nil {
				return nil, err
			}
			uid, e := uuid.Parse(id)
			if e != nil {
				return nil, e
			}
			out.SetChanges(append(out.GetChanges(), resource.MemoryChange_builder{Sequence: seq, Kind: kind, MemoryId: uid[:], ProjectId: projectid.Bytes(p), Revision: rev}.Build()))
			c.Sequence = seq
		}
		if err = rows.Err(); err != nil {
			return nil, err
		}
	}
	if !c.Baseline && !out.GetHasMore() {
		c.Sequence = maxSeq
	}
	b, _ := json.Marshal(c)
	out.SetCursor(base64.RawURLEncoding.EncodeToString(b))
	return out, nil
}
func (s MemoryServer) Watch(r *resource.MemoryWatchRequest, out grpc.ServerStreamingServer[resource.MemoryWatchResponse]) error {
	return s.watchMemory(r, out)
}
