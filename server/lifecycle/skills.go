package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

// The library belongs to the installation, so it lives on the project service
// rather than on a session. A ref scopes it: with one, the answer is what that
// project sees; without one, the installation's own defaults.

func (s ProjectServer) GetSkills(ctx context.Context, r *resource.SkillsRequest) (*resource.SkillsReply, error) {
	project, err := s.projectScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.GetSkills(ctx, &api.SkillsInput{Project: project}))
}

func (s ProjectServer) AddSkill(ctx context.Context, r *resource.SkillRequest) (*resource.SkillsReply, error) {
	project, err := s.changeScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.AddSkill(ctx, &api.SkillInput{Project: project, Name: r.GetName()}))
}

func (s ProjectServer) RemoveSkill(ctx context.Context, r *resource.SkillRequest) (*resource.SkillsReply, error) {
	project, err := s.changeScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.RemoveSkill(ctx, &api.SkillInput{Project: project, Name: r.GetName()}))
}

func (s ProjectServer) SetSkillDefault(ctx context.Context, r *resource.SkillDefaultRequest) (*resource.SkillsReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.SetSkillDefault(ctx, &api.SkillDefaultInput{
		Name: r.GetName(), Enabled: r.GetEnabled(),
	}))
}

func (s ProjectServer) SetProjectSkill(ctx context.Context, r *resource.ProjectSkillRequest) (*resource.SkillsReply, error) {
	project, err := s.changeScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.SetProjectSkill(ctx, &api.ProjectSkillInput{
		Project: project, Name: r.GetName(), Enabled: r.GetEnabled(),
	}))
}

func (s ProjectServer) ClearProjectSkill(ctx context.Context, r *resource.ClearProjectSkillRequest) (*resource.SkillsReply, error) {
	project, err := s.changeScope(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.ClearProjectSkill(ctx, &api.ClearProjectSkillInput{
		Project: project, Name: r.GetName(),
	}))
}

func (s ProjectServer) SyncSkills(ctx context.Context, r *resource.SyncSkillsRequest) (*resource.SyncSkillsReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.SyncSkills(ctx, &api.SyncSkillsInput{Bundle: r.GetBundle()})
	if err != nil {
		return nil, err
	}
	return resource.SyncSkillsReply_builder{Status: &out.Status}.Build(), nil
}

// projectScope resolves an optional ref to the runtime id an installation's
// settings are keyed by. No ref is the installation itself, which is a scope
// and not a missing argument.
func (s ProjectServer) projectScope(ctx context.Context, ref *resource.ProjectRef) (string, error) {
	if ref == nil {
		return "", nil
	}
	v, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{
		Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build(),
	}.Build())
	if err != nil {
		return "", err
	}
	return v.GetRuntimeId(), nil
}

func (s ProjectServer) changeScope(ctx context.Context, ref *resource.ProjectRef) (string, error) {
	if err := s.effect(); err != nil {
		return "", err
	}
	return s.projectScope(ctx, ref)
}

func skillsReply(v *api.SkillsReply, err error) (*resource.SkillsReply, error) {
	if err != nil {
		return nil, err
	}
	out := resource.SkillsReply_builder{Message: &v.Message}
	for _, e := range v.Entries {
		out.Entries = append(out.Entries, resource.SkillEntry_builder{
			Name: &e.Name, Description: &e.Description,
			Override: e.Override, Effective: &e.Effective,
		}.Build())
	}
	return out.Build(), nil
}
