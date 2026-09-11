package integration

import (
	"context"
	"encoding/json"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This test uses the existing CLI login. It never copies credentials and is opt-in
// because it consumes model usage. Raw transcripts remain in the private state dir.
func TestLiveClaude(t *testing.T) {
	if os.Getenv("CXZ_LIVE_TEST") != "1" {
		t.Skip("set CXZ_LIVE_TEST=1 to use the existing Claude login")
	}
	root, e := os.MkdirTemp("", "cxz-live-")
	if e != nil {
		t.Fatal(e)
	}
	t.Log("private evidence:", root)
	bin := filepath.Join(root, "cxz")
	build := exec.Command("go", "build", "-o", bin, "./cmd/cxz")
	build.Dir = "../.."
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build: %v %s", e, b)
	}
	work := filepath.Join(root, "work")
	state := filepath.Join(root, "state")
	os.Mkdir(work, 0700)
	log, e := os.OpenFile(filepath.Join(root, "daemon.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	daemon := exec.Command(bin, "--state", state, "serve")
	daemon.Stdout = log
	daemon.Stderr = log
	if e = daemon.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { daemon.Process.Kill(); daemon.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	conn, e := server.Dial(state)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	for i := 0; i < 100; i++ {
		c, done := context.WithTimeout(ctx, 100*time.Millisecond)
		_, e = client.List(c, &api.Empty{})
		done()
		if e == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if e != nil {
		t.Fatal(e)
	}
	s, e := client.Create(ctx, &api.CreateRequest{Workspace: work, ClientId: core.ID()})
	if e != nil {
		t.Fatal(e)
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
	get := func() *api.Session {
		v, e := client.Get(ctx, &api.SessionRef{Id: id})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	send := func(text string) uint64 {
		s = get()
		before := s.LastSeq
		if _, e = client.Send(ctx, &api.Input{SessionId: id, RunId: s.RunId, ClientId: core.ID(), Text: text}); e != nil {
			t.Fatal(e)
		}
		return before
	}
	until := func(after uint64, pred func(*api.Event) bool) *api.Event {
		c, done := context.WithTimeout(ctx, 90*time.Second)
		defer done()
		stream, e := client.Watch(c, &api.WatchRequest{SessionId: id, AfterSeq: after})
		if e != nil {
			t.Fatal(e)
		}
		for {
			v, e := stream.Recv()
			if e != nil {
				t.Fatal(e)
			}
			if v.Kind == "turn_end" && v.Text == "failed" {
				t.Fatalf("agent failed: %s", v.Payload)
			}
			if pred(v) {
				return v
			}
			if v.Kind == "turn_end" {
				t.Fatal("turn ended before the expected approval/event; inspect private evidence")
			}
		}
	}
	turn := func(v *api.Event) bool { return v.Kind == "turn_end" }
	after := send("Respond with exactly CXZ_LIVE_HELLO. Do not use any tools.")
	until(after, turn)
	t.Log("real conversation passed")
	after = send("Use Bash to run exactly: printf cxz-approved > approval.txt. Do not use any other tool. If permission is denied, stop.")
	approval := until(after, func(v *api.Event) bool { return v.Kind == "approval" })
	s = get()
	if _, e = client.Reply(ctx, &api.Answer{SessionId: id, RunId: s.RunId, ClientId: core.ID(), RequestId: approval.RequestId, Allow: true}); e != nil {
		t.Fatal(e)
	}
	until(approval.Seq, turn)
	b, e := os.ReadFile(filepath.Join(work, "approval.txt"))
	if e != nil || string(b) != "cxz-approved" {
		t.Fatalf("approval side effect: %s %v", b, e)
	}
	t.Log("real approval passed")
	after = send("Use Bash to run exactly: printf forbidden > denied.txt. If denied, stop without retrying or using another tool.")
	approval = until(after, func(v *api.Event) bool { return v.Kind == "approval" })
	s = get()
	if _, e = client.Reply(ctx, &api.Answer{SessionId: id, RunId: s.RunId, ClientId: core.ID(), RequestId: approval.RequestId}); e != nil {
		t.Fatal(e)
	}
	until(approval.Seq, turn)
	if _, e = os.Stat(filepath.Join(work, "denied.txt")); !os.IsNotExist(e) {
		t.Fatal("denied command ran")
	}
	t.Log("real denial passed")
	after = send("Use AskUserQuestion to ask which color I prefer, with Blue and Green options. Ask exactly 'Choose a color'. Do not guess the answer.")
	approval = until(after, func(v *api.Event) bool { return v.Kind == "approval" && v.Text == "AskUserQuestion" })
	var q struct {
		Input struct {
			Questions []struct {
				Question string `json:"question"`
			} `json:"questions"`
		} `json:"input"`
	}
	if e = json.Unmarshal(approval.Payload, &q); e != nil || len(q.Input.Questions) == 0 {
		t.Fatal("question schema")
	}
	answers := map[string]string{}
	for _, v := range q.Input.Questions {
		answers[v.Question] = "Blue"
	}
	a, _ := json.Marshal(answers)
	s = get()
	if _, e = client.Reply(ctx, &api.Answer{SessionId: id, RunId: s.RunId, ClientId: core.ID(), RequestId: approval.RequestId, Allow: true, AnswersJson: string(a)}); e != nil {
		t.Fatal(e)
	}
	until(approval.Seq, turn)
	t.Log("real question passed")
	after = send("Call Bash exactly once with command: sleep 20 && printf late > late.txt\nDo not run in background. If interrupted, do not retry.")
	approval = until(after, func(v *api.Event) bool { return v.Kind == "approval" })
	s = get()
	if _, e = client.Reply(ctx, &api.Answer{SessionId: id, RunId: s.RunId, ClientId: core.ID(), RequestId: approval.RequestId, Allow: true}); e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Second)
	interruptAt := time.Now()
	if _, e = client.Interrupt(ctx, &api.Control{SessionId: id, RunId: s.RunId, ClientId: core.ID()}); e != nil {
		t.Fatal(e)
	}
	v := until(approval.Seq, turn)
	if v.Text != "interrupted" {
		t.Fatalf("interrupt result %s", v.Text)
	}
	t.Log("real interrupt passed")
	// A daemon crash must not disturb the actual CLI's live process or run ID.
	old := get()
	daemon.Process.Kill()
	daemon.Wait()
	daemon = exec.Command(bin, "--state", state, "serve")
	daemon.Stdout = log
	daemon.Stderr = log
	if e = daemon.Start(); e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Second)
	s = get()
	if s.RunId != old.RunId {
		t.Fatal("daemon restart changed run")
	}
	t.Log("real daemon reconnect passed")
	// Stop and explicitly resume the same vendor session; never replay the interrupted prompt.
	if _, e = client.Stop(ctx, &api.Control{SessionId: id, RunId: s.RunId, ClientId: core.ID()}); e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Second)
	s = get()
	s, e = client.Resume(ctx, &api.Control{SessionId: id, RunId: s.RunId, ClientId: core.ID()})
	if e != nil {
		t.Fatal(e)
	}
	if s.VendorId != old.VendorId || s.RunId == old.RunId {
		t.Fatal("vendor resume mismatch")
	}
	after = send("The previous interrupted sleep task is canceled permanently. Do not run tools. Reply with exactly CXZ_RESUMED.")
	until(after, turn)
	if remaining := 22*time.Second - time.Since(interruptAt); remaining > 0 {
		time.Sleep(remaining)
	}
	if _, e = os.Stat(filepath.Join(work, "late.txt")); !os.IsNotExist(e) {
		t.Fatal("interrupted command completed")
	}
	t.Log("real vendor resume passed")
	summary := map[string]any{"passed": []string{"conversation", "approval", "denial", "question", "interrupt", "daemon_reconnect", "vendor_resume"}, "timestamp": time.Now().UTC().Format(time.RFC3339), "agent": "Claude Code", "note": "No credentials or raw model messages in this summary"}
	if e = core.WriteJSON(filepath.Join(root, "summary.json"), summary); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(root, os.TempDir()+"/cxz-live-") {
		t.Fatal("unexpected evidence path")
	}
}
