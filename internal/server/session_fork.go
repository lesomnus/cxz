package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionfork"
	"github.com/lesomnus/cxz/internal/sessionstore"
	"github.com/lesomnus/cxz/internal/supervisor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) ForkSession(ctx context.Context, r *api.SessionForkInput) (*api.Session, error) {
	if s.manager != nil {
		return s.manager.ForkSession(ctx, r)
	}
	if r.ClientId == "" || len(r.ClientId) > 1024 || r.RunId == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id and client_id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw []byte
	err := s.db.QueryRowContext(ctx, "SELECT manifest FROM sessions WHERE create_id=?", r.ClientId).Scan(&raw)
	if err == nil {
		var child core.Session
		if err = json.Unmarshal(raw, &child); err != nil {
			return nil, err
		}
		if child.Fork == nil || child.Fork.SessionID != r.SessionId || child.Fork.RunID != r.RunId {
			return nil, status.Error(codes.AlreadyExists, "client_id reused")
		}
		return s.snapshot(ctx, child)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	source, err := s.manifest(ctx, r.SessionId)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte("cxz.fork:" + source.ID + "\x00" + r.ClientId))
	id := hex.EncodeToString(sum[:12])
	// A lost supervisor reply may have completed the snapshot before the manager
	// registered it. The deterministic identity recovers that exact snapshot.
	var child core.Session
	raw, err = os.ReadFile(filepath.Join(core.Dir(s.root, id), "session.json"))
	if err == nil {
		if err = json.Unmarshal(raw, &child); err != nil {
			return nil, err
		}
		if child.ID != id || child.CreateID != r.ClientId || child.Fork == nil || child.Fork.SessionID != source.ID || child.Fork.RunID != r.RunId {
			return nil, status.Error(codes.AlreadyExists, "fork identity reused")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	} else {
		// Local OAuth belongs to the new session. Reuse the normal session-login
		// challenge and creation key; never duplicate a rotating refresh token.
		target := source
		target.CreateID = r.ClientId
		backend, err := accounts.ResolveBinding(target.Kind, target.AuthBackend, target.AuthProjectID(), target.Account, target.AuthBinding)
		if err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if err = backend.Check(accounts.AuthRoot(s.root, target), target.Account); err != nil {
			return nil, authCheckError(err, target.AuthBackend, r.ClientId)
		}
		request := sessionfork.Request{ID: id, ClientID: r.ClientId, RunID: r.RunId}
		var live core.Snapshot
		if supervisor.Call(ctx, s.root, source.ID, "status", nil, &live) == nil {
			if err = supervisor.Call(ctx, s.root, source.ID, "fork", request, &child); err != nil {
				if strings.Contains(err.Error(), "unknown command") {
					return nil, status.Error(codes.FailedPrecondition, "this supervisor predates Fork; restart the session after updating the project runtime")
				}
				return nil, status.Error(codes.FailedPrecondition, err.Error())
			}
		} else {
			lock, e := core.Lock(filepath.Join(core.Dir(s.root, source.ID), "supervisor.lock"))
			if e != nil {
				return nil, status.Error(codes.FailedPrecondition, "session is starting or stopping; retry when settled")
			}
			// From the parent's store, which is the record. The parent is not
			// running -- that is why this branch exists -- so nothing is
			// appending to it while it is read. See cxz#278.
			parent, e := sessionstore.Open(ctx, sessionstore.Path(core.Dir(s.root, source.ID)), source.ID, false)
			if e != nil {
				lock.Close()
				return nil, e
			}
			events, e := parent.Events(ctx)
			parent.Close()
			if e != nil {
				lock.Close()
				return nil, e
			}
			snapshot := supervisor.Replay(events)
			if snapshot.State != "idle" && snapshot.State != "stopped" && snapshot.State != "failed" && snapshot.State != "interrupted" {
				lock.Close()
				return nil, status.Error(codes.FailedPrecondition, "fork requires a settled conversation")
			}
			child, e = sessionfork.Prepare(ctx, s.root, source, snapshot, events, request)
			lock.Close()
			if e != nil {
				return nil, status.Error(codes.FailedPrecondition, e.Error())
			}
		}
	}
	child.Title = source.Title
	if err = core.WriteJSON(filepath.Join(core.Dir(s.root, id), "session.json"), child); err != nil {
		return nil, err
	}
	raw, err = json.Marshal(child)
	if err != nil {
		return nil, err
	}
	if _, err = s.db.ExecContext(ctx, "INSERT INTO sessions VALUES(?,?,?)", child.ID, raw, child.CreateID); err != nil {
		return nil, err
	}
	// Persist before starting: a disconnected caller can find this child and must
	// never blindly repeat an operation that may already have created a thread.
	return s.launch(ctx, child)
}
