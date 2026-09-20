package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/logview"
	"github.com/lesomnus/cxz/internal/transport"
)

type noticeLog struct {
	session, project, text string
	at                     time.Time
}
type logsResult struct {
	report *reportOverlay
	text   string
}

func (m *model) rememberNotice() {
	if m.notice == "" {
		m.lastLoggedNotice = ""
		return
	}
	session, project := "", ""
	if s := m.current(); s != nil {
		session = s.Id
		project = s.ProjectId
	}
	if project == "" && m.project != nil {
		project = m.project.Id
	}
	key := m.notice
	if key == m.lastLoggedNotice {
		return
	}
	m.lastLoggedNotice = key
	text := m.notice
	if len(text) > logview.FileLimit {
		text = text[:logview.FileLimit] + "\n[notice truncated at 64 KiB]"
	}
	m.noticeLogs = append(m.noticeLogs, noticeLog{session, project, text, time.Now()})
	if len(m.noticeLogs) > 200 {
		m.noticeLogs = append([]noticeLog(nil), m.noticeLogs[len(m.noticeLogs)-200:]...)
	}
}
func (m *model) logsCommand(text string) tea.Cmd {
	if text != "/logs" && text != "/logs project" {
		m.openReport("/logs", "Usage: /logs | /logs project")
		return nil
	}
	m.rememberNotice()
	m.openReport(text, "")
	return m.loadLogs()
}
func (m *model) loadLogs() tea.Cmd {
	p := m.report
	if p == nil {
		return nil
	}
	s := m.current()
	if s == nil {
		p.text = "No session selected."
		return nil
	}
	project := p.title == "/logs project"
	projectID := s.ProjectId
	if projectID == "" && m.project != nil {
		projectID = m.project.Id
	}
	var local logview.Report
	var notices []string
	for _, n := range m.noticeLogs {
		if (!project && n.session == s.Id) || (project && projectID != "" && n.project == projectID) {
			notices = append(notices, fmt.Sprintf("%s [session %s] %s", n.at.Format("01-02 15:04:05"), n.session, n.text))
		}
	}
	if len(notices) == 0 {
		notices = append(notices, "(no notices collected by this TUI)")
	}
	local.Add("TUI notices · current TUI lifetime · latest 200", strings.Join(notices, "\n\n"))
	initial := local.String()
	p.text = initial + "\nLoading server logs…"
	// Each refresh owns a new overlay identity, so older results cannot replace it.
	next := *p
	m.report = &next
	p = m.report
	client, ctx, id, pool := m.client, m.ctx, s.Id, m.wisp
	helperProjectID := projectID
	if !transport.IsRemote(m.contextFor(s.Id)) {
		helperProjectID = m.localProject(&api.Project{Id: projectID}).Id
	} else {
		pool = nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		var report logview.Report
		report.Add("Local diagnostics", initial)
		if project {
			wisp := "No Wisp helper diagnostics collected by this TUI. Historical Wisp logs were not persisted."
			if pool != nil {
				wisp = pool.Logs(helperProjectID)
			}
			report.Add("Wisp · this TUI's project connections", wisp)
		}
		reply, err := client.Logs(ctx, &api.LogsRequest{SessionId: id, Project: project})
		if err != nil {
			report.Add("Server logs unavailable", err.Error()+"\nOlder servers require a cxz manager/runtime update. Local notices remain available above.")
		} else {
			report.Add("Server diagnostics", reply.Text)
		}
		return logsResult{p, report.String()}
	}
}
