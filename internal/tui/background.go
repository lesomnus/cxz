package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

type backgroundHistory struct {
	id       string
	epoch    uint64
	snapshot backgroundSnapshot
	err      error
}

type backgroundSnapshot struct {
	lastSeq uint64
	states  map[string]*agentview.BackgroundState
}

// One reduced snapshot replaces paging through the entire transcript. Older
// servers report unavailable telemetry instead of silently starting a full replay.
func (m *model) loadBackgroundHistory(p historyPage) tea.Cmd {
	if !p.initial || p.start == 0 || p.err != nil || (p.epoch != 0 && p.epoch != m.watchEpoch) {
		return nil
	}
	parent := m.watchContext
	if parent == nil {
		parent = m.ctx
	}
	return m.fetchBackground(parent, p.id, p.epoch)
}

func (m *model) fetchBackground(parent context.Context, id string, epoch uint64) tea.Cmd {
	client, recorder := m.client, m.debugRecorder
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()
		out := backgroundHistory{id: id, epoch: epoch}
		started := time.Now()
		reply, err := client.Background(ctx, &api.SessionRef{Id: id})
		recorder.Add(debugEvent{Kind: "history_rpc", Type: "background", Duration: time.Since(started).Microseconds(), Bytes: proto.Size(reply), ErrorCode: status.Code(err).String()})
		if status.Code(err) == codes.Unimplemented {
			err = fmt.Errorf("background snapshot unavailable: update the manager and project runtime")
		}
		out.err = err
		if err == nil && reply != nil {
			out.snapshot.lastSeq = reply.LastSeq
			out.err = json.Unmarshal(reply.Data, &out.snapshot.states)
		}
		return out
	}
}

func (m *model) backgroundStates() map[string]*agentview.BackgroundState {
	s := m.current()
	if s == nil {
		return nil
	}
	return m.backgroundStatesFor(s.Id)
}

type backgroundStateKey struct {
	snapshotSeq uint64
	count       int
	first, last *api.Event
}

type backgroundStateCache struct {
	key    backgroundStateKey
	states map[string]*agentview.BackgroundState
}

func (m *model) storeBackgroundSnapshot(id string, snapshot backgroundSnapshot) {
	if old, ok := m.backgroundSnapshots[id]; ok && old.lastSeq > snapshot.lastSeq {
		return
	}
	if m.backgroundSnapshots == nil {
		m.backgroundSnapshots = map[string]backgroundSnapshot{}
	}
	m.backgroundSnapshots[id] = snapshot
	delete(m.backgroundStateCache, id)
}

// Spinner frames reuse the reduced state. Only new journal entries, older pages,
// or a newer server snapshot require replaying provider background telemetry.
func (m *model) backgroundStatesFor(id string) map[string]*agentview.BackgroundState {
	snapshot := m.backgroundSnapshots[id]
	history := m.events[id]
	key := backgroundStateKey{snapshotSeq: snapshot.lastSeq, count: len(history)}
	if len(history) > 0 {
		key.first, key.last = history[0], history[len(history)-1]
	}
	if cached, ok := m.backgroundStateCache[id]; ok && cached.key == key {
		return cached.states
	}
	out := map[string]*agentview.BackgroundState{}
	for run, state := range snapshot.states {
		if state == nil {
			continue
		}
		copy := &agentview.BackgroundState{Snapshot: state.Snapshot, Tasks: map[string]agentview.BackgroundTask{}}
		for id, task := range state.Tasks {
			copy.Tasks[id] = task
		}
		out[run] = copy
	}
	var events []*api.Event
	for _, e := range history {
		if e.Kind == "background" && (e.Seq == 0 || e.Seq > snapshot.lastSeq) {
			events = append(events, e)
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	seen := map[uint64]bool{}
	for _, e := range events {
		if e.Seq != 0 && seen[e.Seq] {
			continue
		}
		seen[e.Seq] = true
		if out[e.RunId] == nil {
			out[e.RunId] = &agentview.BackgroundState{}
		}
		out[e.RunId].Apply(e.Payload)
	}
	if m.backgroundStateCache == nil {
		m.backgroundStateCache = map[string]backgroundStateCache{}
	}
	m.backgroundStateCache[id] = backgroundStateCache{key: key, states: out}
	return out
}

func (m *model) hasActiveBackground(s *api.Session) bool {
	if s.State != "idle" && !workingState(s.State) {
		return false
	}
	state := m.backgroundStatesFor(s.Id)[s.RunId]
	if state != nil {
		for _, task := range state.Tasks {
			if task.Active {
				return true
			}
		}
	}
	return false
}

func (m *model) backgroundStatus() string {
	s := m.current()
	if s == nil {
		return ""
	}
	state := m.backgroundStates()[s.RunId]
	if state == nil {
		if m.backgroundLoading[s.Id] {
			return muted.Render("background · loading state · /background")
		}
		return ""
	}
	n := 0
	for _, t := range state.Tasks {
		if t.Active {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return accent.Render(fmt.Sprintf("%s background %d · /background", workingSpinner(m.pulse), n))
}

func (m *model) backgroundReport() string {
	s := m.current()
	if s == nil {
		return "No session."
	}
	state := m.backgroundStates()[s.RunId]
	lines := []string{"Provider-reported background tasks · current run", ""}
	if state == nil || len(state.Tasks) == 0 {
		lines = append(lines, "No background task telemetry recorded for this run.")
	} else {
		ids := make([]string, 0, len(state.Tasks))
		for id := range state.Tasks {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			t := state.Tasks[id]
			status := t.Status
			if !t.Active && (status == "working" || status == "running") {
				status = "awaiting final status"
			}
			lines = append(lines, fmt.Sprintf("%s · %s · %s", id, status, t.Description))
			if t.Summary != "" {
				lines = append(lines, "  "+t.Summary)
			}
			if t.OutputFile != "" {
				lines = append(lines, "  Output: "+t.OutputFile)
			}
		}
	}
	if m.backgroundLoading[s.Id] {
		lines = append(lines, "", "Loading background task state…")
	}
	if err := m.backgroundErrors[s.Id]; err != "" {
		lines = append(lines, "", "Background state unavailable: "+err)
	}
	return strings.Join(append(lines, "", "Output paths are metadata only; files are not fetched. Raw events remain in the journal."), "\n")
}
