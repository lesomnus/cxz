package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
)

// Searching from the transcript is two interactions wearing one bar.
//
// Inside one conversation the question is "where did I say that", and the answer
// is a place to go: the bar counts the matches and the transcript moves, which is
// what every editor's find does and what the hands already expect. Across a
// project or an installation the question is "which conversation was that in",
// and the answer is a list to choose from -- so the results are shown, with
// enough of the sentence around each to recognise it, and chosen before anything
// moves.
//
// Both ask the same server the same way. What differs is when: typing is a
// question inside one conversation, because the corpus is small and the cost of
// being wrong is a scroll; across everything it is Enter, because a list that
// reshuffles under the arrow keys cannot be chosen from.
const (
	searchDebounce = 300 * time.Millisecond
	// A page of session matches is a list to walk with Enter, not to read, so it
	// can be long. A page of cross-project results is read, so it is not.
	sessionMatchLimit = 500
	resultLimit       = 60
	searchSnippet     = 160
)

type searchScope int

const (
	scopeSession searchScope = iota
	scopeProject
	scopeAll
)

func (s searchScope) String() string {
	switch s {
	case scopeProject:
		return "project"
	case scopeAll:
		return "everything"
	}
	return "session"
}

// searchMatch is one hit, with where it is and enough of it to recognise.
type searchMatch struct {
	session, project string
	label            string // The conversation as a person names it.
	seq              uint64
	at               time.Time
	kind             string
	snippet          string
}

type searchOverlay struct {
	scope   searchScope
	query   string
	typed   string // What the last query was run for, to notice a stale reply.
	cursor  int    // Byte offset of the caret in query.
	matches []searchMatch
	index   int // The match the transcript is on, or the row selected.
	// generation rises with every query, so a reply that arrives after the one
	// that replaced it is dropped rather than shown.
	generation uint64
	running    bool
	message    string
	searched   bool
	pending    int32
	cancel     context.CancelFunc
}

type searchResult struct {
	generation uint64
	scope      searchScope
	query      string
	matches    []searchMatch
	pending    int32
	err        error
}

type searchDebounced struct {
	generation uint64
	query      string
}

// searchAvailable keeps the find bar out of the screens that are not a
// conversation, and out of anything already holding the keyboard for its own
// text: a shortcut that steals a character from a dialog is worse than one that
// is unavailable for a moment.
func (m *model) searchAvailable() bool {
	return m.search == nil && m.workflow == nil && m.report == nil && m.redactDialog == nil &&
		m.pasteDialog == nil && m.restartConfirm == nil && m.modelPicker == nil &&
		m.settingsPage == nil && m.library == nil && m.memoryPage == nil && m.sessionArchive == nil &&
		!m.accountView && !m.creating && !m.renaming && !m.errorFocused() && !m.terminalFocused() &&
		!m.questionFocused() && !m.selectingTools()
}

// openSearch starts a search in the scope the keys asked for. Reopening in the
// same scope keeps the query, because the usual reason to press the key again is
// to look for the same thing further back.
func (m *model) openSearch(scope searchScope) tea.Cmd {
	if m.search == nil {
		m.search = &searchOverlay{}
	}
	p := m.search
	// A host-local installation has no projects to narrow to, so asking for one
	// would label an installation-wide search as something it is not.
	if scope == scopeProject {
		if s := m.current(); s == nil || s.ProjectId == "" {
			scope = scopeAll
		}
	}
	if p.scope != scope {
		p.matches, p.index, p.searched, p.message = nil, 0, false, ""
	}
	p.scope = scope
	p.cursor = len(p.query)
	m.panelFocus = false
	if p.query == "" {
		return nil
	}
	return m.runSearch(p, false)
}

func (m *model) closeSearch() {
	if m.search == nil {
		return
	}
	if m.search.cancel != nil {
		m.search.cancel()
	}
	m.search = nil
}

// searchKey drives the bar. Everything that is not navigation is text, because
// a find bar that swallows characters to use as shortcuts is a find bar that
// cannot search for them.
func (m *model) searchKey(k tea.KeyMsg) tea.Cmd {
	p := m.search
	if p == nil {
		return nil
	}
	if k.Paste {
		return m.searchInsert(p, string(k.Runes))
	}
	switch k.String() {
	case "esc", "ctrl+q":
		m.closeSearch()
		return nil
	case "ctrl+d":
		return tea.Quit
	case "ctrl+f":
		p.scope = scopeSession
		return m.runSearch(p, false)
	case "f18", "ctrl+shift+f":
		return m.widen(p)
	case "tab":
		// Every scope is reachable from the bar, because the chord that opens
		// the wider ones is not something every terminal can send.
		p.scope = (p.scope + 1) % 3
		if p.scope == scopeProject && m.current() == nil {
			p.scope = scopeAll
		}
		p.matches, p.index, p.searched = nil, 0, false
		if p.scope == scopeSession {
			return m.runSearch(p, false)
		}
		return nil
	case "enter":
		if p.scope == scopeSession {
			return m.step(p, 1)
		}
		if !p.searched || p.typed != p.query {
			return m.runSearch(p, true)
		}
		return m.openMatch(p)
	case "shift+tab":
		if p.scope == scopeSession {
			return m.step(p, -1)
		}
	case "up":
		return m.move(p, -1)
	case "down":
		return m.move(p, 1)
	case "pgup":
		return m.move(p, -5)
	case "pgdown":
		return m.move(p, 5)
	case "backspace":
		if p.cursor > 0 {
			_, size := lastRune(p.query[:p.cursor])
			p.query = p.query[:p.cursor-size] + p.query[p.cursor:]
			p.cursor -= size
			return m.afterTyping(p)
		}
		return nil
	case "delete":
		if p.cursor < len(p.query) {
			_, size := firstRune(p.query[p.cursor:])
			p.query = p.query[:p.cursor] + p.query[p.cursor+size:]
			return m.afterTyping(p)
		}
		return nil
	case "left":
		if p.cursor > 0 {
			_, size := lastRune(p.query[:p.cursor])
			p.cursor -= size
		}
		return nil
	case "right":
		if p.cursor < len(p.query) {
			_, size := firstRune(p.query[p.cursor:])
			p.cursor += size
		}
		return nil
	case "home", "ctrl+a":
		p.cursor = 0
		return nil
	case "end", "ctrl+e":
		p.cursor = len(p.query)
		return nil
	case "ctrl+u":
		p.query, p.cursor = "", 0
		return m.afterTyping(p)
	}
	if len(k.Runes) > 0 {
		return m.searchInsert(p, string(k.Runes))
	}
	return nil
}

func (m *model) searchInsert(p *searchOverlay, text string) tea.Cmd {
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, text)
	if len(p.query)+len(text) > 4096 {
		return nil
	}
	p.query = p.query[:p.cursor] + text + p.query[p.cursor:]
	p.cursor += len(text)
	return m.afterTyping(p)
}

// afterTyping searches as the query is typed inside one conversation, and waits
// for Enter across more than one. A list that reshuffles between pressing down
// and pressing enter is a list that cannot be chosen from.
func (m *model) afterTyping(p *searchOverlay) tea.Cmd {
	p.message = ""
	if p.scope != scopeSession {
		p.searched = false
		return nil
	}
	p.generation++
	generation, query := p.generation, p.query
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchDebounced{generation: generation, query: query}
	})
}

func (m *model) widen(p *searchOverlay) tea.Cmd {
	if p.scope == scopeSession && m.current() != nil {
		p.scope = scopeProject
	} else {
		p.scope = scopeAll
	}
	p.matches, p.index, p.searched = nil, 0, false
	return nil
}

// runSearch asks the server. force is Enter in a scope that waits for it.
func (m *model) runSearch(p *searchOverlay, force bool) tea.Cmd {
	if strings.TrimSpace(p.query) == "" {
		p.matches, p.index, p.searched, p.message = nil, 0, false, ""
		return nil
	}
	if p.scope != scopeSession && !force {
		return nil
	}
	session := m.current()
	if p.scope == scopeSession && session == nil {
		p.message = "No conversation is open."
		return nil
	}
	if p.cancel != nil {
		p.cancel()
	}
	p.generation++
	p.running, p.typed, p.searched = true, p.query, true
	request := &api.SearchRequest{
		Query: p.query, IgnoreCase: true, IncludeTools: true,
		Snippet: searchSnippet, Limit: resultLimit,
	}
	switch p.scope {
	case scopeSession:
		request.Sessions = []string{session.Id}
		request.Limit = sessionMatchLimit
		request.Snippet = searchSnippet
	case scopeProject:
		if session.ProjectId != "" {
			request.Projects = []string{session.ProjectId}
		}
	}
	generation, scope, query := p.generation, p.scope, p.query
	ctx, cancel := context.WithCancel(m.ctx)
	p.cancel = cancel
	client := m.client
	return func() tea.Msg {
		defer cancel()
		call, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()
		out := searchResult{generation: generation, scope: scope, query: query}
		stream, err := client.Search(call, request)
		if err != nil {
			out.err = err
			return out
		}
		for {
			reply, err := stream.Recv()
			if err != nil {
				if call.Err() != nil && ctx.Err() == nil {
					out.err = fmt.Errorf("search timed out")
				} else if ctx.Err() == nil && !isStreamEnd(err) {
					out.err = err
				}
				return out
			}
			if v := reply.Summary; v != nil {
				out.pending = v.Pending
				return out
			}
			v := reply.Visit
			if v == nil {
				continue
			}
			label := v.Alias
			if label == "" {
				label = v.Title
			}
			if label == "" {
				label = short(v.SessionId)
			}
			for _, h := range v.Hits {
				out.matches = append(out.matches, searchMatch{
					session: v.SessionId, project: v.ProjectName, label: label,
					seq: h.Seq, at: time.UnixMilli(h.TimeMs), kind: h.Kind, snippet: h.Snippet,
				})
			}
		}
	}
}

func (m *model) receiveSearch(v searchResult) tea.Cmd {
	p := m.search
	if p == nil || v.generation != p.generation {
		return nil
	}
	p.running = false
	p.matches, p.index, p.pending = v.matches, 0, v.pending
	if v.err != nil {
		p.message = v.err.Error()
		return nil
	}
	p.message = ""
	if len(v.matches) == 0 {
		p.message = "No match in this " + v.scope.String() + "."
		return nil
	}
	if v.scope == scopeSession {
		// The newest match, which is where a person looking for what they just
		// said expects to land.
		return m.reveal(p.matches[0])
	}
	return nil
}

func (m *model) receiveSearchDebounce(v searchDebounced) tea.Cmd {
	p := m.search
	if p == nil || p.scope != scopeSession || v.generation != p.generation || v.query != p.query {
		return nil
	}
	return m.runSearch(p, false)
}

// step moves the transcript to the next match within one conversation, wrapping
// because a find bar that stops at the end makes a person retype the query.
func (m *model) step(p *searchOverlay, by int) tea.Cmd {
	if len(p.matches) == 0 {
		if p.typed != p.query {
			return m.runSearch(p, true)
		}
		return nil
	}
	p.index = ((p.index+by)%len(p.matches) + len(p.matches)) % len(p.matches)
	return m.reveal(p.matches[p.index])
}

func (m *model) move(p *searchOverlay, by int) tea.Cmd {
	if len(p.matches) == 0 {
		return nil
	}
	if p.scope == scopeSession {
		return m.step(p, by)
	}
	p.index = max(0, min(len(p.matches)-1, p.index+by))
	return nil
}

// openMatch leaves the bar for the conversation a result is in.
func (m *model) openMatch(p *searchOverlay) tea.Cmd {
	if len(p.matches) == 0 {
		return nil
	}
	hit := p.matches[min(p.index, len(p.matches)-1)]
	m.closeSearch()
	if s := m.current(); s != nil && s.Id == hit.session {
		return m.reveal(hit)
	}
	// Another conversation: the position travels with the selection, and the
	// history that has to be loaded to reach it is loaded on the way, by the
	// same path that restores a session after an update.
	m.autoRestorePosition = float64(hit.seq)
	m.wantID = hit.session
	m.panelFocus = false
	m.projectView = false
	m.accountView = false
	return tea.Batch(m.focusComposer(), m.refresh())
}

// reveal moves the transcript onto a match, loading older history until the
// match is in it. Both halves already exist: a row knows which event it belongs
// to, and a position that is not loaded yet is reached by the same mechanism
// that restores a reading position across a restart.
func (m *model) reveal(hit searchMatch) tea.Cmd {
	s := m.current()
	if s == nil || s.Id != hit.session {
		return nil
	}
	position := float64(hit.seq)
	if len(m.historyPositions) > 0 && m.historyPositions[0] <= position {
		for row, at := range m.historyPositions {
			if at >= position {
				// A third of a screen above the match, so the lines that led to
				// it are visible. Leaving the bottom is what stops following;
				// there is no flag for it, and the window's own detached state
				// means something else.
				m.view.SetYOffset(max(0, row-m.view.Height/3))
				return nil
			}
		}
	}
	m.autoRestorePosition = position
	return m.loadOlderHistory()
}

// searchBar is the overlay: a line of query, a line of state, and -- where the
// answer is a list -- the list.
func (m *model) searchBar(view string) string {
	p := m.search
	if p == nil || m.width < 20 {
		return view
	}
	width := max(8, m.width-4)
	content := []string{focus.Bold(true).Render("Search "+p.scope.String()) + "  " + m.searchField(width-20)}
	content = append(content, muted.Render(clip(m.searchStatus(), width)))
	if p.scope != scopeSession {
		rows := strings.Split(view, "\n")
		capacity := max(1, min(len(rows)-6, 12))
		content = append(content, m.searchRows(width, capacity)...)
	}
	return overlayTop(view, content, m.width, true)
}

func (m *model) searchField(width int) string {
	p := m.search
	text := p.query
	if text == "" {
		return muted.Render("type to search")
	}
	// The caret is drawn rather than placed, because the cursor belongs to the
	// composer and taking it would make the transcript look dead.
	at := min(p.cursor, len(text))
	out := text[:at] + focus.Render("▎") + text[at:]
	return clip(safeText(out), max(8, width))
}

func (m *model) searchStatus() string {
	p := m.search
	if p.message != "" {
		return p.message
	}
	if p.running {
		return "searching…"
	}
	parts := []string{}
	if len(p.matches) > 0 {
		if p.scope == scopeSession {
			parts = append(parts, fmt.Sprintf("%d/%d", p.index+1, len(p.matches)))
		} else {
			parts = append(parts, fmt.Sprintf("%d results", len(p.matches)))
		}
	} else if p.scope != scopeSession && !p.searched {
		parts = append(parts, "Enter to search")
	}
	if p.pending > 0 {
		parts = append(parts, fmt.Sprintf("%d not yet indexed", p.pending))
	}
	if p.scope == scopeSession {
		parts = append(parts, "Enter next · Shift+Tab previous")
	} else {
		parts = append(parts, "↑↓ choose · Enter open")
	}
	parts = append(parts, "Tab scope · Esc close")
	return strings.Join(parts, " · ")
}

// searchRows lists results around the selection, so that walking past the
// bottom of the panel keeps the choice visible.
func (m *model) searchRows(width, capacity int) []string {
	p := m.search
	if len(p.matches) == 0 {
		return nil
	}
	start := max(0, min(p.index-capacity/2, len(p.matches)-capacity))
	end := min(len(p.matches), start+capacity)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		hit := p.matches[i]
		head := hit.at.Local().Format("01-02 15:04") + " " + hit.label
		if hit.project != "" && p.scope == scopeAll {
			head += " · " + hit.project
		}
		line := head + "  " + hit.snippet
		if i == p.index {
			out = append(out, focus.Render("▸ ")+clip(safeText(line), max(4, width-2)))
			continue
		}
		out = append(out, "  "+muted.Render(clip(safeText(line), max(4, width-2))))
	}
	return out
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func firstRune(s string) (rune, int) {
	for i, r := range s {
		_ = i
		return r, len(string(r))
	}
	return 0, 0
}

func lastRune(s string) (rune, int) {
	last := rune(0)
	size := 0
	for _, r := range s {
		last, size = r, len(string(r))
	}
	return last, size
}

func isStreamEnd(err error) bool {
	return err != nil && (err == context.Canceled || strings.Contains(err.Error(), "EOF"))
}
