package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/auxkind"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Aux travels as an enum on the resource API and as a name on the runtime view
// models, so every one of these is the same call twice with the kinds
// translated. Nothing here parses a payload to decide where a call goes: the
// session a task belongs to is a field.

func (c *Client) AuxRun(ctx context.Context, r *api.AuxRunInput, opts ...grpc.CallOption) (*api.AuxState, error) {
	v, err := c.sessions.AuxRun(ctx, resource.AuxRunRequest_builder{
		Ref: sr(r.SessionId), Kinds: auxkind.Enums(r.Kinds), Text: &r.Text,
	}.Build(), opts...)
	return auxState(v, err)
}

func (c *Client) AuxStatus(ctx context.Context, r *api.AuxStatusInput, opts ...grpc.CallOption) (*api.AuxState, error) {
	v, err := c.sessions.AuxStatus(ctx, resource.AuxStatusRequest_builder{
		Ref: sr(r.SessionId), AfterTurn: &r.AfterTurn, Limit: &r.Limit,
	}.Build(), opts...)
	return auxState(v, err)
}

func (c *Client) AuxPrefer(ctx context.Context, r *api.AuxPreferInput, opts ...grpc.CallOption) (*api.AuxState, error) {
	v, err := c.sessions.AuxPrefer(ctx, resource.AuxPreferRequest_builder{
		Ref: sr(r.SessionId), Preferences: auxPreferences(r.Preferences),
	}.Build(), opts...)
	return auxState(v, err)
}

func (c *Client) AuxCancel(ctx context.Context, r *api.AuxCancelInput, opts ...grpc.CallOption) (*api.AuxState, error) {
	v, err := c.sessions.AuxCancel(ctx, resource.AuxCancelRequest_builder{
		Ref: sr(r.SessionId), AuxId: &r.AuxId,
	}.Build(), opts...)
	return auxState(v, err)
}

// AuxForget is not on the resource API. Only a session deletion sends it, and
// a deletion is served by the same process that would receive it.
func (c *Client) AuxForget(ctx context.Context, r *api.AuxForgetInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	return nil, errAuxForget
}

func (c *Client) AuxConfig(ctx context.Context, r *api.Empty, opts ...grpc.CallOption) (*api.AuxConfigReply, error) {
	v, err := c.projects.AuxConfig(ctx, resource.AuxConfigRequest_builder{}.Build(), opts...)
	return auxConfig(v, err)
}

func (c *Client) AuxSetConfig(ctx context.Context, r *api.AuxSetConfigInput, opts ...grpc.CallOption) (*api.AuxConfigReply, error) {
	v, err := c.projects.AuxSetConfig(ctx, resource.AuxSetConfigRequest_builder{
		Profiles: auxProfilesOf(r.Profiles),
	}.Build(), opts...)
	return auxConfig(v, err)
}

func (c *Client) AuxModels(ctx context.Context, r *api.AuxModelsInput, opts ...grpc.CallOption) (*api.AuxModelsReply, error) {
	v, err := c.projects.AuxModels(ctx, resource.AuxModelsRequest_builder{Account: &r.Account}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	out := &api.AuxModelsReply{NeedsLogin: v.GetNeedsLogin()}
	for _, m := range v.GetModels() {
		out.Models = append(out.Models, &api.AuxModel{
			Id: m.GetId(), ResolvedId: m.GetResolvedId(), Name: m.GetName(),
			Efforts: m.GetEfforts(), DefaultEffort: m.GetDefaultEffort(), Default: m.GetDefault(),
		})
	}
	return out, nil
}

func (c *Client) AuxLoginInfo(ctx context.Context, r *api.AuxLoginInfoInput, opts ...grpc.CallOption) (*api.AuxLoginInfoReply, error) {
	v, err := c.projects.AuxLoginInfo(ctx, resource.AuxLoginInfoRequest_builder{Account: &r.Account}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.AuxLoginInfoReply{
		Owner: v.GetOwner(), Account: v.GetAccount(),
		Agent: v.GetAgent(), Backend: v.GetBackend(),
	}, nil
}

func auxState(v *resource.AuxState, err error) (*api.AuxState, error) {
	if err != nil {
		return nil, err
	}
	out := &api.AuxState{Title: v.GetTitle(), Message: v.GetMessage(), Current: aux(v.GetCurrent())}
	for _, s := range v.GetSummaries() {
		out.Summaries = append(out.Summaries, &api.AuxSummary{RunId: s.GetRunId(), Turn: s.GetTurn(), ResponseSeq: s.GetResponseSeq(), Text: s.GetText()})
	}
	for _, r := range v.GetRecent() {
		out.Recent = append(out.Recent, aux(r))
	}
	for _, p := range v.GetPreferences() {
		out.Preferences = append(out.Preferences, &api.AuxPreference{
			Kind: auxkind.Name(p.GetKind()), Enabled: p.GetEnabled(), SinceMs: auxMillis(p.GetSince()),
		})
	}
	return out, nil
}

func aux(v *resource.Aux) *api.Aux {
	if v == nil {
		return nil
	}
	out := &api.Aux{
		Id: v.GetId(), SessionId: string(v.GetSessionId()), RunId: v.GetRunId(), Turn: v.GetTurn(),
		ResponseSeq: v.GetResponseSeq(),
		Revision:    v.GetRevision(), State: v.GetState(), Message: v.GetMessage(),
		Kinds: auxkind.Names(v.GetKinds()),
	}
	for _, r := range v.GetResults() {
		out.Results = append(out.Results, &api.AuxResult{
			Kind: auxkind.Name(r.GetKind()), Text: r.GetText(), Truncated: r.GetTruncated(),
		})
	}
	for _, u := range v.GetUsage() {
		out.Usage = append(out.Usage, &api.AuxUsage{
			Account: u.GetAccount(), Model: u.GetModel(), Kind: auxkind.Name(u.GetKind()), Data: u.GetData(),
		})
	}
	return out
}

func auxConfig(v *resource.AuxConfigReply, err error) (*api.AuxConfigReply, error) {
	if err != nil {
		return nil, err
	}
	out := &api.AuxConfigReply{Revision: v.GetRevision(), Message: v.GetMessage(), Owner: v.GetOwner()}
	for _, p := range v.GetProfiles() {
		out.Profiles = append(out.Profiles, auxProfile(p))
	}
	return out, nil
}

func auxProfile(p *resource.AuxProfile) *api.AuxProfile {
	if p == nil {
		return nil
	}
	return &api.AuxProfile{
		Kind: auxkind.Name(p.GetKind()), Enabled: p.GetEnabled(), Account: p.GetAccount(),
		Agent: p.GetAgent(), Backend: p.GetBackend(), Model: p.GetModel(), Effort: p.GetEffort(),
		SinceMs: auxMillis(p.GetSince()),
	}
}

func auxProfileOf(p *api.AuxProfile) *resource.AuxProfile {
	if p == nil {
		return nil
	}
	kind := auxkind.Of(p.Kind)
	return resource.AuxProfile_builder{
		Kind: &kind, Enabled: &p.Enabled, Account: &p.Account, Agent: &p.Agent,
		Backend: &p.Backend, Model: &p.Model, Effort: &p.Effort,
	}.Build()
}

func auxProfilesOf(profiles []*api.AuxProfile) []*resource.AuxProfile {
	out := make([]*resource.AuxProfile, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, auxProfileOf(p))
	}
	return out
}

func auxPreferences(preferences []*api.AuxPreference) []*resource.AuxPreference {
	out := make([]*resource.AuxPreference, 0, len(preferences))
	for _, p := range preferences {
		kind := auxkind.Of(p.Kind)
		out = append(out, resource.AuxPreference_builder{Kind: &kind, Enabled: &p.Enabled}.Build())
	}
	return out
}

func auxMillis(t *timestamppb.Timestamp) int64 {
	if t == nil {
		return 0
	}
	return t.AsTime().UnixMilli()
}

// errAuxForget is a refusal rather than a silent success: a caller that wanted
// aux state removed and was told "fine" would leave it behind.
var errAuxForget = status.Error(codes.Unimplemented, "aux state is forgotten by the deletion that owns the session")

func (c *Client) AuxEvents(ctx context.Context, r *api.AuxStatusInput, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.AuxState], error) {
	s, err := c.sessions.AuxEvents(ctx, resource.AuxStatusRequest_builder{
		Ref: sr(r.SessionId), AfterTurn: &r.AfterTurn, Limit: &r.Limit,
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return auxEventStream{s}, nil
}

// The stream carries the same message the unary call answers with, so the
// adapter is the same conversion applied per item.
type auxEventStream struct {
	grpc.ServerStreamingClient[resource.AuxState]
}

func (s auxEventStream) Recv() (*api.AuxState, error) {
	v, err := s.ServerStreamingClient.Recv()
	if err != nil {
		return nil, err
	}
	return auxState(v, nil)
}
