package server

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/skillconfig"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The library and who sees what are the installation's, so they are answered
// by the manager. A project runtime answers only SyncSkills: it is the side
// being told, and what it is told is already resolved for it.

func (s *Server) requireManagerSkills() error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "the skills library belongs to an installed host manager")
	}
	return nil
}

func (s *Server) GetSkills(ctx context.Context, r *api.SkillsInput) (*api.SkillsReply, error) {
	if err := s.requireManagerSkills(); err != nil {
		return nil, err
	}
	return s.manager.GetSkills(ctx, r)
}

func (s *Server) AddSkill(ctx context.Context, r *api.SkillInput) (*api.SkillsReply, error) {
	if err := s.requireManagerSkills(); err != nil {
		return nil, err
	}
	return s.manager.AddSkill(ctx, r)
}

func (s *Server) RemoveSkill(ctx context.Context, r *api.SkillInput) (*api.SkillsReply, error) {
	if err := s.requireManagerSkills(); err != nil {
		return nil, err
	}
	return s.manager.RemoveSkill(ctx, r)
}

func (s *Server) SetSkillDefault(ctx context.Context, r *api.SkillDefaultInput) (*api.SkillsReply, error) {
	if err := s.requireManagerSkills(); err != nil {
		return nil, err
	}
	return s.manager.SetSkillDefault(ctx, r)
}

func (s *Server) SetProjectSkill(ctx context.Context, r *api.ProjectSkillInput) (*api.SkillsReply, error) {
	if err := s.requireManagerSkills(); err != nil {
		return nil, err
	}
	return s.manager.SetProjectSkill(ctx, r)
}

func (s *Server) ClearProjectSkill(ctx context.Context, r *api.ClearProjectSkillInput) (*api.SkillsReply, error) {
	if err := s.requireManagerSkills(); err != nil {
		return nil, err
	}
	return s.manager.ClearProjectSkill(ctx, r)
}

// SyncSkills stores what the manager decided this project may see, for the
// next agent launch to lay out. A manager has nothing to store: it is the
// caller, and answering here would mean an installation delivering to itself.
func (s *Server) SyncSkills(ctx context.Context, r *api.SyncSkillsInput) (*api.Receipt, error) {
	if s.manager != nil {
		return s.manager.SyncSkills(ctx, r)
	}
	b, err := filemap.Decode(r.Bundle)
	if err != nil {
		return nil, err
	}
	err = skillconfig.SaveRuntime(s.root, b)
	return &api.Receipt{Status: "Skills saved for new agent launches"}, err
}

// RenderDevcontainer reads files the manager host wrote, which a project
// runtime cannot see.
func (s *Server) RenderDevcontainer(ctx context.Context, r *api.RenderDevcontainerInput) (*api.RenderDevcontainerReply, error) {
	if s.manager == nil {
		return nil, status.Error(codes.FailedPrecondition, "the devcontainer a project runs under is read by an installed host manager")
	}
	return s.manager.RenderDevcontainer(ctx, r)
}
