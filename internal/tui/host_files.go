package tui

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/assets"
)

const hostFilePause = 300 * time.Millisecond

type backtickToken struct {
	start, end int // content only, in runes
	text       string
	closed     bool
}

func backtickTokens(value string) []backtickToken {
	r := []rune(value)
	var tokens []backtickToken
	for i := 0; i < len(r); i++ {
		if r[i] != '`' {
			continue
		}
		start := i + 1
		end := start
		for end < len(r) && r[end] != '`' {
			end++
		}
		tokens = append(tokens, backtickToken{start, end, string(r[start:end]), end < len(r)})
		i = end
	}
	return tokens
}

func hostFileTokens(value string) []backtickToken {
	var tokens []backtickToken
	for _, token := range backtickTokens(value) {
		if strings.HasPrefix(token.text, "!") {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func (m *model) hostFilesEnabled() bool {
	if !m.previewInteraction() || m.errorFocused() || m.memoryPage != nil || m.settingsPage != nil || m.terminalFocused() || !m.input.Focused() || m.current() == nil {
		return false
	}
	_, _, _, _, ok := m.chipInput()
	return ok && !m.accountView
}

// A check belongs to one exact draft and session/run. A later key, session
// switch or dismissed composer makes both its timer and filesystem reply stale.
type hostFileCheck struct {
	value, session, run string
	tokens              []backtickToken
}
type hostFileDue struct{ check *hostFileCheck }
type hostFile struct {
	path string
	info os.FileInfo
}
type hostFileMatch struct {
	token backtickToken
	files []hostFile
}
type hostFileChecked struct {
	check    *hostFileCheck
	matches  []hostFileMatch
	tooLarge string
}

func (m *model) syncHostFiles() tea.Cmd {
	if !m.hostFilesEnabled() {
		m.hostFileCheck = nil
		return nil
	}
	value, s := m.input.Value(), m.current()
	if p := m.hostFileCheck; p != nil && p.value == value && p.session == s.Id && p.run == s.RunId {
		return nil
	}
	tokens := hostFileTokens(value)
	if len(tokens) == 0 {
		m.hostFileCheck = nil
		return nil
	}
	p := &hostFileCheck{value: value, session: s.Id, run: s.RunId, tokens: tokens}
	m.hostFileCheck = p
	delay := time.Duration(0)
	for _, token := range tokens {
		if !token.closed {
			delay = hostFilePause
			break
		}
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return hostFileDue{p} })
}

func (m *model) currentHostFileCheck(p *hostFileCheck) bool {
	if p == nil || m.hostFileCheck != p || !m.hostFilesEnabled() {
		return false
	}
	s := m.current()
	return m.input.Value() == p.value && s.Id == p.session && s.RunId == p.run
}

func (m *model) checkHostFiles(d hostFileDue) tea.Cmd {
	if !m.currentHostFileCheck(d.check) {
		return nil
	}
	return func() tea.Msg {
		result := hostFileChecked{check: d.check}
		for _, token := range d.check.tokens {
			paths := filePaths(strings.TrimPrefix(token.text, "!"))
			if len(paths) == 0 {
				continue
			}
			match := hostFileMatch{token: token}
			for _, path := range paths {
				info, err := os.Stat(path)
				if err != nil || !info.Mode().IsRegular() {
					match.files = nil
					break
				}
				if info.Size() > assets.MaxSize {
					result.tooLarge = info.Name()
					match.files = nil
					break
				}
				match.files = append(match.files, hostFile{path, info})
			}
			if len(match.files) > 0 {
				result.matches = append(result.matches, match)
			}
		}
		return result
	}
}

func (m *model) receiveHostFiles(r hostFileChecked) tea.Cmd {
	if !m.currentHostFileCheck(r.check) {
		return nil
	}
	var cmds []tea.Cmd
	// Replace from the right so earlier ranges keep their original positions.
	for i := len(r.matches) - 1; i >= 0; i-- {
		match := r.matches[i]
		end := match.token.end
		if match.token.closed {
			end++
		}
		cmds = append(cmds, m.attachFilesAt(match.files, match.token.start-1, end))
	}
	if r.tooLarge != "" {
		m.notice = "Attachment exceeds 1 GiB: " + safeText(r.tooLarge)
	}
	return tea.Batch(cmds...)
}

// Let path pastes reach the composer intact, including long multi-file drops.
func (m *model) hostFilePaste(k tea.KeyMsg) bool {
	if !k.Paste || !m.hostFilesEnabled() {
		return false
	}
	value, pos, _, _, _ := m.chipInput()
	for _, token := range hostFileTokens(value) {
		if pos >= token.start+1 && pos <= token.end {
			return true
		}
	}
	return len(hostFileTokens(string(k.Runes))) > 0
}

func (m *model) attachFilesAt(files []hostFile, start, end int) tea.Cmd {
	s := m.current()
	value, pos, _, _, ok := m.chipInput()
	r := []rune(value)
	if s == nil || !ok || start < 0 || start > end || end > len(r) {
		return nil
	}
	if m.pastes == nil {
		m.pastes = map[string]*pastedText{}
	}
	var items []*pastedText
	var tokens []string
	for _, file := range files {
		p := m.newFileChip(s.Id, s.RunId, file.path, file.info)
		m.pastes[p.token] = p
		items = append(items, p)
		tokens = append(tokens, p.token)
	}
	replacement := strings.Join(tokens, " ")
	if end == len(r) && (len(replacement) > 0) {
		replacement += " "
	}
	next := string(r[:start]) + replacement + string(r[end:])
	if partialPasteEdit(value, next, m.pastes) || m.input.CharLimit > 0 && len([]rune(next)) > m.input.CharLimit {
		for _, p := range items {
			delete(m.pastes, p.token)
		}
		m.showError("Could not insert attachment at this position.")
		return nil
	}
	delta := len([]rune(replacement)) - (end - start)
	if pos >= end {
		pos += delta
	} else if pos > start {
		pos = start + len([]rune(replacement))
	}
	m.setPathInput(next, pos)
	var cmds []tea.Cmd
	for _, p := range items {
		cmds = append(cmds, m.startFileUpload(p))
	}
	return tea.Batch(cmds...)
}
