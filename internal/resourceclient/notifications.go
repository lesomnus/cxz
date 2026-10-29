package resourceclient

import (
	"context"
	"encoding/hex"
	"io"
	"slices"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/internal/sessionattention"
	"github.com/lesomnus/cxz/resource"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

var attentionKinds = []string{"session.completed", "session.failed", "session.question"}

func (c *Client) MarkConversationRead(ctx context.Context, id string, through uint64) error {
	c.readMu.Lock()
	state := c.readStates[id]
	c.readMu.Unlock()
	ref := resource.SessionRef_builder{RuntimeId: &id}.Build()
	if state == nil {
		var err error
		state, err = c.sessions.ReadState(ctx, resource.SessionReadStateRequest_builder{Ref: ref}.Build())
		if err != nil {
			return err
		}
	}
	if through > state.GetThroughSeq() {
		next, err := c.sessions.MarkRead(ctx, resource.SessionMarkReadRequest_builder{Ref: ref, Generation: state.GetGeneration(), ThroughSeq: through}.Build())
		if err != nil {
			if status.Code(err) == codes.FailedPrecondition || status.Code(err) == codes.NotFound || status.Code(err) == codes.OutOfRange {
				c.readMu.Lock()
				delete(c.readStates, id)
				c.readMu.Unlock()
			}
			return err
		}
		state = next
	}
	c.readMu.Lock()
	old := c.readStates[id]
	if old == nil || old.GetGeneration() != state.GetGeneration() || old.GetThroughSeq() < state.GetThroughSeq() {
		c.readStates[id] = state
	}
	c.readMu.Unlock()
	return nil
}

func (c *Client) WatchAttention(ctx context.Context, sessions []*api.Session, emit func(sessionattention.Update)) error {
	byProject := map[string][]string{}
	for _, s := range sessions {
		byProject[s.ProjectId] = append(byProject[s.ProjectId], s.Id)
	}
	ids := []string{}
	for id := range byProject {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	group, ctx := errgroup.WithContext(ctx)
	for start := 0; start < len(ids); start += 100 {
		topics := []*resource.NotificationTopic{}
		covered := []string{}
		for _, id := range ids[start:min(start+100, len(ids))] {
			topics = append(topics, resource.NotificationTopic_builder{ResourceId: projectid.Bytes(id)}.Build())
			covered = append(covered, byProject[id]...)
		}
		group.Go(func() error { return c.watchAttentionGroup(ctx, topics, covered, emit) })
	}
	return group.Wait()
}

func (c *Client) watchAttentionGroup(ctx context.Context, topics []*resource.NotificationTopic, covered []string, emit func(sessionattention.Update)) error {
	known := map[string]bool{}
	receipted := map[string]bool{}
	reader := ""
	allowed := map[string]bool{}
	for _, id := range covered {
		allowed[id] = true
	}
	delay := time.Second
	for ctx.Err() == nil {
		stream, err := c.notifications.Listen(ctx, resource.NotificationListenRequest_builder{Topics: topics}.Build())
		if err == nil {
			for {
				frame, e := stream.Recv()
				if e != nil {
					err = e
					break
				}
				initial := frame.GetReady() != nil
				if initial {
					id := hex.EncodeToString(frame.GetReady().GetReaderId())
					if reader != "" && reader != id {
						known = map[string]bool{}
						receipted = map[string]bool{}
						c.readMu.Lock()
						c.readStates = map[string]*resource.SessionReadState{}
						c.readMu.Unlock()
					}
					reader = id
					delay = time.Second
				} else if frame.GetInvalidated() == nil {
					continue
				}
				query := func(unread bool, cursor string) (*resource.NotificationReadResponse, error) {
					q, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					return c.notifications.Read(q, resource.NotificationReadRequest_builder{Topics: topics, Size: 100, Cursor: cursor, Filter: resource.NotificationReadFilter_builder{Kinds: attentionKinds, UnreadOnly: unread}.Build()}.Build())
				}
				unread := []*resource.NotificationReadItem{}
				cursor := ""
				for {
					page, e := query(true, cursor)
					if e != nil {
						err = e
						break
					}
					if hex.EncodeToString(page.GetReaderId()) != reader {
						err = status.Error(codes.Aborted, "notification reader changed")
						break
					}
					unread = append(unread, page.GetItems()...)
					cursor = page.GetNext()
					if cursor == "" {
						break
					}
				}
				if err != nil {
					break
				}
				recent, e := query(false, "")
				if e != nil {
					err = e
					break
				}
				if hex.EncodeToString(recent.GetReaderId()) != reader {
					err = status.Error(codes.Aborted, "notification reader changed")
					break
				}
				nextKnown := map[string]bool{}
				nextReceipts := map[string]bool{}
				arrivals := map[string]string{}
				update := sessionattention.Update{Sessions: covered, Unread: map[string]sessionattention.Entry{}}
				for _, item := range recent.GetItems() {
					id := hex.EncodeToString(item.GetNotification().GetId())
					nextKnown[id] = true
					if item.GetReceipt() != nil {
						nextReceipts[id] = true
					}
				}
				for _, item := range append(append([]*resource.NotificationReadItem{}, recent.GetItems()...), unread...) {
					v := item.GetNotification()
					data := &resource.SessionNotification{}
					if v == nil || v.GetPayload() == nil || v.GetPayload().UnmarshalTo(data) != nil || !allowed[data.GetRuntimeId()] {
						continue
					}
					id := hex.EncodeToString(v.GetId())
					nextKnown[id] = true
					if !initial && !known[id] {
						switch v.GetKind() {
						case "session.completed":
							arrivals[id] = "complete"
						case "session.failed":
							arrivals[id] = "error"
						case "session.question":
							arrivals[id] = "question"
						}
					}
				}
				for _, item := range unread {
					v := item.GetNotification()
					id := hex.EncodeToString(v.GetId())
					if receipted[id] || nextReceipts[id] {
						nextReceipts[id] = true
						continue
					}
					data := &resource.SessionNotification{}
					if v.GetPayload() == nil || v.GetPayload().UnmarshalTo(data) != nil || !allowed[data.GetRuntimeId()] {
						continue
					}
					entry := update.Unread[data.GetRuntimeId()]
					entry.Seq = max(entry.Seq, data.GetSeq())
					entry.Error = entry.Error || v.GetKind() == "session.failed"
					entry.Question = entry.Question || v.GetKind() == "session.question"
					update.Unread[data.GetRuntimeId()] = entry
				}
				for _, kind := range arrivals {
					update.Alerts = append(update.Alerts, kind)
				}
				known, receipted = nextKnown, nextReceipts
				emit(update)
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if status.Code(err) == codes.Unimplemented {
			return err
		}
		if conn, ok := c.connection.(*grpc.ClientConn); ok && conn.GetState() == connectivity.Shutdown {
			return err
		}
		if err == io.EOF {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		delay = min(delay*2, 30*time.Second)
	}
	return nil
}

var _ sessionattention.Backend = (*Client)(nil)
