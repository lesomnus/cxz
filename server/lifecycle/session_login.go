package lifecycle

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This stream is transient: authentication input/output never goes through
// resource mutations, audit payloads, session events or supervisor logs.
func (s ProjectServer) SessionLogin(stream grpc.BidiStreamingServer[resource.ProjectLoginRequest, resource.ProjectLoginOutput]) error {
	if err := s.effect(); err != nil {
		return err
	}
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if !first.HasRef() || !first.HasAccount() || first.GetSessionKey() == "" || len(first.GetSessionKey()) > 1024 || len(first.GetInput()) > 0 {
		return status.Error(codes.InvalidArgument, "project, account and session creation key required in first login frame")
	}
	ctx, cancel := context.WithTimeout(stream.Context(), 30*time.Minute)
	defer cancel()
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: first.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	if !p.GetListed() {
		return status.Error(codes.NotFound, "project deleted")
	}
	a, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: first.GetAccount(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	if a.GetAgent() != "claude" || a.GetAuthBackend() != accounts.ProjectLocalOAuth {
		return status.Error(codes.FailedPrecondition, "inline session login requires a Claude project-local-oauth account")
	}
	runtime, ok := s.shared.runtime.(accounts.SessionLoginClient)
	if !ok {
		return status.Error(codes.Unimplemented, "session login transport unavailable")
	}
	// A real descriptor avoids os/exec waiting on an input-copy goroutine after
	// the provider has already exited (e.g. login failure or browser completion).
	in, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer in.Close()
	defer writer.Close()
	inputErr := make(chan error, 1)
	go func() {
		defer writer.Close()
		total := 0
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err == nil {
				total += len(msg.GetInput())
				if msg.HasRef() || msg.HasAccount() || msg.HasSessionKey() || len(msg.GetInput()) > 8192 || total > 64*1024 {
					err = status.Error(codes.InvalidArgument, "invalid login input frame")
				} else {
					_, err = writer.Write(msg.GetInput())
				}
				clear(msg.GetInput())
			}
			if err != nil {
				inputErr <- err
				cancel()
				return
			}
		}
	}()
	output := &sessionLoginOutput{stream: stream, cancel: cancel}
	err = runtime.LoginSession(ctx, p.GetRuntimeId(), a.GetAlias(), first.GetSessionKey(), in, output)
	select {
	case inputErr := <-inputErr:
		return inputErr
	default:
	}
	return err
}

type sessionLoginOutput struct {
	mu     sync.Mutex
	stream grpc.BidiStreamingServer[resource.ProjectLoginRequest, resource.ProjectLoginOutput]
	cancel context.CancelFunc
}

func (w *sessionLoginOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.stream.Send(resource.ProjectLoginOutput_builder{Output: p}.Build()); err != nil {
		w.cancel()
		return 0, err
	}
	return len(p), nil
}
