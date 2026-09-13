package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

type pastedText struct {
	token, body, path string
	owner             string
	file              bool
	upload            *pasteDialog
}

type chipSelection struct {
	token, value, session    string
	start, end, cursor, page int
	question                 *questionDialog
}

// Cursor offsets are rune offsets, including logical newlines (not soft wraps).
func (m *model) chipInput() (string, int, func(int), func(string), bool) {
	if m.workflow != nil || m.projectView || m.accountView || m.creating || m.renaming || m.report != nil || m.modelPicker != nil || m.restartConfirm != nil {
		return "", 0, nil, nil, false
	}
	if d := m.questionDialog; d != nil {
		q := d.questions[d.page]
		if d.sending || q.Secret || !q.Other || d.row != len(q.Options) {
			return "", 0, nil, nil, false
		}
		in := &d.other[d.page]
		return in.Value(), in.Position(), in.SetCursor, in.SetValue, true
	}
	if m.focusList || m.focusApproval {
		return "", 0, nil, nil, false
	}
	li := m.input.LineInfo()
	lines := strings.Split(m.input.Value(), "\n")
	base := 0
	for _, line := range lines[:m.input.Line()] {
		base += len([]rune(line)) + 1
	}
	set := func(pos int) { m.input.SetCursor(pos - base) }
	return m.input.Value(), base + li.StartColumn + li.ColumnOffset, set, m.input.SetValue, true
}

func (m *model) chipKey(k tea.KeyMsg) (bool, tea.Cmd) {
	value, pos, cursor, setValue, ok := m.chipInput()
	id := ""
	if s := m.current(); s != nil {
		id = s.Id
	}
	page := 0
	if m.questionDialog != nil {
		page = m.questionDialog.page
	}
	sel := m.pasteSelection
	if !ok || sel != nil && (sel.value != value || sel.cursor != pos || sel.question != m.questionDialog || sel.page != page || sel.session != id) {
		m.pasteSelection = nil
		sel = nil
	}
	if !ok {
		return false, nil
	}
	key := k.String()
	if k.Paste {
		m.pasteSelection = nil
		return false, nil
	}
	if sel != nil {
		switch key {
		case "t":
			p := m.pastes[sel.token]
			p.file, p.upload = false, nil
			m.notice = "Full text will be sent."
			return true, nil
		case "f":
			p := m.pastes[sel.token]
			if p.upload != nil {
				return true, nil
			}
			m.openPastes()
			d := m.pasteDialog
			if d == nil {
				return true, nil
			}
			for i, token := range d.tokens {
				if token == sel.token {
					d.index = i
					break
				}
			}
			d.direct = true
			cmd := m.pasteKey(k)
			m.pasteDialog = nil
			if d.busy {
				p.upload = d
			}
			m.notice = d.message
			return true, cmd
		case "enter", "ctrl+p":
			cmd := m.openPastes()
			if d := m.pasteDialog; d != nil {
				for i, token := range d.tokens {
					if token == sel.token {
						d.index = i
						break
					}
				}
			}
			return true, cmd
		case "left", "right":
			if key == "left" {
				cursor(sel.start)
			} else {
				cursor(sel.end)
			}
			m.pasteSelection = nil
			return true, nil
		case "d", "backspace", "delete":
			m.pastes[sel.token].upload = nil
			r := []rune(value)
			// SetValue may reset the textarea row. Delete using native keys instead.
			if m.questionDialog == nil {
				cursor(sel.end)
				for i := sel.start; i < sel.end; i++ {
					m.input, _ = m.input.Update(tea.KeyMsg{Type: tea.KeyBackspace})
				}
			} else {
				setValue(string(r[:sel.start]) + string(r[sel.end:]))
				cursor(sel.start)
				d := m.questionDialog
				d.otherSelected[d.page] = strings.TrimSpace(d.other[d.page].Value()) != ""
			}
			m.pasteSelection = nil
			return true, nil
		}
		m.pasteSelection = nil
	}
	if k.Paste {
		return false, nil
	}
	if key != "left" && key != "right" && key != "backspace" && key != "delete" {
		return false, nil
	}
	for token := range m.pastes {
		for offset := 0; offset < len(value); {
			i := strings.Index(value[offset:], token)
			if i < 0 {
				break
			}
			i += offset
			start := utf8.RuneCountInString(value[:i])
			end := start + utf8.RuneCountInString(token)
			offset = i + len(token)
			left := key == "left" || key == "backspace"
			if left && pos > start && pos <= end || !left && pos >= start && pos < end {
				boundary := end
				if left {
					boundary = start
				}
				cursor(boundary)
				m.pasteSelection = &chipSelection{token: token, value: value, session: id, start: start, end: end, cursor: boundary, question: m.questionDialog, page: page}
				if key == "backspace" || key == "delete" {
					return m.chipKey(k)
				}
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *model) decorateInputPastes(view string) string {
	view = decoratePastes(view, m.pastes)
	if s := m.pasteSelection; s != nil {
		value, pos, _, _, ok := m.chipInput()
		if !ok || value != s.value || pos != s.cursor || s.question != m.questionDialog {
			return view
		}
		token := decoratePastes(s.token, m.pastes)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#031e2c")).Background(lipgloss.Color("#aeff98"))
		lines := strings.Split(view, "\n")
		remaining := token
		for i, line := range lines {
			plain := ansi.Strip(line)
			// Textarea may wrap a chip and decorate the cursor inside it.
			for n := len([]rune(remaining)); n > 0; n-- {
				if remaining == token && n < min(8, len([]rune(token))) {
					break
				}
				part := string([]rune(remaining)[:n])
				at := strings.Index(plain, part)
				if at < 0 {
					continue
				}
				start := ansi.StringWidth(plain[:at])
				end := start + ansi.StringWidth(part)
				lines[i] = ansi.Cut(line, 0, start) + style.Render(part) + ansi.Cut(line, end, ansi.StringWidth(line))
				remaining = string([]rune(remaining)[n:])
				break
			}
			if remaining == "" {
				break
			}
		}
		if remaining == "" {
			view = strings.Join(lines, "\n")
		}
	}
	return view
}

// Vertical/word navigation can land inside a token; normalize that destination
// before another edit or frame can expose an internal chip cursor.
func (m *model) snapChipCursor() {
	value, pos, _, _, ok := m.chipInput()
	if !ok {
		return
	}
	for token := range m.pastes {
		for offset := 0; offset < len(value); {
			i := strings.Index(value[offset:], token)
			if i < 0 {
				break
			}
			i += offset
			start := utf8.RuneCountInString(value[:i])
			if pos > start && pos < start+utf8.RuneCountInString(token) {
				m.chipKey(tea.KeyMsg{Type: tea.KeyRight})
				return
			}
			offset = i + len(token)
		}
	}
}

type pasteDialog struct {
	direct        bool
	tokens        []string
	index, offset int
	question      *questionDialog
	page          int
	busy          bool
	message       string
	session, run  string
	previewToken  string
	previewWidth  int
	preview       []string
}
type pasteUploaded struct {
	dialog      *pasteDialog
	token, path string
	err         error
}

type pasteSent struct {
	id, draft string
	result    result
}

func (m *model) sendPastes(draft, text string) tea.Cmd {
	s := m.current()
	if s == nil {
		return nil
	}
	id := s.Id
	cmd := m.action("send", text)
	return func() tea.Msg { return pasteSent{id, draft, cmd().(result)} }
}

func decoratePastes(text string, items map[string]*pastedText) string {
	var pairs []string
	for token, p := range items {
		if p.file {
			// Match the label and unique ID, not the entire chip: metadata may
			// wrap onto another display row. The two labels have equal width.
			prefix, _, _ := strings.Cut(token, " ·")
			pairs = append(pairs, prefix, strings.Replace(prefix, "[Paste ", "[File  ", 1))
		}
	}
	if len(pairs) == 0 {
		return text
	}
	// Cursor blinking inserts SGR sequences inside the label. Replace visible
	// characters while retaining their original styling and cursor escapes.
	plain := ansi.Strip(text)
	replaced := strings.NewReplacer(pairs...).Replace(plain)
	if plain == replaced {
		return text
	}
	var out strings.Builder
	state := byte(0)
	pos := 0
	for len(text) > 0 {
		seq, _, n, next := ansi.DecodeSequence(text, state, nil)
		if n == 0 {
			break
		}
		if visible := ansi.Strip(seq); visible != "" {
			out.WriteString(replaced[pos : pos+len(visible)])
			pos += len(visible)
		} else {
			out.WriteString(seq)
		}
		text, state = text[n:], next
	}
	return out.String()
}

func expandPastes(text string, items map[string]*pastedText) string {
	// One pass: pasted source may itself contain text resembling another chip.
	var pairs []string
	for token, p := range items {
		value := p.body
		if p.file {
			value = fmt.Sprintf("[Attached text file: %s — read this file for the full content]", p.path)
		}
		pairs = append(pairs, token, value)
	}
	if len(pairs) == 0 {
		return text
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

func (m *model) capturePaste(k tea.KeyMsg) bool {
	if !k.Paste || m.workflow != nil || m.projectView || m.accountView || m.creating {
		return false
	}
	text := string(k.Runes)
	d := m.questionDialog
	if d != nil {
		q := d.questions[d.page]
		if q.Secret || !q.Other || d.row != len(q.Options) || d.sending {
			return false
		}
	} else if m.focusList || m.focusApproval || m.report != nil || m.modelPicker != nil || m.restartConfirm != nil {
		return false
	}
	if utf8.RuneCountInString(text) <= 800 && strings.Count(text, "\n") < 3 {
		return false
	}
	if len(text) > 1024*1024 {
		m.notice = "Paste exceeds 1 MiB; attach an existing file instead."
		if d != nil {
			d.message = m.notice
		}
		return true
	}
	if m.pastes == nil {
		m.pastes = map[string]*pastedText{}
	}
	total := len(text)
	for _, p := range m.pastes {
		total += len(p.body)
	}
	if total > 32*1024*1024 {
		m.notice = "Paste cache reached 32 MiB; reconnect before adding more large pastes."
		if d != nil {
			d.message = m.notice
		}
		return true
	}
	token := fmt.Sprintf("[Paste %s · %dL · %dB]", core.ID()[:8], strings.Count(text, "\n")+1, len(text))
	owner := ""
	if s := m.current(); s != nil {
		owner = s.Id
	}
	m.pastes[token] = &pastedText{token: token, body: text, owner: owner}
	if d != nil {
		d.pastes = m.pastes
		before := d.other[d.page].Value()
		d.other[d.page].Focus()
		d.other[d.page], _ = d.other[d.page].Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(token), Paste: true})
		if partialPasteEdit(before, d.other[d.page].Value(), m.pastes) {
			d.other[d.page].SetValue(before)
			delete(m.pastes, token)
			return true
		}
		d.otherSelected[d.page] = true
		if !d.questions[d.page].Multi {
			clear(d.selected[d.page])
		}
		d.message = "Ctrl+P: preview, delete or attach pasted text as a file"
	} else {
		before := m.input.Value()
		m.input.InsertString(token)
		if !strings.Contains(m.input.Value(), token) || partialPasteEdit(before, m.input.Value(), m.pastes) {
			m.input.SetValue(before)
			delete(m.pastes, token)
			m.notice = "Could not insert paste chip at this position; original draft retained."
			return true
		}
		m.notice = "Ctrl+P or /paste: preview, delete or attach pasted text as a file"
	}
	m.resize()
	return true
}

// Keep placeholders indivisible. Whole-chip deletion is allowed; edits through
// part of a chip are rejected rather than silently sending a broken placeholder.
func partialPasteEdit(before, after string, items map[string]*pastedText) bool {
	if before == after {
		return false
	}
	start := 0
	for start < len(before) && start < len(after) && before[start] == after[start] {
		start++
	}
	endOld, endNew := len(before), len(after)
	for endOld > start && endNew > start && before[endOld-1] == after[endNew-1] {
		endOld--
		endNew--
	}
	for token := range items {
		for from := 0; from < len(before); {
			i := strings.Index(before[from:], token)
			if i < 0 {
				break
			}
			a := from + i
			b := a + len(token)
			if start < b && endOld > a && !(start <= a && endOld >= b) {
				return true
			}
			if start == endOld && start > a && start < b {
				return true
			}
			from = b
		}
	}
	return false
}

func (m *model) openPastes() tea.Cmd {
	if m.report != nil || m.modelPicker != nil || m.restartConfirm != nil {
		return nil
	}
	p := &pasteDialog{question: m.questionDialog}
	if s := m.current(); s != nil {
		p.session, p.run = s.Id, s.RunId
	}
	text := m.input.Value()
	if text == "/paste" {
		m.input.Reset()
		text = ""
	}
	if p.question != nil {
		p.page = p.question.page
		if p.question.questions[p.page].Secret {
			p.question.message = "Secret answers cannot be attached."
			return nil
		}
		text = p.question.other[p.page].Value()
	}
	// Preserve document order, not map iteration order.
	for len(text) > 0 {
		first := -1
		token := ""
		for candidate := range m.pastes {
			if i := strings.Index(text, candidate); i >= 0 && (first < 0 || i < first) {
				first = i
				token = candidate
			}
		}
		if first < 0 {
			break
		}
		p.tokens = append(p.tokens, token)
		text = text[first+len(token):]
	}
	if len(p.tokens) == 0 {
		m.notice = "No pasted text in this input."
		if p.question != nil {
			p.question.message = m.notice
		}
		return nil
	}
	m.pasteDialog = p
	return nil
}

func (m *model) pasteKey(k tea.KeyMsg) tea.Cmd {
	d := m.pasteDialog
	if k.String() == "esc" {
		m.pasteDialog = nil
		return nil
	}
	if d.busy {
		return nil
	}
	if s := m.current(); s == nil || s.Id != d.session || s.RunId != d.run || (d.question != nil && d.question != m.questionDialog) {
		m.pasteDialog = nil
		m.notice = "Paste dialog closed because its session/question changed."
		return nil
	}
	p := m.pastes[d.tokens[d.index]]
	switch k.String() {
	case "up":
		d.index = max(0, d.index-1)
		d.offset = 0
	case "down":
		d.index = min(len(d.tokens)-1, d.index+1)
		d.offset = 0
	case "pgup":
		d.offset = max(0, d.offset-max(1, m.view.Height-8))
	case "pgdown":
		d.offset += max(1, m.view.Height-8)
	case "t":
		p.file = false
		p.upload = nil
		d.message = "Full text will be sent."
	case "d", "backspace", "delete":
		p.upload = nil
		if d.question != nil {
			in := &d.question.other[d.page]
			in.SetValue(strings.ReplaceAll(in.Value(), p.token, ""))
		} else {
			m.input.SetValue(strings.ReplaceAll(m.input.Value(), p.token, ""))
		}
		m.pasteDialog = nil
		m.notice = "Paste removed from draft; previously uploaded files are retained with the session."
	case "f":
		if p.path != "" {
			p.file = true
			d.message = "File path will be sent. Press t to send full text instead."
			return nil
		}
		s := m.current()
		if s == nil {
			return nil
		}
		id, run, body, token := s.Id, s.RunId, p.body, p.token
		d.busy = true
		d.message = "Uploading to this session's durable storage…"
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			defer cancel()
			v, err := m.client.Attach(ctx, &api.AttachmentInput{SessionId: id, RunId: run, Content: []byte(body)})
			path := ""
			if v != nil {
				path = v.Path
			}
			return pasteUploaded{d, token, path, err}
		}
	}
	return nil
}

func (m *model) pasteOverlay(view string) string {
	d := m.pasteDialog
	if d == nil {
		return view
	}
	width := max(1, m.width-4)
	rows := []string{accent.Render("Pasted text · preview"), muted.Render("↑/↓ select · t send as full text · f send as file · d remove from draft"), muted.Render("Esc back (nothing sent yet)"), ""}
	start := max(0, d.index-1)
	for i := start; i < min(len(d.tokens), start+3); i++ {
		p := m.pastes[d.tokens[i]]
		mode := "text"
		if p.file {
			mode = "file"
		}
		label := fmt.Sprintf("%d · %d lines · %dB · %s", i+1, strings.Count(p.body, "\n")+1, len(p.body), mode)
		if i == d.index {
			label = accent.Render("› " + label)
		} else {
			label = "  " + label
		}
		rows = append(rows, label)
	}
	rows = append(rows, muted.Render(d.message), "")
	p := m.pastes[d.tokens[d.index]]
	if d.previewToken != p.token || d.previewWidth != width {
		d.preview = strings.Split(ansi.Hardwrap(safeText(p.body), width, true), "\n")
		d.previewToken = p.token
		d.previewWidth = width
	}
	preview := d.preview
	capacity := max(1, m.view.Height-len(rows)-2)
	d.offset = min(d.offset, max(0, len(preview)-capacity))
	rows = append(rows, preview[d.offset:min(len(preview), d.offset+capacity)]...)
	return overlayBox(view, rows, m.width, true)
}
