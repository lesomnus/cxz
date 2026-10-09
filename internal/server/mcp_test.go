package server

import (
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Registrations are the installation's. A project runtime has none, so it
// refuses rather than answering with an empty list, which a caller would read
// as "nothing is registered".
func TestMcpRegistrationsNeedAManager(t *testing.T) {
	s := &Server{}
	if _, err := s.GetMcpServers(t.Context(), &api.McpServersInput{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.PutMcpServer(t.Context(), &api.PutMcpServerInput{Id: "a"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.SetMcpServerDefault(t.Context(), &api.McpServerDefaultInput{Id: "a"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.ClearProjectMcpServer(t.Context(), &api.ClearProjectMcpServerInput{Project: "p", Id: "a"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
}

// A project runtime keeps what it was told, as given, for the next agent
// launch. The digest the launch is compared against is computed from that, so a
// definition that arrived changed would make every session look pending.
func TestSyncMcpServersIsStoredAsGiven(t *testing.T) {
	root := t.TempDir()
	s := &Server{root: root}
	in := &api.SyncMcpServersInput{Servers: map[string]*api.McpServer{
		"a": {Name: "a", Kind: "stdio", Enabled: true, Command: "serve", Args: []string{"--x"}, Env: map[string]string{"TOKEN": "secret"}},
		"b": {Name: "b", Kind: "http", Enabled: true, Url: "https://example.test/mcp"},
	}}
	if _, err := s.SyncMcpServers(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	got, err := mcpconfig.LoadRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 2 {
		t.Fatal("not everything was stored:", got.Servers)
	}
	a := got.Servers["a"]
	if a.Command != "serve" || len(a.Args) != 1 || a.Env["TOKEN"] != "secret" {
		t.Fatal("a definition arrived changed:", a)
	}
	if got.Servers["b"].URL != "https://example.test/mcp" {
		t.Fatal("the URL did not survive the trip:", got.Servers["b"])
	}
}

// A kind this build does not know is refused by validation rather than stored
// as something it is not.
func TestSyncMcpServersRefusesAnUnknownKind(t *testing.T) {
	s := &Server{root: t.TempDir()}
	if _, err := s.SyncMcpServers(t.Context(), &api.SyncMcpServersInput{
		Servers: map[string]*api.McpServer{"a": {Name: "a", Kind: "telepathy"}},
	}); err == nil {
		t.Fatal("stored a connection kind it cannot make")
	}
}
