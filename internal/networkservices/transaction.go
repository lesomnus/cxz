package networkservices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

type transaction struct {
	Owner   string
	Name    string
	Role    string
	OldID   string
	NewID   string
	NewName string
	Image   string
}

func transactionPath(root, role string) string {
	return filepath.Join(root, "network-services", role+".json")
}
func pending(root string) bool {
	for _, role := range []string{"web", "remote"} {
		if _, err := os.Stat(transactionPath(root, role)); !os.IsNotExist(err) {
			return true
		}
	}
	return false
}
func saveTransaction(root string, tx transaction) error {
	path := transactionPath(root, tx.Role)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return core.WriteJSON(path, tx)
}
func discardTransaction(root, role string) error {
	err := os.Remove(transactionPath(root, role))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RecoverLocked is also used by host up/down commands while their independent
// volume lock holder is alive. Finish rollback before applying a human's new
// intent; a later tick must not resurrect a service they explicitly removed.
func RecoverLocked(ctx context.Context, root, name, owner string) error {
	for _, role := range []string{"web", "remote"} {
		if err := recoverService(ctx, root, name, owner, role); err != nil {
			return err
		}
	}
	return nil
}
func recoverService(ctx context.Context, root, name, owner, role string) error {
	b, err := os.ReadFile(transactionPath(root, role))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var tx transaction
	if err = json.Unmarshal(b, &tx); err != nil {
		return err
	}
	canonical := serviceName(name, role)
	if tx.Owner != owner || tx.Name != canonical || tx.Role != role || tx.OldID == "" || tx.Image == "" || !strings.HasPrefix(tx.NewName, canonical+"-next-") {
		return fmt.Errorf("invalid %s recovery transaction", role)
	}
	old, oldErr := dockerx.Owned(ctx, tx.OldID, owner, "")
	if oldErr != nil && !dockerx.Missing(oldErr) {
		return oldErr
	}
	if oldErr == nil && old.Config.Labels["cxz.role"] != role {
		return fmt.Errorf("old %s recovery role changed", role)
	}
	replacement := tx.NewID
	if replacement == "" {
		replacement = tx.NewName
	}
	next, nextErr := dockerx.Owned(ctx, replacement, owner, "")
	if nextErr != nil && !dockerx.Missing(nextErr) {
		return nextErr
	}
	if nextErr == nil && (next.Config.Labels["cxz.role"] != role || next.Config.Image != tx.Image) {
		return fmt.Errorf("replacement %s identity changed", role)
	}
	current, exists, err := ownedService(ctx, name, owner, role)
	if err != nil {
		return err
	}
	if exists && current.ID != tx.OldID && (nextErr != nil || current.ID != next.ID) {
		return fmt.Errorf("%s canonical container changed during recovery", role)
	}
	if dockerx.Missing(oldErr) {
		// A crash after successful readiness/removing the backup only owes cleanup.
		if nextErr == nil && exists && current.ID == next.ID && current.State.Running {
			var origin *url.URL
			if role == "web" {
				origin, err = webOrigin(next)
				if err != nil {
					return err
				}
			}
			probe, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			if err = ready(probe, next.ID, role, origin); err != nil {
				return err
			}
			return discardTransaction(root, role)
		}
		return fmt.Errorf("%s previous container missing; recovery required", role)
	}
	if nextErr == nil {
		if _, err = dockerx.Run(ctx, "rm", "-f", next.ID); err != nil {
			return err
		}
	}
	if strings.TrimPrefix(old.Name, "/") != canonical {
		if _, err = dockerx.Run(ctx, "rename", old.ID, canonical); err != nil {
			return err
		}
	}
	if !old.State.Running {
		if _, err = dockerx.Run(ctx, "start", old.ID); err != nil {
			return err
		}
	}
	return discardTransaction(root, role)
}
