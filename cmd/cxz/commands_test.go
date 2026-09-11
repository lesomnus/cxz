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
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
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
	for _, c := range root.Commands {
		cases = append(cases, []string{c.Name, "--help"})
	}
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
	api.UnimplementedSessionsServer
	requests chan any
}

func (s *rpcStub) Open(_ context.Context, r *api.ProjectRequest) (*api.Session, error) {
	s.requests <- r
	return &api.Session{Id: "session", Workspace: r.Workspace, Agent: r.Agent}, nil
}
func (s *rpcStub) Get(_ context.Context, r *api.SessionRef) (*api.Session, error) {
	return &api.Session{Id: r.Id, RunId: "current-run"}, nil
}
func (s *rpcStub) Send(_ context.Context, r *api.Input) (*api.Receipt, error) {
	s.requests <- r
	return &api.Receipt{Status: "accepted"}, nil
}
func (s *rpcStub) Reply(_ context.Context, r *api.Answer) (*api.Receipt, error) {
	s.requests <- r
	return &api.Receipt{Status: "accepted"}, nil
}
func (s *rpcStub) Down(_ context.Context, r *api.ProjectRequest) (*api.Receipt, error) {
	s.requests <- r
	return &api.Receipt{Status: "stopped"}, nil
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
	api.RegisterSessionsServer(server, stub)
	go server.Serve(listener)
	defer server.Stop()
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
	got := run("new", "--agent", "codex", "--model", "test-model", "--no-attach", "project-name")
	req := (<-stub.requests).(*api.ProjectRequest)
	if req.Agent != "codex" || !req.NewSession || req.Workspace != "project-name" || req.ClientId == "" {
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
	run("down", "project-name")
	if (<-stub.requests).(*api.ProjectRequest).Workspace != "project-name" {
		t.Fatal("wrong project")
	}
}

func TestAgentCompletionWithoutConnection(t *testing.T) {
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
