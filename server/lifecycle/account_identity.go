package lifecycle

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
)

// Public aliases can change; authentication paths, grants, binding IDs and
// running agents keep their original profile key. This private mapping contains
// no credentials and is retained across metadata edits and Manager restarts.
func accountIdentitySchema(ctx context.Context, db *sql.DB) error {
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS account_runtime_identity (account_id TEXT PRIMARY KEY REFERENCES account(id) ON DELETE CASCADE, profile_key TEXT NOT NULL UNIQUE)`,
		`INSERT OR IGNORE INTO account_runtime_identity(account_id,profile_key) SELECT id,alias FROM account`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func runtimeAccountRef(key string) *resource.AccountRef {
	return resource.AccountRef_builder{Id: resourceID(9, key)}.Build()
}

func (s Layer) accountProfile(ctx context.Context, a *resource.Account) (string, error) {
	id := projectid.String(a.GetId())
	var key string
	err := s.shared.db.QueryRowContext(ctx, `SELECT profile_key FROM account_runtime_identity WHERE account_id=?`, id).Scan(&key)
	if !errors.Is(err, sql.ErrNoRows) {
		return key, err
	}
	if _, err := s.shared.db.ExecContext(ctx, `INSERT OR IGNORE INTO account_runtime_identity(account_id,profile_key) VALUES(?,?)`, id, a.GetAlias()); err != nil {
		return "", err
	}
	err = s.shared.db.QueryRowContext(ctx, `SELECT profile_key FROM account_runtime_identity WHERE account_id=?`, id).Scan(&key)
	return key, err
}

// The caller holds shared.mu. Reusing a renamed account's old public alias
// allocates a different profile and UUID instead of sharing its authentication.
func (s Layer) newAccountProfile(ctx context.Context, alias string) (string, error) {
	var exists bool
	if err := s.shared.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account WHERE id=?)`, projectid.String(resourceID(9, alias))).Scan(&exists); err != nil {
		return "", err
	}
	if !exists {
		return alias, nil
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "account-" + hex.EncodeToString(random[:]), nil
}

func (s Layer) accountAlias(ctx context.Context, key string) (string, error) {
	a, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: runtimeAccountRef(key), Select: resource.AccountSelect_builder{Alias: ptr(true)}.Build()}.Build())
	if err != nil {
		return "", fmt.Errorf("resolve account profile: %w", err)
	}
	return a.GetAlias(), nil
}
