package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// An account owns its sessions, not the projects in which those sessions ran.
// Each session goes through the same runtime/data purge as Session.Erase.
// The account remains available for retry until all cleanup succeeds.
func (s AccountServer) Erase(ctx context.Context, ref *resource.AccountRef) (*resource.AccountEraseResponse, error) {
	if ref == nil || (len(ref.GetId()) == 0 && ref.GetAlias() == "") {
		return nil, status.Error(codes.InvalidArgument, "name the account to erase")
	}
	if err := s.effect(); err != nil {
		return nil, err
	}
	runtime, ok := s.shared.runtime.(interface {
		EraseAccount(context.Context, string, string, string) error
	})
	if !ok {
		return nil, status.Error(codes.Unimplemented, "account erasure requires an updated Manager")
	}
	if authority, ok := s.shared.runtime.(interface{ AccountEraseAllowed(context.Context) error }); ok {
		if err := authority.AccountEraseAllowed(ctx); err != nil {
			return nil, err
		}
	}
	s.shared.transition.Lock()
	defer s.shared.transition.Unlock()
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	a, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: ref, Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if status.Code(err) == codes.NotFound {
		return &resource.AccountEraseResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	canonical := resource.AccountRef_builder{Id: a.GetId()}.Build()
	var sessions []*resource.SessionRef
	after := ""
	for {
		page, err := s.Next().Session().List(ctx, resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Account: canonical}.Build()}, Size: 200, After: after}.Build())
		if err != nil {
			return nil, err
		}
		for _, session := range page.GetItems() {
			sessions = append(sessions, resource.SessionRef_builder{Id: session.GetId()}.Build())
		}
		if page.GetNext() == "" {
			break
		}
		if page.GetNext() == after {
			return nil, status.Error(codes.Internal, "session cursor did not advance")
		}
		after = page.GetNext()
	}
	sessionServer := SessionServer{s.Layer, s.Next().Session()}
	for _, session := range sessions {
		if _, err := sessionServer.eraseSession(ctx, session); err != nil {
			return nil, err
		}
	}
	if err := runtime.EraseAccount(ctx, a.GetAlias(), a.GetAgent(), a.GetAuthBackend()); err != nil {
		return nil, err
	}

	s.shared.snapshotMu.Lock()
	defer s.shared.snapshotMu.Unlock()
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	var bindings []*resource.AuthBindingRef
	after = ""
	for {
		page, err := s.Next().AuthBinding().List(ctx, resource.AuthBindingListRequest_builder{Filters: []*resource.AuthBindingFilter{resource.AuthBindingFilter_builder{Account: canonical}.Build()}, Size: 200, After: after}.Build())
		if err != nil {
			return nil, err
		}
		for _, binding := range page.GetItems() {
			bindings = append(bindings, resource.AuthBindingRef_builder{Id: binding.GetId()}.Build())
		}
		if page.GetNext() == "" {
			break
		}
		if page.GetNext() == after {
			return nil, status.Error(codes.Internal, "binding cursor did not advance")
		}
		after = page.GetNext()
	}
	// Publish disappearance and purge registry rows in one transaction. A DB
	// error must not hide the account that still needs cleanup on a retry.
	driver, tx, err := dialect.BeginTx(ctx, entsql.OpenDB(dialect.SQLite, s.shared.db))
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	store, err := enttx.Rebind(s.Next(), driver)
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if _, err := store.AuthBinding().Erase(ctx, binding); err != nil {
			return nil, err
		}
	}
	if _, err := store.Account().Erase(ctx, canonical); err != nil {
		return nil, err
	}
	if s.shared.notifications != nil {
		if err := driver.Exec(ctx, "DELETE FROM session_notification_cursor WHERE session_id IN (SELECT id FROM session WHERE account_id=?)", []any{projectid.String(a.GetId())}, nil); err != nil {
			return nil, err
		}
	}
	for _, query := range []string{
		"DELETE FROM notificationreceiptrecord WHERE notification_id IN (SELECT id FROM notificationrecord WHERE source_id IN (SELECT id FROM session WHERE account_id=?))",
		"DELETE FROM notificationrecord WHERE source_id IN (SELECT id FROM session WHERE account_id=?)",
		"DELETE FROM sessionread WHERE session_id IN (SELECT id FROM session WHERE account_id=?)",
		"DELETE FROM audit WHERE object_id IN (SELECT id FROM session WHERE account_id=?)",
		"DELETE FROM session WHERE account_id=?",
		"DELETE FROM audit WHERE object_id IN (SELECT id FROM authbinding WHERE account_id=?)",
		"DELETE FROM audit WHERE object_id=?",
		"DELETE FROM authbinding WHERE account_id=?",
		"DELETE FROM account WHERE id=?",
	} {
		if err := driver.Exec(ctx, query, []any{projectid.String(a.GetId())}, nil); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return resource.AccountEraseResponse_builder{Erased: ptr(true)}.Build(), nil
}
