package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/xlitest"
	"google.golang.org/grpc"
)

func TestHelpWithoutInstallation(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://invalid.invalid:1")
	state := filepath.Join(t.TempDir(), "must-not-be-created")
	root := newRoot(state)
	cases := [][]string{nil, {"--help"}, {"completion", "zsh"}}
	var visit func(*xli.Command, []string)
	visit = func(parent *xli.Command, path []string) {
		for _, c := range parent.Commands {
			next := append(append([]string{}, path...), c.Name)
			cases = append(cases, append(append([]string{}, next...), "--help"))
			visit(c, next)
		}
	}
	visit(root, nil)
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got := xlitest.Run(t, newRoot(state), args...)
			if got.Err != nil || got.Stdout == "" {
				t.Fatalf("%v: %+v", args, got)
			}
		})
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("help touched state")
	}
}

func TestValidationBeforeConnection(t *testing.T) {
	cases := []struct {
		args []string
		want error
	}{
		{[]string{"new", ".", "--agent", "codex"}, xli.ErrFlagAfterArg},
		{[]string{"get"}, xli.ErrNeedArgs},
		{[]string{"stop", "id", "extra"}, xli.ErrTooManyArgs},
		{[]string{"install", "unexpected"}, xli.ErrTooManyArgs},
		{[]string{"unknown"}, xli.ErrUnknownCmd},
		{[]string{"new", "--unknown"}, xli.ErrUnknownFlag},
		{[]string{"new", "--agent", "invalid", "."}, nil},
		{[]string{"reply", "id", "request", "maybe"}, nil},
		{[]string{"events", "id", "-1"}, nil},
		{[]string{"events", "id", "12junk"}, nil},
		{[]string{"exec", "project"}, xli.ErrNeedArgs},
		{[]string{"--agent", "codex", "new", "."}, nil},
	}
	for _, tc := range cases {
		got := xlitest.Run(t, newRoot("/nonexistent-cxz-test"), tc.args...)
		if got.Err == nil {
			t.Fatalf("accepted %v", tc.args)
		}
		if tc.want != nil && !errors.Is(got.Err, tc.want) {
			t.Errorf("%v: %v", tc.args, got.Err)
		}
		if strings.Contains(got.Err.Error(), "not installed") {
			t.Errorf("connected before validation: %v", tc.args)
		}
	}
}

func TestExecPassThrough(t *testing.T) {
	root := newRoot("unused")
	var got []string
	for _, c := range root.Commands {
		if c.Name == "exec" {
			c.Handler = onRun(func(_ context.Context, c *xli.Command) error { got, _ = arg.Get[[]string](c, "COMMAND"); return nil })
		}
	}
	want := []string{"sh", "-c", "printf '%s' \"$1\"", "--", "--agent", "", "--help"}
	result := xlitest.Run(t, root, append([]string{"exec", "project", "--"}, want...)...)
	if result.Err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v: %q", result.Err, got)
	}
}

type rpcStub struct {
	resource.UnimplementedSessionServiceServer
	requests chan any
}

func (s *rpcStub) Add(_ context.Context, r *resource.SessionAddRequest) (*resource.Session, error) {
	s.requests <- &api.ProjectRequest{Workspace: "project-name", Agent: r.GetAgent(), Model: r.GetModel(), ClientId: r.GetClientId(), NewSession: true, Account: r.GetAccount().GetAlias()}
	return resource.Session_builder{RuntimeId: "session", Agent: r.GetAgent()}.Build(), nil
}
func (s *rpcStub) List(context.Context, *resource.SessionListRequest) (*resource.SessionListResponse, error) {
	return &resource.SessionListResponse{}, nil
}
func (s *rpcStub) Get(_ context.Context, r *resource.SessionGetRequest) (*resource.Session, error) {
	return resource.Session_builder{RuntimeId: r.GetRef().GetRuntimeId(), Status: resource.SessionStatus_builder{RunId: "current-run"}.Build()}.Build(), nil
}
func (s *rpcStub) Send(_ context.Context, r *resource.SessionSendRequest) (*resource.SessionReceipt, error) {
	s.requests <- &api.Input{SessionId: r.GetRef().GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), Text: r.GetText()}
	return resource.SessionReceipt_builder{Status: ptr("accepted")}.Build(), nil
}
func (s *rpcStub) Reply(_ context.Context, r *resource.SessionReplyRequest) (*resource.SessionReceipt, error) {
	s.requests <- &api.Answer{SessionId: r.GetRef().GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), RequestId: r.GetRequestId(), Allow: r.GetAllow(), AnswersJson: r.GetAnswersJson()}
	return resource.SessionReceipt_builder{Status: ptr("accepted")}.Build(), nil
}

type projectStub struct {
	resource.UnimplementedProjectServiceServer
	requests  chan any
	mutations *atomic.Int32
}
type accountStub struct {
	resource.UnimplementedAccountServiceServer
	requests chan any
}

func (s accountStub) Add(_ context.Context, r *resource.AccountAddRequest) (*resource.Account, error) {
	s.requests <- r
	return resource.Account_builder{Alias: r.GetAlias(), Name: r.GetName(), Agent: r.GetAgent(), AuthBackend: r.GetAuthBackend()}.Build(), nil
}

type bindingStub struct {
	resource.UnimplementedAuthBindingServiceServer
}

func (bindingStub) Add(_ context.Context, r *resource.AuthBindingAddRequest) (*resource.AuthBinding, error) {
	return resource.AuthBinding_builder{BindingId: "fixture-binding", AuthBackend: "project-local-oauth"}.Build(), nil
}

func (accountStub) Get(_ context.Context, r *resource.AccountGetRequest) (*resource.Account, error) {
	return resource.Account_builder{Alias: r.GetRef().GetAlias(), Agent: "codex", AuthBackend: "project-local-oauth"}.Build(), nil
}

func testProject() *resource.Project {
	return resource.Project_builder{RuntimeId: "project-name", Workspace: "project-name", Name: "project-name", Alias: "pn"}.Build()
}
func (s projectStub) Get(context.Context, *resource.ProjectGetRequest) (*resource.Project, error) {
	return testProject(), nil
}
func (s projectStub) Add(context.Context, *resource.ProjectAddRequest) (*resource.Project, error) {
	if s.mutations != nil {
		s.mutations.Add(1)
	}
	return testProject(), nil
}
func (s projectStub) Up(context.Context, *resource.ProjectUpRequest) (*resource.Project, error) {
	if s.mutations != nil {
		s.mutations.Add(1)
	}
	return testProject(), nil
}
func (s projectStub) List(context.Context, *resource.ProjectListRequest) (*resource.ProjectListResponse, error) {
	return resource.ProjectListResponse_builder{Items: []*resource.Project{testProject()}}.Build(), nil
}
func (s projectStub) Down(_ context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	s.requests <- &api.ProjectRequest{Workspace: r.GetRef().GetRuntimeId(), ClientId: r.GetClientId()}
	return testProject(), nil
}

func TestCommandsReachAPI(t *testing.T) {
	root, err := os.MkdirTemp("", "cxz-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if err = os.Mkdir(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(root, "run", "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stub := &rpcStub{requests: make(chan any, 10)}
	resource.RegisterSessionServiceServer(server, stub)
	resource.RegisterAccountServiceServer(server, accountStub{requests: stub.requests})
	resource.RegisterAuthBindingServiceServer(server, bindingStub{})
	mutations := &atomic.Int32{}
	resource.RegisterProjectServiceServer(server, projectStub{requests: stub.requests, mutations: mutations})
	go server.Serve(listener)
	defer server.Stop()
	for _, args := range [][]string{{"up", "--trust-config", "project-name"}, {"recreate", "--trust-config", "--yes", "project-name"}, {"new", "--no-attach", "project-name"}} {
		got := xlitest.Run(t, newRoot(root), args...)
		if got.Err == nil || !strings.Contains(got.Err.Error(), "--account") {
			t.Fatalf("missing account guidance: %v", got.Err)
		}
		if strings.Contains(got.Stderr, "preparing workspace") || mutations.Load() != 0 {
			t.Fatal("started preparation before validating account", got.Stderr)
		}
	}
	run := func(args ...string) xlitest.Result {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		got := (xlitest.Harness{Cmd: newRoot(root), Ctx: ctx}).Run(t, args...)
		if got.Err != nil {
			t.Fatalf("%v: %v", args, got.Err)
		}
		var v any
		if json.Unmarshal([]byte(got.Stdout), &v) != nil {
			t.Fatalf("not JSON: %q", got.Stdout)
		}
		return got
	}
	run("account", "add", "--name", "Work Codex", "codex", "work-codex")
	a := (<-stub.requests).(*resource.AccountAddRequest)
	if a.GetAlias() != "work-codex" || a.GetAgent() != "codex" || a.GetName() != "Work Codex" || a.GetAuthBackend() != "" {
		t.Fatal(a)
	}
	got := run("new", "--account", "work-codex", "--agent", "codex", "--model", "test-model", "--no-attach", "project-name")
	req := (<-stub.requests).(*api.ProjectRequest)
	if req.Agent != "codex" || req.Account != "work-codex" || !req.NewSession || req.Workspace != "project-name" || req.ClientId == "" {
		t.Fatal(req)
	}
	if req.Model != "test-model" {
		t.Fatal("model not passed")
	}
	if !strings.Contains(got.Stderr, "preparing workspace") {
		t.Fatal("progress must use stderr")
	}
	run("send", "session", "text --help with spaces")
	input := (<-stub.requests).(*api.Input)
	if input.Text != "text --help with spaces" || input.RunId != "current-run" || input.ClientId == "" {
		t.Fatal(input)
	}
	run("reply", "session", "42", "allow", `{"color":"blue"}`)
	answer := (<-stub.requests).(*api.Answer)
	if answer.RequestId != "42" || !answer.Allow || answer.RunId != "current-run" || answer.AnswersJson != `{"color":"blue"}` {
		t.Fatal(answer)
	}
	run("down", "pn")
	if (<-stub.requests).(*api.ProjectRequest).Workspace != "project-name" {
		t.Fatal("wrong project")
	}
	completion := xlitest.Complete(t, newRoot(root), "up ")
	if completion.Err != nil || !completion.Has("pn") || !completion.Has("project-name") {
		t.Fatalf("project completion: %+v", completion)
	}
	override := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "--state "+root+" up ")
	if override.Err != nil || !override.Has("pn") {
		t.Fatalf("state override completion: %+v", override)
	}
}

func TestAgentCompletionWithoutConnection(t *testing.T) {
	account := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "account add ")
	if account.Err != nil || !account.Has("claude") || !account.Has("codex") {
		t.Fatalf("account agent completion: %+v", account)
	}
	got := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "new --agent ")
	if got.Err != nil || !got.Has("claude") || !got.Has("codex") {
		t.Fatalf("%+v", got)
	}
	paths := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "new ")
	if paths.Err != nil || !paths.WantDirs {
		t.Fatalf("workspace completion: %+v", paths)
	}
	config := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "new --config ")
	if config.Err != nil || !config.WantFiles {
		t.Fatalf("config completion: %+v", config)
	}
	decision := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "reply session 42 ")
	if decision.Err != nil || !decision.Has("allow") || !decision.Has("deny") {
		t.Fatalf("decision completion: %+v", decision)
	}
}

func TestAliases(t *testing.T) {
	for _, alias := range []struct{ name, target string }{{"it", "attach"}, {"watch", "tui"}} {
		root := newRoot("unused")
		called := false
		for _, c := range root.Commands {
			if c.Name == alias.target {
				c.Handler = onRun(func(_ context.Context, c *xli.Command) error { called = true; return nil })
			}
		}
		got := xlitest.Run(t, root, alias.name)
		if got.Err != nil || !called {
			t.Fatalf("%s: %+v", alias.name, got)
		}
	}
}
