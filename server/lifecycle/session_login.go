package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/internal/projectid"
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
	// Only a Manager says which account a login must reach, and it says it to a
	// project runtime. Accepting it from whoever asked for the login would let
	// the asker name the pin its own login is then checked against.
	if s.shared.manager && first.GetLoginIdentity() != "" {
		return status.Error(codes.InvalidArgument, "login identity is not a client's to supply")
	}
	ctx, cancel := context.WithTimeout(stream.Context(), 30*time.Minute)
	defer cancel()
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: first.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
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
	key, err := s.accountProfile(ctx, a)
	if err != nil {
		return err
	}
	// A Manager reads the pin from the authoritative row; a project runtime was
	// handed it, because a person who just cleared the pin expects the next
	// login to be accepted rather than to wait for their own replica.
	pinned := first.GetLoginIdentity()
	if s.shared.manager {
		pinned = a.GetLoginIdentity()
	}
	result, err := runtime.LoginSession(ctx, accounts.SessionLogin{Project: projectid.String(p.GetId()), Account: key, Key: first.GetSessionKey(), Identity: pinned, Input: in, Output: output})
	select {
	case inputErr := <-inputErr:
		return inputErr
	default:
	}
	if err != nil {
		return err
	}
	if result.Identity == "" {
		return nil
	}
	if !s.shared.manager {
		return output.identity(result.Identity)
	}
	// A login waits on a person, so the row read before it started is stale by
	// the time it returns. Read it again rather than write over whatever was
	// decided in the meantime.
	ref := resource.AccountRef_builder{Id: a.GetId()}.Build()
	current, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: ref, Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	if pinned := current.GetLoginIdentity(); pinned != "" {
		if pinned == result.Identity {
			return nil
		}
		// This account was pinned to a different provider account while the
		// login was open. The credential is installed and the pin disagrees
		// with it, which is what refuses the next login; say so now instead of
		// leaving that to be discovered then.
		return status.Error(codes.FailedPrecondition, "this account was pinned to a different provider account while the login was open; the credential just installed does not match it")
	}
	_, err = s.Next().Account().Patch(ctx, resource.AccountPatchRequest_builder{
		Ref:           ref,
		LoginIdentity: ptr(result.Identity),
		DateUpdated:   current.GetDateUpdated(),
	}.Build())
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

// identity answers the Manager that opened this stream, carrying no login
// output of its own so that nothing of it reaches a person's terminal.
func (w *sessionLoginOutput) identity(v string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.stream.Send(resource.ProjectLoginOutput_builder{LoginIdentity: &v}.Build()); err != nil {
		w.cancel()
		return err
	}
	return nil
}
