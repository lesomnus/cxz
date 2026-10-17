package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

// Style only after sanitizing remote text. State events stay in the journal,
// but the transcript omits them: the live status line is their presentation.
func eventView(s *api.Session, e *api.Event, width int) (out string) {
	if e.Kind != "input" && e.Kind != "turn_end" {
		width = max(1, width-2)
		if e.Kind != "assistant" {
			defer func() { out = indentBlock(out) }()
		}
	}
	wrap := func(text string) string { return ansi.Hardwrap(safeText(text), max(1, width), true) }
	switch e.Kind {
	case "permission":
		return accent.Render(wrap("Permission · " + e.Text + " · saved for this session"))
	case "state":
		return ""
	case "update":
		return accent.Render(wrap("↻ " + e.Text))
	case "setting":
		var v struct{ Value string }
		_ = json.Unmarshal(e.Payload, &v)
		if v.Value == "" {
			v.Value = "default"
		}
		return accent.Render(wrap("✓ " + e.Text + " · " + v.Value))
	case "setting_status":
		return muted.Render(wrap("Settings · " + e.Text))
	case "input":
		stamp := "-- -- --:--"
		if e.TimeMs > 0 {
			stamp = time.UnixMilli(e.TimeMs).Local().Format("01-02 15:04")
		}
		// The fill reaches the edge of the conversation, so a cursor drawn against
		// it has nothing between the two. Keep one column of the fill to its left.
		text := strings.Split(ansi.Hardwrap(safeText(e.Text), max(1, width-promptIndent-2), true), "\n")
		rows := []string{"", promptTimestamp.Render(clip(promptIndentText+"  "+stamp, width))}
		for i, line := range text {
			prefix := promptIndentText + "  "
			if i == 0 {
				prefix = promptIndentText + "❯ "
			}
			rows = append(rows, blue.Render(prefix+line))
		}
		rows = append(rows, "")
		for i, line := range rows {
			rows[i] = indexedBackground(line+strings.Repeat(" ", max(0, width-ansi.StringWidth(line))), promptBackground)
		}
		return strings.Join(rows, "\n")
	case "assistant":
		return renderResponse(s.Agent, e.Text, width+2).body
	case "approval":
		return strings.TrimPrefix(approvalLine(s, e, "requested", width+2), "  ")
	case "approval_resolved":
		return ""
	case "tool_call", "tool_result":
		if question(e) {
			return lavender.Render("? Question · open /answer to choose")
		}
		// A row says what its tool is; a bare record does not, and this is the
		// only thing left that can be handed one -- a tool whose row the
		// projection never built, which is a tool only by its kind.
		if label := e.GetToolSummary(); label != nil {
			// This branch is already indented by eventView's deferred wrapper.
			return toolSummaryBody(label, width+2, label.GetState())
		}
		return lavender.Render(wrap("tool › " + e.Text + " · /details"))
	case "compact":
		return accent.Render(wrap("◇ Context compacted · journal retained"))
	case "turn_end":
		return turnSummary(e, nil, 0, width)
	case "diagnostic", "stderr":
		return peach.Render(wrap("diagnostic › " + e.Text + " " + string(e.Payload)))
	}
	return ""
}

func approvalLine(s *api.Session, e *api.Event, state string, width int) string {
	icon, style := "○", warning
	switch state {
	case "allowed":
		icon, style = "✓", accent
	case "denied":
		icon, style = "×", failure
	case "canceled":
		icon, style = "×", failure
	}
	approval := agentview.ApprovalView(s.Agent, e.Text, e.Payload)
	title := approval.Title
	if approval.Summary != "" {
		title += " · " + strings.Join(strings.Fields(safeText(approval.Summary)), " ")
		title = clip(safeText(title), max(1, width-4))
	}
	if question(e) && !(s.Agent == "codex" && e.Text == agentview.CodexElicitation) {
		if state == "allowed" {
			state = "answered"
		}
		return indentBlock(style.Render(ansi.Hardwrap(fmt.Sprintf("%s question %s", icon, state), max(1, width-2), true)))
	}
	return indentBlock(style.Render(ansi.Hardwrap(fmt.Sprintf("%s %s", icon, safeText(title)), max(1, width-2), true)))
}

func toolResultSummary(e *api.Event) string {
	root := fields(e.Payload)
	item := root.object("item")
	text := e.Text
	content := e.Payload
	if raw, ok := root["content"]; ok {
		content = raw
	}
	if text == "" {
		_ = json.Unmarshal(content, &text)
	}
	if text == "" {
		text = item.text("aggregatedOutput")
	}
	if text == "" {
		var blocks []struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(content, &blocks)
		for _, b := range blocks {
			text += b.Text + "\n"
		}
	}
	status := "done"
	if item.text("status") != "" {
		status = item.text("status")
	}
	if string(root["is_error"]) == "true" {
		status = "failed"
	}
	if code, ok := item.number("exitCode"); ok {
		status += fmt.Sprintf(" · exit %.0f", code)
	}
	count := len(strings.Split(strings.TrimRight(text, "\n"), "\n"))
	if text == "" {
		count = 0
	}
	size := len(e.Payload)
	preview := strings.TrimSpace(strings.SplitN(safeText(text), "\n", 2)[0])
	result := fmt.Sprintf("↳ %s · %d lines · %sB · /details", status, count, humanCount(float64(size)))
	if preview != "" {
		result += " · " + preview
	}
	return result
}

// toolDetails answers /details with the newest tool's own records. Which record
// is newest is answered from what is loaded -- a row carries its kind -- and the
// bytes come from the runtime, because a row does not carry them.
func (m *model) toolDetails() tea.Cmd {
	s := m.current()
	if s == nil {
		m.recordLocal("/details", "Select a session first.")
		return nil
	}
	events := m.rows[s.Id]
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.GetToolSummary() == nil {
			continue
		}
		id, seq := s.Id, e.Seq
		client := m.client
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			defer cancel()
			batch, err := client.EventDetails(ctx, &api.EventDetailsRequest{SessionId: id, Seq: seq})
			out := toolDetailsLoaded{session: id, seq: seq, subject: e, err: err}
			if batch != nil {
				out.events = batch.Events
			}
			return out
		}
	}
	m.recordLocal("/details", "No tool activity in the received history yet.")
	return nil
}

type toolDetailsLoaded struct {
	session string
	seq     uint64
	subject *api.Event
	events  []*api.Event
	err     error
}

func (m *model) acceptToolDetails(v toolDetailsLoaded) {
	if s := m.current(); s == nil || s.Id != v.session {
		return
	}
	if v.err != nil {
		m.recordLocal("/details", "Tool details are unavailable: "+v.err.Error())
		return
	}
	m.recordLocal("/details", toolDetailsReport(v.subject, v.events))
}

// toolDetailsReport writes out a tool's own records. A row is one tool, so both
// halves belong to it: what was submitted, and what came back. Which record
// opened the row does not decide what is shown, which it used to -- picking a
// result reported its call, and picking a call reported no result at all.
func toolDetailsReport(subject *api.Event, events []*api.Event) string {
	var call, result *api.Event
	for _, e := range events {
		if e.RunId != subject.RunId || e.RequestId != subject.RequestId {
			continue
		}
		switch e.Kind {
		case "tool_call":
			call = e
		case "tool_result":
			result = e
		}
	}
	var out []string
	if call != nil {
		out = append(out, fmt.Sprintf("Tool input · event %d · %s\n%s", call.Seq, call.Text, indented(call.Payload)))
	}
	if result != nil {
		out = append(out, fmt.Sprintf("Tool result · event %d\n%s\n%s", result.Seq, result.Text, indented(result.Payload)))
	}
	if len(out) == 0 {
		return fmt.Sprintf("Tool · event %d\n%s\n%s", subject.Seq, subject.Text, indented(subject.Payload))
	}
	return strings.Join(out, "\n\n")
}

func indented(payload []byte) string {
	var value any
	if json.Unmarshal(payload, &value) == nil {
		b, _ := json.MarshalIndent(value, "", "  ")
		return string(b)
	}
	return string(payload)
}
