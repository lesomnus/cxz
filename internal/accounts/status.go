package accounts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// AuthStatus describes local credential readiness; no token or subject leaves it.
type AuthStatus struct {
	State, Scope, Message                  string
	CredentialPresent, CanLogin, CanLogout bool
}

func CentralStatus(root, alias string) (AuthStatus, error) {
	if err := Validate(alias, "codex"); err != nil {
		return AuthStatus{}, err
	}
	out := AuthStatus{State: "login_required", Scope: "account", CanLogin: true}
	path := filepath.Join(Config(centralRoot(root), alias), filename("codex"))
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		out.Message = "Log in to this account to use it."
		return out, nil
	} else if err != nil {
		return AuthStatus{}, fmt.Errorf("cannot inspect saved account credentials")
	}
	lock, err := centralLock(root, alias)
	if err != nil {
		return AuthStatus{State: "busy", Scope: "account", Message: "Account authentication is busy."}, nil
	}
	defer lock.Close()
	if _, err := os.Lstat(path); err == nil {
		out.CanLogout = true
	} else if !os.IsNotExist(err) {
		return AuthStatus{}, fmt.Errorf("cannot inspect saved account credentials")
	}
	if _, err := CentralToken(root, alias); err == nil {
		out.State, out.CredentialPresent, out.CanLogin, out.CanLogout = "ready", true, false, true
		out.Message = "Saved login is present. Provider validity has not been checked."
	} else if out.CanLogout {
		out.Message = "Saved credentials could not be validated. Log in again."
	} else {
		out.Message = "Log in to this account to use it."
	}
	return out, nil
}

// CentralLogout retains the immutable identity pin and non-authentication config.
// Broker refresh/login and logout use the same lock. Existing provider tokens
// remain outside our control, but every later broker request requires credentials.
func CentralLogout(ctx context.Context, root, alias string) error {
	if err := Validate(alias, "codex"); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lock, err := centralLock(root, alias)
	if err != nil {
		return fmt.Errorf("account authentication is busy")
	}
	defer lock.Close()
	for _, path := range []string{
		filepath.Join(Config(centralRoot(root), alias), filename("codex")),
		filepath.Join(Dir(centralRoot(root), alias), "source.json"),
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove saved account credentials")
		}
	}
	return revokeAccountGrants(ctx, root, alias)
}
