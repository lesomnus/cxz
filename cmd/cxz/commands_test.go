//go:build !windows

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
	"github.com/lesomnus/cxz/internal/projectconfig"
	"github.com/lesomnus/cxz/internal/settings"
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
	cases := [][]string{{"--help"}, {"completion", "zsh"}}
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

func TestTerminalInfoWithoutInstallation(t *testing.T) {
	state := filepath.Join(t.TempDir(), "absent")
	got := xlitest.Run(t, newRoot(state), "terminal-info", "--plain")
	if got.Err != nil || !strings.Contains(got.Stdout, "TERM=") || !strings.Contains(got.Stdout, "235 #262626") {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("terminal diagnostics touched state")
	}
}

func TestValidationBeforeConnection(t *testing.T) {
	cases := []struct {
		args []string
		want error
	}{
		{[]string{"session", "new", ".", "--agent", "codex"}, xli.ErrFlagAfterArg},
		{[]string{"session", "get"}, xli.ErrNeedArgs},
		{[]string{"session", "stop", "id", "extra"}, xli.ErrTooManyArgs},
		{[]string{"install", "unexpected"}, xli.ErrTooManyArgs},
		{[]string{"unknown"}, xli.ErrUnknownCmd},
		{[]string{"session", "new", "--unknown"}, xli.ErrUnknownFlag},
		{[]string{"session", "new", "--agent", "invalid", "."}, nil},
		{[]string{"session", "reply", "id", "request", "maybe"}, nil},
		{[]string{"session", "events", "id", "-1"}, nil},
		{[]string{"session", "events", "id", "12junk"}, nil},
		{[]string{"project", "exec", "project"}, xli.ErrNeedArgs},
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
	for _, c := range root.Commands.Get("project").Commands {
		if c.Name == "exec" {
			c.Handler = onRun(func(_ context.Context, c *xli.Command) error { got, _ = arg.Get[[]string](c, "COMMAND"); return nil })
		}
	}
	want := []string{"sh", "-c", "printf '%s' \"$1\"", "--", "--agent", "", "--help"}
	result := xlitest.Run(t, root, append([]string{"project", "exec", "project", "--"}, want...)...)
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
	requests     chan any
	mutations    *atomic.Int32
	unregistered *atomic.Bool
	overrides    chan projectconfig.Spec
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
	return resource.Project_builder{RuntimeId: "project-name", Workspace: "project-name", Name: "project-name", Alias: "pn", Status: resource.ProjectStatus_builder{State: "running"}.Build()}.Build()
}
func (s projectStub) Get(context.Context, *resource.ProjectGetRequest) (*resource.Project, error) {
	return testProject(), nil
}
func (s projectStub) Add(context.Context, *resource.ProjectAddRequest) (*resource.Project, error) {
	if s.unregistered != nil {
		s.unregistered.Store(false)
	}
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

func (s projectStub) Devcontainer(_ context.Context, r *resource.DevcontainerRequest) (*resource.DevcontainerReply, error) {
	var spec projectconfig.Spec
	if err := json.Unmarshal(r.GetSpec(), &spec); err != nil {
		return nil, err
	}
	if len(spec.Compose) > 0 && s.overrides != nil {
		s.overrides <- spec
	}
	return &resource.DevcontainerReply{}, nil
}
func (s projectStub) List(context.Context, *resource.ProjectListRequest) (*resource.ProjectListResponse, error) {
	if s.unregistered != nil && s.unregistered.Load() {
		return &resource.ProjectListResponse{}, nil
	}
	return resource.ProjectListResponse_builder{Items: []*resource.Project{testProject()}}.Build(), nil
}
func (s projectStub) Down(_ context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	s.requests <- &api.ProjectRequest{Workspace: r.GetRef().GetRuntimeId(), ClientId: r.GetClientId()}
	return testProject(), nil
}

func (s projectStub) Erase(_ context.Context, r *resource.ProjectRef) (*resource.ProjectEraseResponse, error) {
	s.requests <- r
	return resource.ProjectEraseResponse_builder{Erased: ptr(true)}.Build(), nil
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
	unregistered := &atomic.Bool{}
	overrides := make(chan projectconfig.Spec, 4)
	resource.RegisterProjectServiceServer(server, projectStub{requests: stub.requests, mutations: mutations, unregistered: unregistered, overrides: overrides})
	go server.Serve(listener)
	defer server.Stop()
	table := xlitest.Run(t, newRoot(root), "project", "ls")
	if table.Err != nil || !strings.Contains(table.Stdout, "ALIAS") || !strings.Contains(table.Stdout, "pn") {
		t.Fatal("default API table", table)
	}
	for _, args := range [][]string{{"project", "up", "--trust-config", "project-name"}, {"project", "recreate", "--trust-config", "--yes", "project-name"}, {"session", "new", "--no-attach", "project-name"}} {
		got := xlitest.Run(t, newRoot(root), args...)
		if got.Err == nil || !strings.Contains(got.Err.Error(), "--account") {
			t.Fatalf("missing account guidance: %v", got.Err)
		}
		if strings.Contains(got.Stderr, "preparing workspace") || mutations.Load() != 0 {
			t.Fatal("started preparation before validating account", got.Stderr)
		}
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"up", "project-name"}, "Project \"project-name\" is ready.\nRun cxz to view projects and sessions in the TUI.\n"},
		{[]string{"up", "--format", "json", "project-name"}, `"id":"project-name"`},
		{[]string{"--format", "json", "up", "project-name"}, `"id":"project-name"`},
		{[]string{"up", "--no-attach", "--format", "json", "project-name"}, `"id":"project-name"`},
		{[]string{"up", "--no-attach", "project-name"}, "project-name"},
		{[]string{"up", "--format", "table", "project-name"}, "project-name"},
	} {
		unregistered.Store(true)
		for i := 0; i < 2; i++ {
			mutations.Store(0)
			got := xlitest.Run(t, newRoot(root), tc.args...)
			wantMutations := int32(0)
			if i == 0 {
				wantMutations = 2 // Register and prepare only, without a session.
			}
			if got.Err != nil || !strings.Contains(got.Stdout, tc.want) || len(stub.requests) != 0 || mutations.Load() != wantMutations {
				t.Fatalf("up %v (attempt %d): %+v; mutations %d", tc.args, i, got, mutations.Load())
			}
		}
	}
	if err := os.WriteFile(settings.Path(root), []byte(`{"devcontainer":{"compose":"override.yaml"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	unregistered.Store(true)
	for _, source := range []string{"/first", "/edited"} {
		if err := os.WriteFile(filepath.Join(root, "override.yaml"), []byte("services:\n  dev:\n    volumes:\n      - "+source+":/workspaces\n"), 0600); err != nil {
			t.Fatal(err)
		}
		got := xlitest.Run(t, newRoot(root), "up", "project-name")
		if got.Err != nil || len(overrides) != 1 || len(stub.requests) != 0 {
			t.Fatal("up must publish overrides without creating a session", got)
		}
		if spec := <-overrides; !strings.Contains(string(spec.Compose), source) {
			t.Fatal("up did not reread external override", spec)
		}
	}
	if err := os.Remove(settings.Path(root)); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) xlitest.Result {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		got := (xlitest.Harness{Cmd: newRoot(root), Ctx: ctx}).Run(t, append([]string{"--format", "json"}, args...)...)
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
	got := run("session", "new", "--account", "work-codex", "--agent", "codex", "--model", "test-model", "--no-attach", "project-name")
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
	run("session", "send", "session", "text --help with spaces")
	input := (<-stub.requests).(*api.Input)
	if input.Text != "text --help with spaces" || input.RunId != "current-run" || input.ClientId == "" {
		t.Fatal(input)
	}
	run("session", "reply", "session", "42", "allow", `{"color":"blue"}`)
	answer := (<-stub.requests).(*api.Answer)
	if answer.RequestId != "42" || !answer.Allow || answer.RunId != "current-run" || answer.AnswersJson != `{"color":"blue"}` {
		t.Fatal(answer)
	}
	run("project", "down", "pn")
	if (<-stub.requests).(*api.ProjectRequest).Workspace != "project-name" {
		t.Fatal("wrong project")
	}
	run("down", "pn")
	if (<-stub.requests).(*resource.ProjectRef).GetRuntimeId() != "project-name" {
		t.Fatal("top-level down did not delete resolved project")
	}
	completion := xlitest.Complete(t, newRoot(root), "project up ")
	if completion.Err != nil || !completion.Has("pn") || !completion.Has("project-name") {
		t.Fatalf("project completion: %+v", completion)
	}
	override := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "--state "+root+" project up ")
	if override.Err != nil || !override.Has("pn") {
		t.Fatalf("state override completion: %+v", override)
	}
}

func TestAgentCompletionWithoutConnection(t *testing.T) {
	account := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "account add ")
	if account.Err != nil || !account.Has("claude") || !account.Has("codex") {
		t.Fatalf("account agent completion: %+v", account)
	}
	got := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "session new --agent ")
	if got.Err != nil || !got.Has("claude") || !got.Has("codex") {
		t.Fatalf("%+v", got)
	}
	paths := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "session new ")
	if paths.Err != nil || !paths.WantDirs {
		t.Fatalf("workspace completion: %+v", paths)
	}
	config := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "session new --config ")
	if config.Err != nil || !config.WantFiles {
		t.Fatalf("config completion: %+v", config)
	}
	decision := xlitest.Complete(t, newRoot("/nonexistent-cxz-test"), "session reply session 42 ")
	if decision.Err != nil || !decision.Has("allow") || !decision.Has("deny") {
		t.Fatalf("decision completion: %+v", decision)
	}
}

func TestAliases(t *testing.T) {
	for _, alias := range []struct{ name, target string }{{"watch", "tui"}} {
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

func TestRootDefaultsToTUIConnection(t *testing.T) {
	state := filepath.Join(t.TempDir(), "missing")
	for _, args := range [][]string{nil, {"--state", state}, {"tui"}} {
		got := xlitest.Run(t, newRoot(state), args...)
		if got.Err == nil || !strings.Contains(got.Err.Error(), "cxz is not installed") {
			t.Fatalf("%v: %+v", args, got)
		}
	}
}
