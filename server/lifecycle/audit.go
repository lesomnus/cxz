package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/server/bare"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/audit"
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
	return r.Recorder.Record(ctx, store, change)
}

var _ bare.SnapshotRecorder = managementAudit{}
