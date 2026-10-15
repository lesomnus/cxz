package websandbox

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/taskapitest"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func TestTaskContract(t *testing.T) {
	s := New(42, time.Millisecond)
	defer s.Close()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	s.Register(server)
	go server.Serve(listener)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///task-sandbox", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	actors := []*resource.TaskActor{}
	for _, st := range s.sessions[:2] {
		actors = append(actors, resource.TaskActor_builder{Session: resource.SessionRef_builder{RuntimeId: proto.String(st.value.GetRuntimeId())}.Build(), RunId: proto.String(st.value.GetStatus().GetRunId())}.Build())
	}
	taskapitest.Run(t, conn, taskapitest.Fixture{
		Project: resource.ProjectRef_builder{RuntimeId: proto.String("project-1")}.Build(), OtherProject: resource.ProjectRef_builder{RuntimeId: proto.String("project-2")}.Build(), Actors: actors,
		Restart: func(id string) {
			s.mu.Lock()
			defer s.mu.Unlock()
			st, err := s.find(resource.SessionRef_builder{RuntimeId: proto.String(id)}.Build())
			if err != nil {
				t.Fatal(err)
			}
			st.value.GetStatus().SetRunId("replacement-run")
		},
	})
}
