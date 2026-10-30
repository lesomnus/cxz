package lifecycle

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/frame"
	"github.com/lesomnus/payday/notify"
	"github.com/lesomnus/payday/pdid"
	"github.com/lesomnus/payday/watch"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	SessionCompleted = "session.completed"
	SessionFailed    = "session.failed"
	SessionQuestion  = "session.question"
)

type notificationPublisher struct{}

type notificationState struct {
	mu        sync.Mutex
	layer     Layer
	owner     pdid.Id
	startedMS int64
	service   *pd.NotificationServer
}

// Authenticated Manager transports all represent the installation's operator.
// Provider accounts and client-supplied headers never select a receipt owner.
func (n *notificationState) context(ctx context.Context) context.Context {
	return frame.Into(ctx, frame.New(n.owner, pdid.Nil, frame.Whole()))
}

func newNotifications(ctx context.Context, layer Layer, stack resource.Server, driver dialect.Driver, w *watch.Watch) (*notificationState, error) {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS notification_meta (key TEXT PRIMARY KEY, value BLOB NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS session_notification_cursor (session_id BLOB PRIMARY KEY, generation TEXT NOT NULL, seq INTEGER NOT NULL)`,
	} {
		if _, err := layer.shared.db.ExecContext(ctx, statement); err != nil {
			return nil, err
		}
	}
	alias := "operator"
	owner, err := layer.Next().Operator().Get(ctx, resource.OperatorGetRequest_builder{Ref: resource.OperatorRef_builder{Alias: &alias}.Build(), Select: resource.OperatorSelect_builder{All: ptr(true)}.Build()}.Build())
	if status.Code(err) == codes.NotFound {
		owner, err = layer.Next().Operator().Add(ctx, resource.OperatorAddRequest_builder{Alias: alias}.Build())
	}
	if err != nil {
		return nil, err
	}
	id, err := pdid.From(owner.GetId())
	if err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	if _, err = layer.shared.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_meta VALUES('cursor_key',?)`, key); err != nil {
		return nil, err
	}
	if err = layer.shared.db.QueryRowContext(ctx, `SELECT value FROM notification_meta WHERE key='cursor_key'`).Scan(&key); err != nil {
		return nil, err
	}
	n := &notificationState{layer: layer, owner: id, startedMS: owner.GetDateCreated().AsTime().UnixMilli()}
	n.service, err = pd.Notifications(stack, driver, w, notificationPolicy{n, layer.Next()}, notify.Options{CursorKey: key})
	return n, err
}

type notificationPolicy struct {
	n     *notificationState
	store resource.Server
}

func (p notificationPolicy) WithDriver(driver dialect.Driver) (notify.Policy, error) {
	store, err := enttx.Rebind(p.store, driver)
	if err != nil {
		return nil, err
	}
	p.store = store
	return p, nil
}
func (p notificationPolicy) Publisher(ctx context.Context, tenant pdid.Id, topic notify.Topic) (string, error) {
	if allowed, _ := ctx.Value(notificationPublisher{}).(bool); !allowed {
		return "", status.Error(codes.PermissionDenied, "only the Manager publishes notifications")
	}
	if err := p.Read(ctx, tenant, p.n.owner, topic); err != nil {
		return "", err
	}
	return "cxz.manager", nil
}
func (p notificationPolicy) Reader(ctx context.Context, _ pdid.Id) (pdid.Id, error) {
	f, ok := frame.From(ctx)
	if !ok || f.Actor != p.n.owner {
		return pdid.Nil, status.Error(codes.Unauthenticated, "operator authentication required")
	}
	return p.n.owner, nil
}
func (p notificationPolicy) Read(ctx context.Context, tenant, reader pdid.Id, topic notify.Topic) error {
	if !tenant.IsZero() || reader != p.n.owner || topic.Resource.IsZero() || topic.Resource.Domain() != pd.ProjectDomain {
		return status.Error(codes.PermissionDenied, "notifications require an authorized project topic")
	}
	_, err := p.store.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: resource.ProjectRef_builder{Id: topic.Resource.Bytes()}.Build()}.Build())
	return err
}

// Explicit registration keeps this human-facing service off project listeners.
func (s Layer) Notifications() resource.NotificationServiceServer {
	if s.shared.notifications == nil {
		return nil
	}
	return operatorNotifications{n: s.shared.notifications}
}
func (s Layer) Operator() resource.OperatorServiceServer {
	return resource.UnimplementedOperatorServiceServer{}
}
func (s Layer) SessionRead() resource.SessionReadServiceServer {
	return resource.UnimplementedSessionReadServiceServer{}
}

type operatorNotifications struct {
	resource.UnimplementedNotificationServiceServer
	n *notificationState
}

func (s operatorNotifications) Add(context.Context, *resource.NotificationAddRequest) (*resource.Notification, error) {
	return nil, status.Error(codes.PermissionDenied, "only the Manager publishes notifications")
}
func (s operatorNotifications) Get(ctx context.Context, r *resource.NotificationGetRequest) (*resource.Notification, error) {
	return s.n.service.Get(s.n.context(ctx), r)
}
func (s operatorNotifications) List(ctx context.Context, r *resource.NotificationListRequest) (*resource.NotificationListResponse, error) {
	return s.n.service.List(s.n.context(ctx), r)
}
func (s operatorNotifications) Read(ctx context.Context, r *resource.NotificationReadRequest) (*resource.NotificationReadResponse, error) {
	return s.n.service.Read(s.n.context(ctx), r)
}
func (s operatorNotifications) Count(ctx context.Context, r *resource.NotificationCountRequest) (*resource.NotificationCountResponse, error) {
	return s.n.service.Count(s.n.context(ctx), r)
}
func (s operatorNotifications) Mark(ctx context.Context, r *resource.NotificationMarkRequest) (*resource.NotificationMarkResponse, error) {
	return s.n.service.Mark(s.n.context(ctx), r)
}

type operatorNotificationStream struct {
	grpc.ServerStreamingServer[resource.NotificationListenResponse]
	ctx context.Context
}

func (s operatorNotificationStream) Context() context.Context { return s.ctx }
func (s operatorNotifications) Listen(r *resource.NotificationListenRequest, stream grpc.ServerStreamingServer[resource.NotificationListenResponse]) error {
	return s.n.service.Listen(r, operatorNotificationStream{stream, s.n.context(stream.Context())})
}

type notificationCursor struct {
	generation string
	seq        uint64
}

func (n *notificationState) cursor(ctx context.Context, session *resource.Session, baseline uint64) (notificationCursor, error) {
	var c notificationCursor
	err := n.layer.shared.db.QueryRowContext(ctx, `SELECT generation,seq FROM session_notification_cursor WHERE session_id=?`, session.GetId()).Scan(&c.generation, &c.seq)
	if err == sql.ErrNoRows {
		if source, ok := n.layer.shared.runtime.(interface {
			NotificationBoundary(context.Context, string) (uint64, error)
		}); ok {
			floor, err := source.NotificationBoundary(ctx, session.GetRuntimeId())
			if err != nil {
				return c, err
			}
			baseline = max(baseline, floor)
		}
		c = notificationCursor{core.ID(), baseline}
		_, err = n.layer.shared.db.ExecContext(ctx, `INSERT INTO session_notification_cursor VALUES(?,?,?)`, session.GetId(), c.generation, c.seq)
	}
	return c, err
}

func (n *notificationState) read(ctx context.Context, session *resource.Session) (*resource.SessionRead, error) {
	page, err := n.layer.Next().SessionRead().List(ctx, resource.SessionReadListRequest_builder{
		Size: 1,
		Filters: []*resource.SessionReadFilter{resource.SessionReadFilter_builder{
			Session:  resource.SessionRef_builder{Id: session.GetId()}.Build(),
			ReaderId: n.owner.Bytes(),
		}.Build()},
	}.Build())
	if err != nil {
		return nil, err
	}
	if len(page.GetItems()) == 0 {
		return nil, nil
	}
	return page.GetItems()[0], nil
}

func (s SessionServer) ReadState(ctx context.Context, r *resource.SessionReadStateRequest) (*resource.SessionReadState, error) {
	if s.shared.notifications == nil {
		return nil, status.Error(codes.FailedPrecondition, "read state belongs to the Manager")
	}
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	n := s.shared.notifications
	n.mu.Lock()
	defer n.mu.Unlock()
	c, err := n.cursor(ctx, v, 0)
	if err != nil {
		return nil, err
	}
	read, err := n.read(ctx, v)
	if err != nil {
		return nil, err
	}
	through := uint64(0)
	if read != nil && read.GetGeneration() == c.generation {
		through = read.GetThroughSeq()
	}
	return resource.SessionReadState_builder{ReaderId: n.owner.Bytes(), Generation: c.generation, ThroughSeq: through}.Build(), nil
}

func (s SessionServer) MarkRead(ctx context.Context, r *resource.SessionMarkReadRequest) (*resource.SessionReadState, error) {
	if s.shared.notifications == nil {
		return nil, status.Error(codes.FailedPrecondition, "read state belongs to the Manager")
	}
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	if r.GetThroughSeq() > v.GetStatus().GetLastSeq() {
		page, err := s.shared.runtime.History(ctx, &api.WatchRequest{SessionId: v.GetRuntimeId(), AfterSeq: r.GetThroughSeq() - 1, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(page.GetEvents()) == 0 || page.GetEvents()[0].Seq != r.GetThroughSeq() {
			return nil, status.Error(codes.OutOfRange, "cannot acknowledge events that have not been recorded")
		}
	}
	n := s.shared.notifications
	n.mu.Lock()
	defer n.mu.Unlock()
	c, err := n.cursor(ctx, v, 0)
	if err != nil {
		return nil, err
	}
	if r.GetGeneration() != c.generation {
		return nil, status.Error(codes.FailedPrecondition, "conversation generation changed; read state again")
	}
	read, err := n.read(ctx, v)
	if err != nil {
		return nil, err
	}
	through := r.GetThroughSeq()
	if read != nil && read.GetGeneration() == c.generation {
		through = max(through, read.GetThroughSeq())
	}
	store := n.layer.Next().SessionRead()
	if read == nil {
		_, err = store.Add(ctx, resource.SessionReadAddRequest_builder{
			Session: resource.SessionRef_builder{Id: v.GetId()}.Build(), ReaderId: n.owner.Bytes(), Generation: c.generation, ThroughSeq: through,
		}.Build())
	} else if read.GetThroughSeq() != through || read.GetGeneration() != c.generation {
		_, err = store.Patch(ctx, resource.SessionReadPatchRequest_builder{Ref: resource.SessionReadRef_builder{Id: read.GetId()}.Build(), Generation: &c.generation, ThroughSeq: &through, DateUpdatedForce: ptr(true)}.Build())
	}
	if err != nil {
		return nil, err
	}
	if err = n.markThrough(ctx, v, c.generation, through); err != nil {
		return nil, err
	}
	return resource.SessionReadState_builder{ReaderId: n.owner.Bytes(), Generation: c.generation, ThroughSeq: through}.Build(), nil
}

func (n *notificationState) markThrough(ctx context.Context, v *resource.Session, generation string, through uint64) error {
	ctx = n.context(ctx)
	cursor := ""
	for {
		page, err := n.service.Read(ctx, resource.NotificationReadRequest_builder{
			Topics: []*resource.NotificationTopic{resource.NotificationTopic_builder{ResourceId: v.GetProject().GetId()}.Build()}, Size: 100, Cursor: cursor,
			Filter: resource.NotificationReadFilter_builder{SourceIds: [][]byte{v.GetId()}, UnreadOnly: true}.Build(),
		}.Build())
		if err != nil {
			return err
		}
		refs := []*resource.NotificationRef{}
		for _, item := range page.GetItems() {
			payload := &resource.SessionNotification{}
			if err := item.GetNotification().GetPayload().UnmarshalTo(payload); err != nil {
				continue
			}
			if generation == "" || (payload.GetGeneration() == generation && payload.GetSeq() <= through) {
				refs = append(refs, resource.NotificationRef_builder{Id: item.GetNotification().GetId()}.Build())
			}
		}
		if len(refs) > 0 {
			if _, err = n.service.Mark(ctx, resource.NotificationMarkRequest_builder{Refs: refs}.Build()); err != nil {
				return err
			}
		}
		cursor = page.GetNext()
		if cursor == "" {
			return nil
		}
	}
}

func notificationKind(agent string, e *api.Event) string {
	if e.Kind == "turn_end" {
		if e.Text == "completed" {
			return SessionCompleted
		}
		if e.Text == "failed" {
			return SessionFailed
		}
	}
	if e.Kind == "approval" && !agentview.AutomaticApproval(e.Text, e.Payload) {
		if qs, err := agentview.Questions(agent, e.Text, e.Payload); err == nil && len(qs) > 0 {
			return SessionQuestion
		}
	}
	return ""
}

func (n *notificationState) publish(ctx context.Context, session *resource.Session, c notificationCursor, event *api.Event) error {
	if event.TimeMs > 0 && event.TimeMs < n.startedMS {
		return nil
	}
	kind := notificationKind(session.GetAgent(), event)
	if kind == "" {
		return nil
	}
	payload, err := anypb.New(resource.SessionNotification_builder{RuntimeId: session.GetRuntimeId(), Generation: c.generation, Seq: event.Seq, RequestId: event.RequestId}.Build())
	if err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	ctx = context.WithValue(n.context(ctx), notificationPublisher{}, true)
	notification, err := n.service.Add(ctx, resource.NotificationAddRequest_builder{
		Topic: resource.NotificationTopic_builder{ResourceId: session.GetProject().GetId()}.Build(), SourceId: session.GetId(), Kind: kind, Payload: payload, IdempotencyKey: fmt.Sprintf("%s:%d:%s", c.generation, event.Seq, kind),
	}.Build())
	if err != nil {
		return err
	}
	read, err := n.read(ctx, session)
	if err != nil {
		return err
	}
	if read != nil && read.GetGeneration() == c.generation && read.GetThroughSeq() >= event.Seq {
		_, err = n.service.Mark(ctx, resource.NotificationMarkRequest_builder{Refs: []*resource.NotificationRef{resource.NotificationRef_builder{Id: notification.GetId()}.Build()}}.Build())
	}
	return err
}

// Committed notifications precede checkpoint advancement. A crash can replay a
// publication, whose stable key makes it idempotent, but cannot skip one.
func (s Layer) CollectNotifications(ctx context.Context) error {
	n := s.shared.notifications
	if n == nil {
		return nil
	}
	head, direct := s.shared.runtime.(interface {
		NotificationHead(context.Context, string) (uint64, error)
	})
	refresh := s.Reconcile
	if direct {
		refresh = s.ensureSnapshot
	}
	if err := refresh(ctx); err != nil {
		return err
	}
	var initialized int
	err := s.shared.db.QueryRowContext(ctx, `SELECT 1 FROM notification_meta WHERE key='initialized'`).Scan(&initialized)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	first := initialized == 0
	var failures []error
	after := ""
	for {
		page, err := s.Next().Session().List(ctx, resource.SessionListRequest_builder{Size: 200, After: after}.Build())
		if err != nil {
			return err
		}
	sessionLoop:
		for _, session := range page.GetItems() {
			if !session.GetListed() || session.GetProject() == nil {
				continue
			}
			baseline := uint64(0)
			if first {
				baseline = session.GetStatus().GetLastSeq()
			}
			n.mu.Lock()
			c, err := n.cursor(ctx, session, baseline)
			n.mu.Unlock()
			if err != nil {
				failures = append(failures, fmt.Errorf("session %s: %w", session.GetRuntimeId(), err))
				continue
			}
			end := session.GetStatus().GetLastSeq()
			if direct {
				end, err = head.NotificationHead(ctx, session.GetRuntimeId())
				if err != nil {
					failures = append(failures, fmt.Errorf("session %s: %w", session.GetRuntimeId(), err))
					continue
				}
			}
			if end < c.seq {
				n.mu.Lock()
				err = n.markThrough(ctx, session, "", 0)
				if err == nil {
					c = notificationCursor{generation: core.ID()}
					_, err = s.shared.db.ExecContext(ctx, `UPDATE session_notification_cursor SET generation=?,seq=0 WHERE session_id=?`, c.generation, session.GetId())
				}
				n.mu.Unlock()
				if err != nil {
					failures = append(failures, fmt.Errorf("session %s: %w", session.GetRuntimeId(), err))
					continue
				}
			}
			for pages := 0; c.seq < end && pages < 16; pages++ {
				batch, err := s.shared.runtime.History(ctx, &api.WatchRequest{SessionId: session.GetRuntimeId(), AfterSeq: c.seq, Limit: 1024})
				if err != nil {
					failures = append(failures, fmt.Errorf("session %s: %w", session.GetRuntimeId(), err))
					continue sessionLoop
				}
				if len(batch.GetEvents()) == 0 {
					break
				}
				previous := c.seq
				for _, e := range batch.GetEvents() {
					if e.Seq <= c.seq || e.Seq > end {
						continue
					}
					if err := n.publish(ctx, session, c, e); err != nil {
						failures = append(failures, fmt.Errorf("session %s: %w", session.GetRuntimeId(), err))
						continue sessionLoop
					}
					c.seq = e.Seq
				}
				if c.seq == previous {
					break
				}
				if _, err = s.shared.db.ExecContext(ctx, `UPDATE session_notification_cursor SET seq=? WHERE session_id=? AND generation=?`, c.seq, session.GetId(), c.generation); err != nil {
					failures = append(failures, fmt.Errorf("session %s: %w", session.GetRuntimeId(), err))
					continue sessionLoop
				}
			}
		}
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	if first {
		_, err = s.shared.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_meta VALUES('initialized',X'01')`)
	}
	return errors.Join(append(failures, err)...)
}

func (s Layer) RunNotifications(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	lastError := ""
	var reported time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		work, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := s.CollectNotifications(work)
		cancel()
		if err == nil {
			lastError = ""
		} else if ctx.Err() == nil && (err.Error() != lastError || time.Since(reported) > time.Minute) {
			report(err)
			lastError = err.Error()
			reported = time.Now()
		}
	}
}

var _ resource.NotificationServiceServer = operatorNotifications{}
