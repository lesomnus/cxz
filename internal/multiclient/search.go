package multiclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// A client can be attached to several installations at once, and "search every
// conversation" means every one of them. Each installation already answers in
// order, so merging them is a matter of taking the newest of whatever each has
// in hand -- which is why one visit of lookahead per connection is enough, and
// why a connection that has nothing yet has to be waited for rather than
// guessed at.
//
// Ids are scoped on the way out and unscoped on the way in, as everywhere else
// here: a project id from one installation means nothing to another.
func (c *Client) Search(ctx context.Context, r *api.SearchRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.SearchReply], error) {
	cursors, err := decodeSearchCursors(r.Cursor)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	names := make([]string, 0, len(c.names))
	clients := map[string]api.SessionsClient{}
	for _, name := range c.names {
		s := c.sources[name]
		if s == nil || s.Client == nil {
			continue
		}
		if len(cursors) > 0 && cursors[name] == "" {
			continue // This connection finished the window on an earlier page.
		}
		names = append(names, name)
		clients[name] = s.Client
	}
	c.mu.Unlock()
	if len(names) == 0 {
		return nil, fmt.Errorf("no connection is available to search")
	}

	reading, stop := context.WithCancel(ctx)
	merged := &mergedSearch{ctx: reading, stop: stop, out: make(chan *api.SearchReply, 16), cursors: map[string]string{}}
	messages := make(chan namedReply, 32)
	var wg sync.WaitGroup
	for _, name := range names {
		request, ok := scopeSearchRequest(r, name, cursors[name])
		if !ok {
			continue // Nothing this one holds was selected.
		}
		merged.sources = append(merged.sources, &searchSource{name: name})
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, err := clients[name].Search(reading, request, opts...)
			if err != nil {
				select {
				case messages <- namedReply{name: name, err: err}:
				case <-reading.Done():
				}
				return
			}
			for {
				reply, err := stream.Recv()
				select {
				case messages <- namedReply{name: name, reply: reply, err: err}:
				case <-reading.Done():
					return
				}
				if err != nil {
					return
				}
			}
		}()
	}
	go func() { wg.Wait(); close(messages) }()
	if len(merged.sources) == 0 {
		stop()
		return nil, fmt.Errorf("no connection holds the projects that were selected")
	}
	go merged.run(messages)
	return merged, nil
}

// scopeSearchRequest translates a request for one connection, and reports
// whether that connection was selected at all: a request naming only another
// installation's projects must not become "everything here".
func scopeSearchRequest(r *api.SearchRequest, name, cursor string) (*api.SearchRequest, bool) {
	out := &api.SearchRequest{
		Query: r.Query, Match: r.Match, IgnoreCase: r.IgnoreCase, View: r.View, IncludeTools: r.IncludeTools,
		SinceMs: r.SinceMs, UntilMs: r.UntilMs, Limit: r.Limit, Snippet: r.Snippet, ClientId: r.ClientId, Cursor: cursor,
	}
	for _, id := range r.Projects {
		if scope, local := Split(id); scope == "" || scope == name {
			out.Projects = append(out.Projects, local)
		}
	}
	if len(r.Projects) > 0 && len(out.Projects) == 0 {
		return nil, false
	}
	for _, id := range r.Exclude {
		if scope, local := Split(id); scope == "" || scope == name {
			out.Exclude = append(out.Exclude, local)
		}
	}
	for _, id := range r.Sessions {
		if scope, local := Split(id); scope == "" || scope == name {
			out.Sessions = append(out.Sessions, local)
		}
	}
	return out, true
}

type namedReply struct {
	name  string
	reply *api.SearchReply
	err   error
}

type searchSource struct {
	name    string
	pending *api.SearchVisit
	done    bool
}

// mergedSearch is a ServerStreamingClient of its own, so a caller cannot tell
// one installation from several.
type mergedSearch struct {
	ctx     context.Context
	stop    context.CancelFunc
	out     chan *api.SearchReply
	sources []*searchSource
	cursors map[string]string
	totals  searchTotals
	failure error
}

// totals are counted as plain numbers rather than in a message: a protobuf
// value cannot be copied, and the merge adds to this on every reply.
type searchTotals struct {
	projects, unavailable, sessions, hits, truncated int32
	sinceMS, untilMS                                 int64
}

// The rest of the stream interface. A merged stream has no headers of its own,
// and the only thing its caller does with it is read.
func (m *mergedSearch) Context() context.Context     { return m.ctx }
func (m *mergedSearch) CloseSend() error             { m.stop(); return nil }
func (m *mergedSearch) Header() (metadata.MD, error) { return nil, nil }
func (m *mergedSearch) Trailer() metadata.MD         { return nil }
func (m *mergedSearch) SendMsg(any) error            { return fmt.Errorf("a search stream carries one request") }
func (m *mergedSearch) RecvMsg(any) error            { return fmt.Errorf("read a merged search with Recv") }

func (m *mergedSearch) Recv() (*api.SearchReply, error) {
	reply, ok := <-m.out
	if !ok {
		if m.failure != nil {
			return nil, m.failure
		}
		return nil, fmt.Errorf("search ended")
	}
	return reply, nil
}

func (m *mergedSearch) run(messages <-chan namedReply) {
	defer close(m.out)
	defer m.stop()
	for {
		for m.emitReady() {
		}
		message, ok := <-messages
		if !ok {
			break
		}
		if err := m.absorb(message); err != nil {
			m.failure = err
			return
		}
	}
	for m.emitReady() {
	}
	summary := &api.SearchSummary{
		Projects: m.totals.projects, Unavailable: m.totals.unavailable, Sessions: m.totals.sessions,
		Hits: m.totals.hits, Truncated: m.totals.truncated, SinceMs: m.totals.sinceMS, UntilMs: m.totals.untilMS,
	}
	if len(m.cursors) > 0 {
		summary.HasMore = true
		summary.NextCursor = encodeSearchCursors(m.cursors)
	}
	m.send(&api.SearchReply{Summary: summary})
}

func (m *mergedSearch) source(name string) *searchSource {
	for _, s := range m.sources {
		if s.name == name {
			return s
		}
	}
	return nil
}

func (m *mergedSearch) absorb(message namedReply) error {
	s := m.source(message.name)
	if s == nil {
		return nil
	}
	if message.err != nil {
		s.done = true
		// One unreachable installation does not fail the search; it is reported
		// where every other delay is reported.
		m.totals.unavailable++
		m.send(&api.SearchReply{Progress: &api.SearchProgress{ProjectName: message.name, State: "unavailable", Message: message.err.Error()}})
		return nil
	}
	switch {
	case message.reply.GetVisit() != nil:
		s.pending = message.reply.Visit
	case message.reply.GetProgress() != nil:
		p := message.reply.Progress
		m.send(&api.SearchReply{Progress: &api.SearchProgress{ProjectId: Scope(message.name, p.ProjectId), ProjectName: decorateName(message.name, p.ProjectName), State: p.State, Message: p.Message, Opened: p.Opened, Total: p.Total}})
	case message.reply.GetSummary() != nil:
		v := message.reply.Summary
		m.totals.projects += v.Projects
		m.totals.unavailable += v.Unavailable
		m.totals.sessions += v.Sessions
		m.totals.hits += v.Hits
		m.totals.truncated += v.Truncated
		// One window was asked for, so whichever installation reports it first
		// is saying what the rest will.
		if m.totals.sinceMS == 0 {
			m.totals.sinceMS, m.totals.untilMS = v.SinceMs, v.UntilMs
		}
		if v.HasMore && v.NextCursor != "" {
			m.cursors[message.name] = v.NextCursor
		}
	}
	return nil
}

// emitReady sends the newest visit in hand, but only once every connection has
// either offered one or finished: until then, a newer one could still arrive.
func (m *mergedSearch) emitReady() bool {
	var pick *searchSource
	for _, s := range m.sources {
		if s.pending == nil {
			if !s.done {
				return false
			}
			continue
		}
		if pick == nil || s.pending.ActivityMs > pick.pending.ActivityMs || (s.pending.ActivityMs == pick.pending.ActivityMs && s.pending.SessionId > pick.pending.SessionId) {
			pick = s
		}
	}
	if pick == nil {
		return false
	}
	visit := pick.pending
	pick.pending = nil
	visit.ProjectId = Scope(pick.name, visit.ProjectId)
	visit.ProjectName = decorateName(pick.name, visit.ProjectName)
	visit.SessionId = Scope(pick.name, visit.SessionId)
	m.send(&api.SearchReply{Visit: visit})
	return true
}

// send never outlives its caller: a reader that stops reading cancels its
// context, and a merge still holding a result would be a leaked goroutine.
func (m *mergedSearch) send(r *api.SearchReply) {
	select {
	case m.out <- r:
	case <-m.ctx.Done():
	}
}

func decorateName(connection, name string) string {
	if connection == "" || name == "" {
		return name
	}
	return name + " · " + connection
}

// The cursor a caller gets back holds one cursor per connection, because each
// installation paged its own window. A connection missing from it has nothing
// more to say about that window.
func encodeSearchCursors(in map[string]string) string {
	b, _ := json.Marshal(in)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeSearchCursors(s string) (map[string]string, error) {
	if s == "" {
		return nil, nil
	}
	if len(s) > 1<<18 {
		return nil, fmt.Errorf("cursor is too large")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid search cursor")
	}
	out := map[string]string{}
	if json.Unmarshal(b, &out) != nil || len(out) == 0 {
		return nil, fmt.Errorf("invalid search cursor")
	}
	return out, nil
}
