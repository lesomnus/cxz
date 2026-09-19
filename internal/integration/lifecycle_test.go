package integration

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLifecycle(t *testing.T) {
	// Short paths are intentional: Linux Unix sockets have a 108-byte path limit.
	root, e := os.MkdirTemp("", "cxz-it-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(root)
	repo, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	bin := filepath.Join(root, "cxz")
	fake := filepath.Join(root, "fake")
	for _, v := range [][2]string{{bin, "./cmd/cxz"}, {fake, "./internal/testagent"}} {
		args := []string{"build", "-o", v[0]}
		if os.Getenv("CXZ_TEST_RACE") == "1" {
			args = append(args, "-race")
		}
		args = append(args, v[1])
		c := exec.Command("go", args...)
		c.Dir = repo
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("build: %s %v", b, e)
		}
	}
	state := filepath.Join(root, "state")
	work := filepath.Join(root, "work")
	os.Mkdir(work, 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, e := server.Dial(state)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	var daemon *exec.Cmd
	var log *os.File
	start := func() {
		var e error
		log, e = os.OpenFile(filepath.Join(root, "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		daemon = exec.Command(bin, "--state", state, "manager", "serve", "--agent", fake)
		daemon.Stdout = log
		daemon.Stderr = log
		if e = daemon.Start(); e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			c, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			_, e = client.List(c, &api.Empty{})
			cancel()
			if e == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		b, _ := os.ReadFile(filepath.Join(root, "daemon.log"))
		t.Fatalf("daemon failed: %v %s", e, b)
	}
	killDaemon := func() {
		if daemon != nil {
			daemon.Process.Kill()
			daemon.Wait()
			daemon = nil
			log.Close()
		}
	}
	defer killDaemon()
	start()
	if e = client.EnsureAccount(ctx, "test-claude", "claude"); e != nil {
		t.Fatal(e)
	}
	create := &api.CreateRequest{Workspace: work, ClientId: "create-1", Model: "fixture-model", Account: "test-claude"}
	if _, err := client.Create(ctx, create); err == nil {
		t.Fatal("unauthenticated session started")
	} else {
		details := status.Convert(err).Details()
		if len(details) != 1 {
			t.Fatalf("login details lost across RPC: %v", err)
		}
		info, ok := details[0].(*errdetails.ErrorInfo)
		if !ok || info.Reason != "PROJECT_LOGIN_REQUIRED" || info.Metadata["account"] != "test-claude" {
			t.Fatal(details)
		}
	}
	beforeLogin, err := client.List(ctx, &api.Empty{})
	if err != nil || len(beforeLogin.Sessions) != 0 {
		t.Fatal("created session before login", beforeLogin, err)
	}
	if e = accounts.Install(accounts.SessionRoot(state, create.ClientId), "test-claude", "claude", []byte(`{"claudeAiOauth":{"accessToken":"synthetic-test-only"}}`)); e != nil {
		t.Fatal(e)
	}
	s, e := client.Create(ctx, create)
	if e != nil {
		t.Fatal(e)
	}
	if s.Model != "fixture-model" || s.Account != "test-claude" {
		t.Fatal("model missing from session")
	}
	binding := s.AuthBinding
	// Exercise generated Payday maintenance RPCs through the real runtime and
	// fixture supervisor. An ineligible update must not restart this run.
	if _, err := client.Activity(ctx, &api.ActivityInput{SessionId: s.Id, RunId: s.RunId, ClientId: "fixture-ui", Busy: true}); err != nil {
		t.Fatal(err)
	}
	maintenance, err := client.UpdateAgent(ctx, &api.AgentUpdateInput{SessionId: s.Id, RunId: s.RunId, Binary: "/cxz/tools/claude/9.0.0/linux-x64/claude", Apply: true})
	if err != nil || maintenance.Ready || maintenance.Binary != fake {
		t.Fatal("busy fixture allowed maintenance", maintenance, err)
	}
	if _, err := client.Activity(ctx, &api.ActivityInput{SessionId: s.Id, RunId: "stale", ClientId: "fixture-ui"}); err == nil {
		t.Fatal("stale activity accepted")
	}
	if _, err := client.Activity(ctx, &api.ActivityInput{SessionId: s.Id, RunId: s.RunId, ClientId: "fixture-ui"}); err != nil {
		t.Fatal(err)
	}
	if binding == "" || s.AuthBackend != accounts.ProjectLocalOAuth {
		t.Fatal("missing auth binding/backend", s)
	}
	listed, e := client.List(ctx, &api.Empty{})
	if e != nil || len(listed.GetSessions()) != 1 || listed.Sessions[0].Id != s.Id || listed.Sessions[0].Workspace != work {
		t.Fatalf("resource list lost session or project edge: %v %v", listed, e)
	}
	id := s.Id
	defer func() {
		var p map[string]int
		b, _ := os.ReadFile(filepath.Join(core.Dir(state, id), "pid.json"))
		json.Unmarshal(b, &p)
		if p["agent"] > 0 {
			syscall.Kill(-p["agent"], syscall.SIGKILL)
		}
		if p["supervisor"] > 0 {
			syscall.Kill(p["supervisor"], syscall.SIGKILL)
		}
	}()
	attachment, err := client.Attach(ctx, &api.AttachmentInput{SessionId: id, RunId: s.RunId, Content: []byte("synthetic attachment\nline two\n")})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(attachment.Path)
	if err != nil || string(data) != "synthetic attachment\nline two\n" {
		t.Fatal("attachment RPC did not persist source", err)
	}
	if _, err := client.Attach(ctx, &api.AttachmentInput{SessionId: id, RunId: "stale", Content: []byte("reject")}); err == nil {
		t.Fatal("stale attachment accepted")
	}
	same, e := client.Create(ctx, create)
	if e != nil || same.Id != id {
		t.Fatalf("create idempotency: %v", e)
	}
	if _, e = client.Create(ctx, &api.CreateRequest{Workspace: work, ClientId: "second", Account: "test-claude"}); e == nil {
		t.Fatal("new session reused another session's authentication")
	}
	if e = accounts.Install(accounts.SessionRoot(state, "second"), "test-claude", "claude", []byte(`{"claudeAiOauth":{"accessToken":"second-independent-login"}}`)); e != nil {
		t.Fatal(e)
	}
	second, e := client.Create(ctx, &api.CreateRequest{Workspace: work, ClientId: "second", Account: "test-claude"})
	if e != nil {
		t.Fatalf("concurrent session: %v", e)
	}
	defer client.Stop(context.Background(), &api.Control{SessionId: second.Id, RunId: second.RunId, ClientId: "cleanup-second"})
	if second.Id == id {
		t.Fatal("concurrent sessions merged")
	}
	duplicateProcess := exec.CommandContext(ctx, bin, "--state", state, "_supervise", id)
	if err := duplicateProcess.Run(); err == nil {
		t.Fatal("same session started twice")
	}
	if _, e = client.Stop(ctx, &api.Control{SessionId: second.Id, RunId: second.RunId, ClientId: "stop-second"}); e != nil {
		t.Fatal(e)
	}
	if first, err := client.Get(ctx, &api.SessionRef{Id: id}); err != nil || first.State != "idle" {
		t.Fatalf("stopping second affected first: %v %v", first, err)
	}
	second, e = client.Resume(ctx, &api.Control{SessionId: second.Id, RunId: second.RunId, ClientId: "resume-second"})
	if e != nil {
		t.Fatalf("resume alongside first: %v", e)
	}
	if _, e = client.Stop(ctx, &api.Control{SessionId: second.Id, RunId: second.RunId, ClientId: "stop-second-again"}); e != nil {
		t.Fatal(e)
	}
	get := func() *api.Session {
		t.Helper()
		v, e := client.Get(ctx, &api.SessionRef{Id: id})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	await := func(state string) *api.Session {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			v := get()
			if v.State == state {
				return v
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("expected %s got %s", state, get().State)
		return nil
	}
	send := func(text string) *api.Input {
		t.Helper()
		s = get()
		r := &api.Input{SessionId: id, RunId: s.RunId, ClientId: core.ID(), Text: text}
		if v, e := client.Send(ctx, r); e != nil || v.Status != "accepted" {
			t.Fatalf("send: %v %v", v, e)
		}
		return r
	}
	readUntil := func(after uint64, predicate func(*api.Event) bool) []*api.Event {
		t.Helper()
		c, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		stream, e := client.Watch(c, &api.WatchRequest{SessionId: id, AfterSeq: after})
		if e != nil {
			t.Fatal(e)
		}
		var events []*api.Event
		for {
			v, e := stream.Recv()
			if e != nil {
				t.Fatal(e)
			}
			if len(events) > 0 && v.Seq != events[len(events)-1].Seq+1 {
				t.Fatal("noncontiguous replay")
			}
			events = append(events, v)
			if predicate(v) {
				return events
			}
		}
	}
	r := send("hello")
	readUntil(0, func(v *api.Event) bool { return v.Kind == "turn_end" })
	await("idle")
	if v, e := client.Send(ctx, r); e != nil || v.Status != "accepted" {
		t.Fatalf("retry: %v %v", v, e)
	}
	r.Text = "different"
	if _, e = client.Send(ctx, r); e == nil {
		t.Fatal("conflicting retry accepted")
	}
	// Native context/compaction traverse payday Send, runtime RPC and the
	// supervisor protocol; compaction must never erase the cxz journal.
	beforeContext := get().LastSeq
	send("/context")
	readUntil(beforeContext, func(v *api.Event) bool { return v.Kind == "assistant" && strings.Contains(v.Text, "Context fixture") })
	await("idle")
	beforeCompact := get().LastSeq
	send("/compact")
	readUntil(beforeCompact, func(v *api.Event) bool { return v.Kind == "compact" })
	await("idle")
	postCompact, err := client.History(ctx, &api.WatchRequest{SessionId: id})
	if err != nil {
		t.Fatal(err)
	}
	hello, quota := false, false
	for _, event := range postCompact.Events {
		if event.Kind == "input" && event.Text == "hello" {
			hello = true
		}
		if event.Kind == "usage" && event.Text == "get_usage" {
			quota = true
		}
	}
	if !hello || !quota {
		t.Fatal("journal or quota missing after compaction")
	}
	if get().PermissionMode != "full" {
		t.Fatal("new session must default to full")
	}
	beforeDefaultApproval := get().LastSeq
	send("approval default-full")
	readUntil(beforeDefaultApproval, func(v *api.Event) bool { return v.Kind == "turn_end" })
	s = await("idle")
	if _, err := client.Permission(ctx, &api.PermissionInput{SessionId: id, RunId: s.RunId, ClientId: core.ID(), Mode: "ask"}); err != nil {
		t.Fatal(err)
	}
	send("approval allow")
	s = await("waiting_input")
	if len(s.Pending) != 1 {
		t.Fatal("missing approval")
	}
	reply := &api.Answer{SessionId: id, RunId: s.RunId, ClientId: core.ID(), RequestId: s.Pending[0].RequestId, Allow: true}
	if _, e = client.Reply(ctx, reply); e != nil {
		t.Fatal(e)
	}
	await("idle")
	if _, e = client.Reply(ctx, reply); e != nil {
		t.Fatal("idempotent approval retry", e)
	}
	reply.ClientId = core.ID()
	if _, e = client.Reply(ctx, reply); e == nil {
		t.Fatal("stale approval accepted")
	}
	send("approval deny")
	s = await("waiting_input")
	if _, e = client.Reply(ctx, &api.Answer{SessionId: id, RunId: s.RunId, ClientId: core.ID(), RequestId: s.Pending[0].RequestId}); e != nil {
		t.Fatal(e)
	}
	await("idle")
	send("question")
	s = await("waiting_input")
	a := &api.Answer{SessionId: id, RunId: s.RunId, ClientId: core.ID(), RequestId: s.Pending[0].RequestId, Allow: true}
	if _, e = client.Reply(ctx, a); e == nil {
		t.Fatal("empty question answer accepted")
	}
	a.AnswersJson = `{"Choose a color":"Blue"}`
	if _, e = client.Reply(ctx, a); e != nil {
		t.Fatal(e)
	}
	await("idle")
	send("question")
	s = await("waiting_input")
	structured := &api.Answer{SessionId: id, RunId: s.RunId, ClientId: core.ID(), RequestId: s.Pending[0].RequestId, Allow: true, AnswersJson: `{"Choose a color":{"selected":[],"other":"  custom, color  "}}`}
	if _, err := client.Reply(ctx, structured); err != nil {
		t.Fatal("structured reply through server/runtime", err)
	}
	await("idle")
	if _, err := client.Reply(ctx, structured); err != nil {
		t.Fatal("structured reply retry", err)
	}
	send("wait")
	s = await("working")
	if _, e = client.Interrupt(ctx, &api.Control{SessionId: id, RunId: s.RunId, ClientId: core.ID()}); e != nil {
		t.Fatal(e)
	}
	await("idle")
	before := get()
	send("slow")
	killDaemon()
	time.Sleep(2200 * time.Millisecond)
	start()
	s = await("idle")
	if s.RunId != before.RunId {
		t.Fatal("daemon restart replaced agent")
	}
	events := readUntil(before.LastSeq, func(v *api.Event) bool { return v.Kind == "turn_end" })
	found := false
	for _, v := range events {
		if v.Kind == "assistant" && v.Text == "slow complete" {
			found = true
		}
	}
	if !found {
		t.Fatal("lost detached output")
	}
	send("approval recovery")
	s = await("waiting_input")
	old := s
	var p map[string]int
	b, _ := os.ReadFile(filepath.Join(core.Dir(state, id), "pid.json"))
	json.Unmarshal(b, &p)
	syscall.Kill(p["supervisor"], syscall.SIGKILL)
	await("interrupted")
	s, e = client.Resume(ctx, &api.Control{SessionId: id, RunId: old.RunId, ClientId: core.ID()})
	if e != nil {
		t.Fatal(e)
	}
	if s.RunId == old.RunId || s.VendorId != old.VendorId || len(s.Pending) != 0 || s.Model != "fixture-model" || s.Account != "test-claude" {
		t.Fatalf("bad resumed snapshot: %v", s)
	}
	if _, e = client.Reply(ctx, &api.Answer{SessionId: id, RunId: old.RunId, ClientId: core.ID(), RequestId: old.Pending[0].RequestId, Allow: true}); e == nil {
		t.Fatal("prior-run approval accepted")
	}
	time.Sleep(200 * time.Millisecond)
	if get().State != "idle" {
		t.Fatal("auto-replayed old prompt")
	}
	send("after resume")
	await("idle")

	// No TUI or event watcher participates in these approvals. Policy travels
	// through payday and is owned by the independently running supervisor.
	send("approval full-pending")
	s = await("waiting_input")
	enable := &api.PermissionInput{SessionId: id, RunId: s.RunId, ClientId: core.ID(), Mode: "full"}
	if r, err := client.Permission(ctx, enable); err != nil || r.Status != "accepted" {
		t.Fatal("permission RPC", r, err)
	}
	s = await("idle")
	if s.PermissionMode != "full" {
		t.Fatal("policy missing from resource projection")
	}
	if _, err := client.Permission(ctx, enable); err != nil {
		t.Fatal("permission retry", err)
	}
	// Another session can use manual approval independently, even with the same account.
	second, e = client.Resume(ctx, &api.Control{SessionId: second.Id, RunId: second.RunId, ClientId: core.ID()})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		var processes map[string]int
		raw, _ := os.ReadFile(filepath.Join(core.Dir(state, second.Id), "pid.json"))
		_ = json.Unmarshal(raw, &processes)
		if processes["agent"] > 0 {
			_ = syscall.Kill(-processes["agent"], syscall.SIGKILL)
		}
		if processes["supervisor"] > 0 {
			_ = syscall.Kill(processes["supervisor"], syscall.SIGKILL)
		}
	}()
	if _, err := client.Permission(ctx, &api.PermissionInput{SessionId: second.Id, RunId: second.RunId, ClientId: core.ID(), Mode: "ask"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Send(ctx, &api.Input{SessionId: second.Id, RunId: second.RunId, ClientId: core.ID(), Text: "approval isolated"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var other *api.Session
	for time.Now().Before(deadline) {
		other, _ = client.Get(ctx, &api.SessionRef{Id: second.Id})
		if other != nil && other.State == "waiting_input" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if other == nil || other.State != "waiting_input" || other.PermissionMode == "full" || len(other.Pending) != 1 {
		t.Fatal("session policy leaked", other)
	}
	if _, err := client.Reply(ctx, &api.Answer{SessionId: other.Id, RunId: other.RunId, ClientId: core.ID(), RequestId: other.Pending[0].RequestId}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Stop(ctx, &api.Control{SessionId: second.Id, RunId: second.RunId, ClientId: core.ID()}); err != nil {
		t.Fatal(err)
	}
	before = get()
	send("approval delayed")
	killDaemon()
	time.Sleep(1100 * time.Millisecond)
	start()
	s = await("idle")
	if s.RunId != before.RunId || s.PermissionMode != "full" {
		t.Fatal("detached policy or process lost")
	}
	detached := readUntil(before.LastSeq, func(e *api.Event) bool { return e.Kind == "turn_end" })
	allowed := false
	for _, e := range detached {
		if e.Kind == "assistant" && e.Text == "approval delayed: allow" {
			allowed = true
		}
	}
	if !allowed {
		t.Fatal("supervisor did not approve with manager and client disconnected")
	}
	// SIGKILL of the supervisor must also cancel a shell grandchild, not only
	// the agent itself. The liveness guardian owns the same process-group boundary.
	before = get()
	send("spawn-child")
	readUntil(before.LastSeq, func(v *api.Event) bool { return v.Kind == "assistant" && v.Text == "child started" })
	s = get()
	b, _ = os.ReadFile(filepath.Join(core.Dir(state, id), "pid.json"))
	json.Unmarshal(b, &p)
	syscall.Kill(p["supervisor"], syscall.SIGKILL)
	await("interrupted")
	time.Sleep(2300 * time.Millisecond)
	if _, e = os.Stat(filepath.Join(work, "orphan.txt")); !os.IsNotExist(e) {
		t.Fatal("shell descendant survived supervisor death")
	}
	if _, e = client.Resume(ctx, &api.Control{SessionId: id, RunId: s.RunId, ClientId: core.ID()}); e != nil {
		t.Fatal(e)
	}
	s = get()
	if s.PermissionMode != "full" {
		t.Fatal("supervisor restart lost permission")
	}
	send("approval after-policy-resume")
	s = await("idle")
	s = get()
	if _, e = client.Stop(ctx, &api.Control{SessionId: id, RunId: s.RunId, ClientId: core.ID()}); e != nil {
		t.Fatal(e)
	}
	stopped := await("stopped")
	// Rebuild SQLite from manifests + journal without losing the transcript.
	killDaemon()
	for _, name := range []string{"cxz.db", "cxz.db-wal", "cxz.db-shm", "resources.db", "resources.db-wal", "resources.db-shm"} {
		path := filepath.Join(state, name)
		if _, e = os.Stat(path); e == nil {
			if e = os.Rename(path, path+".backup"); e != nil {
				t.Fatal(e)
			}
		}
	}
	start()
	restored := get()
	if restored.PermissionMode != "full" {
		t.Fatal("SQLite rebuild lost permission policy")
	}
	if restored.AuthBinding != binding || restored.AuthBackend != accounts.ProjectLocalOAuth {
		t.Fatal("auth strategy lost on reconstruction", restored)
	}
	if restored.LastSeq != stopped.LastSeq || restored.VendorId == "" || !strings.Contains(restored.Workspace, "work") || restored.Account != "test-claude" {
		t.Fatal("bad database reconstruction")
	}
	if duplicate, e := client.Create(ctx, create); e != nil || duplicate.Id != id {
		t.Fatal("create idempotency lost after SQLite reconstruction", e)
	}
	for _, name := range []string{filepath.Join(root, "daemon.log"), filepath.Join(core.Dir(state, id), "supervisor.log"), filepath.Join(core.Dir(state, id), "agent.stderr.log")} {
		b, _ := os.ReadFile(name)
		if strings.Contains(string(b), "WARNING: DATA RACE") {
			t.Fatalf("child process race in %s", name)
		}
	}
	// Logs traverse the public payday API, including on a stopped session.
	for _, project := range []bool{false, true} {
		report, err := client.Logs(ctx, &api.LogsRequest{SessionId: id, Project: project})
		if err != nil || !strings.Contains(report.Text, "supervisor") || !strings.Contains(report.Text, "[permission]") {
			t.Fatalf("logs unavailable: %v %v", report, err)
		}
	}
	if get().LastSeq != restored.LastSeq {
		t.Fatal("reading logs mutated the journal")
	}
	// Delete a live session through payday, then restart without resurrecting it.
	if _, e = client.Resume(ctx, &api.Control{SessionId: id, RunId: restored.RunId, ClientId: core.ID()}); e != nil {
		t.Fatal(e)
	}
	if e = client.DeleteSession(ctx, id); e != nil {
		t.Fatal("delete live session", e)
	}
	if e = client.DeleteSession(ctx, second.Id); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(core.Dir(state, id), "session.json")); e != nil {
		t.Fatal("deleted recoverable manifest", e)
	}
	killDaemon()
	start()
	remaining, e := client.List(ctx, &api.Empty{})
	if e != nil || len(remaining.Sessions) != 0 {
		t.Fatal("deleted session resurrected after restart", remaining, e)
	}
	if _, e = client.Get(ctx, &api.SessionRef{Id: id}); e == nil {
		t.Fatal("deleted session accessible")
	}
	t.Log("create, conversation, allow/deny/question, interrupt, replay, daemon/supervisor recovery, SQLite rebuild passed")
}
