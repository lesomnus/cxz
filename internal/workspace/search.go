package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/conversation"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

// Searching every project means reading every project's storage, and each one
// keeps its own: a project's journals live in its own volume, which only its
// container and a helper with that volume mounted can read. So this is a
// fan-out, and a project that is not running costs a container to open.
//
// The order the person asked for -- newest first, across everything -- is what
// makes it more than a loop. Each helper reports its sessions before reading
// any of them, which is a tail read per session rather than a scan, so the
// order is known early and cheaply. The merge then emits one session at a time
// from whichever project owns the newest, so results appear while the rest is
// still being read and nothing ever arrives above something already shown.
//
// Global order has a cost worth naming: the first result waits until every
// selected project has reported its index, because a project nobody has opened
// yet could hold the newest conversation. Opening is the slow part -- a stopped
// project is a container start -- and only a few are opened at once, so a search
// over many projects is quiet for a moment and then streams. Searching one
// project with --project skips all of that. If it ever needs to be faster, the
// fix is to let a helper report its index and wait, so that opening is not
// rationed by the same gate as reading.
const (
	searchHelpers     = 4 // projects open at once
	defaultSearchHits = 200
)

type SearchRequest struct {
	Query conversation.ScanQuery
	// Projects, when set, is the only projects to read; Exclude removes from
	// what it selected. Both are runtime ids: a name or an alias is resolved
	// before the request gets here.
	Projects []string
	Exclude  []string
	Cursor   string
}

// SearchVisit is one session's results. The session's alias and its project's
// name are the manager's to add: a helper reads journals and has heard of
// neither.
type SearchVisit struct {
	conversation.ScanVisit
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name,omitempty"`
	Alias       string `json:"alias,omitempty"`
	State       string `json:"state,omitempty"`
}

// SearchProgress says what is being read, because a search that takes ten
// seconds in silence is indistinguishable from a broken one.
type SearchProgress struct {
	ProjectID   string `json:"project_id,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
	State       string `json:"state"` // opening, reading, done or unavailable
	Message     string `json:"message,omitempty"`
	Opened      int    `json:"opened"`
	Total       int    `json:"total"`
}

type SearchResult struct {
	// Since and Until are the window that was read, which a continued page was
	// never told: it travelled in the cursor.
	Since       time.Time `json:"since,omitempty"`
	Until       time.Time `json:"until,omitempty"`
	Projects    int       `json:"projects"`
	Unavailable int       `json:"unavailable,omitempty"`
	Sessions    int       `json:"sessions"`
	Hits        int       `json:"hits"`
	Truncated   int       `json:"truncated,omitempty"`
	NextCursor  string    `json:"next_cursor,omitempty"`
	HasMore     bool      `json:"has_more,omitempty"`
}

// Search reads every selected project and reports what it finds, newest first.
func (m *Manager) Search(ctx context.Context, r SearchRequest, visit func(SearchVisit) error, progress func(SearchProgress) error) (SearchResult, error) {
	out := SearchResult{}
	cursor := conversation.ScanCursor{Version: 1}
	if r.Cursor != "" {
		var err error
		if cursor, err = conversation.DecodeScanCursor(r.Cursor); err != nil {
			return out, err
		}
		// The window is fixed while it is being paged through. Events arriving
		// now are above it, and must not shift what has already been read.
		r.Query.Since, r.Query.Until = cursor.Window()
	}
	if r.Query.Limit == 0 {
		r.Query.Limit = defaultSearchHits
	}
	if r.Query.Until.IsZero() {
		r.Query.Until = time.Now().UTC()
	}
	out.Since, out.Until = r.Query.Since, r.Query.Until
	projects, err := m.searchProjects(ctx, r, cursor)
	if err != nil {
		return out, err
	}
	out.Projects = len(projects)
	if len(projects) == 0 {
		return out, nil
	}

	reading, stop := context.WithCancel(ctx)
	defer stop()
	messages := make(chan readerMessage, 64)
	readers := make([]*projectReader, 0, len(projects))
	gate := make(chan struct{}, searchHelpers)
	var wg sync.WaitGroup
	for _, p := range projects {
		q := r.Query
		q.Resume = cursor.At[p.id]
		reader := &projectReader{project: p}
		readers = append(readers, reader)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { send(reading, messages, readerMessage{reader: reader, closed: true}) }()
			select {
			case gate <- struct{}{}:
				defer func() { <-gate }()
			case <-reading.Done():
				return
			}
			m.readProject(reading, reader, q, messages)
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	if err = merge(readers, messages, done, r.Query.Limit, visit, progress, stop, &out); err != nil {
		return out, err
	}
	finish(&out, cursor, readers, r.Query)
	return out, nil
}

// merge is the ordering: it emits the session that is newest among what every
// project has in hand, and waits when a project that could still own a newer
// one has not said yet. It is separate from the containers that feed it so that
// the order can be tested without any.
func merge(readers []*projectReader, messages <-chan readerMessage, done <-chan struct{}, limit int, visit func(SearchVisit) error, progress func(SearchProgress) error, stop func(), out *SearchResult) error {
	emitted := 0
	for {
		pick, wait := nextReader(readers)
		if pick == nil && !wait {
			return nil
		}
		if pick == nil {
			if err := absorb(messages, done, readers, progress, out); err != nil {
				return err
			}
			continue
		}
		v := pick.take()
		if v.Truncated {
			out.Truncated++
		}
		// A visit that exactly fills the page is still a page that stopped
		// inside a session: the helper may have read further than there was
		// room to show, so the next page continues below the last hit shown
		// rather than assuming the session was finished.
		if len(v.Hits) >= limit-emitted {
			v.Hits = v.Hits[:limit-emitted]
			pick.partial = &conversation.Resume{Activity: v.Activity.UnixMilli(), Session: v.ID, Seq: v.Hits[len(v.Hits)-1].Seq}
		}
		if err := visit(pick.decorate(v)); err != nil {
			return err
		}
		emitted += len(v.Hits)
		out.Hits += len(v.Hits)
		if len(v.Hits) > 0 {
			// Sessions counts what a person was shown, not what was read: a
			// visit with nothing in it is not a result.
			out.Sessions++
			pick.emitted = conversation.Resume{Activity: v.Activity.UnixMilli(), Session: v.ID, Seq: v.Hits[len(v.Hits)-1].Seq}
		}
		if emitted >= limit {
			stop()
			return nil
		}
	}
}

// absorb takes the next thing a helper said and files it under the project that
// said it. It is the only place the merge waits.
func absorb(messages <-chan readerMessage, done <-chan struct{}, readers []*projectReader, progress func(SearchProgress) error, out *SearchResult) error {
	var m readerMessage
	select {
	case m = <-messages:
	case <-done:
		// Every helper has finished; drain what is left without blocking.
		select {
		case m = <-messages:
		default:
			for _, r := range readers {
				r.closed = true
			}
			return nil
		}
	}
	state := ""
	switch {
	case m.opening:
		state = "opening"
	case m.closed:
		m.reader.closed = true
		state = "done"
		if m.reader.failure != nil {
			return nil // The failure was already reported when it happened.
		}
	case m.failure != nil:
		m.reader.failure = m.failure
		m.reader.closed = true
		out.Unavailable++
		state = "unavailable"
	case m.message.Error != "":
		m.reader.failure = fmt.Errorf("%s", m.message.Error)
		out.Unavailable++
		state = "unavailable"
	case m.message.Indexed:
		m.reader.index = m.message.Index
		m.reader.indexed = true
		state = "reading"
	case m.message.Visit != nil:
		m.reader.pending = append(m.reader.pending, *m.message.Visit)
	case m.message.Result != nil:
		m.reader.result = *m.message.Result
	}
	if state == "" || progress == nil {
		return nil
	}
	opened := 0
	for _, r := range readers {
		if r.indexed || r.closed {
			opened++
		}
	}
	detail := ""
	if m.reader.failure != nil {
		detail = m.reader.failure.Error()
	}
	return progress(SearchProgress{ProjectID: m.reader.project.id, ProjectName: m.reader.project.name, State: state, Message: detail, Opened: opened, Total: len(readers)})
}

// nextReader is the merge. It returns the project to emit from, or asks to wait
// -- which it must do while any project could still turn out to own a newer
// session than the ones already in hand.
func nextReader(readers []*projectReader) (pick *projectReader, wait bool) {
	for _, r := range readers {
		if !r.indexed && !r.closed {
			return nil, true // Its sessions could be newer than anyone's.
		}
	}
	for _, r := range readers {
		next, ok := r.next()
		if !ok {
			continue
		}
		if pick == nil {
			pick = r
			continue
		}
		current, _ := pick.next()
		if next.Activity.After(current.Activity) || (next.Activity.Equal(current.Activity) && next.ID > current.ID) {
			pick = r
		}
	}
	if pick == nil {
		return nil, false
	}
	if len(pick.pending) == 0 {
		// It owns the newest session but has not finished reading it.
		return nil, !pick.closed
	}
	return pick, false
}

// finish decides whether this window has more to give, and where each project
// would pick it up.
func finish(out *SearchResult, cursor conversation.ScanCursor, readers []*projectReader, q conversation.ScanQuery) {
	if cursor.At == nil {
		cursor.At = map[string]conversation.Resume{}
	}
	if cursor.Done == nil {
		cursor.Done = map[string]bool{}
	}
	more := false
	for _, r := range readers {
		id := r.project.id
		// What was shown is what the next page continues below. The helper's
		// own stopping point can be further on, but resuming from there would
		// skip whatever it read and the page had no room for.
		if r.partial != nil {
			cursor.At[id] = *r.partial
		} else if r.emitted != (conversation.Resume{}) {
			cursor.At[id] = r.emitted
		}
		// A project is finished when its sessions have all been shown. Whether
		// its helper has exited is a different question and not this one: a
		// page that filled up stops reading before the last few messages
		// arrive, and waiting for them would be waiting for nothing.
		switch {
		case r.failure != nil, !r.indexed, r.partial != nil, len(r.pending) > 0, r.remaining(), r.result.Next != nil:
			more = true
		default:
			cursor.Done[id] = true
		}
	}
	if !more {
		out.HasMore = false
		return
	}
	cursor.SinceMS, cursor.UntilMS = conversation.TimeMS(q.Since), conversation.TimeMS(q.Until)
	out.HasMore = true
	out.NextCursor = cursor.Encode()
}

type searchProject struct {
	id, name, volume string
	sessions         []*api.Session
}

// searchProjects selects what to read. A project with no storage of its own has
// nothing to search, and one the cursor says is finished is not reopened.
func (m *Manager) searchProjects(ctx context.Context, r SearchRequest, cursor conversation.ScanCursor) ([]*searchProject, error) {
	m.mu.Lock()
	all, err := m.all(ctx)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	include := map[string]bool{}
	for _, v := range r.Projects {
		include[v] = true
	}
	exclude := map[string]bool{}
	for _, v := range r.Exclude {
		exclude[v] = true
	}
	var out []*searchProject
	for _, p := range all {
		if p.Volume == "" || cursor.Done[p.ID] || exclude[p.ID] {
			continue
		}
		if len(include) > 0 && !include[p.ID] {
			continue
		}
		out = append(out, &searchProject{id: p.ID, name: p.Name, volume: p.Volume, sessions: p.Sessions})
	}
	return out, nil
}

type readerMessage struct {
	reader  *projectReader
	message conversation.ScanMessage
	failure error
	opening bool
	closed  bool
}

// send never outlives the search. A page that has filled up stops reading, and
// a helper still talking into a channel nobody reads would be a leaked
// goroutine for every project that had not finished.
func send(ctx context.Context, to chan<- readerMessage, m readerMessage) {
	select {
	case to <- m:
	case <-ctx.Done():
	}
}

// projectReader is one project's place in the merge: what it says it holds,
// what it has delivered, and how far the caller has been shown.
type projectReader struct {
	project *searchProject
	index   []conversation.ScanSession
	pending []conversation.ScanVisit
	result  conversation.ScanResult
	emitted conversation.Resume
	partial *conversation.Resume
	failure error
	indexed bool
	closed  bool
	visited int
}

// next is the session this project would contribute next, from the index it
// reported rather than from what has arrived.
func (r *projectReader) next() (conversation.ScanSession, bool) {
	if len(r.pending) > 0 {
		return r.pending[0].ScanSession, true
	}
	if r.closed || r.visited >= len(r.index) {
		return conversation.ScanSession{}, false
	}
	return r.index[r.visited], true
}

func (r *projectReader) remaining() bool { return r.visited < len(r.index) }

func (r *projectReader) take() conversation.ScanVisit {
	v := r.pending[0]
	r.pending = r.pending[1:]
	r.visited++
	return v
}

func (r *projectReader) decorate(v conversation.ScanVisit) SearchVisit {
	out := SearchVisit{ScanVisit: v, ProjectID: r.project.id, ProjectName: r.project.name}
	for _, s := range r.project.sessions {
		if s.Id == v.ID {
			out.Alias = s.Alias
			out.State = s.State
			if out.Title == "" {
				out.Title = s.Title
			}
			break
		}
	}
	return out
}

// readProject runs one project's helper and forwards what it says. The volume
// is mounted read-only: a search must not be able to change a journal, and the
// project's own container may be using it at the same time.
func (m *Manager) readProject(ctx context.Context, reader *projectReader, q conversation.ScanQuery, messages chan<- readerMessage) {
	send(ctx, messages, readerMessage{reader: reader, opening: true})
	fail := func(err error) { send(ctx, messages, readerMessage{reader: reader, failure: err}) }
	name := "cxz-search-" + core.ID()
	args := []string{"run", "--rm", "-i", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "512m", "--label", "cxz.owner=" + m.Owner, "--label", "cxz.project=" + reader.project.id}
	for _, v := range []memoryMount{{volume: reader.project.volume, project: reader.project.id, target: "/cxz/state"}, {volume: m.ToolsVolume, target: "/cxz/tools"}} {
		b, err := dockerx.Run(ctx, "volume", "inspect", v.volume)
		if err != nil {
			fail(fmt.Errorf("retained storage unavailable: %w", err))
			return
		}
		var volumes []struct{ Labels map[string]string }
		if json.Unmarshal(b, &volumes) != nil || len(volumes) != 1 || volumes[0].Labels["cxz.owner"] != m.Owner || volumes[0].Labels["cxz.project"] != v.project {
			fail(fmt.Errorf("refusing unowned retained storage"))
			return
		}
		args = append(args, "--mount", "type=volume,source="+v.volume+",target="+v.target+",readonly")
	}
	args = append(args, "--entrypoint", "/cxz/tools/cxz", m.Image, "--state", "/cxz/state/data", "_conversation-scan")
	request, err := json.Marshal(conversation.ScanRequest{Project: reader.project.id, Query: q})
	if err != nil {
		fail(err)
		return
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(string(request))
	diagnostics := &boundedBuffer{max: 4 << 10}
	cmd.Stderr = diagnostics
	out, err := cmd.StdoutPipe()
	if err != nil {
		fail(err)
		return
	}
	if err = cmd.Start(); err != nil {
		fail(err)
		return
	}
	decoder := json.NewDecoder(out)
	for {
		var message conversation.ScanMessage
		if err = decoder.Decode(&message); err != nil {
			break
		}
		send(ctx, messages, readerMessage{reader: reader, message: message})
	}
	if err = cmd.Wait(); err != nil && ctx.Err() == nil {
		detail := diagnostics.String()
		if detail == "" {
			detail = err.Error()
		}
		fail(fmt.Errorf("%s", detail))
	}
	if ctx.Err() != nil {
		// A cancelled search leaves no container behind, even though docker run
		// was told to remove it: the signal may arrive before it started.
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = dockerx.Run(stop, "rm", "-f", name)
	}
}

type boundedBuffer struct {
	b   strings.Builder
	max int
}

func (w *boundedBuffer) Write(p []byte) (int, error) {
	if room := w.max - w.b.Len(); room > 0 {
		if len(p) > room {
			p = p[:room]
		}
		w.b.Write(p)
	}
	return len(p), nil
}

func (w *boundedBuffer) String() string { return strings.TrimSpace(w.b.String()) }
