package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

// The library belongs to the installation, not to a project, so it has its own
// service rather than a corner of the project one. `in` says whose view to
// answer with: a project id or workspace path for that project's, nothing for
// the installation's own defaults. It never says what to write.
// See docs/plans/service-boundaries.md.
//
// There is no entity behind this service, so there is no generated server under
// it either. The runtime resolves `in` itself, which is why nothing here looks
// a project up first.
type SkillServer struct {
	Layer
	resource.UnimplementedSkillServiceServer
}

func (s Layer) Skill() resource.SkillServiceServer { return SkillServer{Layer: s} }

func (s SkillServer) List(ctx context.Context, r *resource.SkillListRequest) (*resource.SkillList, error) {
	return skillsReply(s.shared.runtime.GetSkills(ctx, &api.SkillsInput{Project: r.GetIn()}))
}

func (s SkillServer) Add(ctx context.Context, r *resource.SkillAddRequest) (*resource.SkillList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.AddSkill(ctx, &api.SkillInput{Project: r.GetIn(), Name: r.GetName()}))
}

func (s SkillServer) Remove(ctx context.Context, r *resource.SkillRemoveRequest) (*resource.SkillList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.RemoveSkill(ctx, &api.SkillInput{Project: r.GetIn(), Name: r.GetName()}))
}

func (s SkillServer) SetDefault(ctx context.Context, r *resource.SkillDefaultRequest) (*resource.SkillList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.SetSkillDefault(ctx, &api.SkillDefaultInput{
		Name: r.GetName(), Enabled: r.GetEnabled(),
	}))
}

func (s SkillServer) Enable(ctx context.Context, r *resource.SkillEnableRequest) (*resource.SkillList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.SetProjectSkill(ctx, &api.ProjectSkillInput{
		Project: r.GetIn(), Name: r.GetName(), Enabled: r.GetEnabled(),
	}))
}

func (s SkillServer) Clear(ctx context.Context, r *resource.SkillClearRequest) (*resource.SkillList, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return skillsReply(s.shared.runtime.ClearProjectSkill(ctx, &api.ClearProjectSkillInput{
		Project: r.GetIn(), Name: r.GetName(),
	}))
}

func skillsReply(v *api.SkillsReply, err error) (*resource.SkillList, error) {
	if err != nil {
		return nil, err
	}
	out := resource.SkillList_builder{Message: &v.Message}
	for _, e := range v.Entries {
		out.Entries = append(out.Entries, resource.SkillEntry_builder{
			Name: &e.Name, Description: &e.Description,
			Override: e.Override, Effective: &e.Effective,
		}.Build())
	}
	return out.Build(), nil
}
