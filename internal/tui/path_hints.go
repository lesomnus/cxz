package tui

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pathToken struct {
	start, end          int
	text, parent, query string
	host                bool
	quote               string
}
type pathHints struct {
	key, signature   string
	generation       uint64
	token            pathToken
	listing          containerterm.PathListing
	loading          bool
	err              error
	selected, offset int
	cancel           context.CancelFunc
	order            []pathOption
}
type pathHintDue struct{ generation uint64 }
type pathHintResult struct {
	generation uint64
	listing    containerterm.PathListing
	err        error
	partial    bool
}
type pathOption struct {
	text  string
	entry containerterm.PathEntry
}

// Completion is explicitly delimited by backticks. Bare slash commands and
// URLs are never interpreted as filesystem requests.
func pathTokenAt(value string, cursor int) (pathToken, bool) {
	r := []rune(value)
	cursor = min(max(0, cursor), len(r))
	for _, token := range backtickTokens(value) {
		if cursor < token.start || cursor > token.end {
			continue
		}
		text := string(r[token.start:cursor])
		for i := cursor; i < token.end; i++ {
			if r[i] == '\n' || r[i] == '\r' {
				token.end = i
				break
			}
		}
		if strings.HasPrefix(text, "!") {
			parent, query, quote, ok := hostHintPath(text[1:])
			return pathToken{start: token.start, end: token.end, text: text, parent: parent, query: query, host: true, quote: quote}, ok
		}
		if text == "" || strings.ContainsAny(text, "\n\r") || !(strings.HasPrefix(text, "/") || text == "~" || strings.HasPrefix(text, "~/")) {
			return pathToken{}, false
		}
		parent, query := "~/", ""
		if text != "~" {
			slash := strings.LastIndex(text, "/")
			parent, query = text[:slash+1], text[slash+1:]
		}
		return pathToken{start: token.start, end: token.end, text: text, parent: parent, query: query}, true
	}
	return pathToken{}, false
}

func (m *model) pathContext() (pathToken, string, bool) {
	if m.errorFocused() || m.memoryPage != nil || m.settingsPage != nil || m.program == nil || m.ctx == nil || m.ctx.Err() != nil || m.view.Height < 4 || m.terminalFocused() || m.panelFocus || m.questionDialog != nil || m.pasteDialog != nil {
		return pathToken{}, "", false
	}
	value, pos, _, _, ok := m.chipInput()
	if !ok {
		return pathToken{}, "", false
	}
	s := m.current()
	if s == nil || s.ProjectId == "" {
		return pathToken{}, "", false
	}
	token, ok := pathTokenAt(value, pos)
	if !ok {
		return token, "", false
	}
	return token, s.Id + "/" + s.RunId + "/" + s.ProjectId, true
}
func pathSignature(scope string, t pathToken) string {
	return fmt.Sprintf("%s:%d:%d:%s", scope, t.start, t.end, t.text)
}
func (m *model) clearPathHints() {
	if m.pathHints != nil && m.pathHints.cancel != nil {
		m.pathHints.cancel()
	}
	m.pathHints = nil
}
func (m *model) syncPathHints() tea.Cmd {
	token, scope, ok := m.pathContext()
	if !ok {
		m.clearPathHints()
		m.pathHintDismissed = ""
		return nil
	}
	signature := pathSignature(scope, token)
	if signature == m.pathHintDismissed {
		m.clearPathHints()
		return nil
	}
	key := fmt.Sprintf("%s:%t:%s", scope, token.host, token.parent)
	if p := m.pathHints; p != nil && p.key == key {
		if p.signature != signature {
			p.selected, p.offset = 0, 0
			p.order = nil
		}
		p.token, p.signature = token, signature
		return nil
	}
	m.clearPathHints()
	m.pathHintGeneration++
	p := &pathHints{key: key, signature: signature, token: token, generation: m.pathHintGeneration, loading: true}
	m.pathHints = p
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return pathHintDue{p.generation} })
}
func (m *model) fetchPathHints(d pathHintDue) tea.Cmd {
	p := m.pathHints
	if p == nil || p.generation != d.generation {
		return nil
	}
	s := m.current()
	if s == nil {
		return nil
	}
	if p.token.host {
		parent, generation := p.token.parent, p.generation
		ctx, cancel := context.WithTimeout(m.ctx, 4*time.Second)
		p.cancel = cancel
		return func() tea.Msg {
			defer cancel()
			listing, err := readHostPaths(ctx, parent)
			return pathHintResult{generation: generation, listing: listing, err: err}
		}
	}
	projectID, parent, generation := s.ProjectId, p.token.parent, p.generation
	lifetime := m.contextFor(projectID)
	remote := transport.IsRemote(lifetime)
	client, _ := m.client.(containerterm.PathClient)
	var target *api.Project
	for _, candidate := range append([]*api.Project{m.project}, m.panelProjects...) {
		if candidate != nil && candidate.Id == projectID && candidate.ContainerId != "" && candidate.RemoteUser != "" {
			target = &api.Project{Id: candidate.Id, ContainerId: candidate.ContainerId, RemoteUser: candidate.RemoteUser}
			break
		}
	}
	if !remote && m.wisp == nil {
		m.wisp = &containerterm.WispPool{}
	}
	pool := m.wisp
	target = m.localProject(target)
	program := m.program
	ctx, cancel := context.WithTimeout(lifetime, 4*time.Second)
	p.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		emit := func(listing containerterm.PathListing) {
			if ctx.Err() == nil && program != nil {
				program.Send(pathHintResult{generation: generation, listing: listing, partial: true})
			}
		}
		var listing containerterm.PathListing
		var err error
		if remote {
			if client == nil {
				err = status.Error(codes.Unimplemented, "remote container path browsing unavailable")
			} else {
				listing, err = client.Paths(ctx, projectID, parent, emit)
			}
		} else if target == nil {
			err = fmt.Errorf("project metadata unavailable; refresh project view")
		} else {
			listing, err = pool.Paths(lifetime, ctx, target, parent, emit)
		}
		return pathHintResult{generation: generation, listing: listing, err: err}
	}
}
func (m *model) receivePathHints(r pathHintResult) {
	if p := m.pathHints; p != nil && p.generation == r.generation {
		old := m.pathOptions()
		p.listing, p.err, p.loading = r.listing, r.err, r.partial
		p.order = nil
		fresh := m.pathOptions()
		// Keep the selected entry and its successor in place as batches arrive.
		keep := old[:min(len(old), p.selected+2)]
		seen := make(map[string]bool)
		p.order = append([]pathOption(nil), keep...)
		for _, o := range keep {
			seen[o.text] = true
		}
		for _, o := range fresh {
			if !seen[o.text] {
				p.order = append(p.order, o)
			}
		}
	}
}

func (m *model) pathOptions() []pathOption {
	p := m.pathHints
	if p == nil {
		return nil
	}
	if p.order != nil {
		return p.order
	}
	var paths []pathOption
	for _, entry := range p.listing.Entries {
		if fuzzyScore(entry.Name, p.token.query) < 0 {
			continue
		}
		name := entry.Name
		if entry.Directory {
			separator := "/"
			if p.token.host && runtime.GOOS == "windows" && strings.Contains(p.token.parent, `\`) {
				separator = `\`
			}
			name += separator
		}
		paths = append(paths, pathOption{text: p.token.parent + name, entry: entry})
	}
	sort.SliceStable(paths, func(i, j int) bool {
		a, b := paths[i], paths[j]
		x, y := fuzzyScore(a.entry.Name, p.token.query), fuzzyScore(b.entry.Name, p.token.query)
		if x != y {
			return x < y
		}
		if a.entry.Directory != b.entry.Directory {
			return a.entry.Directory
		}
		return a.text < b.text
	})
	return paths
}
func (m *model) pathHintKey(k tea.KeyMsg) (bool, tea.Cmd) {
	p := m.pathHints
	if p == nil {
		return false, nil
	}
	options := m.pathOptions()
	p.selected = max(0, min(p.selected, len(options)-1))
	switch k.String() {
	case "esc":
		m.pathHintDismissed = p.signature
		m.hintDismissed = true
		m.clearPathHints()
		return true, nil
	case "up":
		if len(options) > 0 {
			p.selected = (p.selected + len(options) - 1) % len(options)
		}
		return true, nil
	case "down":
		if len(options) > 0 {
			p.selected = (p.selected + 1) % len(options)
		}
		return true, nil
	case "tab":
		if len(options) > 0 {
			m.applyPathOption(options[p.selected])
		}
		return true, nil
	case "enter":
		if len(options) > 0 {
			m.completePathOption(options[p.selected], true)
		}
		return true, nil
	}
	return false, nil
}

func (m *model) applyPathOption(option pathOption) {
	m.completePathOption(option, false)
}
func (m *model) completePathOption(option pathOption, closeQuote bool) {
	p := m.pathHints
	if p == nil {
		return
	}
	r := []rune(m.input.Value())
	token := p.token
	if token.start > len(r) || token.end > len(r) {
		return
	}
	replacement := option.text
	if strings.ContainsRune(replacement, '`') {
		m.notice = "Path contains a backtick; type it explicitly"
		return
	}
	if token.host {
		replacement = "!" + token.quote + replacement
	}
	if token.host && option.entry.Directory {
		closeQuote = false
	}
	if closeQuote {
		if token.host {
			replacement += token.quote
		}
		replacement += "`"
		if token.end < len(r) && r[token.end] == '`' {
			token.end++
		}
	}
	cursor := len([]rune(replacement))
	value := string(r[:token.start]) + replacement + string(r[token.end:])
	pos := token.start + cursor
	m.setPathInput(value, pos)
}

func (m *model) setPathInput(value string, pos int) {
	m.input.SetValue(value)
	prefix := []rune(value)[:pos]
	row := strings.Count(string(prefix), "\n")
	// Compute columns as runes, not byte indexes (including Korean prose).
	parts := strings.Split(string(prefix), "\n")
	column := len([]rune(parts[len(parts)-1]))
	for m.input.Line() > row {
		m.input.CursorStart()
		m.input.CursorUp()
	}
	m.input.SetCursor(column)
	m.resize()
}

func (m *model) pathHintOverlay(view string) string {
	p := m.pathHints
	if p == nil {
		return view
	}
	options := m.pathOptions()
	capacity := min(7, max(0, len(strings.Split(view, "\n"))-3))
	if capacity == 0 {
		return view
	}
	content := []string{muted.Render("Container paths · ↑/↓ choose · Tab browse · Enter finish · Esc close")}
	if p.token.host {
		content[0] = muted.Render("Host files (cxz client) · ↑/↓ choose · Tab browse · Enter attach · Esc close")
	}
	if p.listing.Truncated {
		scope := "Container paths"
		if p.token.host {
			scope = "Host files (cxz client)"
		}
		content[0] = warning.Render(scope + " · First 2048 entries · Tab browse · Esc close")
	}
	if len(options) > 0 {
		p.selected = max(0, min(p.selected, len(options)-1))
		count := min(capacity, len(options))
		margin := min(2, (count-1)/2)
		p.offset = max(0, min(min(p.offset, p.selected-margin), len(options)-count))
		p.offset = max(p.offset, p.selected+margin-count+1)
		p.offset = min(p.offset, len(options)-count)
		for idx := p.offset; idx < p.offset+count; idx++ {
			o := options[idx]
			name := safeText(strings.TrimPrefix(o.text, p.token.parent))
			marker := "  "
			if idx == p.selected {
				marker = accent.Render("› ")
			}
			style := lipgloss.NewStyle()
			if o.entry.Directory {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#8DAFFF"))
			}
			if o.entry.Executable {
				style = accent
			}
			if o.entry.Symlink {
				style = blue
			}
			text := marker + highlightPath(name, p.token.query, style)
			if o.entry.Symlink {
				text += blue.Render(" → " + safeText(o.entry.LinkTarget))
			}
			content = append(content, text)
		}
	} else if p.loading {
		content = append(content, muted.Render("Loading directory…"))
	} else if p.err != nil {
		message := "Directory unavailable · check container and permissions"
		if strings.Contains(p.err.Error(), "wisp unavailable") || strings.Contains(p.err.Error(), "wisp disconnected") {
			message = "Wisp unavailable · update project runtime or reopen path hints"
		}
		if strings.Contains(p.err.Error(), "metadata unavailable") {
			message = "Project metadata unavailable · refresh project view"
		}
		switch status.Code(p.err) {
		case codes.Unimplemented:
			message = "Update remote manager · cxz install --recreate"
		case codes.Unavailable:
			message = "Remote connection unavailable · reconnect and retry"
		case codes.DeadlineExceeded:
			message = "Directory lookup timed out · reopen path hints to retry"
		}
		if p.token.host {
			message = "Host directory unavailable · check client path and permissions"
		}
		content = append(content, warning.Render(message))
	} else {
		content = append(content, muted.Render("No matching entries"))
	}
	return overlayBox(view, content, m.width, true)
}

func highlightPath(name, query string, style lipgloss.Style) string {
	q := []rune(strings.ToLower(query))
	var out strings.Builder
	i := 0
	for _, r := range name {
		if i < len(q) && unicode.ToLower(r) == q[i] {
			out.WriteString(magenta.Render(string(r)))
			i++
		} else {
			out.WriteString(style.Render(string(r)))
		}
	}
	return out.String()
}

// Preserve path separators as word boundaries without changing prose deletion.
func (m *model) deletePathWord(k tea.KeyMsg) bool {
	if k.String() != "ctrl+w" && k.String() != "alt+backspace" {
		return false
	}
	value, pos, _, _, ok := m.chipInput()
	if !ok {
		return false
	}
	token, ok := pathTokenAt(value, pos)
	if !ok || pos <= token.start {
		return false
	}
	boundary := token.start
	if token.host {
		boundary++
	}
	if pos <= boundary {
		return false
	}
	separators := "/"
	if token.host && runtime.GOOS == "windows" {
		separators += `\`
	}
	r := []rune(value)
	start := pos
	if strings.ContainsRune(separators, r[start-1]) {
		start--
	}
	for start > boundary && !strings.ContainsRune(separators, r[start-1]) && !unicode.IsSpace(r[start-1]) {
		start--
	}
	if start == pos {
		start--
	}
	m.setPathInput(string(r[:start])+string(r[pos:]), start)
	return true
}
