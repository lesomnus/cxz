package lifecycle

import (
	"context"
	"database/sql"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/taskapitest"
	"github.com/lesomnus/cxz/internal/taskmcp"
	"github.com/lesomnus/cxz/internal/taskmcptest"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

type taskFixture struct {
	*fixture
	lock     sync.Mutex
	sessions []*api.Session
}

func (f *taskFixture) Get(_ context.Context, r *api.SessionRef) (*api.Session, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	for _, s := range f.sessions {
		if s.Id == r.Id {
			return proto.Clone(s).(*api.Session), nil
		}
	}
	return nil, status.Error(codes.NotFound, "session purged")
}
func (f *taskFixture) ResourceSnapshot(context.Context) (*api.ProjectList, *api.SessionList, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	return &api.ProjectList{Projects: []*api.Project{proto.Clone(f.p).(*api.Project)}}, proto.Clone(&api.SessionList{Sessions: f.sessions}).(*api.SessionList), nil
}

func taskStack(t *testing.T) (*taskFixture, resource.Server, *sql.DB) {
	t.Helper()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db") + "?_pragma=foreign_keys(1)", MaxOpenConns: 1}).Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &taskFixture{fixture: &fixture{p: &api.Project{Id: "task-project", Workspace: "/tasks", Name: "Tasks", State: "running"}}}
	for _, id := range []string{"task-session-1", "task-session-2"} {
		f.sessions = append(f.sessions, &api.Session{Id: id, ProjectId: f.p.Id, Workspace: f.p.Workspace, Title: id, CreateId: id, Agent: "codex", Account: "work", AuthBackend: accounts.ProjectLocalOAuth, AuthBinding: accounts.BindingID(f.p.Id, "work", accounts.ProjectLocalOAuth), State: "idle", RunId: id + "-run", CreatedAt: time.Now().UnixMilli()})
	}
	f.s = f.sessions[0]
	stack, err := Build(t.Context(), db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Project().List(t.Context(), &resource.ProjectListRequest{}); err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](stack)
	_, err = layer.Next().Project().Add(t.Context(), resource.ProjectAddRequest_builder{Id: resourceID(7, "other-project"), RuntimeId: "other-project", Workspace: "/other"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	return f, stack, db
}

func taskConn(t *testing.T, stack resource.Server) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	resource.RegisterServer(server, stack)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///task-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestTaskContract(t *testing.T) {
	f, stack, _ := taskStack(t)
	actors := make([]*resource.TaskActor, 0)
	for _, s := range f.sessions {
		actors = append(actors, resource.TaskActor_builder{Session: sessionRef(s.Id), RunId: ptr(s.RunId)}.Build())
	}
	taskapitest.Run(t, taskConn(t, stack), taskapitest.Fixture{
		Project: projectRef(f.p.Id), OtherProject: projectRef("other-project"), Actors: actors,
		Restart: func(id string) {
			f.lock.Lock()
			defer f.lock.Unlock()
			for _, s := range f.sessions {
				if s.Id == id {
					s.RunId += "-restarted"
				}
			}
		},
	})
}

func TestTaskMCPContract(t *testing.T) {
	f, stack, _ := taskStack(t)
	f.sessions[1].State = "waiting_input"
	layer, _ := resource.Find[Layer](stack)
	if err := layer.RefreshSession(t.Context(), f.sessions[1].Id); err != nil {
		t.Fatal(err)
	}
	actors := []taskmcp.Binding{}
	for _, s := range f.sessions {
		actors = append(actors, taskmcp.Binding{Project: f.p.Id, Session: s.Id, Run: s.RunId})
	}
	taskmcptest.Run(t, taskConn(t, stack), taskmcptest.Fixture{
		Actors: actors, OtherProject: projectRef("other-project"),
		Restart: func() taskmcp.Binding {
			f.lock.Lock()
			f.sessions[1].RunId += "-new"
			binding := taskmcp.Binding{Project: f.p.Id, Session: f.sessions[1].Id, Run: f.sessions[1].RunId}
			f.lock.Unlock()
			layer, _ := resource.Find[Layer](stack)
			if err := layer.RefreshSession(t.Context(), binding.Session); err != nil {
				t.Fatal(err)
			}
			return binding
		},
	})
}

func TestTaskPersistenceAndPurgedSessionReference(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := t.Context()
	v, err := stack.Task().Add(ctx, resource.TaskAddRequest_builder{Project: projectRef(f.p.Id), ClientId: "retained", Name: "Keep after purge", SourceSession: f.s.Id, SourceRun: f.s.RunId, SourceSeq: 1}.Build())
	if err != nil {
		t.Fatal(err)
	}
	ref := resource.TaskRef_builder{Id: v.GetId(), Project: projectRef(f.p.Id)}.Build()
	actor := resource.TaskActor_builder{Session: sessionRef(f.s.Id), RunId: ptr(f.s.RunId)}.Build()
	v, err = stack.Task().Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref, Actor: actor, DateUpdated: v.GetDateUpdated()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	// Session purge physically removes its projection. Tasks have snapshot IDs,
	// so foreign-key enforcement must not delete or block retained work records.
	if _, err := db.ExecContext(ctx, "DELETE FROM session"); err != nil {
		t.Fatal(err)
	}
	f.lock.Lock()
	f.sessions = nil
	f.lock.Unlock()
	rebuilt, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := rebuilt.Task().Get(ctx, resource.TaskGetRequest_builder{Ref: ref, Select: resource.TaskSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetSourceSession() != f.s.Id || saved.GetAssignee() != f.s.Id || !proto.Equal(saved.GetDateUpdated(), v.GetDateUpdated()) {
		t.Fatalf("lost durable work: %v", saved)
	}
	retried, err := rebuilt.Task().Add(ctx, resource.TaskAddRequest_builder{Project: projectRef(f.p.Id), ClientId: "retained", Name: "Keep after purge", SourceSession: f.s.Id, SourceRun: f.s.RunId, SourceSeq: 1}.Build())
	if err != nil || !proto.Equal(retried.GetDateUpdated(), saved.GetDateUpdated()) {
		t.Fatalf("creation retry after source purge: %v %v", retried, err)
	}
	if _, err := rebuilt.Task().Release(ctx, resource.TaskClaimRequest_builder{Ref: ref, Actor: actor, DateUpdated: saved.GetDateUpdated()}.Build()); err != nil {
		t.Fatal(err)
	}
}

type taskRouterFixture struct {
	*taskFixture
	listener *bufconn.Listener
	offline  atomic.Bool
}

func (f *taskRouterFixture) TaskConnection(_ context.Context, project string) (*grpc.ClientConn, error) {
	if f.offline.Load() {
		return nil, status.Error(codes.Unavailable, "project is stopped")
	}
	if project != f.p.Id {
		return nil, status.Error(codes.NotFound, "project not found")
	}
	return grpc.NewClient("passthrough:///task-project", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return f.listener.Dial() }))
}

func TestTaskManagerForwardsWithoutShadowStorage(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	f, child, childDB := taskStack(t)
	listener := bufconn.Listen(1 << 20)
	runtime := grpc.NewServer()
	resource.RegisterServer(runtime, child)
	go runtime.Serve(listener)
	defer runtime.Stop()
	managerFixture := &taskRouterFixture{taskFixture: f, listener: listener}
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "manager.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := Build(ctx, db, managerFixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Project().List(ctx, &resource.ProjectListRequest{}); err != nil {
		t.Fatal(err)
	}
	layer, _ := resource.Find[Layer](manager)
	// Aliases can differ between inventories. Forward canonical project/session
	// identities rather than asking the runtime to resolve a manager-only alias.
	_, err = layer.Next().Project().Patch(ctx, resource.ProjectPatchRequest_builder{Ref: projectRef(f.p.Id), Alias: ptr("manager-project"), DateUpdatedForce: ptr(true)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	_, err = layer.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(f.s.Id), Alias: ptr("manager-owner"), DateUpdatedForce: ptr(true)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	project := resource.ProjectRef_builder{Alias: ptr("manager-project")}.Build()
	client := resource.NewTaskServiceClient(taskConn(t, manager))
	v, err := client.Add(ctx, resource.TaskAddRequest_builder{Project: project, ClientId: "forwarded", Name: "Remote work"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	ref := resource.TaskRef_builder{Id: v.GetId(), Project: project}.Build()
	v, err = client.Claim(ctx, resource.TaskClaimRequest_builder{Ref: ref, DateUpdated: v.GetDateUpdated(), Actor: resource.TaskActor_builder{Session: resource.SessionRef_builder{Alias: ptr("manager-owner")}.Build(), RunId: ptr(f.s.RunId)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Watch(ctx, resource.TaskWatchRequest_builder{Project: project, Filters: []*resource.TaskFilter{resource.TaskFilter_builder{Project: project}.Build()}}.Build())
	if err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv()
	if err != nil || len(frame.GetItems()) != 1 {
		t.Fatalf("forwarded watch: %v %v", frame, err)
	}
	_, err = child.Task().Patch(ctx, resource.TaskPatchRequest_builder{Ref: resource.TaskRef_builder{Id: v.GetId(), Project: projectRef(f.p.Id)}.Build(), DateUpdated: v.GetDateUpdated(), Name: ptr("Updated inside project")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	frame, err = stream.Recv()
	if err != nil || frame.GetItems()[0].GetValue().GetName() != "Updated inside project" {
		t.Fatalf("forwarded change: %v %v", frame, err)
	}
	var local, remote int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM task").Scan(&local); err != nil {
		t.Fatal(err)
	}
	if err := childDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM task").Scan(&remote); err != nil {
		t.Fatal(err)
	}
	if local != 0 || remote != 1 {
		t.Fatalf("shadow task storage: manager=%d runtime=%d", local, remote)
	}
	managerFixture.offline.Store(true)
	_, err = client.List(ctx, resource.TaskListRequest_builder{Project: project}.Build())
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("offline project fell back to manager: %v", err)
	}
}
