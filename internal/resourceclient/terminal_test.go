package resourceclient

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type terminalProjects struct {
	resource.ProjectServiceClient
	stream *terminalStream
}

func (c terminalProjects) Terminal(context.Context, ...grpc.CallOption) (grpc.BidiStreamingClient[resource.ProjectTerminalRequest, resource.ProjectTerminalReply], error) {
	return c.stream, nil
}

type terminalStream struct {
	grpc.BidiStreamingClient[resource.ProjectTerminalRequest, resource.ProjectTerminalReply]
	frames       []*resource.ProjectTerminalRequest
	replies      []*resource.ProjectTerminalReply
	err, sendErr error
}

func (s *terminalStream) Send(r *resource.ProjectTerminalRequest) error {
	s.frames = append(s.frames, proto.Clone(r).(*resource.ProjectTerminalRequest))
	return s.sendErr
}
func (s *terminalStream) Recv() (*resource.ProjectTerminalReply, error) {
	if len(s.replies) > 0 {
		r := s.replies[0]
		s.replies = s.replies[1:]
		return r, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return nil, io.EOF
}

func TestRemoteTerminalFramesAndIncompleteExit(t *testing.T) {
	for _, complete := range []bool{true, false} {
		ready, exited := true, true
		s := &terminalStream{replies: []*resource.ProjectTerminalReply{
			resource.ProjectTerminalReply_builder{Ready: &ready}.Build(),
			resource.ProjectTerminalReply_builder{Output: []byte("한글 bytes")}.Build(),
		}}
		if complete {
			s.replies = append(s.replies, resource.ProjectTerminalReply_builder{Exited: &exited}.Build())
		}
		c := &Client{projects: terminalProjects{stream: s}}
		terminal, err := c.OpenTerminal(context.Background(), "project", 800, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer terminal.Close()
		if s.frames[0].GetColumns() != 500 || s.frames[0].GetRows() != 1 {
			t.Fatal("initial size not bounded", s.frames)
		}
		input := strings.Repeat("z", 70000)
		if n, err := io.WriteString(terminal, input); err != nil || n != len(input) {
			t.Fatal(n, err)
		}
		var combined strings.Builder
		for _, frame := range s.frames[1:] {
			if len(frame.GetInput()) > 32768 || frame.HasRef() {
				t.Fatal("unbounded or retargeted input")
			}
			combined.Write(frame.GetInput())
		}
		if combined.String() != input {
			t.Fatal("input lost")
		}
		if err := terminal.Resize(73, 19); err != nil {
			t.Fatal(err)
		}
		last := s.frames[len(s.frames)-1]
		if last.GetColumns() != 73 || last.GetRows() != 19 || last.HasRef() || last.HasInput() {
			t.Fatal("invalid resize", last)
		}
		// Deliberately split Unicode across reads: the transport preserves bytes.
		var output strings.Builder
		buf := make([]byte, 2)
		for {
			n, err := terminal.Read(buf)
			output.Write(buf[:n])
			if err != nil {
				break
			}
		}
		if output.String() != "한글 bytes" {
			t.Fatal(output.String())
		}
		if err := terminal.Wait(); complete && err != nil || !complete && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal("exit status", complete, err)
		}
	}
}

func TestRemoteTerminalOldManager(t *testing.T) {
	s := &terminalStream{sendErr: io.EOF, err: status.Error(codes.Unimplemented, "unknown method Terminal")}
	c := &Client{projects: terminalProjects{stream: s}}
	_, err := c.OpenTerminal(context.Background(), "project", 80, 24)
	if err == nil || !strings.Contains(err.Error(), "host manager") || !strings.Contains(err.Error(), "cxz install --recreate") {
		t.Fatal("missing upgrade instruction", err)
	}
}
