package lifecycle

import (
	"context"
	"io"
	"time"

	"github.com/lesomnus/cxz/internal/editor"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s ProjectServer) editorRuntime(ctx context.Context, ref *resource.ProjectRef) (editor.Runtime, string, error) {
	if err := s.effect(); err != nil {
		return nil, "", err
	}
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, "", err
	}
	if !p.GetListed() {
		return nil, "", status.Error(codes.NotFound, "project deleted")
	}
	runtime, ok := s.shared.runtime.(editor.Runtime)
	if !ok {
		return nil, "", status.Error(codes.Unimplemented, "browser editor unavailable; update the Manager")
	}
	return runtime, p.GetRuntimeId(), nil
}

func (s ProjectServer) Editor(ctx context.Context, r *resource.ProjectEditorRequest) (*resource.ProjectEditorReply, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	runtime, id, err := s.editorRuntime(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	result, err := runtime.Editor(ctx, id)
	if err != nil {
		return nil, err
	}
	return resource.ProjectEditorReply_builder{Workspace: ptr(result.Workspace), ConnectionToken: ptr(result.Token)}.Build(), nil
}

func (s ProjectServer) EditorTunnel(stream grpc.BidiStreamingServer[resource.ProjectEditorTunnelRequest, resource.ProjectEditorTunnelReply]) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if !first.HasRef() || len(first.GetInput()) != 0 {
		return status.Error(codes.InvalidArgument, "first editor frame must identify a project")
	}
	runtime, id, err := s.editorRuntime(ctx, first.GetRef())
	if err != nil {
		return err
	}
	tunnel, err := runtime.OpenEditorTunnel(ctx, id)
	if err != nil {
		return err
	}
	defer tunnel.Close()
	stop := context.AfterFunc(ctx, func() { tunnel.Close() })
	defer stop()
	if err = stream.Send(resource.ProjectEditorTunnelReply_builder{Ready: ptr(true)}.Build()); err != nil {
		return err
	}
	inputDone := make(chan error, 1)
	go func() {
		for {
			r, err := stream.Recv()
			if err == nil {
				if r.HasRef() || len(r.GetInput()) == 0 || len(r.GetInput()) > 65536 {
					err = status.Error(codes.InvalidArgument, "invalid editor byte frame")
				} else {
					_, err = tunnel.Write(r.GetInput())
				}
			}
			if err != nil {
				inputDone <- err
				return
			}
		}
	}()
	outputDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := tunnel.Read(buf)
			if n > 0 {
				if e := stream.Send(resource.ProjectEditorTunnelReply_builder{Output: append([]byte(nil), buf[:n]...)}.Build()); e != nil {
					outputDone <- e
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					err = nil
				}
				outputDone <- err
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-inputDone:
		return err
	case err := <-outputDone:
		return err
	}
}
