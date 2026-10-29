package lifecycle

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type notificationFixture struct {
	*fixture
	events []*api.Event
	floor  uint64
}

func (f *notificationFixture) NotificationHead(context.Context, string) (uint64, error) {
	return f.s.LastSeq, nil
}
func (f *notificationFixture) NotificationBoundary(context.Context, string) (uint64, error) {
	return f.floor, nil
}
func (f *notificationFixture) History(_ context.Context, r *api.WatchRequest) (*api.EventBatch, error) {
	out := &api.EventBatch{}
	for _, e := range f.events {
		if e.Seq > r.AfterSeq {
			out.Events = append(out.Events, e)
			if len(out.Events) >= int(r.Limit) {
				break
			}
		}
	}
	return out, nil
}
func (f *notificationFixture) append(kind, text string) {
	f.s.LastSeq++
	f.events = append(f.events, &api.Event{SessionId: f.s.Id, RunId: f.s.RunId, Seq: f.s.LastSeq, Kind: kind, Text: text})
}
func notificationTestStack(t *testing.T, path string, f *notificationFixture) (context.Context, *sql.DB, resource.Server, Layer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	t.Cleanup(cancel)
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + path + "?_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stack, err := Build(ctx, db, f, nil, Options{Notifications: true})
	if err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](stack)
	return ctx, db, stack, layer
}
func notificationTestFixture() *notificationFixture {
	f := &notificationFixture{fixture: &fixture{
		p: &api.Project{Id: "111111111111111111111111", Workspace: "/workspace/test", Name: "test", State: "running"},
		s: &api.Session{Id: "222222222222222222222222", ProjectId: "111111111111111111111111", Workspace: "/workspace/test", CreateId: "notify-test", Agent: "codex", RunId: "run-1", State: "idle", CreatedAt: time.Now().UnixMilli(), Account: "test-codex", AuthBackend: accounts.ProjectLocalOAuth},
	}}
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	return f
}
func notificationPage(t *testing.T, ctx context.Context, stack resource.Server, layer Layer, unread bool) *resource.NotificationReadResponse {
	t.Helper()
	v, err := stack.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef("222222222222222222222222"), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	page, err := layer.Notifications().Read(ctx, resource.NotificationReadRequest_builder{Topics: []*resource.NotificationTopic{resource.NotificationTopic_builder{ResourceId: v.GetProject().GetId()}.Build()}, Filter: resource.NotificationReadFilter_builder{UnreadOnly: unread}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestNotificationsPersistAndShareReadState(t *testing.T) {
	f := notificationTestFixture()
	path := filepath.Join(t.TempDir(), "resources.db")
	ctx, db, stack, layer := notificationTestStack(t, path, f)
	if err := layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	f.append("input", "hello")
	f.append("assistant", "answer")
	f.append("turn_end", "completed")
	if err := layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	page := notificationPage(t, ctx, stack, layer, true)
	if len(page.GetItems()) != 1 || page.GetItems()[0].GetNotification().GetKind() != SessionCompleted {
		t.Fatalf("completion missing: %v", page)
	}
	state, err := stack.Session().ReadState(ctx, resource.SessionReadStateRequest_builder{Ref: sessionRef(f.s.Id)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	for _, seq := range []uint64{2, 3, 1} {
		seen, err := stack.Session().MarkRead(ctx, resource.SessionMarkReadRequest_builder{Ref: sessionRef(f.s.Id), Generation: state.GetGeneration(), ThroughSeq: seq}.Build())
		if err != nil {
			t.Fatal(err)
		}
		if seq == 1 && seen.GetThroughSeq() != 3 {
			t.Fatal("read cursor regressed")
		}
		unread := notificationPage(t, ctx, stack, layer, true)
		if (len(unread.GetItems()) > 0) != (seq == 2) {
			t.Fatalf("read boundary %d: %v", seq, unread)
		}
	}
	all := notificationPage(t, ctx, stack, layer, false)
	readAt := all.GetItems()[0].GetReceipt().GetDataRead().AsTime()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, _, stack, layer = notificationTestStack(t, path, f)
	if err := layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := stack.Session().ReadState(ctx, resource.SessionReadStateRequest_builder{Ref: sessionRef(f.s.Id)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if restored.GetThroughSeq() != 3 || !bytes.Equal(restored.GetReaderId(), state.GetReaderId()) || restored.GetGeneration() != state.GetGeneration() {
		t.Fatalf("identity/cursor not durable: %v", restored)
	}
	if len(notificationPage(t, ctx, stack, layer, true).GetItems()) != 0 {
		t.Fatal("read notification became unread after restart")
	}
	if !notificationPage(t, ctx, stack, layer, false).GetItems()[0].GetReceipt().GetDataRead().AsTime().Equal(readAt) {
		t.Fatal("first read time changed")
	}
	f.append("turn_end", "failed")
	if err := layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if p := notificationPage(t, ctx, stack, layer, true); len(p.GetItems()) != 1 || p.GetItems()[0].GetNotification().GetKind() != SessionFailed {
		t.Fatalf("offline catchup: %v", p)
	}
}

func TestNotificationsAcknowledgeBeforePublicationAndReplayIdempotently(t *testing.T) {
	f := notificationTestFixture()
	ctx, db, stack, layer := notificationTestStack(t, filepath.Join(t.TempDir(), "resources.db"), f)
	if err := layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := stack.Session().ReadState(ctx, resource.SessionReadStateRequest_builder{Ref: sessionRef(f.s.Id)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	f.append("input", "hello")
	f.append("turn_end", "completed")
	if _, err = stack.Session().MarkRead(ctx, resource.SessionMarkReadRequest_builder{Ref: sessionRef(f.s.Id), Generation: state.GetGeneration(), ThroughSeq: 2}.Build()); err != nil {
		t.Fatal(err)
	}
	if err = layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if len(notificationPage(t, ctx, stack, layer, true).GetItems()) != 0 {
		t.Fatal("late publication ignored persisted read cursor")
	}
	if _, err = db.ExecContext(ctx, `UPDATE session_notification_cursor SET seq=0`); err != nil {
		t.Fatal(err)
	}
	if err = layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if len(notificationPage(t, ctx, stack, layer, false).GetItems()) != 1 {
		t.Fatal("replayed a publication twice")
	}
	for _, r := range []*resource.SessionMarkReadRequest{
		resource.SessionMarkReadRequest_builder{Ref: sessionRef(f.s.Id), Generation: "stale", ThroughSeq: 1}.Build(),
		resource.SessionMarkReadRequest_builder{Ref: sessionRef(f.s.Id), Generation: state.GetGeneration(), ThroughSeq: 99}.Build(),
	} {
		if _, err = stack.Session().MarkRead(ctx, r); err == nil {
			t.Fatal("invalid read cursor accepted")
		}
	}
	if _, err = layer.Notifications().Add(ctx, &resource.NotificationAddRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("client publication allowed", err)
	}
	if _, err = stack.Operator().Get(ctx, &resource.OperatorGetRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatal("operator store exposed", err)
	}
	var tenants int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM tenant`).Scan(&tenants); err != nil || tenants != 0 {
		t.Fatal("fabricated a tenant", tenants, err)
	}
}

func TestNotificationKindsExcludeToolFailuresAndInterruptions(t *testing.T) {
	for _, e := range []*api.Event{{Kind: "tool", Text: "failed"}, {Kind: "turn_end", Text: "interrupted"}, {Kind: "usage_status"}, {Kind: "approval", Text: "Bash"}} {
		if notificationKind("codex", e) != "" {
			t.Fatalf("unexpected alert: %v", e)
		}
	}
	question := &api.Event{Kind: "approval", Text: "item/tool/requestUserInput", Payload: []byte(`{"params":{"questions":[{"id":"answer","question":"What next?","options":[]}]}}`)}
	if notificationKind("codex", question) != SessionQuestion {
		t.Fatal("free-form question was not recognized")
	}
}
