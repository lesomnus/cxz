package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
)

type pastedText struct {
	token, body, path string
	owner             string
	file              bool
}
type pasteDialog struct {
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
			pairs = append(pairs, token, strings.Replace(token, "[Paste ", "[File  ", 1))
		}
	}
	if len(pairs) == 0 {
		return text
	}
	return strings.NewReplacer(pairs...).Replace(text)
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
		if p.question == nil {
			if s := m.current(); s != nil {
				for token, v := range m.pastes {
					if v.owner == s.Id {
						p.tokens = append(p.tokens, token)
					}
				}
				sort.Strings(p.tokens)
			}
		}
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
	case "i":
		if d.question != nil {
			return nil
		}
		if !strings.Contains(m.input.Value(), p.token) {
			m.input.InsertString(p.token)
		}
		m.pasteDialog = nil
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
		d.message = "Full text will be sent."
	case "d", "backspace", "delete":
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
	rows := []string{accent.Render("Pasted text · preview"), muted.Render("↑/↓ select · t text · f file · d delete · i insert · Esc back"), ""}
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
