package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/internal/sessionpurge"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Deleting a session is a tombstone, and that is deliberate: the journal, the
// agent profile and the uploads outlive the record so a mistaken delete stays
// recoverable. Purge is the deliberate opposite and the only path in cxz that
// destroys a conversation.
//
// The order is the safety argument. Stop the agent and stop listing the session,
// destroy the data, and erase the record last. A failure between any two steps
// leaves a tombstone pointing at less data than before -- never a listed session
// whose journal is already gone.
func (s SessionServer) Purge(ctx context.Context, r *resource.SessionPurgeRequest) (*resource.SessionPurgeReply, error) {
	return ProjectServer{s.Layer, s.Next().Project()}.purgeSession(ctx, r)
}

func (s ProjectServer) purgeSession(ctx context.Context, request *resource.SessionPurgeRequest) (*resource.SessionPurgeReply, error) {
	handle := sessionHandle(request.GetRef())
	if handle == "" {
		return nil, status.Error(codes.InvalidArgument, "name the session to purge")
	}
	r := sessionpurge.Request{Session: handle, DryRun: request.GetDryRun()}
	v, err := func() (*resource.Session, error) {
		s.shared.transition.RLock()
		defer s.shared.transition.RUnlock()
		return s.resolveSession(ctx, r.Session)
	}()
	if err != nil {
		return nil, err
	}
	// The runtime knows the session only by its own id, whatever handle the
	// caller typed.
	r.Session = v.GetRuntimeId()
	if !r.DryRun {
		if _, err = s.Session().Erase(ctx, sessionRef(r.Session)); err != nil {
			return nil, err
		}
	}
	s.shared.transition.Lock()
	defer s.shared.transition.Unlock()
	out, err := s.shared.runtime.PurgeSession(ctx, &api.SessionPurgeInput{SessionId: r.Session, DryRun: r.DryRun})
	if err != nil {
		return nil, err
	}
	reply := sessionpurge.Reply{Session: r.Session, DryRun: out.DryRun, Retained: out.Retained}
	for _, t := range out.Targets {
		reply.Targets = append(reply.Targets, sessionpurge.Target{
			Kind: t.Kind, Path: t.Path, Files: int(t.Files), Bytes: t.Bytes,
		})
	}
	if !r.DryRun {
		if err = s.eraseRecord(ctx, sessionRef(r.Session)); err != nil {
			return nil, err
		}
		reply.Targets = append(reply.Targets, sessionpurge.Target{Kind: "record", Path: "manager database", Files: 1})
	}
	answer := resource.SessionPurgeReply_builder{
		Ref: request.GetRef(), DryRun: ptr(reply.DryRun), Retained: reply.Retained,
	}
	for _, t := range reply.Targets {
		answer.Targets = append(answer.Targets, resource.SessionPurgeTarget_builder{
			Kind: ptr(t.Kind), Path: ptr(t.Path), Files: ptr(int32(t.Files)), Bytes: ptr(t.Bytes),
		}.Build())
	}
	return answer.Build(), nil
}

// sessionHandle is whichever handle the caller had at hand. A purge resolves it
// itself rather than through the usual getter, because the id is tried before
// the alias: resolving the wrong session is the one mistake with no undo.
func sessionHandle(ref *resource.SessionRef) string {
	switch {
	case ref == nil:
		return ""
	case ref.GetRuntimeId() != "":
		return ref.GetRuntimeId()
	case ref.GetAlias() != "":
		return ref.GetAlias()
	case len(ref.GetId()) > 0:
		return string(ref.GetId())
	}
	return ""
}

// resolveSession accepts whichever handle the caller had at hand. The id is
// tried first and the alias only if nothing answers to it, because a purge that
// resolved the wrong session would be the one mistake with no undo.
func (s ProjectServer) resolveSession(ctx context.Context, handle string) (*resource.Session, error) {
	// Reconcile first, so a dry run reports on a session the runtime has but the
	// projection has not caught up with yet.
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	get := func(ref *resource.SessionRef) (*resource.Session, error) {
		return s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	}
	v, err := get(resource.SessionRef_builder{RuntimeId: &handle}.Build())
	if status.Code(err) == codes.NotFound && sessionalias.Valid(handle) {
		return get(resource.SessionRef_builder{Alias: &handle}.Build())
	}
	return v, err
}

// eraseRecord overwrites the title before erasing the row. Erasing alone hides a
// session from every read, but the title is the caller's own words; a purge that
// left it sitting in the database file would not be the deletion it promised.
func (s ProjectServer) eraseRecord(ctx context.Context, ref *resource.SessionRef) error {
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	_, err := s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Name: ptr("purged"), AliasNull: ptr(true), StatusNull: ptr(true), Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build())
	if err != nil {
		return err
	}
	_, err = s.Next().Session().Erase(ctx, ref)
	return err
}
