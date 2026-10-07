package tui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// searchClient answers a search the way the manager does: a visit per
// conversation, newest first, then a summary.
type searchClient struct {
	recordingClient
	requests []*api.SearchRequest
	visits   []*api.SearchVisit
	pending  int32
	err      error
}

func (c *searchClient) List(context.Context, *api.Empty, ...grpc.CallOption) (*api.SessionList, error) {
	return &api.SessionList{Sessions: []*api.Session{{Id: "s", ProjectId: "p", RunId: "run", Agent: "claude", State: "idle"}}}, nil
}

func (c *searchClient) Projects(context.Context, *api.Empty, ...grpc.CallOption) (*api.ProjectList, error) {
	return &api.ProjectList{Projects: []*api.Project{{Id: "p", Name: "project"}}}, nil
}

func (c *searchClient) Search(_ context.Context, r *api.SearchRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[api.SearchReply], error) {
	c.requests = append(c.requests, r)
	if c.err != nil {
		return nil, c.err
	}
	replies := make([]*api.SearchReply, 0, len(c.visits)+1)
	for _, v := range c.visits {
		replies = append(replies, &api.SearchReply{Visit: v})
	}
	replies = append(replies, &api.SearchReply{Summary: &api.SearchSummary{Hits: int32(len(c.visits)), Pending: c.pending}})
	return &searchStreamStub{replies: replies}, nil
}

type searchStreamStub struct {
	grpc.ClientStream
	replies []*api.SearchReply
}

func (s *searchStreamStub) Recv() (*api.SearchReply, error) {
	if len(s.replies) == 0 {
		return nil, io.EOF
	}
	out := s.replies[0]
	s.replies = s.replies[1:]
	return out, nil
}
func (s *searchStreamStub) Context() context.Context     { return context.Background() }
func (s *searchStreamStub) CloseSend() error             { return nil }
func (s *searchStreamStub) Header() (metadata.MD, error) { return nil, nil }
func (s *searchStreamStub) Trailer() metadata.MD         { return nil }
func (s *searchStreamStub) SendMsg(any) error            { return nil }
func (s *searchStreamStub) RecvMsg(any) error            { return nil }

func visit(session, label string, seqs ...uint64) *api.SearchVisit {
	out := &api.SearchVisit{SessionId: session, Alias: label, ProjectName: "project", Agent: "claude"}
	for i, seq := range seqs {
		out.Hits = append(out.Hits, &api.SearchHit{
			Seq: seq, TimeMs: time.Now().Add(-time.Duration(i) * time.Minute).UnixMilli(),
			Kind: "input", Snippet: "the relay refused the certificate",
		})
	}
	return out
}

func searchModel(t *testing.T, client *searchClient) *model {
	t.Helper()
	m := conversationModel()
	m.ctx = context.Background()
	m.client = client
	return m
}

func press(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	case "f18":
		return tea.KeyMsg{Type: tea.KeyF18}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// typing feeds the bar and fires the debounce afterwards, which is the order a
// clock delivers them in: the ticks for characters that were typed past are
// still delivered, and dropped by the generation they were taken at.
func typing(t *testing.T, m *model, text string) {
	t.Helper()
	var ticks []tea.Msg
	for _, r := range text {
		_, cmd := m.Update(press(string(r)))
		if cmd != nil {
			if tick := cmd(); tick != nil {
				ticks = append(ticks, tick)
			}
		}
	}
	for _, tick := range ticks {
		run(t, m, tick)
	}
}

func run(t *testing.T, m *model, msg tea.Msg) {
	t.Helper()
	for i := 0; i < 8; i++ {
		_, cmd := m.Update(msg)
		if cmd == nil {
			return
		}
		next := cmd()
		if next == nil {
			return
		}
		// A tick would otherwise wait out its own delay in the test.
		if debounce, ok := next.(searchDebounced); ok {
			msg = debounce
			continue
		}
		msg = next
	}
}

// Ctrl+F searches the conversation that is open, as it does everywhere else.
func TestSessionSearchJumpsToTheNewestMatch(t *testing.T) {
	client := &searchClient{visits: []*api.SearchVisit{visit("s", "seal", 90, 40, 12)}}
	m := searchModel(t, client)
	run(t, m, press("ctrl+f"))
	if m.search == nil || m.search.scope != scopeSession {
		t.Fatal("Ctrl+F did not open a session search")
	}
	typing(t, m, "relay")
	if len(client.requests) != 1 {
		t.Fatalf("%d searches for one word typed", len(client.requests))
	}
	r := client.requests[0]
	if r.Query != "relay" || len(r.Sessions) != 1 || r.Sessions[0] != "s" || !r.IgnoreCase {
		t.Fatalf("%+v", r)
	}
	if len(m.search.matches) != 3 || m.search.index != 0 {
		t.Fatalf("%+v", m.search)
	}
	// The newest match, reached by the same path that restores a reading
	// position: the history it needs is loaded on the way.
	if m.autoRestorePosition != 90 {
		t.Fatal("the transcript was not sent to the newest match:", m.autoRestorePosition)
	}
	if !strings.Contains(ansi.Strip(m.searchStatus()), "1/3") {
		t.Fatal(m.searchStatus())
	}
}

// Enter walks the matches, and wraps rather than making a person retype.
func TestSessionSearchStepsAndWraps(t *testing.T) {
	client := &searchClient{visits: []*api.SearchVisit{visit("s", "seal", 90, 40, 12)}}
	m := searchModel(t, client)
	run(t, m, press("ctrl+f"))
	typing(t, m, "relay")
	for _, want := range []float64{40, 12, 90} {
		run(t, m, press("enter"))
		if m.autoRestorePosition != want {
			t.Fatalf("expected the match at %v, got %v", want, m.autoRestorePosition)
		}
	}
	run(t, m, press("shift+tab"))
	if m.autoRestorePosition != 12 {
		t.Fatal("Shift+Tab did not step backwards:", m.autoRestorePosition)
	}
}

// A match already on screen is scrolled to rather than reloaded.
func TestSessionSearchScrollsWithoutReloading(t *testing.T) {
	client := &searchClient{visits: []*api.SearchVisit{visit("s", "seal", 7)}}
	m := searchModel(t, client)
	m.historyPositions = []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	m.view.Height = 6
	m.view.SetContent(strings.Repeat("transcript\n", 40))
	run(t, m, press("ctrl+f"))
	typing(t, m, "relay")
	if m.autoRestorePosition != 0 {
		t.Fatal("a loaded match asked for history anyway")
	}
	if m.view.YOffset != 4 {
		t.Fatal("the match is not on screen with room above it:", m.view.YOffset)
	}
}

// A wider search waits for Enter: a list that reshuffles under the arrow keys
// cannot be chosen from.
func TestProjectSearchWaitsForEnterThenOpens(t *testing.T) {
	client := &searchClient{visits: []*api.SearchVisit{
		visit("other", "lab", 31),
		visit("s", "seal", 12),
	}}
	m := searchModel(t, client)
	run(t, m, press("f18"))
	if m.search == nil || m.search.scope != scopeProject {
		t.Fatal("Ctrl+Shift+F did not widen to the project")
	}
	typing(t, m, "relay")
	if len(client.requests) != 0 {
		t.Fatal("typing searched a project")
	}
	run(t, m, press("enter"))
	if len(client.requests) != 1 || len(client.requests[0].Projects) != 1 || client.requests[0].Projects[0] != "p" {
		t.Fatalf("%+v", client.requests)
	}
	if len(m.search.matches) != 2 || m.search.index != 0 {
		t.Fatalf("%+v", m.search.matches)
	}
	// Nothing moved yet: the transcript waits for a choice.
	if m.autoRestorePosition != 0 || m.wantID != "" {
		t.Fatal("a result list moved the transcript")
	}
	view := ansi.Strip(m.searchBar(strings.Repeat("transcript\n", 20)))
	if !strings.Contains(view, "lab") || !strings.Contains(view, "the relay refused") {
		t.Fatal("results are not listed with their text:\n" + view)
	}
	run(t, m, press("down"))
	if m.search.index != 1 {
		t.Fatal("the arrow keys do not move the selection")
	}
	run(t, m, press("enter"))
	if m.search != nil {
		t.Fatal("choosing a result left the bar open")
	}
	// The chosen conversation is this one, so it is revealed in place.
	if m.autoRestorePosition != 12 {
		t.Fatal("the chosen match was not revealed:", m.autoRestorePosition)
	}
}

// A result in another conversation travels with its position.
func TestSearchOpensAnotherConversation(t *testing.T) {
	client := &searchClient{visits: []*api.SearchVisit{visit("elsewhere", "lab", 31)}}
	m := searchModel(t, client)
	run(t, m, press("f18"))
	typing(t, m, "relay")
	run(t, m, press("enter"))
	// Opening is the assertion; the refresh it also asks for belongs to the
	// session that is being switched to.
	m.Update(press("enter"))
	if m.wantID != "elsewhere" || m.autoRestorePosition != 31 {
		t.Fatalf("wantID=%q position=%v", m.wantID, m.autoRestorePosition)
	}
	if m.panelFocus || m.projectView {
		t.Fatal("opening a result left the panel in the way")
	}
}

// With the panel holding the keyboard there is no conversation to search, so
// both keys ask the whole installation.
func TestPanelSearchesEverything(t *testing.T) {
	for _, pressed := range []string{"ctrl+f", "f18"} {
		client := &searchClient{}
		m := searchModel(t, client)
		m.panelFocus = true
		run(t, m, press(pressed))
		if m.search == nil || m.search.scope != scopeAll {
			t.Fatalf("%s from the panel did not search everything", pressed)
		}
		if m.panelFocus {
			t.Fatal("the bar did not take the keyboard from the panel")
		}
		typing(t, m, "relay")
		run(t, m, press("enter"))
		if len(client.requests) != 1 || len(client.requests[0].Projects) != 0 || len(client.requests[0].Sessions) != 0 {
			t.Fatalf("%+v", client.requests)
		}
	}
}

// Tab reaches every scope, because the chord that opens the wider ones is not
// something every terminal can send.
func TestTabCyclesScope(t *testing.T) {
	m := searchModel(t, &searchClient{})
	run(t, m, press("ctrl+f"))
	for _, want := range []searchScope{scopeProject, scopeAll, scopeSession} {
		run(t, m, press("tab"))
		if m.search.scope != want {
			t.Fatalf("expected %v, got %v", want, m.search.scope)
		}
	}
}

// Typing is debounced, and a reply for a query that has been typed past is not
// shown: both are the same question of whether the answer is still the one that
// was asked for.
func TestTypingIsDebouncedAndStaleRepliesAreDropped(t *testing.T) {
	client := &searchClient{visits: []*api.SearchVisit{visit("s", "seal", 5)}}
	m := searchModel(t, client)
	run(t, m, press("ctrl+f"))
	// Three characters typed before any tick is delivered: one search, for the
	// whole word, because the two older ticks were taken at a generation the
	// bar has moved past.
	typing(t, m, "rel")
	if len(client.requests) != 1 || client.requests[0].Query != "rel" {
		t.Fatalf("%+v", client.requests)
	}
	// A reply from a query that has since been replaced is dropped.
	m.search.generation++
	m.Update(searchResult{generation: m.search.generation - 1, matches: []searchMatch{{seq: 999}}})
	if len(m.search.matches) == 1 && m.search.matches[0].seq == 999 {
		t.Fatal("a stale reply was shown")
	}
}

// What the index could not reach is said in the bar, because an answer from a
// store that is behind is incomplete.
func TestSearchReportsWhatIsNotIndexed(t *testing.T) {
	client := &searchClient{visits: []*api.SearchVisit{visit("s", "seal", 5)}, pending: 2}
	m := searchModel(t, client)
	run(t, m, press("ctrl+f"))
	typing(t, m, "relay")
	if !strings.Contains(m.searchStatus(), "2 not yet indexed") {
		t.Fatal(m.searchStatus())
	}
	// And so is an empty result, which is otherwise indistinguishable from a
	// search that did not run.
	client.visits = nil
	run(t, m, press("enter"))
	typing(t, m, "x")
	if !strings.Contains(m.searchStatus(), "No match") {
		t.Fatal(m.searchStatus())
	}
}

// The bar is above the transcript it is searching, and Esc gives it back.
func TestSearchBarSitsAtTheTopAndCloses(t *testing.T) {
	m := searchModel(t, &searchClient{})
	run(t, m, press("ctrl+f"))
	typing(t, m, "relay")
	rows := strings.Split(ansi.Strip(m.searchBar(strings.Repeat("transcript\n", 20))), "\n")
	if len(rows) < 4 || !strings.Contains(rows[1], "Search session") || !strings.Contains(rows[1], "relay") {
		t.Fatal("the bar is not at the top:\n" + strings.Join(rows[:4], "\n"))
	}
	if !strings.Contains(rows[len(rows)-2], "transcript") {
		t.Fatal("the bar displaced the transcript instead of overlaying it")
	}
	run(t, m, press("esc"))
	if m.search != nil {
		t.Fatal("Esc did not close the bar")
	}
}

// A search is not opened over something that is already taking text.
func TestSearchStaysOutOfTheWayOfDialogs(t *testing.T) {
	m := searchModel(t, &searchClient{})
	m.report = &reportOverlay{title: "/help"}
	run(t, m, press("ctrl+f"))
	if m.search != nil {
		t.Fatal("the find bar opened over a dialog")
	}
}
