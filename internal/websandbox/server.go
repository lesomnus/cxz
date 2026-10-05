// Package websandbox simulates the web RPC subset in memory. It deliberately
// imports no Manager, provider, account, Docker or filesystem implementation.
package websandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const maxEvents = 3000

type Server struct {
	mu       sync.Mutex
	projects []*resource.Project
	sessions []*session
	seed     uint64
	delay    time.Duration
	rev      uint64
	ctx      context.Context
	cancel   context.CancelFunc
}
type session struct {
	value      *resource.Session
	effort     string
	scenario   string
	events     []*resource.SessionEvent
	generation uint64
	turn       uint64
	seen       map[string]bool
	seenOrder  []string
}
type Projects struct {
	resource.UnimplementedProjectServiceServer
	S *Server
}
type Sessions struct {
	resource.UnimplementedSessionServiceServer
	S *Server
}

func New(seed uint64, delay time.Duration) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{seed: seed, delay: delay, ctx: ctx, cancel: cancel}
	for i, name := range []string{"Design playground", "Secondary project"} {
		id := fmt.Sprintf("project-%d", i+1)
		s.projects = append(s.projects, resource.Project_builder{Id: []byte(fmt.Sprintf("project-%08d", i+1)), RuntimeId: id, Alias: id, Name: name, Listed: true, Status: resource.ProjectStatus_builder{State: "running"}.Build()}.Build())
	}
	for i, name := range []string{"Project checklist", "Conversation", "Long history", "Approval question", "Simulated error", "Seeded random work", "Stopped session", "Other project"} {
		id := fmt.Sprintf("session-%d", i+1)
		agent := "claude"
		if i%2 == 1 {
			agent = "codex"
		}
		st := &session{effort: "high", scenario: []string{"checklist", "conversation", "long", "approval", "error", "random", "stopped", "conversation"}[i], seen: map[string]bool{}}
		project := s.projects[0]
		if i == 7 {
			project = s.projects[1]
		}
		st.value = resource.Session_builder{Id: []byte(fmt.Sprintf("session-%08d", i+1)), RuntimeId: id, Alias: id, Name: name, Agent: agent, Model: "sandbox-" + agent, Listed: true, Project: project, Status: resource.SessionStatus_builder{State: "idle", RunId: id + "-run-1", PermissionMode: "default"}.Build()}.Build()
		s.sessions = append(s.sessions, st)
		s.telemetry(st)
		s.event(st, "input", "Show the current state of this project.", "", nil)
		s.event(st, "assistant", answer(st.scenario), "", nil)
		if st.scenario == "long" {
			for n := 0; n < 2100; n++ {
				if n%40 == 0 {
					s.event(st, "input", fmt.Sprintf("Review history batch %d.", n/40+1), "", nil)
				}
				s.event(st, "assistant", fmt.Sprintf("History item %04d — Check scrolling through a long conversation.", n+1), "", nil)
			}
		}
		if st.scenario == "approval" {
			s.approval(st)
		}
		if st.scenario == "error" {
			s.event(st, "diagnostic", "Simulated usage limit — no real provider was called.", "", []byte(`{"code":"usageLimitExceeded","simulated":true}`))
		}
		if st.scenario == "stopped" {
			st.value.GetStatus().SetState("stopped")
		}
	}
	return s
}

// Seed provider-shaped snapshots so the shared composer can preview its status
// row. These are simulated values, not real account quota or model capacity.
func (s *Server) telemetry(st *session) {
	emit := func(kind, text string, value any) {
		raw, _ := json.Marshal(value)
		s.event(st, kind, text, "", raw)
	}
	s.models(st)
	reset := time.Now().Add(time.Hour).Unix()
	if st.value.GetAgent() == "codex" {
		emit("usage", "account/rateLimits/updated", map[string]any{"rateLimits": map[string]any{"primary": map[string]any{"usedPercent": 30, "windowDurationMins": 300, "resetsAt": reset}}})
		emit("usage", "thread/tokenUsage/updated", map[string]any{"tokenUsage": map[string]any{"last": map[string]any{"totalTokens": 24000}, "modelContextWindow": 128000}})
		return
	}
	emit("usage", "get_usage", map[string]any{"rate_limits": map[string]any{"five_hour": map[string]any{"utilization": 30, "resets_at": time.Unix(reset, 0).UTC().Format(time.RFC3339)}}})
	emit("usage", "context/message", map[string]any{"model": st.value.GetModel(), "usage": map[string]any{"input_tokens": 4000, "cache_read_input_tokens": 20000}})
	emit("turn_end", "completed", map[string]any{"modelUsage": map[string]any{st.value.GetModel(): map[string]any{"contextWindow": 200000}}})
}

// Deliberately simulated choices, shaped like the provider capability catalog.
func (s *Server) models(st *session) {
	model := "sandbox-" + st.value.GetAgent()
	effective := st.value.GetModel()
	if effective == "" {
		effective = model
	}
	effort := st.effort
	if effort == "" {
		effort = "high"
	}
	choices := []map[string]any{
		{"id": model, "name": "Sandbox standard", "efforts": []string{"low", "medium", "high"}, "default_effort": "high", "default": true},
		{"id": model + "-compact", "name": "Sandbox compact", "efforts": []string{"low", "high"}, "default_effort": "high"},
	}
	raw, _ := json.Marshal(map[string]any{"models": choices, "model": st.value.GetModel(), "effort": st.effort, "effective_model": effective, "effective_effort": effort, "source": "sandbox"})
	s.event(st, "models", "catalog", "", raw)
}
func (s *Server) Close() { s.cancel() }
func (s *Server) Register(r grpc.ServiceRegistrar) {
	resource.RegisterProjectServiceServer(r, &Projects{S: s})
	resource.RegisterSessionServiceServer(r, &Sessions{S: s})
}
func (s *Server) event(st *session, kind, text, request string, payload []byte) {
	seq := st.value.GetStatus().GetLastSeq() + 1
	e := resource.SessionEvent_builder{RunId: st.value.GetStatus().GetRunId(), Seq: seq, TimeMs: 1700000000000 + int64(seq)*100, Kind: kind, Text: text, RequestId: request, Payload: payload}.Build()
	st.events = append(st.events, e)
	if len(st.events) > maxEvents {
		st.events = append([]*resource.SessionEvent(nil), st.events[len(st.events)-maxEvents:]...)
	}
	st.value.GetStatus().SetLastSeq(seq)
	s.rev++
}
func (s *Server) state(st *session, state string) {
	st.value.GetStatus().SetState(state)
	s.event(st, "state", state, "", nil)
}
func matchProject(p *resource.Project, r *resource.ProjectRef) bool {
	return r == nil || r.GetRuntimeId() == p.GetRuntimeId() || r.GetAlias() == p.GetAlias() || bytes.Equal(r.GetId(), p.GetId())
}
func matchSession(st *session, r *resource.SessionRef) bool {
	return r == nil || r.GetRuntimeId() == st.value.GetRuntimeId() || r.GetAlias() == st.value.GetAlias() || bytes.Equal(r.GetId(), st.value.GetId())
}
func (s *Server) find(r *resource.SessionRef) (*session, error) {
	if r != nil {
		for _, st := range s.sessions {
			if matchSession(st, r) {
				return st, nil
			}
		}
	}
	return nil, status.Error(codes.NotFound, "sandbox session not found")
}
func sessionMatches(st *session, filters []*resource.SessionFilter) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if matchSession(st, f.GetRef()) && matchProject(st.value.GetProject(), f.GetProject()) && (!f.HasListed() || f.GetListed() == st.value.GetListed()) {
			return true
		}
	}
	return false
}
func projectMatches(p *resource.Project, filters []*resource.ProjectFilter) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if matchProject(p, f.GetRef()) && (!f.HasListed() || f.GetListed() == p.GetListed()) {
			return true
		}
	}
	return false
}
func limit(n int32) int {
	if n <= 0 {
		return 50
	}
	return min(int(n), 100)
}
func (p *Projects) Get(_ context.Context, r *resource.ProjectGetRequest) (*resource.Project, error) {
	p.S.mu.Lock()
	defer p.S.mu.Unlock()
	for _, v := range p.S.projects {
		if r.GetRef() != nil && matchProject(v, r.GetRef()) {
			return proto.Clone(v).(*resource.Project), nil
		}
	}
	return nil, status.Error(codes.NotFound, "sandbox project not found")
}
func (p *Projects) List(_ context.Context, r *resource.ProjectListRequest) (*resource.ProjectListResponse, error) {
	p.S.mu.Lock()
	defer p.S.mu.Unlock()
	var items []*resource.Project
	for _, v := range p.S.projects {
		if v.GetRuntimeId() > r.GetAfter() && projectMatches(v, r.GetFilters()) {
			items = append(items, proto.Clone(v).(*resource.Project))
		}
	}
	next := ""
	if len(items) > limit(r.GetSize()) {
		items = items[:limit(r.GetSize())]
		next = items[len(items)-1].GetRuntimeId()
	}
	return resource.ProjectListResponse_builder{Items: items, Next: next}.Build(), nil
}
func (p *Projects) Watch(r *resource.ProjectWatchRequest, stream grpc.ServerStreamingServer[resource.ProjectWatchResponse]) error {
	if !r.GetSkipSnapshot() {
		list, _ := p.List(stream.Context(), resource.ProjectListRequest_builder{Filters: r.GetFilters()}.Build())
		var items []*resource.ProjectWatchItem
		for _, v := range list.GetItems() {
			items = append(items, resource.ProjectWatchItem_builder{Id: v.GetId(), Value: v}.Build())
		}
		if err := stream.Send(resource.ProjectWatchResponse_builder{Items: items}.Build()); err != nil {
			return err
		}
	}
	select {
	case <-stream.Context().Done():
		return stream.Context().Err()
	case <-p.S.ctx.Done():
		return p.S.ctx.Err()
	}
}
func (x *Sessions) Get(_ context.Context, r *resource.SessionGetRequest) (*resource.Session, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	return proto.Clone(st.value).(*resource.Session), nil
}
func (x *Sessions) List(_ context.Context, r *resource.SessionListRequest) (*resource.SessionListResponse, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []*resource.Session
	for _, st := range s.sessions {
		if st.value.GetRuntimeId() > r.GetAfter() && sessionMatches(st, r.GetFilters()) {
			items = append(items, proto.Clone(st.value).(*resource.Session))
		}
	}
	next := ""
	if len(items) > limit(r.GetSize()) {
		items = items[:limit(r.GetSize())]
		next = items[len(items)-1].GetRuntimeId()
	}
	return resource.SessionListResponse_builder{Items: items, Next: next}.Build(), nil
}
func (x *Sessions) Watch(r *resource.SessionWatchRequest, stream grpc.ServerStreamingServer[resource.SessionWatchResponse]) error {
	s := x.S
	s.mu.Lock()
	last := s.rev
	s.mu.Unlock()
	first := !r.GetSkipSnapshot()
	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	for {
		s.mu.Lock()
		rev := s.rev
		var items []*resource.SessionWatchItem
		if first || rev != last {
			for _, st := range s.sessions {
				if sessionMatches(st, r.GetFilters()) {
					v := proto.Clone(st.value).(*resource.Session)
					items = append(items, resource.SessionWatchItem_builder{Id: v.GetId(), Value: v}.Build())
				}
			}
		}
		s.mu.Unlock()
		if first || rev != last {
			if err := stream.Send(resource.SessionWatchResponse_builder{Items: items}.Build()); err != nil {
				return err
			}
			last = rev
			first = false
		}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-tick.C:
		}
	}
}
func (x *Sessions) History(_ context.Context, r *resource.SessionEventsRequest) (*resource.SessionEventBatch, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	var events []*resource.SessionEvent
	for _, e := range st.events {
		if e.GetSeq() > r.GetAfterSeq() {
			events = append(events, proto.Clone(e).(*resource.SessionEvent))
			if len(events) == 128 {
				break // Match the production history page size.
			}
		}
	}
	return resource.SessionEventBatch_builder{Events: events}.Build(), nil
}
func (x *Sessions) Events(r *resource.SessionEventsRequest, stream grpc.ServerStreamingServer[resource.SessionEvent]) error {
	query := proto.Clone(r).(*resource.SessionEventsRequest)
	tick := time.NewTicker(60 * time.Millisecond)
	defer tick.Stop()
	for {
		batch, err := x.History(stream.Context(), query)
		if err != nil {
			return err
		}
		for _, e := range batch.GetEvents() {
			if err = stream.Send(e); err != nil {
				return err
			}
			query.SetAfterSeq(e.GetSeq())
		}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-x.S.ctx.Done():
			return x.S.ctx.Err()
		case <-tick.C:
		}
	}
}
func receipt(id string) *resource.SessionReceipt {
	return resource.SessionReceipt_builder{ClientId: proto.String(id), Status: proto.String("accepted")}.Build()
}
func checkRun(st *session, run string) error {
	if run != "" && run != st.value.GetStatus().GetRunId() {
		return status.Error(codes.FailedPrecondition, "stale sandbox run")
	}
	return nil
}
func remember(st *session, id string) {
	if id == "" {
		return
	}
	st.seen[id] = true
	st.seenOrder = append(st.seenOrder, id)
	if len(st.seenOrder) > 256 {
		delete(st.seen, st.seenOrder[0])
		st.seenOrder = st.seenOrder[1:]
	}
}
func (x *Sessions) Send(_ context.Context, r *resource.SessionSendRequest) (*resource.SessionReceipt, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	if st.seen[r.GetClientId()] {
		return receipt(r.GetClientId()), nil
	}
	if err = checkRun(st, r.GetRunId()); err != nil {
		return nil, err
	}
	if st.value.GetStatus().GetState() != "idle" {
		return nil, status.Error(codes.FailedPrecondition, "resume the session, resolve its question, or interrupt its current turn first")
	}
	if strings.TrimSpace(r.GetText()) == "" || len(r.GetText()) > 65536 {
		return nil, status.Error(codes.InvalidArgument, "enter 1–65536 bytes of text")
	}
	fields := strings.Fields(r.GetText())
	if fields[0] == "/model" || fields[0] == "/effort" {
		if len(fields) != 2 {
			return nil, status.Error(codes.InvalidArgument, "choose a model or effort value")
		}
		value := fields[1]
		if value == "default" {
			value = ""
		}
		model := "sandbox-" + st.value.GetAgent()
		if fields[0] == "/model" {
			if st.effort != "" {
				return nil, status.Error(codes.FailedPrecondition, "reset /effort default before changing models")
			}
			if value != "" && value != model && value != model+"-compact" {
				return nil, status.Error(codes.InvalidArgument, "model not in catalog")
			}
			st.value.SetModel(value)
		} else {
			efforts := []string{"low", "medium", "high"}
			if strings.HasSuffix(st.value.GetModel(), "-compact") {
				efforts = []string{"low", "high"}
			}
			if value != "" && !slices.Contains(efforts, value) {
				return nil, status.Error(codes.InvalidArgument, "effort not supported by model")
			}
			st.effort = value
		}
		remember(st, r.GetClientId())
		raw, _ := json.Marshal(map[string]string{"value": value})
		s.event(st, "setting", strings.TrimPrefix(fields[0], "/"), r.GetClientId(), raw)
		s.models(st)
		return receipt(r.GetClientId()), nil
	}
	remember(st, r.GetClientId())
	st.turn++
	st.generation++
	s.event(st, "input", r.GetText(), "", nil)
	s.state(st, "running")
	go s.respond(st, st.generation)
	return receipt(r.GetClientId()), nil
}
func (s *Server) respond(st *session, generation uint64) {
	s.mu.Lock()
	scenario, turn := st.scenario, st.turn
	s.mu.Unlock()
	rng := rand.New(rand.NewPCG(s.seed+turn, s.seed^uint64(len(st.value.GetRuntimeId()))))
	names := []string{"Read README.md", "Search components", "Check layout", "Run sample tests", "Build preview"}
	count := 3
	if scenario == "random" {
		count = 2 + rng.IntN(4)
	}
	for i := 0; i < count; i++ {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(s.delay):
		}
		s.mu.Lock()
		if st.generation != generation {
			s.mu.Unlock()
			return
		}
		name := names[i%len(names)]
		if scenario == "random" {
			name = names[rng.IntN(len(names))]
		}
		text := fmt.Sprintf("%s · %d/%d", name, i+1, count)
		s.event(st, "tool", text, "", []byte(`{"simulated":true,"status":"running"}`))
		s.mu.Unlock()
	}
	select {
	case <-s.ctx.Done():
		return
	case <-time.After(s.delay):
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.generation != generation {
		return
	}
	if scenario == "approval" {
		s.approval(st)
		return
	}
	if scenario == "error" {
		s.event(st, "diagnostic", "Simulated request failed. Retry or switch scenarios; no account was charged.", "", nil)
		s.state(st, "idle")
		return
	}
	s.event(st, "tool_result", "Sample tasks completed", "", []byte(`{"simulated":true,"status":"completed","output":"All fixture checks passed"}`))
	s.event(st, "assistant", answer(scenario), "", nil)
	s.state(st, "idle")
}
func answer(scenario string) string {
	if scenario == "checklist" {
		return "## Current status\n\nThe web preview is ready for review. **No real files or accounts were used.**\n\n- [x] Check the layout\n- [ ] Check small screens\n\n```go\nfmt.Println(\"Hello, sandbox!\")\n```\n\n> Next, you can review the approval screen."
	}
	return "## Preview ready\n\nThis is a **simulated response** from the sandbox agent.\n\n| Task | Result |\n| --- | --- |\n| Layout | Ready |\n| Tests | Fixture only |\n\n```typescript\nconst greeting = \"Hello, sandbox!\";\nconsole.log(greeting);\n```\n\nTry another message, an approval, or a narrow viewport."
}
func (s *Server) approval(st *session) {
	id := fmt.Sprintf("question-%d", st.value.GetStatus().GetLastSeq()+1)
	s.event(st, "approval", "item/tool/requestUserInput", id, []byte(`{"params":{"questions":[{"id":"environment","question":"Which environment?","isOther":true,"options":[{"label":"Development","description":"Preview the development layout"},{"label":"Production","description":"Preview the production layout"}]}]}}`))
	st.value.GetStatus().SetPending([]*resource.SessionEvent{st.events[len(st.events)-1]})
	s.state(st, "waiting")
}
func (x *Sessions) Reply(_ context.Context, r *resource.SessionReplyRequest) (*resource.SessionReceipt, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	if st.seen[r.GetClientId()] {
		return receipt(r.GetClientId()), nil
	}
	if err = checkRun(st, r.GetRunId()); err != nil {
		return nil, err
	}
	pending := st.value.GetStatus().GetPending()
	if len(pending) != 1 || pending[0].GetRequestId() != r.GetRequestId() {
		return nil, status.Error(codes.NotFound, "sandbox question no longer pending")
	}
	remember(st, r.GetClientId())
	s.event(st, "approval_resolved", fmt.Sprintf("allow=%t", r.GetAllow()), r.GetRequestId(), nil)
	st.value.GetStatus().SetPending(nil)
	s.event(st, "assistant", "Your selection was recorded for this preview.\n\n"+r.GetAnswersJson(), "", nil)
	s.state(st, "idle")
	return receipt(r.GetClientId()), nil
}
func (s *Server) cancelTurn(st *session) {
	st.generation++
	for _, p := range st.value.GetStatus().GetPending() {
		s.event(st, "approval_resolved", "canceled", p.GetRequestId(), nil)
	}
	st.value.GetStatus().SetPending(nil)
}
func (x *Sessions) Interrupt(_ context.Context, r *resource.SessionControl) (*resource.SessionReceipt, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	if err = checkRun(st, r.GetRunId()); err != nil {
		return nil, err
	}
	s.cancelTurn(st)
	if st.value.GetStatus().GetState() != "stopped" {
		s.state(st, "idle")
	}
	return receipt(r.GetClientId()), nil
}
func (x *Sessions) Stop(_ context.Context, r *resource.SessionControl) (*resource.Session, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	if err = checkRun(st, r.GetRunId()); err != nil {
		return nil, err
	}
	s.cancelTurn(st)
	s.state(st, "stopped")
	return proto.Clone(st.value).(*resource.Session), nil
}
func (x *Sessions) Resume(_ context.Context, r *resource.SessionControl) (*resource.Session, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	if err = checkRun(st, r.GetRunId()); err != nil {
		return nil, err
	}
	if st.value.GetStatus().GetState() == "stopped" {
		st.generation++
		st.value.GetStatus().SetRunId(fmt.Sprintf("%s-run-%d", st.value.GetRuntimeId(), st.generation+1))
		s.state(st, "idle")
	}
	return proto.Clone(st.value).(*resource.Session), nil
}
