package websandbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type fixtureStream[T any] struct {
	grpc.ServerStream
	ctx     context.Context
	replies []*T
}

func (s *fixtureStream[T]) Context() context.Context { return s.ctx }
func (s *fixtureStream[T]) Send(r *T) error          { s.replies = append(s.replies, r); return nil }

func TestEditorFixturesProjectIsolationAndCancellation(t *testing.T) {
	server := New(1, time.Millisecond)
	defer server.Close()
	p := &Projects{S: server}
	project := func(id string) *resource.ProjectRef {
		return resource.ProjectRef_builder{RuntimeId: proto.String(id)}.Build()
	}
	for _, id := range []string{"project-1", "project-2"} {
		listing := &fixtureStream[resource.ProjectPathsReply]{ctx: t.Context()}
		if err := p.Paths(resource.ProjectPathsRequest_builder{Ref: project(id), Path: proto.String("/workspace")}.Build(), listing); err != nil {
			t.Fatal(err)
		}
		if len(listing.replies) != 1 || len(listing.replies[0].GetEntries()) != 3 {
			t.Fatal("missing workspace fixtures")
		}
		download := &fixtureStream[resource.ProjectDownloadReply]{ctx: t.Context()}
		if err := p.Download(resource.ProjectDownloadRequest_builder{Ref: project(id), Path: proto.String("/workspace/README.md")}.Build(), download); err != nil {
			t.Fatal(err)
		}
		if len(download.replies) != 1 || !strings.Contains(string(download.replies[0].GetData()), id) || download.replies[0].GetTotalSize() != int64(len(download.replies[0].GetData())) {
			t.Fatal("wrong project file snapshot")
		}
		reply, err := p.Editor(t.Context(), resource.ProjectEditorRequest_builder{Ref: project(id)}.Build())
		if err != nil || !reply.GetSimulated() || reply.GetWorkspace() != "/workspace" || reply.GetConnectionToken() != "" {
			t.Fatal("fake Connect is not isolated from real credentials", err)
		}
	}
	listing := &fixtureStream[resource.ProjectPathsReply]{ctx: t.Context()}
	if err := p.Paths(resource.ProjectPathsRequest_builder{Ref: project("project-1"), Path: proto.String("/workspace/../../etc")}.Build(), listing); status.Code(err) != codes.PermissionDenied {
		t.Fatal("fixture path escape accepted", err)
	}
	if _, err := p.Editor(t.Context(), resource.ProjectEditorRequest_builder{Ref: project("missing")}.Build()); status.Code(err) != codes.NotFound {
		t.Fatal("unknown project connected", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.Editor(ctx, resource.ProjectEditorRequest_builder{Ref: project("project-1")}.Build()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled Connect completed", err)
	}
}
