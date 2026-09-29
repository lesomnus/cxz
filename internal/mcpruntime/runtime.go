// Package mcpruntime owns project-local MCP processes. Agents use a small stdio
// bridge; each external stdio connection gets its own process and workspace.
package mcpruntime

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpconfig"
)

type Launch struct {
	Token  string             `json:"token"`
	Config mcpconfig.Snapshot `json:"config"`
}
type Hello struct{ Session, Server, Token string }
type State struct {
	State   string `json:"state"`
	Updated int64  `json:"updated"`
}
type Builtin func(context.Context, string, core.Session, io.ReadWriteCloser) error

var providers = map[string]Builtin{}
var instructions = map[string]string{}

func Register(id, name, instruction string, f Builtin) {
	mcpconfig.RegisterBuiltin(id, name)
	providers[id] = f
	instructions[id] = instruction
}
func Socket(root string) string         { return filepath.Join(root, "run", "mcp.sock") }
func LaunchPath(root, id string) string { return filepath.Join(core.Dir(root, id), "mcp-launch.json") }
func StatePath(root, id, server string) string {
	return filepath.Join(core.Dir(root, id), "mcp-"+server+".status.json")
}
func Prepare(root, id string) (Launch, error) {
	s, e := mcpconfig.LoadRuntime(root)
	if e != nil {
		return Launch{}, e
	}
	v := Launch{Token: core.ID() + core.ID(), Config: s}
	e = core.WriteJSON(LaunchPath(root, id), v)
	return v, e
}
func ReadLaunch(root, id string) (Launch, error) {
	var v Launch
	b, e := os.ReadFile(LaunchPath(root, id))
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}

type Runtime struct {
	root     string
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]Hello
	closed   bool
	wg       sync.WaitGroup
}

func Start(root string) (*Runtime, error) {
	if e := os.MkdirAll(filepath.Join(root, "run"), 0700); e != nil {
		return nil, e
	}
	path := Socket(root)
	_ = os.Remove(path)
	l, e := net.Listen("unix", path)
	if e != nil {
		return nil, e
	}
	if e = os.Chmod(path, 0600); e != nil {
		l.Close()
		return nil, e
	}
	r := &Runtime{root: root, listener: l, conns: map[net.Conn]Hello{}}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			r.mu.Lock()
			if r.closed {
				r.mu.Unlock()
				c.Close()
				return
			}
			r.conns[c] = Hello{}
			r.wg.Add(1)
			r.mu.Unlock()
			go func() {
				defer r.wg.Done()
				defer c.Close()
				defer func() { r.mu.Lock(); delete(r.conns, c); r.mu.Unlock() }()
				r.serve(c)
			}()
		}
	}()
	return r, nil
}
func (r *Runtime) Close() error {
	r.mu.Lock()
	r.closed = true
	r.listener.Close()
	for c := range r.conns {
		c.Close()
	}
	r.mu.Unlock()
	r.wg.Wait()
	return nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (r *Runtime) serve(c net.Conn) {
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReaderSize(c, 4096)
	line, e := reader.ReadSlice('\n')
	if e != nil {
		return
	}
	var h Hello
	if json.Unmarshal(line, &h) != nil || mcpconfig.ValidateID(h.Server) != nil || !sessionID(h.Session) {
		return
	}
	launch, e := ReadLaunch(r.root, h.Session)
	if e != nil || len(launch.Token) < 32 || subtle.ConstantTimeCompare([]byte(h.Token), []byte(launch.Token)) != 1 {
		return
	}
	r.mu.Lock()
	r.conns[c] = h
	r.mu.Unlock()
	def, ok := launch.Config.Servers[h.Server]
	if !ok || def.Validate() != nil {
		return
	}
	var session core.Session
	b, e := os.ReadFile(filepath.Join(core.Dir(r.root, h.Session), "session.json"))
	if e != nil || json.Unmarshal(b, &session) != nil || session.ID != h.Session {
		return
	}
	_ = c.SetDeadline(time.Time{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := func(s string) {
		_ = core.WriteJSON(StatePath(r.root, h.Session, h.Server), State{s, time.Now().UnixMilli()})
	}
	state("connecting")
	finalState := "disconnected"
	defer func() { state(finalState) }()
	if def.Kind == "builtin" {
		f := providers[h.Server]
		if f == nil {
			return
		}
		state("connected")
		_ = f(ctx, r.root, session, bufferedConn{c, reader})
		return
	}
	if def.Kind != "stdio" {
		return
	}
	log, e := os.OpenFile(filepath.Join(core.Dir(r.root, h.Session), "mcp-"+h.Server+".stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if e != nil {
		return
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, def.Command, def.Args...)
	cmd.Dir = session.Workspace
	// Do not pass account credentials or manager secrets to external programs.
	cmd.Env = []string{}
	for _, k := range []string{"PATH", "HOME", "LANG", "LC_ALL", "TMPDIR"} {
		if v, ok := os.LookupEnv(k); ok {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	for k, v := range def.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Stderr = &limitedWriter{w: log, left: 1 << 20}
	in, e := cmd.StdinPipe()
	if e != nil {
		return
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		return
	}
	if e = cmd.Start(); e != nil {
		fmt.Fprintln(log, e)
		finalState = "failed"
		return
	}
	state("connected")
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(in, reader); in.Close(); done <- struct{}{} }()
	go func() { _, _ = io.Copy(c, out); done <- struct{}{} }()
	<-done
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cancel()
	c.Close()
	in.Close()
	out.Close()
	_ = cmd.Wait()
	<-done
}
func sessionID(s string) bool {
	if len(s) != 24 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

type limitedWriter struct {
	w    io.Writer
	left int
}

func (w *limitedWriter) Write(b []byte) (int, error) {
	n := len(b)
	if w.left > 0 {
		v := b[:min(len(b), w.left)]
		_, e := w.w.Write(v)
		w.left -= len(v)
		if e != nil {
			return 0, e
		}
	}
	return n, nil
}

// Disconnect affects this MCP connection only, never the agent process.
func (r *Runtime) Disconnect(session, server string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for c, h := range r.conns {
		if h.Session == session && h.Server == server {
			c.Close()
		}
	}
}

func Instructions(snapshot mcpconfig.Snapshot) string {
	ids := make([]string, 0, len(snapshot.Servers))
	for id := range snapshot.Servers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var parts []string
	for _, id := range ids {
		if text := instructions[id]; text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}
