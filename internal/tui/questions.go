package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

type questionDialog struct {
	id, run, request  string
	questions         []agentview.Question
	selected          [][]bool
	other             []textinput.Model
	otherSelected     []bool
	page, row, offset int
	message           string
	sending           bool
	pastes            map[string]*pastedText
}

func (m *model) openQuestion(p *api.Event) tea.Cmd {
	s := m.current()
	if s != nil && (p == nil || !question(p)) {
		for _, candidate := range s.Pending {
			if question(candidate) {
				p = candidate
				break
			}
		}
	}
	if s == nil || p == nil || !question(p) {
		m.notice = "Select a pending question first"
		return nil
	}
	if p.RunId != "" && p.RunId != s.RunId {
		m.notice = "Question belongs to an old run"
		return nil
	}
	qs, err := agentview.Questions(s.Agent, p.Text, p.Payload)
	if err != nil {
		m.notice = "Cannot open question form: " + err.Error() + ". /approval shows the raw request."
		return nil
	}
	d := &questionDialog{id: s.Id, run: s.RunId, request: p.RequestId, questions: qs}
	for _, q := range qs {
		d.selected = append(d.selected, make([]bool, len(q.Options)))
		in := textinput.New()
		in.Cursor.Style = inputCursorStyle
		in.Placeholder = "Type your answer"
		in.CharLimit = 0
		in.Prompt = ""
		if q.Secret {
			in.EchoMode = textinput.EchoPassword
		}
		d.other = append(d.other, in)
		d.otherSelected = append(d.otherSelected, false)
	}
	m.questionDialog = d
	if m.questionSeen == nil {
		m.questionSeen = map[string]bool{}
	}
	m.questionSeen[s.Id+"/"+s.RunId+"/"+p.RequestId] = true
	m.report = nil
	if m.modelPicker != nil && m.modelPicker.cancel != nil {
		m.modelPicker.cancel()
	}
	m.modelPicker = nil
	m.resize()
	return nil
}

func (m *model) closeQuestion() {
	m.questionDialog = nil
	m.resize()
}

func (m *model) syncQuestion() {
	s := m.current()
	if d := m.questionDialog; d != nil {
		valid := false
		if s != nil && s.Id == d.id && s.RunId == d.run {
			for _, p := range s.Pending {
				if p.RequestId == d.request {
					valid = true
				}
			}
		}
		if !valid {
			m.closeQuestion()
			m.notice = "Question no longer pending"
		}
	}
	if s == nil || m.projectView || m.accountView || m.questionDialog != nil || m.report != nil || m.modelPicker != nil || m.restartConfirm != nil {
		return
	}
	for _, p := range s.Pending {
		key := s.Id + "/" + s.RunId + "/" + p.RequestId
		if question(p) && (p.RunId == "" || p.RunId == s.RunId) && !m.questionSeen[key] && !m.approvalSent[key] {
			if m.questionSeen == nil {
				m.questionSeen = map[string]bool{}
			}
			m.questionSeen[key] = true
			m.openQuestion(p)
			return
		}
	}
}

func (d *questionDialog) count() int {
	q := d.questions[d.page]
	n := len(q.Options)
	if q.Other {
		n++
	}
	return n
}
func (d *questionDialog) texts() []string {
	var out []string
	for i, in := range d.other {
		text := ""
		if d.otherSelected[i] {
			text = expandPastes(in.Value(), d.pastes)
		}
		out = append(out, text)
	}
	return out
}
func (m *model) questionNext() tea.Cmd {
	d := m.questionDialog
	if d.sending {
		return nil
	}
	if _, err := agentview.EncodeQuestionAnswers(d.questions[d.page:d.page+1], d.selected[d.page:d.page+1], d.texts()[d.page:d.page+1]); err != nil {
		d.message = "Choose an option or enter an answer first"
		return nil
	}
	if d.page < len(d.questions)-1 {
		d.page++
		d.row = 0
		d.offset = 0
		d.message = ""
		return nil
	}
	answers, err := agentview.EncodeQuestionAnswers(d.questions, d.selected, d.texts())
	if err != nil {
		d.message = err.Error()
		return nil
	}
	s := m.current()
	if s != nil && s.Id == d.id && s.RunId == d.run {
		for _, p := range s.Pending {
			if p.RequestId == d.request {
				cmd := m.replyApproval(p, true, answers, false)
				if cmd != nil {
					d.sending = true
					d.message = "Sending answer…"
				} else {
					d.message = m.notice
				}
				return cmd
			}
		}
	}
	d.message = "Question is stale; no answer sent"
	return nil
}

func (m *model) questionKey(k tea.KeyMsg) tea.Cmd {
	d := m.questionDialog
	if d.sending && k.String() != "esc" && k.String() != "ctrl+c" {
		return nil
	}
	q := d.questions[d.page]
	n := d.count()
	if k.Paste {
		goto input
	}
	switch k.String() {
	case "esc", "ctrl+q":
		m.closeQuestion()
		return nil
	case "ctrl+c":
		return tea.Quit
	case "ctrl+s":
		return m.questionNext()
	case "left", "right":
		if d.row < n {
			goto input
		}
		delta := 1
		if k.String() == "left" {
			delta = 2
		}
		d.row = n + (d.row-n+delta)%3
	case "up", "shift+tab":
		d.row = (d.row + n + 2) % (n + 3)
	case "down", "tab":
		d.row = (d.row + 1) % (n + 3)
	case "pgup":
		d.offset -= 5
		return nil
	case "pgdown":
		d.offset += 5
		return nil
	case "enter", " ":
		if d.row < len(q.Options) {
			value := !d.selected[d.page][d.row]
			if !q.Multi {
				value = true
				clear(d.selected[d.page])
				d.otherSelected[d.page] = false
			}
			d.selected[d.page][d.row] = value
		} else if d.row == n {
			return m.questionNext()
		} else if d.row == n+1 {
			if d.page > 0 {
				d.page--
				d.row = 0
				d.offset = 0
			}
		} else if d.row == n+2 {
			m.closeQuestion()
			return nil
		} else if k.String() == "enter" {
			d.otherSelected[d.page] = true
			if !q.Multi {
				clear(d.selected[d.page])
			}
			return m.questionNext()
		} else {
			goto input
		}
	default:
		goto input
	}
	d.offset = 0
	return nil
input:
	if q.Other && d.row == len(q.Options) {
		before := d.other[d.page].Value()
		d.other[d.page].Focus()
		var cmd tea.Cmd
		d.other[d.page], cmd = d.other[d.page].Update(k)
		m.snapChipCursor()
		if d.other[d.page].Value() != before {
			if partialPasteEdit(before, d.other[d.page].Value(), d.pastes) {
				d.other[d.page].SetValue(before)
				d.message = "Ctrl+P to preview or delete the paste."
				return nil
			}
			d.otherSelected[d.page] = strings.TrimSpace(d.other[d.page].Value()) != ""
			if !q.Multi {
				clear(d.selected[d.page])
			}
		}
		return cmd
	}
	return nil
}

func (m *model) questionOverlay(view string) string {
	if m.pasteDialog != nil {
		return view
	}
	d := m.questionDialog
	if d == nil {
		return view
	}
	q := d.questions[d.page]
	width := max(1, m.width-6)
	lines := []string{accent.Bold(true).Render(fmt.Sprintf("Question %d/%d · %s", d.page+1, len(d.questions), safeText(q.Header)))}
	add := func(s string) { lines = append(lines, strings.Split(ansi.Hardwrap(s, width, true), "\n")...) }
	add(safeText(q.Text))
	mode := "Choose one"
	if q.Multi {
		mode = "Choose one or more"
	}
	add(muted.Render(mode))
	add("")
	focus := len(lines)
	for i, o := range q.Options {
		marker := "[ ]"
		if !q.Multi {
			marker = "○"
		}
		if d.selected[d.page][i] {
			marker = "[✓]"
			if !q.Multi {
				marker = "●"
			}
		}
		prefix := "  "
		if i == d.row {
			prefix = "› "
			focus = len(lines)
		}
		label := prefix + marker + " " + safeText(o.Label)
		if d.selected[d.page][i] {
			label = magenta.Bold(true).Render(label)
		} else if i == d.row {
			label = accent.Render(label)
		}
		add(label)
		if o.Description != "" {
			add(muted.Render("    " + safeText(o.Description)))
		}
		if i == d.row && o.Preview != "" {
			add(questionPreview(o.Preview, width))
		}
	}
	if q.Other {
		in := d.other[d.page]
		in.Width = max(1, width-10)
		in.Blur()
		prefix := "  "
		if d.row == len(q.Options) {
			prefix = "› "
			focus = len(lines)
			in.Focus()
			in.Cursor.Blink = m.pulse%10 >= 5
		}
		label := prefix + "Other: "
		if d.otherSelected[d.page] {
			label = magenta.Render(label)
		} else if d.row == len(q.Options) {
			label = accent.Render(label)
		}
		add(label + m.decorateInputPastes(in.View()))
	}
	n := d.count()
	add("")
	buttonLine := ""
	for i, label := range []string{"Next", "Back", "Cancel"} {
		if i == 0 && d.page == len(d.questions)-1 {
			label = "Submit"
		}
		prefix := "  "
		if d.row == n+i {
			prefix = "› "
			label = accent.Reverse(true).Render("[ " + label + " ]")
		} else {
			label = "[ " + label + " ]"
		}
		button := prefix + label
		if buttonLine != "" && ansi.StringWidth(buttonLine+" "+button) > width {
			add(buttonLine)
			buttonLine = ""
		}
		if d.row == n+i {
			focus = len(lines)
		}
		if buttonLine != "" {
			buttonLine += " "
		}
		buttonLine += button
	}
	add(buttonLine)
	add("")
	height := max(1, len(strings.Split(view, "\n"))-4)
	start := max(0, focus-height+2) + d.offset
	start = max(0, min(start, max(0, len(lines)-height)))
	body := append([]string{}, lines[start:min(len(lines), start+height)]...)
	body = append(body, muted.Render("↑/↓ Tab move · Space/Enter choose · Ctrl+S next/submit · Esc close"))
	if d.message != "" {
		body = append(body, warning.Render(safeText(d.message)))
	}
	return overlayBox(view, body, m.width, true)
}

// Align the preview with option descriptions, not with the list indicator.
func questionPreview(text string, width int) string {
	const indent = "    "
	boxWidth := width - len(indent)
	if boxWidth < 12 {
		return indent + "Preview\n" + indentBlock(markdownView(text, max(1, width-2)))
	}
	inner := boxWidth - 4
	rows := []string{indent + muted.Render("╭─ Preview "+strings.Repeat("─", boxWidth-12)+"╮")}
	for _, line := range strings.Split(ansi.Hardwrap(markdownView(text, inner), inner, true), "\n") {
		rows = append(rows, indent+muted.Render("│")+" "+line+strings.Repeat(" ", max(0, inner-ansi.StringWidth(line)))+" "+muted.Render("│"))
	}
	rows = append(rows, indent+muted.Render("╰"+strings.Repeat("─", boxWidth-2)+"╯"))
	return strings.Join(rows, "\n")
}
