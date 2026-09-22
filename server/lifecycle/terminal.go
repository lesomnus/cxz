package lifecycle

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Terminal bypasses resource mutations and event journals. The client supplies
// only a registered project reference, never a container, user, or command.
func (s ProjectServer) Terminal(stream grpc.BidiStreamingServer[resource.ProjectTerminalRequest, resource.ProjectTerminalReply]) error {
	if err := s.effect(); err != nil {
		return err
	}
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	validSize := func(r *resource.ProjectTerminalRequest) bool {
		return r.GetColumns() >= 1 && r.GetColumns() <= 500 && r.GetRows() >= 1 && r.GetRows() <= 100
	}
	if !first.HasRef() || first.HasInput() || !validSize(first) {
		return status.Error(codes.InvalidArgument, "project and terminal size required in first terminal frame")
	}
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: first.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	if !p.GetListed() {
		return status.Error(codes.NotFound, "project deleted")
	}
	runtime, ok := s.shared.runtime.(containerterm.TerminalClient)
	if !ok {
		return status.Error(codes.Unimplemented, "container terminal transport unavailable")
	}
	terminal, err := runtime.OpenTerminal(ctx, p.GetRuntimeId(), int(first.GetColumns()), int(first.GetRows()))
	if err != nil {
		return err
	}
	// Closing also unblocks a pending PTY read/write when the stream disappears.
	go func() { <-ctx.Done(); _ = terminal.Close() }()
	defer terminal.Close()
	if err := stream.Send(resource.ProjectTerminalReply_builder{Ready: ptr(true)}.Build()); err != nil {
		_ = terminal.Close()
		_ = terminal.Wait()
		return err
	}
	inputErr := make(chan error, 1)
	go func() {
		for {
			r, err := stream.Recv()
			if err == nil {
				resize := r.HasColumns() || r.HasRows()
				switch {
				case r.HasRef() || (resize && (!validSize(r) || r.HasInput())) || (!resize && (!r.HasInput() || len(r.GetInput()) > 32768)):
					err = status.Error(codes.InvalidArgument, "invalid terminal input or resize frame")
				case resize:
					err = terminal.Resize(int(r.GetColumns()), int(r.GetRows()))
				default:
					_, err = terminal.Write(r.GetInput())
				}
				clear(r.GetInput())
			}
			if err != nil {
				if err == io.EOF {
					err = status.Error(codes.Canceled, "terminal input closed")
				}
				inputErr <- err
				return
			}
		}
	}()
	// Keep the handler free to return on invalid input/cancellation even if a
	// slow peer has filled the output flow-control window. Returning releases
	// blocked stream operations; this goroutine always reaps the PTY process.
	outputDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 32768)
		var sendErr error
		for {
			n, readErr := terminal.Read(buf)
			if n > 0 {
				sendErr = stream.Send(resource.ProjectTerminalReply_builder{Output: append([]byte(nil), buf[:n]...)}.Build())
				if sendErr != nil {
					_ = terminal.Close()
					break
				}
			}
			if readErr != nil {
				break
			} // Unix PTYs also return EIO on normal exit.
		}
		waitErr := terminal.Wait()
		if sendErr == nil && ctx.Err() == nil {
			exitError := ""
			if waitErr != nil {
				exitError = waitErr.Error()
			}
			sendErr = stream.Send(resource.ProjectTerminalReply_builder{Exited: ptr(true), Error: &exitError}.Build())
		}
		outputDone <- sendErr
	}()
	select {
	case err := <-inputErr:
		return err
	case <-ctx.Done():
		return status.FromContextError(ctx.Err()).Err()
	case err := <-outputDone:
		select {
		case inputErr := <-inputErr:
			return inputErr
		default:
		}
		return err
	}
}
