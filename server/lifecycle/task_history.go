package lifecycle

import (
	"context"
	entAudit "github.com/lesomnus/cxz/internal/ent/audit"
	"github.com/lesomnus/cxz/internal/ent/predicate"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/bare"
	"github.com/lesomnus/cxz/server/pd"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"uuid"
)

// Task policy has already resolved the project and read the requested Task.
// The legacy runtime transport authenticates connections, without a payday
// actor frame. Bind the audit query to that authorized object rather than
// manufacturing an actor/tenant or exposing the generic audit service.
type taskHistoryScope struct {
	bare.Scope
	subject uuid.UUID
}

func (s taskHistoryScope) AuditScope(context.Context) (predicate.Audit, error) {
	return entAudit.ObjectIdEQ(s.subject), nil
}

func (s taskInventory) History(ctx context.Context, r *resource.TaskHistoryRequest) (*resource.AuditListResponse, error) {
	if s.shared.resources == nil {
		return nil, status.Error(codes.Internal, "task audit storage unavailable")
	}
	var subject uuid.UUID
	copy(subject[:], r.GetRef().GetId())
	read, err := pd.NewSink(s.shared.resources, bare.WithScope(taskHistoryScope{Scope: pd.Wall(), subject: subject}))
	if err != nil {
		return nil, err
	}
	page, err := read.Audit().List(ctx, resource.AuditListRequest_builder{Size: r.GetSize(), After: r.GetAfter()}.Build())
	if err != nil {
		return nil, err
	}
	// The Task endpoint answers with change metadata, not actor credentials,
	// trace context, snapshots or potentially large patch payloads.
	items := make([]*resource.Audit, 0, len(page.GetItems()))
	for _, row := range page.GetItems() {
		items = append(items, resource.Audit_builder{Id: row.GetId(), ObjectId: row.GetObjectId(), Domain: row.GetDomain(), Action: row.GetAction(), Fields: row.GetFields(), DateCreated: row.GetDateCreated()}.Build())
	}
	return resource.AuditListResponse_builder{Items: items, Next: page.GetNext()}.Build(), nil
}
