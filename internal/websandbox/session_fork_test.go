package websandbox

import (
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestForkCopiesSettingsAndHistoryButKeepsIndependentSessions(t *testing.T) {
	for _, id := range []string{"session-1", "session-2"} {
		t.Run(id, func(t *testing.T) {
			s := New(1, time.Millisecond)
			defer s.Close()
			x := &Sessions{S: s}
			source, _ := s.find(ref(id))
			request := resource.SessionControl_builder{Ref: ref(id), RunId: proto.String(source.value.GetStatus().GetRunId()), ClientId: proto.String("fork-preview")}.Build()
			original := proto.Clone(source.value)
			child, err := x.Fork(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if child.GetRuntimeId() == source.value.GetRuntimeId() || child.GetAlias() == source.value.GetAlias() || child.GetAgent() != source.value.GetAgent() || child.GetName() != source.value.GetName() || child.GetModel() != source.value.GetModel() || !proto.Equal(child.GetProject(), source.value.GetProject()) {
				t.Fatal("wrong fork identity/settings", child)
			}
			again, err := x.Fork(t.Context(), request)
			if err != nil || again.GetRuntimeId() != child.GetRuntimeId() {
				t.Fatal("duplicate fork", again, err)
			}
			st, _ := s.find(ref(child.GetRuntimeId()))
			if len(st.events) != len(source.events)+1 || !proto.Equal(st.events[1], source.events[1]) || st.effort != source.effort {
				t.Fatal("history or effort missing")
			}
			st.events[1].SetText("child-only")
			if source.events[1].GetText() == "child-only" || !proto.Equal(source.value, original) {
				t.Fatal("fork mutated parent")
			}
			source.value.GetStatus().SetState("working")
			request.SetClientId("another-fork")
			if _, err := x.Fork(t.Context(), request); status.Code(err) != codes.FailedPrecondition {
				t.Fatal("forked in-flight turn", err)
			}
		})
	}
}
