package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/containerterm"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/wisp"
)

type redactDialog struct {
	draft              string
	start, end, cursor int
	body               []rune
	id, run            string
}
type redaction struct {
	body    []byte
	id, run string
}
type redactSent struct {
	id, request string
	err         error
	tokens      []string
}

type secretFiles interface {
	PutSecret(context.Context, context.Context, *api.Project, string, []byte) (string, error)
	DeleteSecret(context.Context, context.Context, *api.Project, string) error
}

func (m *model) openRedact() {
	if len(m.redactions) >= 16 {
		m.notice = "At most 16 pending secrets; remove a chip first"
		return
	}
	s := m.current()
	if s == nil || m.creating {
		return
	}
	value, pos, _, _, ok := m.chipInput()
	if !ok {
		return
	}
	m.redactDialog = &redactDialog{id: s.Id, run: s.RunId, draft: value, start: pos, end: pos, cursor: pos}
}
func (m *model) redactKey(k tea.KeyMsg) tea.Cmd {
	d := m.redactDialog
	s := m.current()
	if s == nil || s.Id != d.id || s.RunId != d.run {
		clear(d.body)
		m.redactDialog = nil
		return nil
	}
	if k.Paste || k.Type == tea.KeyRunes {
		if utf8.RuneCountInString(string(k.Runes)) > wisp.MaxSecretBytes-len(d.body) || len(string(d.body))+len(string(k.Runes)) > wisp.MaxSecretBytes {
			m.notice = "Secret limit: 64 KiB"
			return nil
		}
		d.body = append(d.body, k.Runes...)
		return nil
	}
	switch k.String() {
	case "esc":
		clear(d.body)
		m.redactDialog = nil
		m.setPathInput(d.draft, d.cursor)
		if token := m.inlineContext(); token != nil {
			m.inlineDismissed = token.signature
		}
	case "ctrl+x":
		clear(d.body)
		d.body = d.body[:0]
	case "backspace":
		if len(d.body) > 0 {
			d.body[len(d.body)-1] = 0
			d.body = d.body[:len(d.body)-1]
		}
	case "enter":
		if len(d.body) == 0 {
			return nil
		}
		if m.redactions == nil {
			m.redactions = map[string]*redaction{}
		}
		if m.pastes == nil {
			m.pastes = map[string]*pastedText{}
		}
		token := "[Redacted]"
		for n := 2; m.pastes[token] != nil; n++ {
			token = fmt.Sprintf("[Redacted %d]", n)
		}
		if m.input.Value() != d.draft {
			m.notice = "Draft changed; cancel and select @redact again"
			return nil
		}
		r := []rune(d.draft)
		value := string(r[:d.start]) + token + string(r[d.end:])
		m.setPathInput(value, d.start+len([]rune(token)))
		if m.input.Value() != value {
			m.setPathInput(d.draft, d.cursor)
			m.notice = "Could not insert secret chip"
			return nil
		}
		m.redactions[token] = &redaction{body: []byte(string(d.body)), id: d.id, run: d.run}
		m.pastes[token] = &pastedText{token: token, owner: d.id, secret: true}
		clear(d.body)
		m.redactDialog = nil
		m.resize()
	}
	return nil
}
func (m *model) redactOverlay(view string) string {
	d := m.redactDialog
	if d == nil {
		return view
	}
	n := len(d.body)
	caret := " "
	if m.pulse%2 == 0 {
		caret = accent.Render("▏")
	}
	count := fmt.Sprintf("%03d", n)
	zeros := len(count) - len(strings.TrimLeft(count, "0"))
	if n == 0 {
		zeros = len(count)
	}
	rows := []string{accent.Render("Secret · @redact"), "", "[" + strings.Repeat("*", min(3, n)) + strings.Repeat(" ", 3-min(3, n)) + "] " + muted.Render(count[:zeros]) + count[zeros:] + caret, "", muted.Render("Enter insert chip · Ctrl+X clear · Esc cancel"), muted.Render("Only the file path is sent. Host tmpfs · sweep after 8 hours idle."), muted.Render("The agent can read this file; its output may expose the secret.")}
	return overlayBox(view, rows, m.width, true)
}
func (m *model) hasRedactions(text string) bool {
	for token, p := range m.pastes {
		if p.secret && strings.Contains(text, token) {
			return true
		}
	}
	return false
}
func (m *model) pruneRedactions() {
	if d := m.redactDialog; d != nil {
		if s := m.current(); s == nil || s.Id != d.id || s.RunId != d.run || s.State == "stopped" || s.State == "failed" {
			clear(d.body)
			m.redactDialog = nil
		}
	}
	if m.redactSending {
		return
	}
	for token, r := range m.redactions {
		present := strings.Contains(m.drafts[r.id], token)
		if s := m.current(); s != nil && s.Id == r.id {
			present = strings.Contains(m.input.Value(), token) && s.RunId == r.run
		}
		if !present {
			clear(r.body)
			delete(m.redactions, token)
			delete(m.pastes, token)
		}
	}
}
func (m *model) sendRedactions(draft string) tea.Cmd {
	if m.redactSending {
		return nil
	}
	s := m.current()
	if s == nil {
		return nil
	}
	// Snapshot all replacements before asynchronous work; never expand inserted text twice.
	var pairs []string
	bodies := map[string][]byte{}
	for token, p := range m.pastes {
		if !strings.Contains(draft, token) {
			continue
		}
		if p.secret {
			r := m.redactions[token]
			if r == nil || r.id != s.Id || r.run != s.RunId {
				for _, b := range bodies {
					clear(b)
				}
				m.notice = "Secret expired; remove chip and use @redact again"
				return nil
			}
			bodies[token] = append([]byte(nil), r.body...)
		} else {
			pairs = append(pairs, token, expandPastes(token, map[string]*pastedText{token: p}))
		}
	}
	var target *api.Project
	for _, p := range append([]*api.Project{m.project}, m.panelProjects...) {
		if p != nil && p.Id == s.ProjectId {
			target = &api.Project{Id: p.Id, ContainerId: p.ContainerId, RemoteUser: p.RemoteUser}
			break
		}
	}
	if m.wisp == nil {
		m.wisp = &containerterm.WispPool{}
	}
	if m.redactStore == nil {
		m.redactStore = m.wisp
	}
	pool, lifetime, client, id, run := m.redactStore, m.ctx, m.client, s.Id, s.RunId
	request := core.ID()
	m.redactSending = true
	m.notice = "Preparing secret file…"
	m.input.Reset()
	m.queueInput(id, run, request, draft)
	return func() tea.Msg {
		tokens := make([]string, 0, len(bodies))
		for token := range bodies {
			tokens = append(tokens, token)
		}
		defer func() {
			for _, body := range bodies {
				clear(body)
			}
		}()
		ctx, cancel := context.WithTimeout(lifetime, 30*time.Second)
		defer cancel()
		paths := []string{}
		fail := func(err error) tea.Msg {
			cleanup, done := context.WithTimeout(lifetime, 3*time.Second)
			defer done()
			for _, path := range paths {
				_ = pool.DeleteSecret(lifetime, cleanup, target, path)
			}
			return redactSent{err: err, tokens: tokens, id: id, request: request}
		}
		for token, body := range bodies {
			path, err := pool.PutSecret(lifetime, ctx, target, id+"/"+run, body)
			if err != nil {
				return fail(err)
			}
			if path == "" {
				return fail(fmt.Errorf("secret path unavailable"))
			}
			paths = append(paths, path)
			pairs = append(pairs, token, "(secret placed at "+path+")")
		}
		text := strings.NewReplacer(pairs...).Replace(draft)
		_, err := client.Send(ctx, &api.Input{SessionId: id, RunId: run, ClientId: request, Text: text})
		if err != nil {
			return fail(fmt.Errorf("secret message send failed; not retried automatically"))
		}
		return redactSent{tokens: tokens, id: id, request: request}
	}
}
