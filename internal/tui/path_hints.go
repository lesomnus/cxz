package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/containerterm"
)

type pathToken struct {
	start, end          int
	text, parent, query string
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
}
type pathHintDue struct{ generation uint64 }
type pathHintResult struct {
	generation uint64
	listing    containerterm.PathListing
	err        error
}
type pathOption struct {
	text, description string
}

// Completion is explicitly delimited by backticks. Bare slash commands and
// URLs are never interpreted as filesystem requests.
func pathTokenAt(value string, cursor int) (pathToken, bool) {
	r := []rune(value)
	cursor = min(max(0, cursor), len(r))
	opening := -1
	for i := 0; i < cursor; i++ {
		if r[i] == '`' {
			if opening < 0 {
				opening = i
			} else {
				opening = -1
			}
		}
	}
	if opening < 0 {
		return pathToken{}, false
	}
	start := opening + 1
	text := string(r[start:cursor])
	if text == "" || !(strings.HasPrefix(text, "/") || text == "~" || strings.HasPrefix(text, "~/")) {
		return pathToken{}, false
	}
	if strings.ContainsAny(text, "\n\r") {
		return pathToken{}, false
	}
	end := cursor
	for end < len(r) && r[end] != '`' && r[end] != '\n' && r[end] != '\r' {
		end++
	}
	parent, query := "~/", ""
	if text != "~" {
		slash := strings.LastIndex(text, "/")
		parent, query = text[:slash+1], text[slash+1:]
	}
	return pathToken{start: start, end: end, text: text, parent: parent, query: query}, true
}

func (m *model) pathContext() (pathToken, string, bool) {
	if m.program == nil || m.ctx == nil || m.ctx.Err() != nil || m.view.Height < 4 || m.terminalFocused() || m.panelFocus || m.questionDialog != nil || m.pasteDialog != nil {
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
	key := scope + ":" + token.parent
	if p := m.pathHints; p != nil && p.key == key {
		if p.signature != signature {
			p.selected, p.offset = 0, 0
		}
		p.token, p.signature = token, signature
		return nil
	}
	m.clearPathHints()
	m.pathHintGeneration++
	p := &pathHints{key: key, signature: signature, token: token, generation: m.pathHintGeneration, loading: true}
	m.pathHints = p
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return pathHintDue{p.generation} })
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
	projectID, parent, generation := s.ProjectId, p.token.parent, p.generation
	ctx, cancel := context.WithTimeout(m.ctx, 4*time.Second)
	p.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		projects, err := m.client.Projects(ctx, &api.Empty{})
		if err != nil {
			return pathHintResult{generation: generation, err: err}
		}
		for _, project := range projects.Projects {
			if project.Id == projectID {
				listing, err := containerterm.ListPaths(ctx, project, parent)
				return pathHintResult{generation: generation, listing: listing, err: err}
			}
		}
		return pathHintResult{generation: generation, err: fmt.Errorf("project container unavailable")}
	}
}
func (m *model) receivePathHints(r pathHintResult) {
	if p := m.pathHints; p != nil && p.generation == r.generation {
		p.listing, p.err, p.loading = r.listing, r.err, false
	}
}

func (m *model) pathOptions() []pathOption {
	p := m.pathHints
	if p == nil {
		return nil
	}
	var paths []pathOption
	for _, entry := range p.listing.Entries {
		if fuzzyScore(entry.Name, p.token.query) < 0 {
			continue
		}
		name := entry.Name
		description := "file"
		if entry.Directory {
			name += "/"
			description = "directory"
		}
		paths = append(paths, pathOption{text: p.token.parent + name, description: description})
	}
	sort.SliceStable(paths, func(i, j int) bool {
		return fuzzyScore(strings.TrimPrefix(paths[i].text, p.token.parent), p.token.query) < fuzzyScore(strings.TrimPrefix(paths[j].text, p.token.parent), p.token.query)
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
	}
	return false, nil
}

func (m *model) applyPathOption(option pathOption) {
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
	cursor := len([]rune(replacement))
	if strings.ContainsRune(replacement, '`') {
		m.notice = "Path contains a backtick; type it explicitly"
		return
	}
	value := string(r[:token.start]) + replacement + string(r[token.end:])
	pos := token.start + cursor
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
	content := []string{muted.Render("Container paths · ↑/↓ choose · Tab complete · Esc close")}
	if p.listing.Truncated {
		content[0] = warning.Render("First 2048 entries · Tab complete · Esc close")
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
			text := "  " + name + "  " + o.description
			style := muted
			if idx == p.selected {
				text = "› " + name + "  " + o.description
				style = accent
			}
			content = append(content, style.Render(text))
		}
	} else if p.loading {
		content = append(content, muted.Render("Loading directory…"))
	} else if p.err != nil {
		content = append(content, warning.Render("Directory unavailable · check container and permissions"))
	} else {
		content = append(content, muted.Render("No matching entries"))
	}
	return overlayBox(view, content, m.width, true)
}
