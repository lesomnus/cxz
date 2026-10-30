package lifecycle

import (
	"github.com/lesomnus/cxz/resource"
	"testing"
	"time"
)

func TestAuditAgeRetentionUsesPaydayAndPreservesRecentWrites(t *testing.T) {
	ctx, db, stack, layer, f := auditStack(t)
	client := resource.NewSessionServiceClient(taskConn(t, stack))
	if _, err := client.Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(f.s.Id)}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(f.s.Id), Name: ptr("Manual title")}.Build()); err != nil {
		t.Fatal(err)
	}
	assertAuditCount(t, ctx, db, 1)
	now := time.Now()
	n, err := layer.RetainAudit(ctx, now, 90)
	if err != nil || n != 0 {
		t.Fatal(n, err)
	}
	// Use the deployed driver's time representation, not raw integer guesses.
	if _, err = db.ExecContext(ctx, "UPDATE audit SET date_created=?", now.Add(-91*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	n, err = layer.RetainAudit(ctx, now, 0)
	if err != nil || n != 0 {
		t.Fatal(n, err)
	}
	assertAuditCount(t, ctx, db, 1)
	n, err = layer.RetainAudit(ctx, now, 90)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	assertAuditCount(t, ctx, db, 0)
	if _, err = client.Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(f.s.Id), Name: ptr("Still usable")}.Build()); err != nil {
		t.Fatal(err)
	}
	assertAuditCount(t, ctx, db, 1)
}
