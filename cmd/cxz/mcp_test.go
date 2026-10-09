package main

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type mcpSink struct {
	api.SessionsClient
	calls   []string
	project string
	session string
	id      string
	enabled bool
	server  *api.McpServer
}

func (s *mcpSink) reply() *api.McpServersReply {
	return &api.McpServersReply{Entries: []*api.McpEntry{{
		Id: "a", Server: &api.McpServer{Name: "a", Kind: "stdio", Command: "serve"}, Effective: true,
	}}}
}

func (s *mcpSink) GetMcpServers(_ context.Context, r *api.McpServersInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	s.calls, s.project = append(s.calls, "GetMcpServers"), r.Project
	return s.reply(), nil
}
func (s *mcpSink) PutMcpServer(_ context.Context, r *api.PutMcpServerInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	s.calls, s.id, s.server = append(s.calls, "PutMcpServer"), r.Id, r.Server
	return s.reply(), nil
}
func (s *mcpSink) RemoveMcpServer(_ context.Context, r *api.McpServerInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	s.calls, s.id = append(s.calls, "RemoveMcpServer"), r.Id
	return s.reply(), nil
}
func (s *mcpSink) SetMcpServerDefault(_ context.Context, r *api.McpServerDefaultInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	s.calls, s.id, s.enabled = append(s.calls, "SetMcpServerDefault"), r.Id, r.Enabled
	return s.reply(), nil
}
func (s *mcpSink) SetProjectMcpServer(_ context.Context, r *api.ProjectMcpServerInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	s.calls, s.project, s.id, s.enabled = append(s.calls, "SetProjectMcpServer"), r.Project, r.Id, r.Enabled
	return s.reply(), nil
}
func (s *mcpSink) ClearProjectMcpServer(_ context.Context, r *api.ClearProjectMcpServerInput, _ ...grpc.CallOption) (*api.McpServersReply, error) {
	s.calls, s.project, s.id = append(s.calls, "ClearProjectMcpServer"), r.Project, r.Id
	return s.reply(), nil
}
func (s *mcpSink) McpSessions(_ context.Context, r *api.McpSessionsInput, _ ...grpc.CallOption) (*api.McpSessionsReply, error) {
	s.calls, s.project = append(s.calls, "McpSessions"), r.Project
	return &api.McpSessionsReply{Sessions: []*api.McpSessionStatus{{SessionId: "S", Pending: true}}}, nil
}
func (s *mcpSink) McpLogs(_ context.Context, r *api.McpLogsInput, _ ...grpc.CallOption) (*api.McpLogsReply, error) {
	s.calls, s.session, s.id = append(s.calls, "McpLogs"), r.SessionId, r.Id
	return &api.McpLogsReply{Text: "stderr", Message: "note"}, nil
}
func (s *mcpSink) RestartMcp(_ context.Context, r *api.RestartMcpInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	s.calls, s.session, s.id = append(s.calls, "RestartMcp"), r.SessionId, r.Id
	return &api.Receipt{Status: "closed"}, nil
}

// The eight commands stayed, and each one now names the call it makes. enable
// and disable split by scope: with no project they set the installation's
// default, and with one they decide for that project -- which the action string
// decided by whether a field happened to be empty.
func TestEachMcpCommandNamesItsCall(t *testing.T) {
	for _, tc := range []struct {
		op, project, want string
		enabled           bool
	}{
		{op: "list", want: "GetMcpServers"},
		{op: "remove", want: "RemoveMcpServer"},
		{op: "enable", want: "SetMcpServerDefault", enabled: true},
		{op: "disable", want: "SetMcpServerDefault"},
		{op: "enable", project: "P", want: "SetProjectMcpServer", enabled: true},
		{op: "disable", project: "P", want: "SetProjectMcpServer"},
		{op: "inherit", project: "P", want: "ClearProjectMcpServer"},
	} {
		sink := &mcpSink{}
		if _, err := callMCP(t.Context(), sink, tc.op, tc.project, "", "a", nil); err != nil {
			t.Fatal(tc.op, err)
		}
		if sink.calls[0] != tc.want {
			t.Fatalf("%s with project %q called %s, want %s", tc.op, tc.project, sink.calls[0], tc.want)
		}
		if sink.enabled != tc.enabled {
			t.Fatalf("%s carried enabled=%v", tc.op, sink.enabled)
		}
		// The live sessions are a second question, asked only when a project
		// was named. A global list does not reach for a container.
		wantSessions := tc.project != ""
		if got := len(sink.calls) == 2 && sink.calls[1] == "McpSessions"; got != wantSessions {
			t.Fatalf("%s with project %q made %v", tc.op, tc.project, sink.calls)
		}
	}
}

// Logs and a restart are the session's. They name no project, so there is no
// pair that can disagree about which connection is meant.
func TestMcpSessionCommandsNameOnlyASession(t *testing.T) {
	for _, tc := range []struct{ op, want string }{
		{"logs", "McpLogs"},
		{"restart", "RestartMcp"},
	} {
		sink := &mcpSink{}
		out, err := callMCP(t.Context(), sink, tc.op, "P", "S", "a", nil)
		if err != nil {
			t.Fatal(tc.op, err)
		}
		if len(sink.calls) != 1 || sink.calls[0] != tc.want {
			t.Fatalf("%s made %v", tc.op, sink.calls)
		}
		if sink.session != "S" || sink.id != "a" {
			t.Fatalf("%s addressed %q %q", tc.op, sink.session, sink.id)
		}
		if sink.project != "" {
			t.Fatalf("%s named a project: %q", tc.op, sink.project)
		}
		if tc.op == "logs" && out.Log != "stderr" {
			t.Fatal("the logs did not come back:", out.Log)
		}
	}
}
