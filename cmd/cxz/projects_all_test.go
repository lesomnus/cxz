//go:build !windows

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/xli/xlitest"
	"google.golang.org/grpc"
)

type allStub struct {
	resource.UnimplementedProjectServiceServer
	items []*resource.Project
	fail  map[string]bool

	mu        sync.Mutex
	recreated []string
	upped     []string
}

func ownedProject(id, workspace, state string) *resource.Project {
	return resource.Project_builder{RuntimeId: id, Workspace: workspace, Name: id, Alias: id, Listed: true,
		Status: resource.ProjectStatus_builder{State: state, ContainerId: "container-" + id}.Build()}.Build()
}

func (s *allStub) List(context.Context, *resource.ProjectListRequest) (*resource.ProjectListResponse, error) {
	return resource.ProjectListResponse_builder{Items: s.items}.Build(), nil
}

func (s *allStub) InspectForeign(context.Context, *resource.InspectForeignRequest) (*resource.InspectForeignResponse, error) {
	return resource.InspectForeignResponse_builder{Items: []*resource.ForeignContainer{
		resource.ForeignContainer_builder{ContainerId: ptr("container-theirs"), Workspace: ptr("/work/theirs"), Name: ptr("theirs")}.Build(),
	}}.Build(), nil
}

func (s *allStub) Get(_ context.Context, r *resource.ProjectGetRequest) (*resource.Project, error) {
	for _, p := range s.items {
		if p.GetRuntimeId() == r.GetRef().GetRuntimeId() {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no project %s", r.GetRef().GetRuntimeId())
}

func (s *allStub) Devcontainer(context.Context, *resource.DevcontainerRequest) (*resource.DevcontainerReply, error) {
	return &resource.DevcontainerReply{}, nil
}

func (s *allStub) Up(_ context.Context, r *resource.ProjectUpRequest) (*resource.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upped = append(s.upped, r.GetRef().GetRuntimeId())
	return s.Get(context.Background(), resource.ProjectGetRequest_builder{Ref: r.GetRef()}.Build())
}

func (s *allStub) Recreate(_ context.Context, r *resource.ProjectRecreateRequest) (*resource.Project, error) {
	id := r.GetRef().GetRuntimeId()
	s.mu.Lock()
	s.recreated = append(s.recreated, id)
	s.mu.Unlock()
	if !r.GetConfirmed() {
		return nil, fmt.Errorf("%s was recreated without confirmation", id)
	}
	if s.fail[id] {
		return nil, fmt.Errorf("image build failed")
	}
	return s.Get(context.Background(), resource.ProjectGetRequest_builder{Ref: r.GetRef()}.Build())
}

func (s *allStub) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.recreated...)
}

func allProjectsCLI(t *testing.T, stub *allStub) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(root, "run", "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	resource.RegisterProjectServiceServer(server, stub)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	return root
}

// The sentinel targets every running owned project, and only those: a stopped
// project has no container to replace and is not this command's to start, and a
// foreign container belongs to another owner.
func TestRecreateAllTakesEveryRunningOwnedProject(t *testing.T) {
	stub := &allStub{items: []*resource.Project{
		ownedProject("web", "/work/web", "running"),
		ownedProject("api", "/work/api", "running"),
		ownedProject("archive", "/work/archive", "absent"),
		ownedProject("paused", "/work/paused", "stopped"),
	}}
	root := allProjectsCLI(t, stub)

	// Without --yes it is a review, and nothing is touched.
	got := xlitest.Run(t, newRoot(root), "project", "recreate", allProjects)
	if got.Err == nil || !strings.Contains(got.Err.Error(), "--yes") {
		t.Fatal("recreate all ran unconfirmed:", got.Err)
	}
	for _, want := range []string{"/work/api", "/work/web", "2 projects", "2 not running", "1 foreign"} {
		if !strings.Contains(got.Stderr, want) {
			t.Fatalf("preview is missing %q: %s", want, got.Stderr)
		}
	}
	if strings.Contains(got.Stderr, "/work/paused") || strings.Contains(got.Stderr, "/work/theirs") {
		t.Fatal("preview targeted a project it must skip:", got.Stderr)
	}
	if len(stub.calls()) != 0 {
		t.Fatal("the review recreated something:", stub.calls())
	}

	got = xlitest.Run(t, newRoot(root), "project", "recreate", "--yes", allProjects)
	if got.Err != nil {
		t.Fatal(got.Err, got.Stderr)
	}
	if allProjects != "@all" {
		t.Fatal("the documented spelling changed:", allProjects)
	}
	// Alphabetical by workspace, so a run reads the same way twice.
	if calls := stub.calls(); strings.Join(calls, ",") != "api,web" {
		t.Fatal("wrong targets or order:", calls)
	}
	if len(stub.upped) != 0 {
		t.Fatal("recreate brought a project up instead:", stub.upped)
	}
	for _, want := range []string{"recreated api /work/api", "recreated web /work/web"} {
		if !strings.Contains(got.Stdout, want) {
			t.Fatalf("missing %q: %s", want, got.Stdout)
		}
	}
}

// One project failing is not the others' failure, but it must not be reported
// as success either: everything is attempted, and the command still fails.
func TestRecreateAllContinuesPastAFailure(t *testing.T) {
	stub := &allStub{items: []*resource.Project{
		ownedProject("api", "/work/api", "running"),
		ownedProject("web", "/work/web", "running"),
		ownedProject("zero", "/work/zero", "running"),
	}, fail: map[string]bool{"web": true}}
	root := allProjectsCLI(t, stub)
	got := xlitest.Run(t, newRoot(root), "project", "recreate", "--yes", allProjects)
	if got.Err == nil {
		t.Fatal("a failed project was reported as success")
	}
	if !strings.Contains(got.Err.Error(), "/work/web") || !strings.Contains(got.Err.Error(), "1 of 3") {
		t.Fatal("the failure does not name what failed:", got.Err)
	}
	if calls := stub.calls(); strings.Join(calls, ",") != "api,web,zero" {
		t.Fatal("a failure stopped the remaining projects:", calls)
	}
	if !strings.Contains(got.Stdout, "recreated zero") {
		t.Fatal("the project after the failure was not reported:", got.Stdout)
	}
}

func TestRecreateAllRefusesPerProjectSettings(t *testing.T) {
	stub := &allStub{items: []*resource.Project{ownedProject("web", "/work/web", "running")}}
	root := allProjectsCLI(t, stub)
	for _, flag := range [][]string{{"--config", "devcontainer.json"}, {"--alias", "one"}, {"--name", "One"}, {"--account", "work"}, {"--model", "opus"}, {"--agent", "codex"}} {
		args := append([]string{"project", "recreate", "--yes"}, flag...)
		got := xlitest.Run(t, newRoot(root), append(args, allProjects)...)
		if got.Err == nil || !strings.Contains(got.Err.Error(), flag[0]) {
			t.Fatalf("%v: %v", flag, got.Err)
		}
	}
	if len(stub.calls()) != 0 {
		t.Fatal("a refused flag still recreated something:", stub.calls())
	}
}

// "!all" reads the same and has to be quoted in a shell, so it is accepted
// rather than resolved as a project that does not exist.
func TestQuotedBangSpellingIsTheSameTarget(t *testing.T) {
	stub := &allStub{items: []*resource.Project{ownedProject("web", "/work/web", "running")}}
	root := allProjectsCLI(t, stub)
	got := xlitest.Run(t, newRoot(root), "project", "recreate", "--yes", "!all")
	if got.Err != nil {
		t.Fatal(got.Err, got.Stderr)
	}
	if calls := stub.calls(); strings.Join(calls, ",") != "web" {
		t.Fatal("!all did not reach every project:", calls)
	}
}

// The sentinel is only a sentinel where it means something; everywhere else it
// says so rather than being resolved as a path.
func TestAllSentinelIsRecreateOnly(t *testing.T) {
	stub := &allStub{items: []*resource.Project{ownedProject("web", "/work/web", "running")}}
	root := allProjectsCLI(t, stub)
	// `session new` never reaches this: it is refused earlier for having no
	// --account, which is its own message and the right one.
	for _, command := range [][]string{{"project", "up"}, {"project", "down"}} {
		got := xlitest.Run(t, newRoot(root), append(command, allProjects)...)
		if got.Err == nil || !strings.Contains(got.Err.Error(), "only by cxz project recreate") {
			t.Fatalf("%v: %v", command, got.Err)
		}
	}
	if len(stub.upped) != 0 || len(stub.calls()) != 0 {
		t.Fatal("the sentinel reached the API")
	}
}
