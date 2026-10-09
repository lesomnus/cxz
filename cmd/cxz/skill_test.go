package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/xli"
	"google.golang.org/grpc"
)

type skillSink struct {
	api.SessionsClient
	call    string
	project string
	name    string
	enabled bool
}

func (s *skillSink) GetSkills(_ context.Context, r *api.SkillsInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	s.call, s.project = "GetSkills", r.Project
	return &api.SkillsReply{}, nil
}
func (s *skillSink) AddSkill(_ context.Context, r *api.SkillInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	s.call, s.project, s.name = "AddSkill", r.Project, r.Name
	return &api.SkillsReply{}, nil
}
func (s *skillSink) RemoveSkill(_ context.Context, r *api.SkillInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	s.call, s.project, s.name = "RemoveSkill", r.Project, r.Name
	return &api.SkillsReply{}, nil
}
func (s *skillSink) SetSkillDefault(_ context.Context, r *api.SkillDefaultInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	s.call, s.project, s.name, s.enabled = "SetSkillDefault", "", r.Name, r.Enabled
	return &api.SkillsReply{}, nil
}
func (s *skillSink) SetProjectSkill(_ context.Context, r *api.ProjectSkillInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	s.call, s.project, s.name, s.enabled = "SetProjectSkill", r.Project, r.Name, r.Enabled
	return &api.SkillsReply{}, nil
}
func (s *skillSink) ClearProjectSkill(_ context.Context, r *api.ClearProjectSkillInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	s.call, s.project, s.name = "ClearProjectSkill", r.Project, r.Name
	return &api.SkillsReply{}, nil
}

// The six commands stayed, and each one now names the call it makes. enable and
// disable split by scope: with no project they set the installation's default,
// and with one they decide for that project -- which the action string decided
// by whether a field happened to be empty.
func TestEachSkillCommandNamesItsCall(t *testing.T) {
	for _, tc := range []struct {
		op, project, want string
		enabled           bool
	}{
		{op: "list", want: "GetSkills"},
		{op: "list", project: "P", want: "GetSkills"},
		{op: "add", want: "AddSkill"},
		{op: "remove", want: "RemoveSkill"},
		{op: "enable", want: "SetSkillDefault", enabled: true},
		{op: "disable", want: "SetSkillDefault"},
		{op: "enable", project: "P", want: "SetProjectSkill", enabled: true},
		{op: "disable", project: "P", want: "SetProjectSkill"},
		{op: "inherit", project: "P", want: "ClearProjectSkill"},
	} {
		sink := &skillSink{}
		if _, err := callSkill(t.Context(), sink, tc.op, tc.project, "review"); err != nil {
			t.Fatal(tc.op, err)
		}
		if sink.call != tc.want {
			t.Fatalf("%s with project %q called %s, want %s", tc.op, tc.project, sink.call, tc.want)
		}
		if sink.project != tc.project {
			t.Fatalf("%s lost the project: %q", tc.op, sink.project)
		}
		if sink.enabled != tc.enabled {
			t.Fatalf("%s carried enabled=%v", tc.op, sink.enabled)
		}
	}
}

// Printing has to tell a project's own decision apart from what it inherits.
// An entry that agrees with the default is not an entry that decided.
func TestPrintedStateSeparatesOwnFromInherited(t *testing.T) {
	on := true
	var buf bytes.Buffer
	c := &xli.Command{Name: "list", Writer: &buf}
	err := printSkills(c, &api.SkillsReply{Entries: []*api.SkillEntry{
		{Name: "review", Effective: true, Override: &on},
		{Name: "deploy", Effective: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		switch {
		case strings.HasPrefix(line, "review"):
			if !strings.Contains(line, "on (project)") {
				t.Fatal("a project's own decision is not reported as its own:", line)
			}
		case strings.HasPrefix(line, "deploy"):
			if !strings.Contains(line, "on (inherited)") {
				t.Fatal("an inherited entry is not reported as inherited:", line)
			}
		default:
			t.Fatal("unexpected line:", line)
		}
	}
}
