//go:build !windows && !js

package taskmcp_test

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/projectid"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/cxz/internal/mcpruntime"
	"github.com/lesomnus/cxz/internal/taskmcp"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type scopedTasks struct {
	resource.UnimplementedTaskServiceServer
}

func (scopedTasks) List(_ context.Context, r *resource.TaskListRequest) (*resource.TaskListResponse, error) {
	if projectid.RefString(r.GetProject()) != projectid.Canonical("P") || r.GetSize() != 20 {
		return nil, status.Error(codes.InvalidArgument, "wrong project scope or page size")
	}
	return resource.TaskListResponse_builder{Items: []*resource.Task{}}.Build(), nil
}

func TestBuiltinActivationBridgeAndLaunchFence(t *testing.T) {
	root := t.TempDir()
	if err := core.Prepare(root); err != nil {
		t.Fatal(err)
	}
	cfg, err := mcpconfig.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Servers["cxz_tasks"].Kind != "builtin" || !cfg.Servers["cxz_tasks"].Enabled {
		t.Fatal("Task MCP is not an enabled builtin")
	}
	if _, err := mcpconfig.SetProject(root, "P", "cxz_tasks", false); err != nil {
		t.Fatal(err)
	}
	cfg, err = mcpconfig.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Resolve("P").Servers["cxz_tasks"]; ok {
		t.Fatal("project cannot disable Task MCP")
	}
	if _, ok := cfg.Resolve("Q").Servers["cxz_tasks"]; !ok {
		t.Fatal("another project's activation changed")
	}
	if err := mcpconfig.SaveRuntime(root, mcpconfig.Snapshot{Servers: map[string]mcpconfig.Server{"cxz_tasks": cfg.Servers["cxz_tasks"]}}); err != nil {
		t.Fatal(err)
	}
	if got := mcpruntime.Instructions(mcpconfig.Snapshot{Servers: map[string]mcpconfig.Server{"cxz_tasks": cfg.Servers["cxz_tasks"]}}); got != taskmcp.Instructions {
		t.Fatal("Task usage instructions are not delivered")
	}
	if got := mcpruntime.Instructions(mcpconfig.Snapshot{}); got != "" {
		t.Fatal("disabled Task MCP still consumes context")
	}
	listener, err := net.Listen("unix", transport.Socket(root))
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	resource.RegisterTaskServiceServer(g, scopedTasks{})
	go g.Serve(listener)
	defer g.Stop()
	runtime, err := mcpruntime.Start(root)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	session := core.Session{ID: core.ID(), ProjectID: "P", Workspace: root}
	if err := os.MkdirAll(core.Dir(root, session.ID), 0700); err != nil {
		t.Fatal(err)
	}
	if err := core.WriteJSON(filepath.Join(core.Dir(root, session.ID), "session.json"), session); err != nil {
		t.Fatal(err)
	}
	launch, err := mcpruntime.Prepare(root, session.ID, "first-run")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	c, err := net.Dial("unix", mcpruntime.Socket(root))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := json.NewEncoder(c).Encode(mcpruntime.Hello{Session: session.ID, Server: "cxz_tasks", Token: launch.Token}); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "builtin-test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, &mcp.IOTransport{Reader: c, Writer: c}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	out, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "task_list", Arguments: map[string]any{}})
	if err != nil || out.IsError {
		t.Fatalf("builtin did not reach the scoped Task API: %v %v", out, err)
	}
	if _, err := mcpruntime.Prepare(root, session.ID, "second-run"); err != nil {
		t.Fatal(err)
	}
	out, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "task_list", Arguments: map[string]any{}})
	if err == nil && !out.IsError {
		t.Fatal("an existing connection adopted a new launch run")
	}
	// The production stdio bridge reconnects using the new launch capability.
	in, iw := io.Pipe()
	or, ow := io.Pipe()
	defer in.Close()
	defer iw.Close()
	defer or.Close()
	defer ow.Close()
	go mcpruntime.Bridge(ctx, root, session.ID, "cxz_tasks", in, ow)
	cs2, err := mcp.NewClient(&mcp.Implementation{Name: "bridge-test", Version: "1"}, nil).Connect(ctx, &mcp.IOTransport{Reader: or, Writer: iw}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs2.Close()
	out, err = cs2.CallTool(ctx, &mcp.CallToolParams{Name: "task_list", Arguments: map[string]any{}})
	if err != nil || out.IsError {
		t.Fatalf("stdio bridge did not reach Task API: %v %v", out, err)
	}
}
