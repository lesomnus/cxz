package multiclient

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionattention"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (c *Client) MarkConversationRead(ctx context.Context, id string, through uint64) error {
	_, local, client, err := c.route(ctx, id)
	if err != nil {
		return err
	}
	backend, ok := client.(sessionattention.Backend)
	if !ok {
		return status.Error(codes.Unimplemented, "connection does not support durable read state")
	}
	return backend.MarkConversationRead(ctx, local, through)
}

func (c *Client) SupportsAttention() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, source := range c.sources {
		if _, ok := source.Client.(sessionattention.Backend); ok {
			return true
		}
	}
	return false
}
func (c *Client) WatchAttention(ctx context.Context, sessions []*api.Session, emit func(sessionattention.Update)) error {
	groups := map[string][]*api.Session{}
	for _, s := range sessions {
		name, _ := c.routeName(ctx, s.Id)
		v := proto.Clone(s).(*api.Session)
		_, v.Id = Split(v.Id)
		_, v.ProjectId = Split(v.ProjectId)
		groups[name] = append(groups[name], v)
	}
	group, ctx := errgroup.WithContext(ctx)
	for name, values := range groups {
		group.Go(func() error {
			lastError := ""
			for ctx.Err() == nil {
				_, _, client, err := c.route(ctx, Scope(name, values[0].Id))
				if err == nil {
					if backend, ok := client.(sessionattention.Backend); ok {
						err = backend.WatchAttention(ctx, values, func(update sessionattention.Update) {
							out := sessionattention.Update{Unread: map[string]sessionattention.Entry{}, Alerts: update.Alerts, Err: update.Err}
							for _, id := range update.Sessions {
								out.Sessions = append(out.Sessions, Scope(name, id))
							}
							for id, entry := range update.Unread {
								out.Unread[Scope(name, id)] = entry
							}
							emit(out)
						})
					} else {
						err = status.Error(codes.Unimplemented, "connection does not support notifications")
					}
				}
				if ctx.Err() != nil {
					return nil
				}
				if err != nil && err.Error() != lastError {
					lastError = err.Error()
					emit(sessionattention.Update{Err: err})
				}
				timer := time.NewTimer(3 * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil
				case <-timer.C:
				}
			}
			return nil
		})
	}
	return group.Wait()
}

var _ sessionattention.Backend = (*Client)(nil)
