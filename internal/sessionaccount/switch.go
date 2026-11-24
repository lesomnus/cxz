// Package sessionaccount re-homes a settled session onto another account of the
// same agent. The caller holds the session's offline supervisor lease: nothing
// may be running with the old account's configuration directory.
//
// A session's profile is keyed by its creation key, not by its account, so the
// profile itself stays. Inside it, every account has its own configuration
// directory. Switching moves the provider history -- the same folder a fork
// copies -- from the old account's directory to the new one. Credentials stay
// where they are: a login belongs to the account it was made for and is never
// carried to another one.
package sessionaccount

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
)

type Target struct {
	Account     string
	AuthBackend string
	AuthBinding string
}

// HistoryFolder is the provider's own conversation store inside its config dir.
func HistoryFolder(kind string) string {
	if kind == "codex" {
		return "sessions"
	}
	return "projects"
}

// Switch moves the provider history and returns the manifest to persist. It is
// idempotent: a retry after the history moved but before the manifest was saved
// finds the history already in place and completes.
func Switch(root string, session core.Session, target Target) (core.Session, error) {
	kind := session.Kind
	if kind == "" {
		kind = "claude"
	}
	if session.Account == "" {
		return session, fmt.Errorf("session has no account to switch from")
	}
	if session.CreateID == "" {
		return session, fmt.Errorf("session creation key required")
	}
	if err := accounts.Validate(target.Account, kind); err != nil {
		return session, err
	}
	if target.Account == session.Account {
		if target.AuthBackend != session.AuthBackend || target.AuthBinding != session.AuthBinding {
			return session, fmt.Errorf("session already uses this account with a different binding")
		}
		return session, nil
	}
	// A new binding is derived from the canonical project UUID; only bindings
	// made before project UUIDs existed used the legacy handle.
	if _, err := accounts.ResolveBinding(kind, target.AuthBackend, session.ProjectID, target.Account, target.AuthBinding); err != nil {
		return session, err
	}
	profile := accounts.SessionRoot(root, session.CreateID)
	if err := accounts.Prepare(profile, target.Account, kind); err != nil {
		return session, err
	}
	folder := HistoryFolder(kind)
	source := filepath.Join(accounts.Config(profile, session.Account), folder)
	destination := filepath.Join(accounts.Config(profile, target.Account), folder)
	moved, err := move(source, destination)
	if err != nil {
		return session, err
	}
	out := session
	out.Account, out.AuthBackend, out.AuthBinding = target.Account, target.AuthBackend, target.AuthBinding
	out.LegacyProjectID = ""
	if out.Fork != nil && out.Fork.NativePath != "" {
		// A fork that never ran still names its snapshot of the parent's
		// transcript; it lives in the history that just moved.
		fork := *out.Fork
		if rel, ok := below(source, fork.NativePath); ok {
			fork.NativePath = filepath.Join(destination, rel)
		}
		out.Fork = &fork
	}
	if moved {
		if err = core.SyncDir(filepath.Dir(source)); err != nil {
			return session, err
		}
		if err = core.SyncDir(filepath.Dir(destination)); err != nil {
			return session, err
		}
	}
	return out, nil
}

// move renames source to destination. Both are inside one session profile, so a
// rename is atomic. A missing source is fine when the session never ran, or when
// an earlier attempt already moved it.
func move(source, destination string) (bool, error) {
	info, err := os.Lstat(source)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("provider history is not a real directory")
	}
	if existing, err := os.Lstat(destination); err == nil {
		if !existing.IsDir() {
			return false, fmt.Errorf("target account's provider history is not a real directory")
		}
		// An empty directory carries nothing; anything else is a conversation
		// this switch must not merge into or overwrite.
		entries, err := os.ReadDir(destination)
		if err != nil {
			return false, err
		}
		if len(entries) > 0 {
			return false, fmt.Errorf("target account already holds provider history for this session")
		}
		if err = os.Remove(destination); err != nil {
			return false, err
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err = os.Rename(source, destination); err != nil {
		return false, fmt.Errorf("move provider history: %w", err)
	}
	return true, nil
}

func below(parent, path string) (string, bool) {
	rel, err := filepath.Rel(parent, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}
