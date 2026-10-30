package lifecycle

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/internal/ent"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/trail"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
)

// RetainAudit reuses payday's batched, policy-aware deletion. It deliberately
// neither measures audit bytes nor vacuums the shared resource database.
func (s Layer) RetainAudit(ctx context.Context, now time.Time, days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, s.shared.db)))
	p := trail.Policy{Keep: trail.Keep{Retain: time.Duration(days) * 24 * time.Hour, Discard: true}}
	n, _, err := p.Collect(ctx, pd.TrailStore(client), trail.Kinds{}, now.Add(-time.Duration(days)*24*time.Hour))
	return n, err
}
