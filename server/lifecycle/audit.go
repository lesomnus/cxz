package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/bare"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/audit"
	"github.com/lesomnus/protobuf-patch/patchpb"
	"google.golang.org/protobuf/proto"
)

type projectionAuditKey struct{}

// Runtime projections already have a journal. Updating their resource view must
// still notify Watch subscribers, without duplicating the journal in audit.
func projectionContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, projectionAuditKey{}, true)
}

type managementAudit struct{ bare.Recorder }

func (r managementAudit) Snapshots(method string) bare.SnapshotOptions {
	if next, ok := r.Recorder.(bare.SnapshotRecorder); ok {
		return next.Snapshots(method)
	}
	return bare.SnapshotOptions{}
}

func (r managementAudit) Record(ctx context.Context, store bare.Server, change bare.Change) error {
	if projection, _ := ctx.Value(projectionAuditKey{}).(bool); projection {
		return nil
	}
	key, err := audit.Identifier(change.Key)
	if err != nil {
		return err
	}
	switch key.Domain() {
	case pd.OperatorDomain, pd.SessionReadDomain, pd.NotificationRecordDomain, pd.NotificationReceiptRecordDomain:
		// Read cursors and notification delivery/read state are operational data,
		// not independent management changes to retain in a second permanent log.
		return nil
	}
	// Body history belongs to MemoryVersion, not to duplicate audit payloads.
	if key.Domain() == pd.MemoryDomain {
		if change.Patch != nil {
			doc := proto.Clone(change.Patch).(*patchpb.Patch)
			entries := []*patchpb.Entry{}
			for _, e := range doc.GetDelta().GetEntries() {
				body := false
				if path := e.GetPath().GetSegments(); len(path) > 0 {
					body = path[0].GetField().GetNumber() == 16 || path[0].GetField().GetName() == "content"
				}
				for _, sel := range e.GetTargets().GetSelectors() {
					body = body || sel.GetKey().GetField().GetNumber() == 16 || sel.GetKey().GetField().GetName() == "content"
				}
				if !body {
					entries = append(entries, e)
				}
			}
			if len(entries) == 0 {
				change.Patch = nil
			} else {
				doc.GetDelta().SetEntries(entries)
				change.Patch = doc
			}
		}
		for _, image := range []*proto.Message{&change.Previous, &change.Value} {
			if m, ok := (*image).(*resource.Memory); ok {
				copy := proto.Clone(m).(*resource.Memory)
				copy.SetContent("")
				*image = copy
			}
		}
	}
	return r.Recorder.Record(ctx, store, change)
}

var _ bare.SnapshotRecorder = managementAudit{}
