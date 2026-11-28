package resourceclient

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/resource"
)

func (c *Client) LoginSession(ctx context.Context, r accounts.SessionLogin) (accounts.SessionLoginResult, error) {
	var out accounts.SessionLoginResult
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	input := r.Input
	stream, err := c.projects.SessionLogin(ctx)
	if err != nil {
		input.Close()
		return out, err
	}
	if err := stream.Send(resource.ProjectLoginRequest_builder{Ref: pr(r.Project), Account: ar(r.Account), SessionKey: &r.Key, LoginIdentity: &r.Identity}.Build()); err != nil {
		input.Close()
		return out, err
	}
	sent := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		defer clear(buf)
		for {
			n, err := input.Read(buf)
			if n > 0 {
				if sendErr := stream.Send(resource.ProjectLoginRequest_builder{Input: buf[:n]}.Build()); sendErr != nil {
					sent <- sendErr
					return // Recv obtains the server's final status, even on Send EOF.
				}
			}
			if err != nil {
				if err == io.EOF {
					_ = stream.CloseSend()
				} else {
					cancel()
				}
				sent <- err
				return
			}
		}
	}()
	defer func() {
		cancel()
		input.Close() // Unblock Read even when the provider exits without input.
		<-sent
	}()
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return accounts.SessionLoginResult{}, err
		}
		// The identity the runtime reached is for the Manager to record, and is
		// not part of what the person logging in is shown.
		if v := msg.GetLoginIdentity(); v != "" {
			out.Identity = v
		}
		if _, err := r.Output.Write(msg.GetOutput()); err != nil {
			return accounts.SessionLoginResult{}, err
		}
	}
}
