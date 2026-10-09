package lifecycle

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/auxkind"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Aux is a model task done beside a session. The session-scoped calls are on
// the session's own service, which is where a session's operations belong, and
// the configuration calls are on the project service, where the rest of an
// installation's settings already are.
//
// A kind arrives as an enum and leaves as a name, so a kind this build does not
// know is refused here rather than mistaken for another one further down.

func auxKinds(kinds []resource.AuxKind) ([]string, error) {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		name := auxkind.Name(k)
		if name == "" {
			return nil, status.Errorf(codes.InvalidArgument, "unknown aux kind %v", k)
		}
		out = append(out, name)
	}
	return out, nil
}

func (s SessionServer) AuxRun(ctx context.Context, r *resource.AuxRunRequest) (*resource.AuxState, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	kinds, err := auxKinds(r.GetKinds())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.AuxRun(ctx, &api.AuxRunInput{SessionId: v.GetRuntimeId(), Kinds: kinds, Text: r.GetText()})
	if err != nil {
		return nil, err
	}
	return s.auxState(ctx, r.GetRef(), out)
}

func (s SessionServer) AuxStatus(ctx context.Context, r *resource.AuxStatusRequest) (*resource.AuxState, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.AuxStatus(ctx, &api.AuxStatusInput{
		SessionId: v.GetRuntimeId(), AfterTurn: r.GetAfterTurn(), Limit: r.GetLimit(),
	})
	if err != nil {
		return nil, err
	}
	return s.auxState(ctx, r.GetRef(), out)
}

func (s SessionServer) AuxPrefer(ctx context.Context, r *resource.AuxPreferRequest) (*resource.AuxState, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	in := &api.AuxPreferInput{SessionId: v.GetRuntimeId()}
	for _, p := range r.GetPreferences() {
		name := auxkind.Name(p.GetKind())
		if name == "" {
			return nil, status.Errorf(codes.InvalidArgument, "unknown aux kind %v", p.GetKind())
		}
		in.Preferences = append(in.Preferences, &api.AuxPreference{Kind: name, Enabled: p.GetEnabled()})
	}
	out, err := s.shared.runtime.AuxPrefer(ctx, in)
	if err != nil {
		return nil, err
	}
	return s.auxState(ctx, r.GetRef(), out)
}

func (s SessionServer) AuxCancel(ctx context.Context, r *resource.AuxCancelRequest) (*resource.AuxState, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.AuxCancel(ctx, &api.AuxCancelInput{SessionId: v.GetRuntimeId(), AuxId: r.GetAuxId()})
	if err != nil {
		return nil, err
	}
	return s.auxState(ctx, r.GetRef(), out)
}

// auxState records a generated title on the session it names before answering.
// The title is a result of the task rather than something read out of an opaque
// reply, so this does not have to guess whether one arrived.
func (s SessionServer) auxState(ctx context.Context, ref *resource.SessionRef, out *api.AuxState) (*resource.AuxState, error) {
	if out.Title != "" {
		current, err := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{
			Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true)}.Build(),
		}.Build())
		if err != nil {
			return nil, err
		}
		if current.GetName() != out.Title {
			if _, err := s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{
				Ref: ref, Name: ptr(out.Title), DateUpdatedForce: ptr(true),
			}.Build()); err != nil {
				return nil, err
			}
		}
	}
	reply := resource.AuxState_builder{
		Title: ptr(out.Title), Message: ptr(out.Message), Current: auxOf(out.Current),
	}
	for _, v := range out.Summaries {
		reply.Summaries = append(reply.Summaries, resource.AuxSummary_builder{
			RunId: &v.RunId, Turn: &v.Turn, Text: &v.Text,
		}.Build())
	}
	for _, v := range out.Preferences {
		kind := auxkind.Of(v.Kind)
		reply.Preferences = append(reply.Preferences, resource.AuxPreference_builder{
			Kind: &kind, Enabled: &v.Enabled, Since: auxSince(v.SinceMs),
		}.Build())
	}
	return reply.Build(), nil
}

func (s ProjectServer) AuxConfig(ctx context.Context, r *resource.AuxConfigRequest) (*resource.AuxConfigReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.AuxConfig(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	return auxConfigReply(out), nil
}

func (s ProjectServer) AuxSetConfig(ctx context.Context, r *resource.AuxSetConfigRequest) (*resource.AuxConfigReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	in := &api.AuxSetConfigInput{}
	for _, p := range r.GetProfiles() {
		// A profile that is being turned on has to name a registered account;
		// one being turned off does not, because there is nothing to reach.
		profile, err := s.auxProfile(ctx, p, p.GetEnabled())
		if err != nil {
			return nil, err
		}
		in.Profiles = append(in.Profiles, profile)
	}
	out, err := s.shared.runtime.AuxSetConfig(ctx, in)
	if err != nil {
		return nil, err
	}
	return auxConfigReply(out), nil
}

func (s ProjectServer) AuxModels(ctx context.Context, r *resource.AuxModelsRequest) (*resource.AuxModelsReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	agent, backend, err := s.auxAccount(ctx, r.GetAccount())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.AuxModels(ctx, &api.AuxModelsInput{Account: r.GetAccount(), Agent: agent, Backend: backend})
	if err != nil {
		return nil, err
	}
	reply := resource.AuxModelsReply_builder{NeedsLogin: ptr(out.NeedsLogin)}
	for _, m := range out.Models {
		reply.Models = append(reply.Models, resource.AuxModel_builder{
			Id: &m.Id, ResolvedId: &m.ResolvedId, Name: &m.Name,
			Efforts: m.Efforts, DefaultEffort: &m.DefaultEffort, Default: &m.Default,
		}.Build())
	}
	return reply.Build(), nil
}

func (s ProjectServer) AuxLoginInfo(ctx context.Context, r *resource.AuxLoginInfoRequest) (*resource.AuxLoginInfoReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	agent, backend, err := s.auxAccount(ctx, r.GetAccount())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.AuxLoginInfo(ctx, &api.AuxLoginInfoInput{Account: r.GetAccount(), Agent: agent, Backend: backend})
	if err != nil {
		return nil, err
	}
	return resource.AuxLoginInfoReply_builder{
		Owner: ptr(out.Owner), Account: ptr(out.Account),
		Agent: ptr(out.Agent), Backend: ptr(out.Backend),
	}.Build(), nil
}

// auxAccount reads what an account authenticates as off the registered account
// rather than taking a caller's word for it, which is what keeps a request from
// naming one account and authenticating as another.
func (s ProjectServer) auxAccount(ctx context.Context, account string) (string, string, error) {
	if account == "" {
		return "", "", status.Error(codes.InvalidArgument, "select a registered account")
	}
	a, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{
		Ref: accountRef(account), Select: resource.AccountSelect_builder{All: ptr(true)}.Build(),
	}.Build())
	if err != nil {
		return "", "", err
	}
	return a.GetAgent(), a.GetAuthBackend(), nil
}

// auxProfile fills in what an account authenticates as, for the same reason
// auxAccount does.
func (s ProjectServer) auxProfile(ctx context.Context, p *resource.AuxProfile, required bool) (*api.AuxProfile, error) {
	kind := auxkind.Name(p.GetKind())
	if kind == "" {
		return nil, status.Errorf(codes.InvalidArgument, "unknown aux kind %v", p.GetKind())
	}
	out := &api.AuxProfile{
		Kind: kind, Enabled: p.GetEnabled(), Account: p.GetAccount(),
		Model: p.GetModel(), Effort: p.GetEffort(),
	}
	if out.Account == "" {
		if required {
			return nil, status.Error(codes.InvalidArgument, "select a registered account")
		}
		return out, nil
	}
	agent, backend, err := s.auxAccount(ctx, out.Account)
	if err != nil {
		return nil, err
	}
	out.Agent, out.Backend = agent, backend
	return out, nil
}

func auxConfigReply(out *api.AuxConfigReply) *resource.AuxConfigReply {
	reply := resource.AuxConfigReply_builder{
		Revision: &out.Revision, Message: &out.Message, Owner: &out.Owner,
	}
	for _, p := range out.Profiles {
		reply.Profiles = append(reply.Profiles, auxProfileOf(p))
	}
	return reply.Build()
}

func auxProfileOf(p *api.AuxProfile) *resource.AuxProfile {
	if p == nil {
		return nil
	}
	kind := auxkind.Of(p.Kind)
	return resource.AuxProfile_builder{
		Kind: &kind, Enabled: &p.Enabled, Account: &p.Account, Agent: &p.Agent,
		Backend: &p.Backend, Model: &p.Model, Effort: &p.Effort, Since: auxSince(p.SinceMs),
	}.Build()
}

func auxOf(a *api.Aux) *resource.Aux {
	if a == nil {
		return nil
	}
	out := resource.Aux_builder{
		Id: &a.Id, SessionId: []byte(a.SessionId), RunId: &a.RunId, Turn: &a.Turn,
		Revision: &a.Revision, State: &a.State, Message: &a.Message,
		Kinds: auxkind.Enums(a.Kinds),
	}
	for _, r := range a.Results {
		kind := auxkind.Of(r.Kind)
		out.Results = append(out.Results, resource.AuxResult_builder{
			Kind: &kind, Text: &r.Text, Truncated: &r.Truncated,
		}.Build())
	}
	for _, u := range a.Usage {
		kind := auxkind.Of(u.Kind)
		out.Usage = append(out.Usage, resource.AuxUsage_builder{
			Account: &u.Account, Model: &u.Model, Kind: &kind, Data: u.Data,
		}.Build())
	}
	return out.Build()
}

func auxSince(ms int64) *timestamppb.Timestamp {
	if ms == 0 {
		return nil
	}
	return timestamppb.New(time.UnixMilli(ms))
}
