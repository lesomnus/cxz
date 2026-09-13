package resourceclient

import (
	"context"
	"github.com/lesomnus/cxz/resource"
)

// Snapshot-inclusive streams close the list/subscribe race. A disconnect ends
// both subscriptions; reconnect starts from fresh snapshots, not stale cursors.
func (c *Client) WatchChanges(ctx context.Context, projects, sessions []string, changed func()) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errors := make(chan error, (len(projects)+31)/32+(len(sessions)+31)/32)
	for start := 0; start < len(sessions); start += 32 {
		sessions := append([]string(nil), sessions[start:min(start+32, len(sessions))]...)
		go func() {
			if len(sessions) == 0 {
				return
			}
			filters := make([]*resource.SessionFilter, 0, len(sessions))
			for _, id := range sessions {
				filters = append(filters, resource.SessionFilter_builder{Ref: resource.SessionRef_builder{RuntimeId: ptr(id)}.Build()}.Build())
			}
			stream, err := c.sessions.Watch(ctx, resource.SessionWatchRequest_builder{Filters: filters}.Build())
			if err != nil {
				errors <- err
				return
			}
			for {
				_, err := stream.Recv()
				if err != nil {
					errors <- err
					return
				}
				changed()
			}
		}()
	}
	for start := 0; start < len(projects); start += 32 {
		projects := append([]string(nil), projects[start:min(start+32, len(projects))]...)
		go func() {
			if len(projects) == 0 {
				return
			}
			filters := make([]*resource.ProjectFilter, 0, len(projects))
			for _, id := range projects {
				filters = append(filters, resource.ProjectFilter_builder{Ref: pr(id)}.Build())
			}
			stream, err := c.projects.Watch(ctx, resource.ProjectWatchRequest_builder{Filters: filters}.Build())
			if err != nil {
				errors <- err
				return
			}
			for {
				_, err := stream.Recv()
				if err != nil {
					errors <- err
					return
				}
				changed()
			}
		}()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errors:
		return err
	}
}
