package lifecycle

import (
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type auditFixture struct {
	*notificationFixture
	snapshots atomic.Int64
}

func (f *auditFixture) ResourceSnapshot(ctx context.Context) (*api.ProjectList, *api.SessionList, error) {
	f.snapshots.Add(1)
	return f.fixture.ResourceSnapshot(ctx)
}

func (f *auditFixture) Get(context.Context, *api.SessionRef) (*api.Session, error) {
	return proto.CloneOf(f.s), nil
}

func (f *auditFixture) Create(_ context.Context, r *api.CreateRequest) (*api.Session, error) {
	f.s = proto.CloneOf(f.s)
	f.s.Id = "333333333333333333333333"
	f.s.CreateId = r.ClientId
	f.s.Title = r.Title
	return proto.CloneOf(f.s), nil
}

func (f *auditFixture) Stop(context.Context, *api.Control) (*api.Receipt, error) {
	f.s.State = "stopped"
	return &api.Receipt{Status: "stopped"}, nil
}

func (f *auditFixture) Send(_ context.Context, r *api.Input) (*api.Receipt, error) {
	f.append("input", r.Text)
	f.s.State = "working"
	return &api.Receipt{ClientId: r.ClientId, Status: "accepted"}, nil
}

func (f *auditFixture) UpdateAgent(context.Context, *api.AgentUpdateInput) (*api.AgentUpdateStatus, error) {
	return &api.AgentUpdateStatus{Ready: true}, nil
}

func (f *auditFixture) GetHistoryFloor(context.Context, *api.SessionRef) (*api.HistoryFloorReply, error) {
	return &api.HistoryFloorReply{}, nil
}

func (f *auditFixture) Watch(_ *api.WatchRequest, stream grpc.ServerStreamingServer[api.Event]) error {
	f.s.State = "idle"
	f.append("state", "idle")
	return stream.Send(f.events[len(f.events)-1])
}

func (f *auditFixture) AuxStatus(context.Context, *api.AuxStatusInput) (*api.AuxState, error) {
	f.s.Title = "Automatic title"
	return &api.AuxState{Title: f.s.Title}, nil
}

func auditStack(t *testing.T) (context.Context, *sql.DB, resource.Server, Layer, *auditFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db") + "?_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &auditFixture{notificationFixture: notificationTestFixture()}
	f.p.Id = projectid.Canonical(f.p.Id)
	f.s.ProjectId = f.p.Id
	f.s.AuthBinding = accounts.BindingID(f.p.Id, f.s.Account, f.s.AuthBackend)
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](stack)
	return ctx, db, stack, layer, f
}

func assertAuditCount(t *testing.T, ctx context.Context, db *sql.DB, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("audit count: got %d, want %d", got, want)
	}
}

func TestAuditConversationAndPollingStaySilent(t *testing.T) {
	ctx, db, stack, layer, f := auditStack(t)
	client := resource.NewSessionServiceClient(taskConn(t, stack))
	ref := sessionRef(f.s.Id)
	get := resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build()
	if _, err := client.Get(ctx, get); err != nil {
		t.Fatal(err)
	}
	assertAuditCount(t, ctx, db, 0)
	if _, err := client.Send(ctx, resource.SessionSendRequest_builder{Ref: ref, RunId: ptr(f.s.RunId), ClientId: ptr("send"), Text: ptr("conversation body")}.Build()); err != nil {
		t.Fatal(err)
	}
	if err := layer.RefreshSession(ctx, f.s.Id); err != nil {
		t.Fatal(err)
	}
	before := f.snapshots.Load()
	for range 4 {
		f.s.LastSeq++
		if _, err := client.UpdateAgent(ctx, resource.SessionUpdateRequest_builder{Ref: ref, RunId: ptr(f.s.RunId)}.Build()); err != nil {
			t.Fatal(err)
		}
		if _, err := client.GetHistoryFloor(ctx, resource.SessionHistoryFloorRequest_builder{Ref: ref}.Build()); err != nil {
			t.Fatal(err)
		}
		if _, err := client.History(ctx, resource.SessionEventsRequest_builder{Ref: ref}.Build()); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Get(ctx, get); err != nil {
			t.Fatal(err)
		}
	}
	if f.snapshots.Load() != before {
		t.Fatal("readiness/history polling performed full reconciliation")
	}
	events, err := client.Events(ctx, resource.SessionEventsRequest_builder{Ref: ref}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Recv(); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Recv(); err != io.EOF {
		t.Fatal(err)
	}
	if _, err := client.AuxStatus(ctx, resource.AuxStatusRequest_builder{Ref: ref}.Build()); err != nil {
		t.Fatal(err)
	}
	current, err := client.Get(ctx, get)
	if err != nil || current.GetName() != "Automatic title" || current.GetStatus().GetState() != "idle" {
		t.Fatal("projection stopped updating", current, err)
	}
	assertAuditCount(t, ctx, db, 0)
}

func TestAuditProjectionStillPublishesWatch(t *testing.T) {
	ctx, db, stack, layer, f := auditStack(t)
	client := resource.NewSessionServiceClient(taskConn(t, stack))
	stream, err := client.Watch(ctx, resource.SessionWatchRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Ref: sessionRef(f.s.Id)}.Build()}}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if first, err := stream.Recv(); err != nil || len(first.GetItems()) != 1 {
		t.Fatal(first, err)
	}
	f.s.State = "working"
	f.s.LastSeq++
	if err := layer.RefreshSession(ctx, f.s.Id); err != nil {
		t.Fatal(err)
	}
	next, err := stream.Recv()
	if err != nil || len(next.GetItems()) != 1 || next.GetItems()[0].GetValue().GetStatus().GetState() != "working" {
		t.Fatal("audit suppression hid a live state change", next, err)
	}
	f.p.State = "stopped"
	if err := layer.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	assertAuditCount(t, ctx, db, 0)
}

func TestAuditExplicitManagementIsRetained(t *testing.T) {
	ctx, db, stack, _, f := auditStack(t)
	client := resource.NewSessionServiceClient(taskConn(t, stack))
	if _, err := client.Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(f.s.Id)}.Build()); err != nil {
		t.Fatal(err)
	}
	created, err := client.Add(ctx, resource.SessionAddRequest_builder{
		Project: projectRef(f.p.Id), Account: accountRef(f.s.Account), AuthBinding: bindingRef(f.s.AuthBinding),
		Name: "Created session", Agent: f.s.Agent, ClientId: "create-another",
	}.Build())
	if err != nil {
		t.Fatal(err)
	}
	assertAuditCount(t, ctx, db, 1)
	ref := resource.SessionRef_builder{Id: created.GetId()}.Build()
	if _, err := client.Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Name: ptr("Manual title")}.Build()); err != nil {
		t.Fatal(err)
	}
	assertAuditCount(t, ctx, db, 2)
	if _, err := client.Erase(ctx, ref); err != nil {
		t.Fatal(err)
	}
	assertAuditCount(t, ctx, db, 3)
	if _, err := client.Restore(ctx, ref); err != nil {
		t.Fatal(err)
	}
	assertAuditCount(t, ctx, db, 4)
	rows, err := db.QueryContext(ctx, "SELECT action,length(patch) FROM audit ORDER BY date_created")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for _, want := range []string{"/cxz.SessionService/Add", "/cxz.SessionService/Patch", "/cxz.SessionService/Erase", "/cxz.SessionService/Restore"} {
		var got string
		var patchBytes int
		if !rows.Next() {
			t.Fatal("missing management audit", want)
		}
		if err := rows.Scan(&got, &patchBytes); err != nil || got != want || patchBytes == 0 {
			t.Fatal(got, want, err)
		}
	}
}

func TestAuditNotificationsAndReadCursorsStaySilent(t *testing.T) {
	f := notificationTestFixture()
	ctx, db, stack, layer := notificationTestStack(t, filepath.Join(t.TempDir(), "resources.db"), f)
	if err := layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	f.append("input", "prompt")
	f.append("assistant", "response")
	f.append("turn_end", "completed")
	if err := layer.CollectNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if len(notificationPage(t, ctx, stack, layer, true).GetItems()) != 1 {
		t.Fatal("notification was lost")
	}
	state, err := stack.Session().ReadState(ctx, resource.SessionReadStateRequest_builder{Ref: sessionRef(f.s.Id)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Session().MarkRead(ctx, resource.SessionMarkReadRequest_builder{Ref: sessionRef(f.s.Id), Generation: state.GetGeneration(), ThroughSeq: f.s.LastSeq}.Build()); err != nil {
		t.Fatal(err)
	}
	if len(notificationPage(t, ctx, stack, layer, true).GetItems()) != 0 {
		t.Fatal("read notification stayed unread")
	}
	assertAuditCount(t, ctx, db, 0)
}
