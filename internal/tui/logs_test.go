package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type logsClient struct {
	api.SessionsClient
	project bool
	err     error
}

func (c *logsClient) Logs(_ context.Context, r *api.LogsRequest, _ ...grpc.CallOption) (*api.LogsReply, error) {
	c.project = r.Project
	return &api.LogsReply{Text: "supervisor full details"}, c.err
}
func TestLogsRetainNoticeWithOldServerAndScroll(t *testing.T) {
	m := conversationModel()
	c := &logsClient{err: status.Error(codes.Unimplemented, "unknown method Logs")}
	m.client = c
	full := "Permission RPC unavailable: " + strings.Repeat("long installation instructions ", 40) + "END OF ERROR"
	m.notice = full
	m.Update(pulseTick{})
	m.notice = "new status"
	m.Update(pulseTick{})
	cmd := m.logsCommand("/logs")
	m.Update(cmd())
	if !strings.Contains(m.report.text, full) || !strings.Contains(m.report.text, "unknown method Logs") {
		t.Fatal(m.report.text)
	}
	m.width = 45
	before := m.view.YOffset
	first := ansi.Strip(m.reportView(strings.Repeat("\n", 12)))
	if strings.Contains(first, "…") {
		t.Fatal("logs clipped instead of wrapped")
	}
	m.reportKey(tea.KeyMsg{Type: tea.KeyEnd})
	m.reportView(strings.Repeat("\n", 12))
	if m.report.offset == 0 || m.view.YOffset != before {
		t.Fatal("scroll did not stay in overlay")
	}
	m.reportKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.report != nil {
		t.Fatal("overlay did not close")
	}
}
func TestLogsScopeRefreshAndStaleResults(t *testing.T) {
	m := conversationModel()
	c := &logsClient{}
	m.client = c
	m.current().ProjectId = "project"
	m.noticeLogs = []noticeLog{{session: "s", project: "project", text: "selected notice"}, {session: "other", project: "project", text: "sibling notice"}, {session: "foreign", project: "foreign", text: "foreign notice"}}
	cmd := m.logsCommand("/logs")
	old := cmd()
	m.Update(old)
	if strings.Contains(m.report.text, "sibling notice") || !strings.Contains(m.report.text, "selected notice") {
		t.Fatal(m.report.text)
	}
	cmd = m.logsCommand("/logs project")
	fresh := cmd()
	m.Update(fresh)
	if !c.project || !strings.Contains(m.report.text, "sibling notice") || strings.Contains(m.report.text, "foreign notice") {
		t.Fatal(m.report.text)
	}
	current := m.report
	m.Update(old)
	if m.report != current || !strings.Contains(m.report.text, "sibling notice") {
		t.Fatal("stale result replaced report")
	}
	refresh := m.reportKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if refresh == nil {
		t.Fatal("missing refresh")
	}
	result := refresh()
	m.reportKey(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(result)
	if m.report != nil {
		t.Fatal("late result reopened overlay")
	}
}
