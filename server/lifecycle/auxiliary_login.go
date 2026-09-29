package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"io"
	"os"
	"time"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This stream is transient: authentication input/output never goes through
// resource mutations, audit payloads, session events or supervisor logs.
func (s ProjectServer) AuxiliaryLogin(stream grpc.BidiStreamingServer[resource.ProjectLoginRequest, resource.ProjectLoginOutput]) error {
	if err := s.effect(); err != nil {
		return err
	}
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if first.HasRef() || !first.HasAccount() || first.HasSessionKey() || len(first.GetInput()) > 0 {
		return status.Error(codes.InvalidArgument, "only account is required in first auxiliary login frame")
	}
	ctx, cancel := context.WithTimeout(stream.Context(), 15*time.Minute)
	defer cancel()
	a, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: first.GetAccount(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	if _, err := accounts.Resolve(a.GetAgent(), a.GetAuthBackend()); err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	runtime, ok := s.shared.runtime.(interface {
		AuxiliaryLogin(context.Context, auxiliary.Profile, io.Reader, io.Writer, io.Writer) error
	})
	if !ok {
		return status.Error(codes.Unimplemented, "auxiliary login transport unavailable; update the Manager")
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
	err = runtime.AuxiliaryLogin(ctx, auxiliary.Profile{Account: a.GetAlias(), Agent: a.GetAgent(), Backend: a.GetAuthBackend()}, in, output, output)
	select {
	case inputErr := <-inputErr:
		return inputErr
	default:
	}
	return err
}
