package cxzupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type FrozenProject struct {
	ID, Container, User string
	Before              Health
}
type ManagerTransaction struct {
	Old                                         Health
	ID, Owner, Name, OldID, NewID, State, Error string
	Release                                     Release
	Projects                                    []FrozenProject
}

func ManagerTransactionPath(root string) string { return filepath.Join(root, "manager-update.json") }
func readManagerTransaction(root string) (ManagerTransaction, error) {
	var tx ManagerTransaction
	b, e := os.ReadFile(ManagerTransactionPath(root))
	if e != nil {
		return tx, e
	}
	e = json.Unmarshal(b, &tx)
	return tx, e
}
func ManagerHelperName(owner string) string { return "cxz-" + owner[:12] + "-update" }
func ScheduleManager(ctx context.Context, root, name, owner string, r Release) error {
	if len(owner) < 12 {
		return fmt.Errorf("invalid installation owner")
	}
	socket, e := dockerSocket()
	if e != nil {
		return e
	}
	old, e := dockerx.Owned(ctx, name, owner, "")
	if e != nil {
		return e
	}
	tx, e := readManagerTransaction(root)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if tx.ID != "" && tx.State != "healthy" && tx.State != "rolled_back" && tx.Release.Revision != r.Revision {
		return fmt.Errorf("previous manager update needs recovery")
	}
	if tx.ID == "" || tx.State == "healthy" || tx.State == "rolled_back" {
		tx = ManagerTransaction{ID: core.ID(), Owner: owner, Name: strings.TrimPrefix(old.Name, "/"), OldID: old.ID, Release: r, State: "staged"}
		if e = core.WriteJSON(ManagerTransactionPath(root), tx); e != nil {
			return e
		}
	}
	helper := ManagerHelperName(owner)
	if v, err := dockerx.Inspect(ctx, helper); err == nil {
		if v.Config.Labels["cxz.owner"] != owner || v.Config.Labels["cxz.role"] != "update" {
			return fmt.Errorf("foreign update helper")
		}
		if v.State.Running {
			return nil
		}
		if _, err = dockerx.Run(ctx, "rm", v.ID); err != nil {
			return err
		}
	}
	// Pull before starting the helper; unavailable artifacts never stop the manager.
	if _, e = dockerx.Run(ctx, "pull", r.Image); e != nil {
		return e
	}
	args := []string{"run", "-d", "--name", helper, "--restart", "on-failure:3", "--label", "cxz.owner=" + owner, "--label", "cxz.role=update", "--entrypoint", "/usr/local/bin/cxz", "--mount", "type=bind,source=" + socket + ",target=/var/run/docker.sock"}
	found := false
	for _, m := range old.Mounts {
		if m.Destination == root || m.Destination == "/cxz/tools" {
			if m.Type != "volume" {
				return fmt.Errorf("manager update requires named state/tools volumes")
			}
			args = append(args, "--mount", "type=volume,source="+m.Name+",target="+m.Destination)
			if m.Destination == root {
				found = true
			}
		}
	}
	if !found {
		return fmt.Errorf("manager state volume unavailable")
	}
	args = append(args, old.Config.Image, "_update-manager", root)
	_, e = dockerx.Run(ctx, args...)
	return e
}
func freezeProjects(ctx context.Context, root string, tx *ManagerTransaction) error {
	containers, e := dockerx.List(ctx, "label=cxz.owner="+tx.Owner)
	if e != nil {
		return e
	}
	// Always enumerate again: a previous helper may have persisted only a prefix.
	for _, v := range containers {
		id := v.Config.Labels["cxz.project"]
		if id == "" || !v.State.Running {
			continue
		}
		hasState := false
		for _, m := range v.Mounts {
			if m.Destination == "/cxz/state" {
				hasState = true
			}
		}
		if !hasState {
			continue
		}
		b, e := dockerx.Run(ctx, "exec", v.ID, "stat", "-c", "%u", "/cxz/state/data/run/update.sock")
		if e != nil {
			return fmt.Errorf("project %s requires bootstrap: %w", id, e)
		}
		user := strings.TrimSpace(string(b))
		h, e := ContainerHealth(ctx, v.ID, user, "/cxz/state/data", "prepare", tx.ID)
		if e != nil {
			return e
		}
		found := false
		for i, p := range tx.Projects {
			if p.Container != v.ID {
				continue
			}
			if !reflect.DeepEqual(p.Before.Sessions, h.Sessions) {
				return fmt.Errorf("project sessions changed during interrupted update")
			}
			tx.Projects[i].User = user
			found = true
		}
		if !found {
			tx.Projects = append(tx.Projects, FrozenProject{ID: id, Container: v.ID, User: user, Before: h})
		}
		if e = core.WriteJSON(ManagerTransactionPath(root), tx); e != nil {
			return e
		}
	}
	return nil
}
func releaseProjects(ctx context.Context, tx ManagerTransaction) {
	for _, p := range tx.Projects {
		_, _ = ContainerHealth(ctx, p.Container, p.User, "/cxz/state/data", "release", tx.ID)
	}
}
func managerHealthy(ctx context.Context, root string, tx ManagerTransaction) error {
	h, e := Call(ctx, root, "health", "")
	if e != nil {
		return e
	}
	if h.Build.Revision != tx.Release.Revision || h.Build.Protocol != Protocol || h.Build.Schema != Schema {
		return fmt.Errorf("manager build/compatibility mismatch")
	}
	for _, p := range tx.Projects {
		h, e = ContainerHealth(ctx, p.Container, p.User, "/cxz/state/data", "status", "")
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(h.Sessions, p.Before.Sessions) {
			return fmt.Errorf("project session identity changed")
		}
	}
	return nil
}
func waitManager(ctx context.Context, root string, tx ManagerTransaction) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if e := managerHealthy(ctx, root, tx); e == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
func ReplaceManager(ctx context.Context, root string) (result error) {
	lock, e := core.Lock(filepath.Join(root, "rollout.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	tx, e := readManagerTransaction(root)
	if e != nil {
		return e
	}
	if e = tx.Release.Validate(); e != nil {
		return e
	}
	if tx.State == "healthy" || tx.State == "rolled_back" {
		return nil
	}
	touched := tx.State == "starting" || tx.State == "rolling_back"
	save := func(state string) error { tx.State = state; return core.WriteJSON(ManagerTransactionPath(root), tx) }
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if !touched || tx.State == "healthy" || tx.State == "rolled_back" {
			releaseProjects(cleanup, tx)
			_, _ = Call(cleanup, root, "release", tx.ID)
		}
		if result != nil && !touched {
			tx.Projects = nil
			_ = save("staged")
		}
		if result != nil {
			s, err := Load(root)
			if err == nil {
				s.State = "waiting"
				s.Reason = result.Error()
				_ = Save(root, s)
			}
		}
	}()
	old, e := dockerx.Owned(ctx, tx.OldID, tx.Owner, "")
	if e != nil {
		return e
	}
	if old.Config.Labels["cxz.role"] != "daemon" {
		return fmt.Errorf("invalid old manager role")
	}
	if !old.State.Running {
		touched = true
	}
	// Refresh admission on every attempt, including recovery after a helper crash.
	// With the old manager gone its persisted lease must still cover the gap.
	if old.State.Running {
		h, err := Call(ctx, root, "prepare", tx.ID)
		if err != nil {
			return err
		}
		tx.Old = h
	} else if h, err := Call(ctx, root, "health", ""); err == nil {
		if h.Lease != tx.ID {
			return fmt.Errorf("replacement lease expired; operator recovery required")
		}
	} else if !leaseOwned(root, tx.ID) {
		return fmt.Errorf("manager lease expired; operator recovery required")
	}
	if e = freezeProjects(ctx, root, &tx); e != nil {
		return e
	}
	if tx.State == "staged" {
		if e = save("prepared"); e != nil {
			return e
		}
	}
	if v, err := dockerx.Inspect(ctx, tx.Name); err == nil && v.ID != tx.OldID {
		if v.Config.Labels["cxz.owner"] != tx.Owner || v.Config.Labels["cxz.role"] != "daemon" || v.Config.Image != tx.Release.Image {
			return fmt.Errorf("replacement container identity changed")
		}
		tx.NewID = v.ID
	}
	if tx.State != "rolling_back" {
		e = func() error {
			if old.State.Running {
				touched = true
				if _, e = dockerx.Run(ctx, "stop", "--time", "30", tx.OldID); e != nil {
					return e
				}
				old, e = dockerx.Owned(ctx, tx.OldID, tx.Owner, "")
				if e != nil || old.State.Running {
					return fmt.Errorf("old manager did not stop")
				}
			}
			previous := tx.Name + "-previous-" + tx.ID
			if strings.TrimPrefix(old.Name, "/") != previous {
				if _, e = dockerx.Run(ctx, "rename", tx.OldID, previous); e != nil {
					return e
				}
			}
			if e = save("starting"); e != nil {
				return e
			}
			if tx.NewID == "" {
				tx.NewID, e = CloneManager(ctx, tx.OldID, tx.Name, tx.Owner, tx.Release.Image)
			}
			if e == nil {
				e = save("starting")
			}
			if e == nil {
				_, e = dockerx.Run(ctx, "start", tx.NewID)
			}
			if e == nil {
				check, cancel := context.WithTimeout(ctx, 45*time.Second)
				e = waitManager(check, root, tx)
				cancel()
			}
			if e == nil {
				// New Wisp connections and newly bootstrapped projects use the new image.
				_, e = dockerx.Run(ctx, "exec", tx.NewID, "/usr/local/bin/cxz", "_publish-tools")
			}
			return e
		}()
		if e == nil {
			if e = save("healthy"); e != nil {
				return e
			}
			return recordManagerSuccess(root, tx)
		}
		tx.Error = e.Error()
		if e = save("rolling_back"); e != nil {
			return e
		}
	}
	if !leaseOwned(root, tx.ID) {
		return fmt.Errorf("manager lease expired; refusing to terminate replacement")
	}
	if tx.NewID != "" {
		v, err := dockerx.Owned(ctx, tx.NewID, tx.Owner, "")
		if err == nil {
			if v.Config.Image != tx.Release.Image || v.Config.Labels["cxz.role"] != "daemon" {
				return fmt.Errorf("rollback target changed")
			}
			if _, err = dockerx.Run(ctx, "rm", "-f", v.ID); err != nil {
				return err
			}
		} else {
			// Distinguish already removed from a Docker daemon/ownership error.
			if _, inspectErr := dockerx.Inspect(ctx, tx.Name); inspectErr == nil {
				return err
			}
			if _, listErr := dockerx.List(ctx, "label=cxz.owner="+tx.Owner); listErr != nil {
				return listErr
			}
		}
	}
	old, e = dockerx.Owned(ctx, tx.OldID, tx.Owner, "")
	if e != nil {
		return e
	}
	if strings.TrimPrefix(old.Name, "/") != tx.Name {
		if _, e = dockerx.Run(ctx, "rename", tx.OldID, tx.Name); e != nil {
			return e
		}
	}
	if _, e = dockerx.Run(ctx, "start", tx.OldID); e != nil {
		return e
	}
	// A rollback is complete only after the old manager is serving again.
	check, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for {
		if h, err := Call(check, root, "health", ""); err == nil && h.Lease == tx.ID && h.Build.Revision == tx.Old.Build.Revision {
			break
		}
		select {
		case <-check.Done():
			return check.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if _, e = dockerx.Run(ctx, "exec", tx.OldID, "/usr/local/bin/cxz", "_publish-tools"); e != nil {
		return e
	}
	if e = save("rolled_back"); e != nil {
		return e
	}
	s, _ := Load(root)
	s.State = "rolled_back"
	s.Reason = tx.Error
	s.FailedAt = time.Now()
	s.FailedRevision = tx.Release.Revision
	return Save(root, s)
}
func recordManagerSuccess(root string, tx ManagerTransaction) error {
	s, e := Load(root)
	if e != nil {
		return e
	}
	s.State = "waiting"
	s.Reason = "manager updated; project runtimes and sessions pending"
	s.AppliedSequence = tx.Release.Sequence
	// The old stopped container is deliberately retained for rollback. It does not
	// consume the new name or run alongside the replacement.
	return Save(root, s)
}

// ReserveInstall shares the helper name with automatic replacement. Docker's
// atomic name allocation fences manual installation even across frontend hosts.
func ReserveInstall(ctx context.Context, name, owner, image string) (func(), error) {
	if len(owner) < 12 {
		return nil, fmt.Errorf("invalid installation owner")
	}
	helper := ManagerHelperName(owner)
	if v, e := dockerx.Inspect(ctx, helper); e == nil {
		if v.Config.Labels["cxz.owner"] != owner || v.Config.Labels["cxz.role"] != "update" || v.State.Running {
			return nil, fmt.Errorf("installation update already in progress")
		}
		h, e := ContainerHealth(ctx, name, "", "/var/lib/cxz", "health", "")
		if e != nil || h.Lease != "" {
			return nil, fmt.Errorf("automatic update requires recovery before installation")
		}
		if _, e = dockerx.Run(ctx, "rm", v.ID); e != nil {
			return nil, e
		}
	}
	b, e := dockerx.Run(ctx, "create", "--name", helper, "--label", "cxz.owner="+owner, "--label", "cxz.role=install", "--entrypoint", "/bin/true", image)
	if e != nil {
		return nil, e
	}
	id := strings.TrimSpace(string(b))
	return func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = dockerx.Run(cleanup, "rm", id)
	}, nil
}
