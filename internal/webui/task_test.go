package webui

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/lesomnus/cxz/internal/taskapi"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type readyTasks struct {
	resource.UnimplementedTaskServiceServer
}

func (readyTasks) Watch(_ *resource.TaskWatchRequest, out grpc.ServerStreamingServer[resource.TaskWatchResponse]) error {
	if err := taskapi.WatchReady(out); err != nil {
		return err
	}
	<-out.Context().Done()
	return status.FromContextError(out.Context().Err()).Err()
}

func TestTaskReadinessReachesConnectBeforeAnyChanges(t *testing.T) {
	g := grpc.NewServer()
	defer g.Stop()
	resource.RegisterTaskServiceServer(g, readyTasks{})
	server := httptest.NewServer(testManagerRPC(t, g))
	defer server.Close()
	client := connect.NewClient[resource.TaskWatchRequest, resource.TaskWatchResponse](server.Client(), server.URL+resource.TaskService_Watch_FullMethodName)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := client.CallServerStream(ctx, connect.NewRequest(resource.TaskWatchRequest_builder{SkipSnapshot: true}.Build()))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if got := stream.ResponseHeader().Get(taskapi.WatchReadyHeader); got != "1" {
		t.Fatalf("Task subscription readiness header = %q", got)
	}
	cancel()
}
