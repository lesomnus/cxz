package websandbox

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func ref(id string) *resource.SessionRef {
	return resource.SessionRef_builder{RuntimeId: proto.String(id)}.Build()
}
func send(t *testing.T, x *Sessions, id, client string) {
	t.Helper()
	_, err := x.Send(t.Context(), resource.SessionSendRequest_builder{Ref: ref(id), Text: proto.String("fixture input"), ClientId: proto.String(client)}.Build())
	if err != nil {
		t.Fatal(err)
	}
}
func idle(t *testing.T, x *Sessions, id string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		v, err := x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: ref(id)}.Build())
		if err != nil {
			t.Fatal(err)
		}
		if v.GetStatus().GetState() == "idle" {
			return
		}
		select {
		case <-deadline:
			t.Fatal("job did not finish")
		case <-time.After(time.Millisecond):
		}
	}
}
func history(t *testing.T, x *Sessions, id string) []*resource.SessionEvent {
	t.Helper()
	var events []*resource.SessionEvent
	var after uint64
	for {
		v, err := x.History(t.Context(), resource.SessionEventsRequest_builder{Ref: ref(id), AfterSeq: proto.Uint64(after)}.Build())
		if err != nil {
			t.Fatal(err)
		}
		page := v.GetEvents()
		events = append(events, page...)
		if len(page) < 128 {
			return events
		}
		after = page[len(page)-1].GetSeq()
	}
}
func TestSeededJobsAndIsolation(t *testing.T) {
	run := func(seed uint64) []string {
		s := New(seed, time.Millisecond)
		defer s.Close()
		x := &Sessions{S: s}
		untouched := history(t, x, "session-1")
		send(t, x, "session-6", "once")
		send(t, x, "session-6", "once")
		idle(t, x, "session-6")
		var tools []string
		inputs := 0
		for _, e := range history(t, x, "session-6") {
			if e.GetKind() == "tool" {
				tools = append(tools, e.GetText())
			}
			if e.GetKind() == "input" {
				inputs++
			}
		}
		if inputs != 2 {
			t.Fatalf("duplicate send created %d inputs", inputs)
		}
		other := history(t, x, "session-1")
		if len(other) != len(untouched) || other[len(other)-1].GetSeq() != untouched[len(untouched)-1].GetSeq() {
			t.Fatal("cross-session event leak")
		}
		return tools
	}
	a, b, c := run(42), run(42), run(100)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same seed differs: %v %v", a, b)
	}
	if reflect.DeepEqual(a, c) {
		t.Fatalf("different seeds gave identical work: %v", a)
	}
}
func TestControlsCancelPendingWork(t *testing.T) {
	s := New(1, 20*time.Millisecond)
	defer s.Close()
	x := &Sessions{S: s}
	send(t, x, "session-1", "a")
	control := resource.SessionControl_builder{Ref: ref("session-1")}.Build()
	stopped, err := x.Stop(t.Context(), control)
	if err != nil {
		t.Fatal(err)
	}
	n := len(history(t, x, "session-1"))
	time.Sleep(120 * time.Millisecond)
	if len(history(t, x, "session-1")) != n {
		t.Fatal("stopped job appended results")
	}
	_, err = x.Send(t.Context(), resource.SessionSendRequest_builder{Ref: ref("session-1"), Text: proto.String("not yet")}.Build())
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	resumed, err := x.Resume(t.Context(), control)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.GetStatus().GetRunId() == stopped.GetStatus().GetRunId() {
		t.Fatal("resume reused old run")
	}
	_, err = x.Send(t.Context(), resource.SessionSendRequest_builder{Ref: ref("session-1"), RunId: proto.String(stopped.GetStatus().GetRunId()), Text: proto.String("stale")}.Build())
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatal("stale run accepted", err)
	}
	send(t, x, "session-1", "b")
	if _, err = x.Interrupt(t.Context(), control); err != nil {
		t.Fatal(err)
	}
	idle(t, x, "session-1")
}
func TestApprovalFilteringAndBoundedHistory(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Sessions{S: s}
	v, err := x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: ref("session-4")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	p := v.GetStatus().GetPending()[0]
	_, err = x.Reply(t.Context(), resource.SessionReplyRequest_builder{Ref: ref("session-4"), RunId: proto.String(p.GetRunId()), RequestId: proto.String(p.GetRequestId()), ClientId: proto.String("reply"), Allow: proto.Bool(true), AnswersJson: proto.String(`{"environment":"Development"}`)}.Build())
	if err != nil {
		t.Fatal(err)
	}
	idle(t, x, "session-4")
	list, err := x.List(t.Context(), resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Project: resource.ProjectRef_builder{RuntimeId: proto.String("project-2")}.Build()}.Build()}}.Build())
	if err != nil || len(list.GetItems()) != 1 {
		t.Fatalf("filter: %v %v", list, err)
	}
	first, _ := x.List(t.Context(), resource.SessionListRequest_builder{Size: 2}.Build())
	next, _ := x.List(t.Context(), resource.SessionListRequest_builder{Size: 2, After: first.GetNext()}.Build())
	if first.GetItems()[1].GetId()[15] == next.GetItems()[0].GetId()[15] {
		t.Fatal("pagination duplicate")
	}
	s.mu.Lock()
	st, _ := s.find(ref("session-3"))
	for i := 0; i < 4000; i++ {
		s.event(st, "input", "bounded", "", nil)
	}
	s.mu.Unlock()
	firstPage, err := x.History(t.Context(), resource.SessionEventsRequest_builder{Ref: ref("session-3")}.Build())
	if err != nil || len(firstPage.GetEvents()) != 128 {
		t.Fatal("history must use bounded pages", err)
	}
	es := history(t, x, "session-3")
	if len(es) != maxEvents || es[0].GetSeq() <= 1 {
		t.Fatal("unbounded history")
	}
	batch, err := x.History(context.Background(), resource.SessionEventsRequest_builder{Ref: ref("session-3"), AfterSeq: proto.Uint64(es[len(es)-2].GetSeq())}.Build())
	if err != nil || len(batch.GetEvents()) != 1 {
		t.Fatal("cursor replay", err)
	}
	// Returned messages must never mutate the server's in-memory state.
	batch.GetEvents()[0].SetText("external edit")
	if strings.Contains(history(t, x, "session-3")[maxEvents-1].GetText(), "external edit") {
		t.Fatal("RPC response aliases state")
	}
}

func TestModelControlsDoNotStartChatAndUseModelEfforts(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Sessions{S: s}
	configure := func(text, client string) error {
		_, err := x.Send(t.Context(), resource.SessionSendRequest_builder{Ref: ref("session-1"), Text: proto.String(text), ClientId: proto.String(client)}.Build())
		return err
	}
	if err := configure("/model sandbox-claude-compact", "blocked"); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("explicit effort must be cleared: %v", err)
	}
	for _, command := range []string{"/effort default", "/model sandbox-claude-compact", "/effort low"} {
		if err := configure(command, command); err != nil {
			t.Fatal(err)
		}
	}
	if err := configure("/effort low", "/effort low"); err != nil {
		t.Fatal(err)
	}
	if err := configure("/effort medium", "unsupported"); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("compact model accepted unsupported effort: %v", err)
	}
	if err := configure("/model invented", "unknown"); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("model changed without clearing effort: %v", err)
	}
	var inputs, settings int
	for _, e := range history(t, x, "session-1") {
		if e.GetKind() == "input" {
			inputs++
		}
		if e.GetKind() == "setting" {
			settings++
		}
	}
	if inputs != 1 || settings != 3 {
		t.Fatalf("settings created chat or duplicate event: inputs=%d settings=%d", inputs, settings)
	}
	v, err := x.Get(t.Context(), resource.SessionGetRequest_builder{Ref: ref("session-1")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if v.GetModel() != "sandbox-claude-compact" || v.GetStatus().GetState() != "idle" {
		t.Fatal("model control started a turn or failed to apply")
	}
}
