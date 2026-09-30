package lifecycle

import (
	"context"
	"encoding/json"

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
func (s ProjectServer) purgeSession(ctx context.Context, spec []byte) (*resource.DockerReply, error) {
	var r sessionpurge.Request
	if len(spec) > sessionpurge.MaxSpec || json.Unmarshal(spec, &r) != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid session purge request")
	}
	if r.Session == "" {
		return nil, status.Error(codes.InvalidArgument, "name the session to purge")
	}
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
	resolved, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	s.shared.transition.Lock()
	defer s.shared.transition.Unlock()
	out, err := s.shared.runtime.Docker(ctx, &api.DockerInput{Action: "session-purge", Spec: resolved})
	if err != nil {
		return nil, err
	}
	var reply sessionpurge.Reply
	if err = json.Unmarshal([]byte(out.Status), &reply); err != nil {
		return nil, err
	}
	reply.Session = v.GetAlias()
	if reply.Session == "" {
		reply.Session = r.Session
	}
	if !r.DryRun {
		if err = s.eraseRecord(ctx, sessionRef(r.Session)); err != nil {
			return nil, err
		}
		reply.Targets = append(reply.Targets, sessionpurge.Target{Kind: "record", Path: "manager database", Files: 1})
	}
	b, err := json.Marshal(reply)
	if err != nil {
		return nil, err
	}
	return resource.DockerReply_builder{Status: ptr(string(b))}.Build(), nil
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
