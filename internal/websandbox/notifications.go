package websandbox

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/pdid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type sandboxNotice struct {
	value   *resource.Notification
	data    *resource.SessionNotification
	ordinal uint64
}
type sandboxNotifications struct {
	resource.UnimplementedNotificationServiceServer
	s        *Server
	reader   []byte
	revision uint64
	rows     []sandboxNotice
	receipts map[string]*resource.NotificationReceipt
	reads    map[string]uint64
}

func newSandboxNotifications(s *Server) *sandboxNotifications {
	return &sandboxNotifications{s: s, reader: pdid.New(16).Bytes(), receipts: map[string]*resource.NotificationReceipt{}, reads: map[string]uint64{}}
}
func (n *sandboxNotifications) generation(st *session) string {
	return fmt.Sprintf("%x/%x", n.reader, st.value.GetId())
}
func (n *sandboxNotifications) record(st *session, e *resource.SessionEvent) {
	kind := ""
	if e.GetKind() == "turn_end" {
		if e.GetText() == "completed" {
			kind = "session.completed"
		}
		if e.GetText() == "failed" {
			kind = "session.failed"
		}
	} else if e.GetKind() == "approval" {
		if e.GetText() == "AskUserQuestion" || e.GetText() == "item/tool/requestUserInput" {
			kind = "session.question"
		}
	}
	if kind == "" {
		return
	}
	data := resource.SessionNotification_builder{RuntimeId: st.value.GetRuntimeId(), Generation: n.generation(st), Seq: e.GetSeq(), RequestId: e.GetRequestId()}.Build()
	payload, _ := anypb.New(data)
	n.revision++
	value := resource.Notification_builder{Id: pdid.New(14).Bytes(), Kind: kind, Topic: resource.NotificationTopic_builder{ResourceId: st.value.GetProject().GetId()}.Build(), SourceId: st.value.GetId(), Payload: payload, DateCreated: timestamppb.Now(), DateUpdated: timestamppb.Now()}.Build()
	n.rows = append(n.rows, sandboxNotice{value, data, n.revision})
	if n.reads[st.value.GetRuntimeId()] >= e.GetSeq() {
		n.mark(value)
	}
	if len(n.rows) > 1000 {
		delete(n.receipts, hex.EncodeToString(n.rows[0].value.GetId()))
		n.rows = append([]sandboxNotice(nil), n.rows[1:]...)
	}
}
func (n *sandboxNotifications) mark(v *resource.Notification) *resource.NotificationReceipt {
	k := hex.EncodeToString(v.GetId())
	if old := n.receipts[k]; old != nil {
		return old
	}
	r := resource.NotificationReceipt_builder{Id: pdid.New(15).Bytes(), NotificationId: v.GetId(), ReaderId: n.reader, DataRead: timestamppb.Now(), DateUpdated: timestamppb.Now(), DateCreated: timestamppb.Now()}.Build()
	n.receipts[k] = r
	n.revision++
	return r
}
func noticeTopics(topics []*resource.NotificationTopic, tenant []byte) error {
	if len(tenant) > 0 || len(topics) == 0 || len(topics) > 100 {
		return status.Error(codes.InvalidArgument, "use global project topics")
	}
	return nil
}
func topicMatches(a, b *resource.NotificationTopic) bool {
	return a != nil && b != nil && proto.Equal(a, b)
}
func (n *sandboxNotifications) matches(row sandboxNotice, topics []*resource.NotificationTopic, f *resource.NotificationReadFilter) bool {
	found := false
	for _, t := range topics {
		found = found || topicMatches(t, row.value.GetTopic())
	}
	if !found {
		return false
	}
	if len(f.GetKinds()) > 0 {
		found = false
		for _, k := range f.GetKinds() {
			found = found || k == row.value.GetKind()
		}
		if !found {
			return false
		}
	}
	if len(f.GetSourceIds()) > 0 {
		found = false
		for _, id := range f.GetSourceIds() {
			found = found || bytes.Equal(id, row.value.GetSourceId())
		}
		if !found {
			return false
		}
	}
	return !f.GetUnreadOnly() || n.receipts[hex.EncodeToString(row.value.GetId())] == nil
}
func (n *sandboxNotifications) Add(context.Context, *resource.NotificationAddRequest) (*resource.Notification, error) {
	return nil, status.Error(codes.PermissionDenied, "only the Manager publishes notifications")
}
func (n *sandboxNotifications) Get(_ context.Context, r *resource.NotificationGetRequest) (*resource.Notification, error) {
	n.s.mu.Lock()
	defer n.s.mu.Unlock()
	for _, row := range n.rows {
		if bytes.Equal(row.value.GetId(), r.GetRef().GetId()) {
			return proto.Clone(row.value).(*resource.Notification), nil
		}
	}
	return nil, status.Error(codes.NotFound, "notification not found")
}
func (n *sandboxNotifications) Read(_ context.Context, r *resource.NotificationReadRequest) (*resource.NotificationReadResponse, error) {
	if err := noticeTopics(r.GetTopics(), r.GetTenantId()); err != nil {
		return nil, err
	}
	n.s.mu.Lock()
	defer n.s.mu.Unlock()
	limit := int(r.GetSize())
	if limit == 0 {
		limit = 50
	}
	if limit < 0 {
		return nil, status.Error(codes.InvalidArgument, "negative size")
	}
	limit = min(limit, 100)
	before := uint64(0)
	if r.GetCursor() != "" {
		var err error
		before, err = strconv.ParseUint(r.GetCursor(), 10, 64)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
	}
	b := resource.NotificationReadResponse_builder{ReaderId: n.reader}
	last := uint64(0)
	for i := len(n.rows) - 1; i >= 0; i-- {
		row := n.rows[i]
		if before > 0 && row.ordinal >= before || !n.matches(row, r.GetTopics(), r.GetFilter()) {
			continue
		}
		if len(b.Items) == limit {
			b.Next = strconv.FormatUint(last, 10)
			break
		}
		b.Items = append(b.Items, resource.NotificationReadItem_builder{Notification: row.value, Receipt: n.receipts[hex.EncodeToString(row.value.GetId())]}.Build())
		last = row.ordinal
	}
	return b.Build(), nil
}
func (n *sandboxNotifications) List(ctx context.Context, r *resource.NotificationListRequest) (*resource.NotificationListResponse, error) {
	p, err := n.Read(ctx, resource.NotificationReadRequest_builder{Topics: r.GetTopics(), TenantId: r.GetTenantId(), Size: r.GetSize(), Cursor: r.GetCursor()}.Build())
	if err != nil {
		return nil, err
	}
	b := resource.NotificationListResponse_builder{Next: p.GetNext()}
	for _, item := range p.GetItems() {
		b.Items = append(b.Items, item.GetNotification())
	}
	return b.Build(), nil
}
func (n *sandboxNotifications) Count(_ context.Context, r *resource.NotificationCountRequest) (*resource.NotificationCountResponse, error) {
	if err := noticeTopics(r.GetTopics(), r.GetTenantId()); err != nil {
		return nil, err
	}
	n.s.mu.Lock()
	defer n.s.mu.Unlock()
	b := resource.NotificationCountResponse_builder{ReaderId: n.reader}
	all := map[string]bool{}
	for _, topic := range r.GetTopics() {
		count := uint64(0)
		sources := map[string]bool{}
		for _, row := range n.rows {
			if n.matches(row, []*resource.NotificationTopic{topic}, r.GetFilter()) {
				count++
				key := hex.EncodeToString(row.value.GetSourceId())
				sources[key] = true
				all[key] = true
			}
		}
		b.Topics = append(b.Topics, resource.NotificationTopicCount_builder{Topic: topic, Count: count, SourceCount: uint64(len(sources))}.Build())
		b.Count += count
	}
	b.SourceCount = uint64(len(all))
	return b.Build(), nil
}
func (n *sandboxNotifications) Mark(_ context.Context, r *resource.NotificationMarkRequest) (*resource.NotificationMarkResponse, error) {
	if len(r.GetTenantId()) > 0 || len(r.GetRefs()) < 1 || len(r.GetRefs()) > 100 {
		return nil, status.Error(codes.InvalidArgument, "mark 1–100 global notifications")
	}
	n.s.mu.Lock()
	defer n.s.mu.Unlock()
	values := []*resource.Notification{}
	seen := map[string]bool{}
	for _, ref := range r.GetRefs() {
		key := hex.EncodeToString(ref.GetId())
		if seen[key] {
			return nil, status.Error(codes.InvalidArgument, "duplicate notification")
		}
		seen[key] = true
		found := false
		for _, row := range n.rows {
			if bytes.Equal(row.value.GetId(), ref.GetId()) {
				values = append(values, row.value)
				found = true
				break
			}
		}
		if !found {
			return nil, status.Error(codes.NotFound, "notification not found")
		}
	}
	b := resource.NotificationMarkResponse_builder{ReaderId: n.reader}
	for _, value := range values {
		b.Items = append(b.Items, n.mark(value))
	}
	return b.Build(), nil
}
func (n *sandboxNotifications) Listen(r *resource.NotificationListenRequest, stream grpc.ServerStreamingServer[resource.NotificationListenResponse]) error {
	if err := noticeTopics(r.GetTopics(), r.GetTenantId()); err != nil {
		return err
	}
	n.s.mu.Lock()
	revision := n.revision
	n.s.mu.Unlock()
	if err := stream.Send(resource.NotificationListenResponse_builder{Ready: resource.NotificationListenReady_builder{ReaderId: n.reader}.Build()}.Build()); err != nil {
		return err
	}
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-n.s.ctx.Done():
			return nil
		case <-tick.C:
		}
		n.s.mu.Lock()
		next := n.revision
		n.s.mu.Unlock()
		if next == revision {
			continue
		}
		revision = next
		if err := stream.Send(resource.NotificationListenResponse_builder{Invalidated: resource.NotificationInvalidation_builder{Topics: r.GetTopics()}.Build()}.Build()); err != nil {
			return err
		}
	}
}
func (x *Sessions) ReadState(_ context.Context, r *resource.SessionReadStateRequest) (*resource.SessionReadState, error) {
	x.S.mu.Lock()
	defer x.S.mu.Unlock()
	st, err := x.S.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	n := x.S.notifications
	return resource.SessionReadState_builder{ReaderId: n.reader, Generation: n.generation(st), ThroughSeq: n.reads[st.value.GetRuntimeId()]}.Build(), nil
}
func (x *Sessions) MarkRead(_ context.Context, r *resource.SessionMarkReadRequest) (*resource.SessionReadState, error) {
	x.S.mu.Lock()
	defer x.S.mu.Unlock()
	st, err := x.S.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	n := x.S.notifications
	if r.GetGeneration() != n.generation(st) {
		return nil, status.Error(codes.FailedPrecondition, "conversation generation changed")
	}
	if r.GetThroughSeq() > st.value.GetStatus().GetLastSeq() {
		return nil, status.Error(codes.OutOfRange, "unknown event")
	}
	id := st.value.GetRuntimeId()
	n.reads[id] = max(n.reads[id], r.GetThroughSeq())
	for _, row := range n.rows {
		if row.data.GetRuntimeId() == id && row.data.GetGeneration() == r.GetGeneration() && row.data.GetSeq() <= n.reads[id] {
			n.mark(row.value)
		}
	}
	return resource.SessionReadState_builder{ReaderId: n.reader, Generation: n.generation(st), ThroughSeq: n.reads[id]}.Build(), nil
}
