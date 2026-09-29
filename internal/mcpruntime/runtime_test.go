package mcpruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeIsolationAndLaunchSnapshot(t *testing.T) {
	root := t.TempDir()
	if e := core.Prepare(root); e != nil {
		t.Fatal(e)
	}
	cfg := mcpconfig.Snapshot{Servers: map[string]mcpconfig.Server{"echo": {Name: "Echo", Kind: "stdio", Command: "sh", Args: []string{"-c", "while read x; do printf '%s:%s\\n' \"$MARK\" \"$x\"; done"}, Env: map[string]string{"MARK": "original"}}}}
	if e := mcpconfig.SaveRuntime(root, cfg); e != nil {
		t.Fatal(e)
	}
	id := core.ID()
	os.MkdirAll(core.Dir(root, id), 0700)
	core.WriteJSON(filepath.Join(core.Dir(root, id), "session.json"), core.Session{ID: id, Workspace: root})
	launch, e := Prepare(root, id)
	if e != nil {
		t.Fatal(e)
	}
	runtime, e := Start(root)
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	// Editing desired settings does not grant or revoke tools from existing runs.
	mcpconfig.SaveRuntime(root, mcpconfig.Snapshot{})
	connect := func(token, server string) net.Conn {
		t.Helper()
		c, e := net.Dial("unix", Socket(root))
		if e != nil {
			t.Fatal(e)
		}
		c.SetDeadline(time.Now().Add(3 * time.Second))
		json.NewEncoder(c).Encode(Hello{id, server, token})
		return c
	}
	bad := connect("wrong", "echo")
	if _, e := bufio.NewReader(bad).ReadByte(); e == nil {
		t.Fatal("accepted wrong capability")
	}
	bad.Close()
	bad = connect(launch.Token, "other")
	if _, e := bufio.NewReader(bad).ReadByte(); e == nil {
		t.Fatal("accepted disabled tool")
	}
	bad.Close()
	a, b := connect(launch.Token, "echo"), connect(launch.Token, "echo")
	defer a.Close()
	defer b.Close()
	io.WriteString(a, "one\n")
	io.WriteString(b, "two\n")
	for c, want := range map[net.Conn]string{a: "original:one\n", b: "original:two\n"} {
		got, e := bufio.NewReader(c).ReadString('\n')
		if e != nil || got != want {
			t.Fatalf("%q %v", got, e)
		}
	}
}
func TestRuntimeCloseDisconnectsClients(t *testing.T) {
	root := t.TempDir()
	core.Prepare(root)
	id := core.ID()
	os.MkdirAll(core.Dir(root, id), 0700)
	core.WriteJSON(filepath.Join(core.Dir(root, id), "session.json"), core.Session{ID: id, Workspace: root})
	mcpconfig.SaveRuntime(root, mcpconfig.Snapshot{Servers: map[string]mcpconfig.Server{"cat": {Name: "cat", Kind: "stdio", Command: "cat"}}})
	launch, _ := Prepare(root, id)
	r, e := Start(root)
	if e != nil {
		t.Fatal(e)
	}
	c, e := net.Dial("unix", Socket(root))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	json.NewEncoder(c).Encode(Hello{id, "cat", launch.Token})
	io.WriteString(c, "hello\n")
	bufio.NewReader(c).ReadString('\n')
	done := make(chan struct{})
	go func() { r.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runtime leaked connection")
	}
	_ = context.Background()
}

func TestBridgeReconnectDoesNotReplayCalls(t *testing.T) {
	root := t.TempDir()
	core.Prepare(root)
	id := core.ID()
	os.MkdirAll(core.Dir(root, id), 0700)
	core.WriteJSON(filepath.Join(core.Dir(root, id), "session.json"), core.Session{ID: id, Workspace: root})
	// A tiny protocol peer can initialize and echo request IDs, but hangs on work.
	Register("reconnect_test", "Test", "", func(ctx context.Context, root string, s core.Session, c io.ReadWriteCloser) error {
		scanner := bufio.NewScanner(c)
		ready := false
		for scanner.Scan() {
			var v map[string]any
			if json.Unmarshal(scanner.Bytes(), &v) != nil {
				return nil
			}
			method, _ := v["method"].(string)
			if method == "initialize" {
				ready = true
			}
			if method == "hang" || v["id"] == nil {
				continue
			}
			if !ready {
				return nil
			}
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": v["id"], "result": map[string]any{}})
			if _, e := c.Write(append(b, '\n')); e != nil {
				return e
			}
		}
		return nil
	})
	defer func() { delete(providers, "reconnect_test") }()
	mcpconfig.SaveRuntime(root, mcpconfig.Snapshot{Servers: map[string]mcpconfig.Server{"reconnect_test": {Name: "test", Kind: "builtin"}}})
	Prepare(root, id)
	r, e := Start(root)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { r.Close() }()
	input, writer := io.Pipe()
	output, client := io.Pipe()
	defer input.Close()
	defer writer.Close()
	defer output.Close()
	defer client.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go Bridge(ctx, root, id, "reconnect_test", input, client)
	reader := bufio.NewReader(output)
	send := func(v string) {
		t.Helper()
		if _, e := io.WriteString(writer, v+"\n"); e != nil {
			t.Fatal(e)
		}
	}
	read := func() map[string]any {
		t.Helper()
		done := make(chan []byte, 1)
		go func() { b, _ := reader.ReadBytes('\n'); done <- b }()
		select {
		case b := <-done:
			var v map[string]any
			if e := json.Unmarshal(b, &v); e != nil {
				t.Fatal(e, string(b))
			}
			return v
		case <-time.After(5 * time.Second):
			t.Fatal("bridge timeout")
			return nil
		}
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	read()
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	send(`{"jsonrpc":"2.0","id":2,"method":"hang"}`)
	r.Close()
	if got := read(); got["error"] == nil || got["id"] != float64(2) {
		t.Fatal(got)
	}
	r, e = Start(root)
	if e != nil {
		t.Fatal(e)
	}
	send(`{"jsonrpc":"2.0","id":3,"method":"ping"}`)
	if got := read(); got["result"] == nil || got["id"] != float64(3) {
		t.Fatal(got)
	}
}
