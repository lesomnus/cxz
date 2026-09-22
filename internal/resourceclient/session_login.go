package resourceclient

import (
	"context"
	"io"

	"github.com/lesomnus/cxz/resource"
)

func (c *Client) LoginSession(ctx context.Context, project, account, key string, input io.ReadCloser, output io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer input.Close()
	stream, err := c.projects.SessionLogin(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(resource.ProjectLoginRequest_builder{Ref: pr(project), Account: ar(account), SessionKey: &key}.Build()); err != nil {
		return err
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
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := output.Write(msg.GetOutput()); err != nil {
			return err
		}
	}
}
