package websandbox

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type sandboxTerminalStream struct {
	fixtureStream[resource.ProjectTerminalReply]
	requests []*resource.ProjectTerminalRequest
}

func (s *sandboxTerminalStream) Recv() (*resource.ProjectTerminalRequest, error) {
	if len(s.requests) == 0 {
		return nil, io.EOF
	}
	r := s.requests[0]
	s.requests = s.requests[1:]
	return r, nil
}

func TestSimulatedTerminalProjectFilesInputExitAndNoJournal(t *testing.T) {
	server := New(1, time.Millisecond)
	defer server.Close()
	p := &Projects{S: server}
	before := len(server.sessions[0].events)
	for _, id := range []string{"project-1", "project-2"} {
		stream := &sandboxTerminalStream{fixtureStream: fixtureStream[resource.ProjectTerminalReply]{ctx: t.Context()}, requests: []*resource.ProjectTerminalRequest{
			resource.ProjectTerminalRequest_builder{Ref: resource.ProjectRef_builder{RuntimeId: &id}.Build(), Columns: proto.Uint32(80), Rows: proto.Uint32(24)}.Build(),
			resource.ProjectTerminalRequest_builder{Input: []byte("echo cancelled\x03echo unicode-\xe2")}.Build(),
			resource.ProjectTerminalRequest_builder{Input: []byte("\x98\x83\x7fok\rcat README.md\r")}.Build(),
			resource.ProjectTerminalRequest_builder{Columns: proto.Uint32(100), Rows: proto.Uint32(20)}.Build(),
			resource.ProjectTerminalRequest_builder{Input: []byte("exit\r")}.Build(),
		}}
		if err := p.Terminal(stream); err != nil {
			t.Fatal(err)
		}
		var output strings.Builder
		for _, r := range stream.replies {
			output.Write(r.GetOutput())
		}
		if !stream.replies[0].GetReady() || !stream.replies[len(stream.replies)-1].GetExited() || !strings.Contains(output.String(), "\r\nunicode-ok\r\n") || !strings.Contains(output.String(), "# "+id) || !strings.Contains(output.String(), "^C") {
			t.Fatalf("wrong shell output/status: %q", output.String())
		}
	}
	if len(server.sessions[0].events) != before {
		t.Fatal("terminal activity entered the agent journal")
	}
	stream := &sandboxTerminalStream{fixtureStream: fixtureStream[resource.ProjectTerminalReply]{ctx: t.Context()}, requests: []*resource.ProjectTerminalRequest{
		resource.ProjectTerminalRequest_builder{Ref: resource.ProjectRef_builder{RuntimeId: proto.String("missing")}.Build(), Columns: proto.Uint32(80), Rows: proto.Uint32(24)}.Build(),
	}}
	if err := p.Terminal(stream); status.Code(err) != codes.NotFound {
		t.Fatal("unknown project shell", err)
	}
}
