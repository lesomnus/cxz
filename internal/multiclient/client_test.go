package multiclient

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/transport"
	"google.golang.org/grpc"
)

type daemon struct {
	api.SessionsClient
	mu      sync.Mutex
	calls   []string
	offline atomic.Bool
	block   bool
}

func (d *daemon) List(ctx context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.SessionList, error) {
	if d.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if d.offline.Load() {
		return nil, errors.New("host offline")
	}
	return &api.SessionList{Sessions: []*api.Session{{Id: "same", ProjectId: "project", ProjectName: "project1", Pending: []*api.Event{{SessionId: "same"}}}}}, nil
}
func (d *daemon) Projects(context.Context, *api.Empty, ...grpc.CallOption) (*api.ProjectList, error) {
	return &api.ProjectList{Projects: []*api.Project{{Id: "project", Name: "project1", Workspace: "/workspace"}}}, nil
}
func (d *daemon) record(s string) { d.mu.Lock(); defer d.mu.Unlock(); d.calls = append(d.calls, s) }
func (d *daemon) Send(_ context.Context, r *api.Input, _ ...grpc.CallOption) (*api.Receipt, error) {
	d.record("send:" + r.SessionId + ":" + r.Text)
	return &api.Receipt{}, nil
}
func (d *daemon) Reply(_ context.Context, r *api.Answer, _ ...grpc.CallOption) (*api.Receipt, error) {
	d.record("reply:" + r.SessionId + ":" + r.RequestId)
	return &api.Receipt{}, nil
}
func (d *daemon) Get(_ context.Context, r *api.SessionRef, _ ...grpc.CallOption) (*api.Session, error) {
	return &api.Session{Id: r.Id, ProjectId: "project"}, nil
}
func (d *daemon) Open(_ context.Context, r *api.ProjectRequest, _ ...grpc.CallOption) (*api.Session, error) {
	d.record("open:" + r.Workspace)
	return &api.Session{Id: "new", ProjectId: "project"}, nil
}
func (d *daemon) Docker(_ context.Context, r *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	d.record("docker:" + r.Action)
	return &api.Receipt{}, nil
}
func (d *daemon) History(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (*api.EventBatch, error) {
	return &api.EventBatch{Events: []*api.Event{{SessionId: r.SessionId, Text: "hello"}}}, nil
}
func (d *daemon) CopyMemory(_ context.Context, r *api.CopyMemoryRequest, _ ...grpc.CallOption) (*api.Receipt, error) {
	d.record("copy:" + r.SessionId + ":" + r.TargetId)
	return &api.Receipt{}, nil
}

type stream struct {
	grpc.ServerStreamingClient[api.Event]
	id string
}

func (s stream) Recv() (*api.Event, error) { return &api.Event{SessionId: s.id, Text: "live"}, nil }
func (d *daemon) Watch(_ context.Context, r *api.WatchRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[api.Event], error) {
	return stream{id: r.SessionId}, nil
}

func await(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(time.Millisecond)
	}
}
func TestDuplicateIDsRouteToSelectedDaemon(t *testing.T) {
	a, b := &daemon{}, &daemon{}
	c := New(context.Background(), []Source{{Name: "home", Client: a}, {Name: "work", Client: b, Remote: true}}, "work")
	defer c.Close()
	ctx := context.Background()
	await(t, func() bool { v, _ := c.List(ctx, &api.Empty{}); return len(v.Sessions) == 2 })
	list, _ := c.List(ctx, &api.Empty{})
	if list.Sessions[0].Id != "home::same" || list.Sessions[1].Id != "work::same" || list.Sessions[1].Pending[0].SessionId != "work::same" {
		t.Fatal(list)
	}
	projects, _ := c.Projects(ctx, &api.Empty{})
	if projects.Projects[0].Name != "project1 via home" || projects.Projects[1].Name != "project1 via work" {
		t.Fatal(projects)
	}
	// An explicit session reference wins even while another connection's panel is focused.
	req := &api.Input{SessionId: "work::same", Text: "hello"}
	if _, err := c.Send(c.ContextFor(ctx, "home::project"), req); err != nil {
		t.Fatal(err)
	}
	if req.SessionId != "work::same" {
		t.Fatal("request mutated")
	}
	if _, err := c.Reply(ctx, &api.Answer{SessionId: "work::same", RequestId: "approval", Allow: true}); err != nil {
		t.Fatal(err)
	}
	local := c.ContextFor(ctx, "home::project")
	if transport.IsRemote(local) || !transport.IsRemote(c.ContextFor(ctx, "work::same")) {
		t.Fatal("wrong helper context")
	}
	out, err := c.Open(local, &api.ProjectRequest{Workspace: "/new"})
	if err != nil || out.Id != "home::new" {
		t.Fatal(out, err)
	}
	if _, err = c.Docker(local, &api.DockerInput{Action: "info"}); err != nil {
		t.Fatal(err)
	}
	batch, err := c.History(ctx, &api.WatchRequest{SessionId: "work::same"})
	if err != nil || batch.Events[0].SessionId != "work::same" {
		t.Fatal(batch, err)
	}
	watch, err := c.Watch(ctx, &api.WatchRequest{SessionId: "home::same"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := watch.Recv()
	if err != nil || event.SessionId != "home::same" {
		t.Fatal(event, err)
	}
	if _, err = c.CopyMemory(ctx, &api.CopyMemoryRequest{SessionId: "work::same", TargetId: "home::same"}); err == nil {
		t.Fatal("cross-host copy accepted")
	}
	if _, err = c.CopyMemory(ctx, &api.CopyMemoryRequest{SessionId: "home::same", TargetId: "home::other"}); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	gotA := strings.Join(a.calls, ",")
	a.mu.Unlock()
	b.mu.Lock()
	gotB := strings.Join(b.calls, ",")
	b.mu.Unlock()
	if gotA != "open:/new,docker:info,copy:same:other" || gotB != "send:same:hello,reply:same:approval" {
		t.Fatal(gotA, gotB)
	}
	if _, err = c.Send(ctx, &api.Input{SessionId: "typo::same"}); err == nil {
		t.Fatal("unknown connection fell back")
	}
}

func TestSlowConnectionDoesNotBlockAndOfflineCacheRecovers(t *testing.T) {
	healthy := &daemon{}
	c := New(context.Background(), []Source{{Name: "fast", Client: healthy}, {Name: "slow", Client: &daemon{block: true}}}, "slow")
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notices := make(chan struct{}, 20)
	go c.WatchChanges(ctx, nil, nil, func() {
		select {
		case notices <- struct{}{}:
		default:
		}
	})
	await(t, func() bool { v, _ := c.List(ctx, &api.Empty{}); return len(v.Sessions) == 1 })
	if !strings.Contains(c.ConnectionStatus(), "slow: connecting") {
		t.Fatal(c.ConnectionStatus())
	}
	ps, _ := c.Projects(ctx, &api.Empty{})
	if len(ps.Projects) != 2 || ps.Projects[1].State != "connection" {
		t.Fatal(ps)
	}
	healthy.offline.Store(true)
	c.refreshSource(ctx, "fast::project")
	await(t, func() bool { return strings.Contains(c.ConnectionStatus(), "fast: unavailable") })
	v, _ := c.List(ctx, &api.Empty{})
	if len(v.Sessions) != 1 {
		t.Fatal("lost offline snapshot")
	}
	ps, _ = c.Projects(ctx, &api.Empty{})
	if !strings.Contains(ps.Projects[0].Name, "offline") {
		t.Fatal(ps)
	}
	healthy.offline.Store(false)
	c.refreshSource(ctx, "fast::project")
	await(t, func() bool { return strings.Contains(c.ConnectionStatus(), "fast: online") })
	select {
	case <-notices:
	case <-time.After(time.Second):
		t.Fatal("no change notification")
	}
}
func TestInitialAttachWaitsForConnector(t *testing.T) {
	release := make(chan struct{})
	c := New(context.Background(), []Source{{Name: "work", Open: func() (api.SessionsClient, io.Closer, error) { <-release; return &daemon{}, nil, nil }}}, "work")
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		s, err := c.Get(ctx, &api.SessionRef{Id: "work::same"})
		if err == nil && s.Id != "work::same" {
			err = errors.New("wrong scope")
		}
		done <- err
	}()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
