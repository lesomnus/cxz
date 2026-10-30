package lifecycle

import (
	"github.com/lesomnus/cxz/resource"
	"path/filepath"
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

func TestNotificationRetentionKeepsLatestUnreadAndRecentReadHistory(t *testing.T) {
	f := notificationTestFixture()
	ctx, db, stack, layer := notificationTestStack(t, filepath.Join(t.TempDir(), "resources.db"), f)
	if err := layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		f.append("turn_end", "completed")
		if err := layer.CollectNotifications(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := layer.RetainNotifications(ctx, time.Now(), 30); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if page := notificationPage(t, ctx, stack, layer, true); len(page.GetItems()) != 1 {
		t.Fatal(page)
	}
	state, err := stack.Session().ReadState(ctx, resource.SessionReadStateRequest_builder{Ref: sessionRef(f.s.Id)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stack.Session().MarkRead(ctx, resource.SessionMarkReadRequest_builder{Ref: sessionRef(f.s.Id), Generation: state.GetGeneration(), ThroughSeq: f.s.LastSeq}.Build()); err != nil {
		t.Fatal(err)
	}
	if n, err := layer.RetainNotifications(ctx, time.Now(), 30); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE notificationreceiptrecord SET data_read=?", time.Now().Add(-31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, err := layer.RetainNotifications(ctx, time.Now(), 30); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	for _, table := range []string{"notificationrecord", "notificationreceiptrecord"} {
		var count int
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	var cursor int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM session_notification_cursor").Scan(&cursor); err != nil || cursor != 1 {
		t.Fatal(cursor, err)
	}
	if err = layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if page := notificationPage(t, ctx, stack, layer, false); len(page.GetItems()) != 0 {
		t.Fatal("expired notifications republished", page)
	}
}
