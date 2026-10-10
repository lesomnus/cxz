package websandbox

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type uploadFixtureStream struct {
	grpc.ServerStream
	ctx      context.Context
	messages []*resource.SessionUploadRequest
	reply    *resource.SessionAttachment
}

func (s *uploadFixtureStream) Context() context.Context { return s.ctx }
func (s *uploadFixtureStream) Recv() (*resource.SessionUploadRequest, error) {
	if len(s.messages) == 0 {
		return nil, io.EOF
	}
	r := s.messages[0]
	s.messages = s.messages[1:]
	return r, nil
}
func (s *uploadFixtureStream) SendAndClose(reply *resource.SessionAttachment) error {
	s.reply = reply
	return nil
}

func TestUploadsPreserveBytesAndEnforceRunSizeAndProjectScope(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := &Sessions{S: s}
	content := []byte("한글\r\n\x00\xff")
	header := func(run string, size int64) *resource.SessionUploadRequest {
		return resource.SessionUploadRequest_builder{Ref: resource.SessionRef_builder{RuntimeId: proto.String("session-1")}.Build(), RunId: &run, Name: proto.String("report.txt"), Size: &size}.Build()
	}
	stream := &uploadFixtureStream{ctx: t.Context(), messages: []*resource.SessionUploadRequest{header("session-1-run-1", int64(len(content))), resource.SessionUploadRequest_builder{Content: content}.Build()}}
	if err := x.Upload(stream); err != nil {
		t.Fatal(err)
	}
	path := stream.reply.GetPath()
	if !bytes.Equal(s.uploads[path].content, content) || s.uploadBytes != int64(len(content)) {
		t.Fatal("upload changed bytes")
	}
	p := &Projects{S: s}
	download := &fixtureStream[resource.ProjectDownloadReply]{ctx: t.Context()}
	request := resource.ProjectDownloadRequest_builder{Ref: resource.ProjectRef_builder{RuntimeId: proto.String("project-1")}.Build(), Path: &path}.Build()
	if err := p.Download(request, download); err != nil || len(download.replies) != 1 || !bytes.Equal(download.replies[0].GetData(), content) {
		t.Fatal("uploaded file not readable", err)
	}
	request.SetRef(resource.ProjectRef_builder{RuntimeId: proto.String("project-2")}.Build())
	if err := p.Download(request, download); status.Code(err) != codes.NotFound {
		t.Fatal("file crossed project boundary", err)
	}
	for _, invalid := range []*resource.SessionUploadRequest{header("stale", int64(len(content))), header("session-1-run-1", int64(len(content)+1))} {
		bad := &uploadFixtureStream{ctx: t.Context(), messages: []*resource.SessionUploadRequest{invalid, resource.SessionUploadRequest_builder{Content: content}.Build()}}
		if err := x.Upload(bad); err == nil || bad.reply != nil {
			t.Fatal("invalid upload committed")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cancelled := &uploadFixtureStream{ctx: ctx, messages: []*resource.SessionUploadRequest{header("session-1-run-1", 0)}}
	if err := x.Upload(cancelled); err == nil {
		t.Fatal("cancelled upload committed")
	}
	if _, err := x.Purge(t.Context(), resource.SessionPurgeRequest_builder{Ref: resource.SessionRef_builder{RuntimeId: proto.String("session-1")}.Build()}.Build()); err != nil || s.uploadBytes != 0 || len(s.uploads) != 0 {
		t.Fatal("purge retained uploads", err)
	}
}
