package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const questionChromeRows = 4 // Top border, action buttons, scroll hint, bottom border.

type questionLine struct {
	text string
	item int // Option/Other index; -1 for question text and separators.
}

type questionButton struct {
	text       string
	item, x, w int
}

type questionLayout struct {
	x, y, width, height, capacity int
	lines                         []questionLine
	buttons                       []questionButton
}

func (d *questionDialog) activeRow() int {
	if d.hovering {
		return d.hoverRow
	}
	return d.row
}

// Reserve space below the transcript. Optional terminal/tool panels consume the
// remaining space, and even the smallest supported screen keeps transcript rows.
func (m *model) questionHeight() int {
	if m.questionDialog == nil || m.projectView || m.accountView || m.workflow != nil || m.settingsPage != nil || m.memoryPage != nil {
		return 0
	}
	available := m.height - m.input.Height() - 5 - m.errorHeight()
	return max(0, min(available-1, max(10, available/2)))
}

// Wrap content before adding its prefix, so both soft wraps and explicit newlines
// retain the label/description indentation. Terminal tabs must be expanded first.
func questionWrap(text, prefix, continuation string, width int) []string {
	text = strings.ReplaceAll(safeText(text), "\t", "    ")
	rows := strings.Split(ansi.Hardwrap(text, max(1, width-ansi.StringWidth(prefix)), true), "\n")
	for i := range rows {
		if i == 0 {
			rows[i] = prefix + rows[i]
		} else {
			rows[i] = continuation + rows[i]
		}
	}
	return rows
}

func (m *model) questionLayout() questionLayout {
	d := m.questionDialog
	l := questionLayout{x: m.contentOffset() + 2, y: m.view.Height + 1, width: max(4, m.width-4), height: m.questionHeight()}
	l.capacity = max(0, l.height-questionChromeRows)
	width := max(1, l.width-4) // Borders and one space of padding on either side.
	q := d.questions[d.page]
	add := func(text string, item int) { l.lines = append(l.lines, questionLine{text, item}) }
	for _, row := range questionWrap(q.Text, "", "", width) {
		add(row, -1)
	}
	mode := "Choose one"
	if q.Multi {
		mode = "Choose one or more"
	}
	add(muted.Render(mode), -1)
	add("", -1)
	active := d.activeRow()
	for i, o := range q.Options {
		marker := "○"
		if q.Multi {
			marker = "[ ]"
		}
		if d.selected[d.page][i] {
			marker = "●"
			if q.Multi {
				marker = "[✓]"
			}
		}
		prefix := "  "
		if i == active {
			prefix = "› "
		}
		prefix += marker + " "
		for _, row := range questionWrap(o.Label, prefix, strings.Repeat(" ", ansi.StringWidth(prefix)), width) {
			if d.selected[d.page][i] {
				row = magenta.Bold(true).Render(row)
			} else if i == active {
				row = accent.Render(row)
			}
			add(row, i)
		}
		if o.Description != "" {
			for _, row := range questionWrap(o.Description, "    ", "    ", width) {
				add(muted.Render(row), i)
			}
		}
		// Hover changes only highlighting. Moving previews during hover would
		// change the item under a stationary pointer and prevent stable hit tests.
		if i == d.row && o.Preview != "" {
			if d.previewSource != o.Preview || d.previewWidth != width {
				d.previewSource, d.previewWidth = o.Preview, width
				d.previewRendered = questionPreview(o.Preview, width)
			}
			for _, row := range strings.Split(d.previewRendered, "\n") {
				add(row, i)
			}
		}
	}
	if q.Other {
		d.other[d.page].Width = max(1, width-10)
		in := d.other[d.page]
		in.Blur()
		prefix := "  "
		if active == len(q.Options) {
			prefix = "› "
			if d.row == active && !d.sending {
				in.Focus()
				in.Cursor.Blink = m.pulse%10 >= 5
			}
		}
		label := prefix + "Other: "
		if d.otherSelected[d.page] {
			label = magenta.Render(label)
		} else if active == len(q.Options) {
			label = accent.Render(label)
		}
		add(label+m.decorateInputPastes(in.View()), len(q.Options))
	}
	if d.reveal && l.capacity > 0 {
		for i, line := range l.lines {
			if line.item != d.row {
				continue
			}
			if i < d.offset {
				d.offset = i
			} else if i >= d.offset+l.capacity {
				d.offset = i - l.capacity + 1
			}
			break
		}
		d.reveal = false
	}
	d.offset = max(0, min(d.offset, max(0, len(l.lines)-l.capacity)))
	x := 2 // Border + padding, relative to the box.
	for i, label := range []string{"Next", "Back", "Cancel"} {
		if i == 0 && d.page == len(d.questions)-1 {
			label = "Submit"
		}
		text := "[ " + label + " ]"
		w := ansi.StringWidth(text)
		l.buttons = append(l.buttons, questionButton{text: text, item: d.count() + i, x: x, w: w})
		x += w + 2
	}
	return l
}

func (m *model) questionPanel() string {
	if m.questionDialog == nil || m.questionHeight() < questionChromeRows {
		return ""
	}
	d, l := m.questionDialog, m.questionLayout()
	title := clip(fmt.Sprintf(" Question %d/%d · %s ", d.page+1, len(d.questions), pickerLabel(d.questions[d.page].Header)), l.width-4)
	rows := []string{accent.Render("╭─" + title + strings.Repeat("─", max(0, l.width-3-ansi.StringWidth(title))) + "╮")}
	framed := func(text string, selected bool) string {
		text = clip(text, l.width-4)
		body := " " + text + strings.Repeat(" ", max(0, l.width-3-ansi.StringWidth(text)))
		if selected {
			body = indexedBackground(body, 238)
		}
		return accent.Render("│") + body + accent.Render("│")
	}
	for i := 0; i < l.capacity; i++ {
		line := questionLine{item: -1}
		if d.offset+i < len(l.lines) {
			line = l.lines[d.offset+i]
		}
		rows = append(rows, framed(line.text, line.item >= 0 && line.item == d.activeRow()))
	}
	buttons := ""
	for _, b := range l.buttons {
		if buttons != "" {
			buttons += "  "
		}
		text := b.text
		if b.item == d.activeRow() {
			text = indexedBackground(accent.Render(text), 238)
		}
		buttons += text
	}
	rows = append(rows, framed(buttons, false))
	hint := muted.Render(fmt.Sprintf("%d–%d/%d · PgUp/Dn · Esc close", d.offset+1, min(len(l.lines), d.offset+l.capacity), len(l.lines)))
	if d.message != "" {
		hint = warning.Render(pickerLabel(d.message))
	}
	rows = append(rows, framed(hint, false), accent.Render("╰"+strings.Repeat("─", l.width-2)+"╯"))
	for i := range rows {
		rows[i] = "  " + rows[i] + "  "
	}
	return strings.Join(rows, "\n")
}

func (m *model) scrollQuestion(delta int) {
	d := m.questionDialog
	d.offset += delta
	d.reveal = false
	m.questionLayout()
}

// Global terminal coordinates: question rows and conversation rows have separate
// scrolling targets. Hover never changes the keyboard row; a click commits it.
func (m *model) questionMouse(v tea.MouseMsg) tea.Cmd {
	d := m.questionDialog
	d.hovering = false
	l := m.questionLayout()
	if v.X < l.x || v.X >= l.x+l.width || v.Y < l.y || v.Y >= l.y+l.height {
		if v.X >= m.contentOffset() && v.X < m.contentOffset()+m.width && v.Y >= 0 && v.Y < m.view.Height &&
			(v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown) {
			m.view, _ = m.view.Update(v)
			return m.loadOlderHistory()
		}
		return nil
	}
	if v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
		delta := 3
		if v.Button == tea.MouseButtonWheelUp {
			delta = -delta
		}
		m.scrollQuestion(delta)
	}
	item := -1
	row := v.Y - l.y - 1
	if v.X > l.x && v.X < l.x+l.width-1 && row >= 0 && row < l.capacity && d.offset+row < len(l.lines) {
		item = l.lines[d.offset+row].item
	} else if row == l.capacity {
		for _, b := range l.buttons {
			if v.X >= l.x+b.x && v.X < l.x+b.x+b.w {
				item = b.item
				break
			}
		}
	}
	if item < 0 || d.sending {
		return nil
	}
	d.hovering, d.hoverRow = true, item
	if v.Button != tea.MouseButtonLeft || v.Action != tea.MouseActionPress {
		return nil
	}
	d.row = item
	d.reveal = true
	if item == len(d.questions[d.page].Options) && d.questions[d.page].Other {
		d.otherSelected[d.page] = true
		if !d.questions[d.page].Multi {
			clear(d.selected[d.page])
		}
		return d.other[d.page].Focus() // Clicking Other edits it; Enter submits it.
	}
	page := d.page
	key := tea.KeyMsg{Type: tea.KeyEnter}
	if item < len(d.questions[d.page].Options) && d.questions[d.page].Multi {
		key = tea.KeyMsg{Type: tea.KeySpace}
	}
	cmd := m.questionKey(key)
	if m.questionDialog == d && d.page == page {
		d.hovering, d.hoverRow = true, item
	}
	return cmd
}
