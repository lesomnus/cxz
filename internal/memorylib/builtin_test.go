//go:build !windows

package memorylib

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/cxz/internal/mcpruntime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuiltinActivationAndRuntimeSessions(t *testing.T) {
	root := t.TempDir()
	core.Prepare(root)
	cfg, e := mcpconfig.Load(root)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Servers["cxz_memory"].Kind != "builtin" || !cfg.Servers["cxz_memory"].Enabled {
		t.Fatal(cfg)
	}
	on := cfg.Resolve("P")
	if mcpruntime.Instructions(on) == "" {
		t.Fatal("missing builtin instructions")
	}
	off := false
	mcpconfig.Apply(root, mcpconfig.Request{Action: "enable", Project: "P", ID: "cxz_memory", Enabled: &off})
	cfg, _ = mcpconfig.Load(root)
	if len(cfg.Resolve("P").Servers) != 0 || mcpruntime.Instructions(cfg.Resolve("P")) != "" {
		t.Fatal("disabled memory consumes context")
	}
	if len(cfg.Resolve("Q").Servers) != 1 {
		t.Fatal("other project affected")
	}
	if e = mcpconfig.SaveRuntime(root, on); e != nil {
		t.Fatal(e)
	}
	runtime, e := mcpruntime.Start(root)
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connect := func() (core.Session, *mcp.ClientSession) {
		t.Helper()
		s := core.Session{ID: core.ID(), ProjectID: "P", Workspace: root}
		os.MkdirAll(core.Dir(root, s.ID), 0700)
		core.WriteJSON(filepath.Join(core.Dir(root, s.ID), "session.json"), s)
		launch, e := mcpruntime.Prepare(root, s.ID)
		if e != nil {
			t.Fatal(e)
		}
		c, e := net.Dial("unix", mcpruntime.Socket(root))
		if e != nil {
			t.Fatal(e)
		}
		json.NewEncoder(c).Encode(mcpruntime.Hello{Session: s.ID, Server: "cxz_memory", Token: launch.Token})
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
		cs, e := client.Connect(ctx, &mcp.IOTransport{Reader: c, Writer: c}, nil)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { cs.Close() })
		return s, cs
	}
	a, ca := connect()
	_, cb := connect()
	out, e := ca.CallTool(ctx, &mcp.CallToolParams{Name: "memory_update", Arguments: map[string]any{"document": "decisions.md", "content": "shared decision"}})
	if e != nil || out.IsError {
		t.Fatal(out, e)
	}
	out, e = cb.CallTool(ctx, &mcp.CallToolParams{Name: "memory_read", Arguments: map[string]any{"id": New(root, a).OwnID(), "document": "decisions.md"}})
	if e != nil || out.IsError {
		t.Fatal(out, e)
	}
	// Verify reconnectable stdio bridge also speaks the SDK's real protocol.
	in, iw := io.Pipe()
	or, ow := io.Pipe()
	defer in.Close()
	defer iw.Close()
	defer or.Close()
	defer ow.Close()
	go mcpruntime.Bridge(ctx, root, a.ID, "cxz_memory", in, ow)
	client := mcp.NewClient(&mcp.Implementation{Name: "bridge-test", Version: "1"}, nil)
	cs, e := client.Connect(ctx, &mcp.IOTransport{Reader: or, Writer: iw}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer cs.Close()
	tools, e := cs.ListTools(ctx, nil)
	if e != nil || len(tools.Tools) != 5 {
		t.Fatal(tools, e)
	}
}
