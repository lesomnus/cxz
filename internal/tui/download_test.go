package tui

import (
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/transport"
	"strings"
	"testing"
	"time"
)

func TestDownloadArgumentHints(t *testing.T) {
	for _, value := range []string{"/download ", "/download ./re", "/download /tmp/한", "/download ~/rep", "/download \"/tmp/my fi", "/download `/tmp/my fi"} {
		token, ok := pathTokenAt(value, len([]rune(value)))
		if !ok || !token.download || token.host {
			t.Fatal(value, token, ok)
		}
	}
	m := pathModel()
	m.input.SetValue("/download /tmp/re")
	token, _ := pathTokenAt(m.input.Value(), len([]rune(m.input.Value())))
	m.pathHints = &pathHints{token: token}
	m.completePathOption(pathOption{text: "/tmp/report file.zip"}, true)
	if m.input.Value() != "/download /tmp/report file.zip" {
		t.Fatal(m.input.Value())
	}
	if got := downloadArgument("/download `/tmp/report file.zip`"); got != "/tmp/report file.zip" {
		t.Fatal(got)
	}
	m.syncPathHints()
	if m.pathHints != nil {
		t.Fatal("completed file keeps swallowing Enter")
	}
	if strings.Contains(m.input.Value(), "`") {
		t.Fatal("added backtick to bare path")
	}
}
func TestDownloadPreventsAutomaticRestart(t *testing.T) {
	m := conversationModel()
	now := time.Now()
	m.autoStarted = now.Add(-time.Hour)
	m.lastUIInput = now.Add(-time.Hour)
	if !m.frontendIdle(now) {
		t.Fatal("fixture should be idle")
	}
	m.download = &fileDownload{}
	// The download itself must keep the frontend alive even when there is no draft.
	if m.frontendIdle(now) {
		t.Fatal("download allowed automatic restart")
	}
}

func TestDownloadRelativePathHintsUseProjectWorkspace(t *testing.T) {
	m := pathModel()
	m.ctx = transport.WithRemote(m.ctx)
	c := &remotePathClient{}
	m.client = c
	m.project = &api.Project{Id: m.current().ProjectId, ContainerId: "container", RemoteUser: "dev", RemoteWorkspace: "/work/repo"}
	m.input.SetValue("/download ./dist/")
	m.syncPathHints()
	m.program = nil
	result := m.fetchPathHints(pathHintDue{m.pathHints.generation})().(pathHintResult)
	if result.err != nil || c.path != "/work/repo/dist" {
		t.Fatal(c.path, result.err)
	}
	m.receivePathHints(result)
	if options := m.pathOptions(); len(options) != 1 || options[0].text != "./dist/remote/" {
		t.Fatal(options)
	}
}
