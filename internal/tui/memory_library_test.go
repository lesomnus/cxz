package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/memorylib"
	"io"
	"strings"
	"testing"
)

type libraryClient struct {
	api.SessionsClient
	store *memorylib.Store
}

func (c *libraryClient) Library(ctx context.Context, id string, q memorylib.Request) (memorylib.Reply, error) {
	return c.store.Do(ctx, q)
}
func libraryModel(t *testing.T) *model {
	t.Helper()
	m := conversationModel()
	store := memorylib.New(t.TempDir(), core.Session{ID: "s", ProjectID: "p", Kind: "codex"})
	_, e := store.Do(context.Background(), memorylib.Request{Action: "update", Document: "handoff.md", Content: "PR still pending"})
	if e != nil {
		t.Fatal(e)
	}
	m.client = &libraryClient{SessionsClient: m.client, store: store}
	applyLibrary(t, m, m.openMemory(m.current()))
	return m
}
func applyLibrary(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	_, next := m.Update(cmd())
	if next != nil {
		applyLibrary(t, m, next)
	}
}
func TestLibraryBrowseDeleteRestoreAndExternalRefresh(t *testing.T) {
	m := libraryModel(t)
	if m.library == nil || len(m.library.data.Memories) != 1 {
		t.Fatal("library not opened")
	}
	applyLibrary(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyEnter}))
	applyLibrary(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if !strings.Contains(m.libraryScreen(), "PR still pending") {
		t.Fatal(m.libraryScreen())
	}
	m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.library.dialog != "delete" {
		t.Fatal("missing confirmation")
	}
	applyLibrary(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}))
	if len(m.library.data.Documents) != 0 {
		t.Fatal("document retained")
	}
	applyLibrary(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}))
	if len(m.library.data.Documents) != 1 {
		t.Fatal("trash missing")
	}
	applyLibrary(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")}))
	if len(m.library.data.Documents) != 0 {
		t.Fatal("not restored")
	}
}
func TestLibraryLateResultAndSnapshotName(t *testing.T) {
	m := libraryModel(t)
	cmd := m.loadLibrary(memorylib.Request{Action: "list"})
	m.libraryKey(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(cmd())
	if m.library != nil {
		t.Fatal("late response reopened library")
	}
	applyLibrary(t, m, m.openMemory(m.current()))
	m.libraryKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m.library.input.SetValue("배포 전")
	applyLibrary(t, m, m.libraryKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if len(m.library.data.Memories) != 2 {
		t.Fatal(m.library.message, m.library.data)
	}
}

type seedClient struct {
	api.SessionsClient
	id      string
	request memorylib.Request
}

func (c *seedClient) Library(_ context.Context, id string, q memorylib.Request) (memorylib.Reply, error) {
	c.id = id
	c.request = q
	return memorylib.Reply{}, nil
}
func TestLibrarySeedsNewSessionAfterAccountSelection(t *testing.T) {
	m := projectModel()
	client := &seedClient{SessionsClient: m.client}
	m.client = client
	m.seedMemory = "saved-source"
	m.createProjectSession = func(_ context.Context, _ string, account string, _ io.Reader, _ io.Writer, _ io.Writer) (*api.Session, error) {
		if account != "chosen" {
			t.Fatal(account)
		}
		return &api.Session{Id: "new-session", Agent: "codex"}, nil
	}
	batch := m.startAccountWorkflow("chosen", "codex", "", true)().(tea.BatchMsg)
	defer m.workflow.close()
	done := batch[0]().(workflowDone)
	if done.err != nil || client.id != "new-session" || client.request.Action != "fork" || client.request.ID != "saved-source" || m.seedMemory != "" {
		t.Fatal(done.err, client, m.seedMemory)
	}
}
