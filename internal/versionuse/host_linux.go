package versionuse

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/versionpin"
	"github.com/lesomnus/cxz/internal/workspace"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Project struct{ ID, Container, User string }
type Transaction struct {
	Pin               versionpin.Pin
	Installation      transport.Installation
	Projects          []Project
	OldManager, Phase string
}

func transactionPath(root string) string { return filepath.Join(root, "use-transaction.json") }
func Load(root string) (Transaction, error) {
	var tx Transaction
	b, e := os.ReadFile(transactionPath(root))
	if e == nil {
		e = json.Unmarshal(b, &tx)
	}
	return tx, e
}
func save(root string, tx Transaction) error { return core.WriteJSON(transactionPath(root), tx) }
func tool(p versionpin.Pin) string           { return "/cxz/tools/use/" + p.Version + "/cxz" }
func helperName(owner string) string         { return "cxz-" + owner[:12] + "-update" }
func policy(ctx context.Context, v transport.Installation, p versionpin.Pin, action string) error {
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	return dockerx.Input(ctx, bytes.NewReader(b), "run", "--rm", "-i", "--label", "cxz.owner="+v.Owner, "--mount", "type=volume,source="+v.StateVolume+",target=/var/lib/cxz", p.Image, "_use-policy", "/var/lib/cxz", action)
}
func Clear(ctx context.Context, root string) error {
	if tx, e := Load(root); e == nil && tx.Phase != "complete" {
		return fmt.Errorf("version switch unfinished; retry cxz use %s", tx.Pin.Selection())
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	v, e := transport.Load(root)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	p, e := versionpin.Load(root)
	if e != nil {
		return e
	}
	if p.Version == "" {
		return nil
	}
	return policy(ctx, v, p, "clear")
}

// Prepare downloads/pulls and checks every runnable artifact before interrupting work.
func Prepare(ctx context.Context, root string, p versionpin.Pin, out io.Writer) (*Transaction, error) {
	v, e := transport.Load(root)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if len(v.Owner) < 12 {
		return nil, fmt.Errorf("invalid installation owner")
	}
	previous, e := Load(root)
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	if previous.Phase != "" && previous.Phase != "complete" {
		if previous.Pin.Version != p.Version || previous.Pin.Revision != p.Revision || previous.Pin.Image != p.Image || previous.Pin.Channel != p.Channel {
			return nil, fmt.Errorf("unfinished switch to %s; retry that exact release first", previous.Pin.Selection())
		}
		return &previous, nil
	}
	if _, e = dockerx.Run(ctx, "image", "inspect", p.Image); e != nil {
		if _, e = dockerx.Run(ctx, "pull", p.Image); e != nil {
			return nil, e
		}
	}
	if e = policy(ctx, v, p, "validate"); e != nil {
		return nil, e
	}
	if e = checkImage(ctx, p); e != nil {
		return nil, e
	}
	old, e := dockerx.Owned(ctx, v.Container, v.Owner, "")
	if e != nil {
		return nil, e
	}
	if old.Config.Labels["cxz.role"] != "daemon" {
		return nil, fmt.Errorf("installed container is not a manager")
	}
	tx := Transaction{Pin: p, Installation: v, OldManager: old.ID, Phase: "prepared"}
	// Atomic Docker name reservation is shared with the automatic update PR.
	if existing, e := dockerx.Inspect(ctx, helperName(v.Owner)); e == nil {
		if existing.Config.Labels["cxz.owner"] != v.Owner || existing.State.Running {
			return nil, fmt.Errorf("another installer/update helper is running")
		}
		if existing.Config.Labels["cxz.role"] != "update" && !(existing.Config.Labels["cxz.role"] == "use" && previous.Phase == "complete" && existing.Config.Labels["cxz.use"] == previous.Pin.Generation) {
			return nil, fmt.Errorf("installation reservation already exists")
		}
		if _, e = dockerx.Run(ctx, "rm", existing.ID); e != nil {
			return nil, e
		}
	}
	if _, e = dockerx.Run(ctx, "create", "--name", helperName(v.Owner), "--label", "cxz.owner="+v.Owner, "--label", "cxz.role=use", "--label", "cxz.use="+p.Generation, "--entrypoint", "/bin/true", p.Image); e != nil {
		return nil, e
	}
	success := false
	defer func() {
		if !success {
			_, _ = dockerx.Run(context.Background(), "rm", helperName(v.Owner))
		}
	}()
	// Shared immutable helper path is readable by every project's remote user.
	if _, e = dockerx.Run(ctx, "run", "--rm", "--entrypoint", "sh", "--mount", "type=volume,source="+v.ToolsVolume+",target=/cxz/tools", p.Image, "-c", `mkdir -p "$(dirname "$1")" && cp /usr/local/bin/cxz "$1.next" && chmod 755 "$1.next" && mv "$1.next" "$1"`, "sh", tool(p)); e != nil {
		return nil, e
	}
	if e = inventory(ctx, &tx); e != nil {
		return nil, e
	}
	if e = save(root, tx); e != nil {
		return nil, e
	}
	success = true
	fmt.Fprintf(out, "Prepared %s for manager and %d running projects. Active agent work will be interrupted.\n", p.Version, len(tx.Projects))
	return &tx, nil
}
func inventory(ctx context.Context, tx *Transaction) error {
	v, p := tx.Installation, tx.Pin
	containers, e := dockerx.List(ctx, "label=cxz.owner="+v.Owner)
	if e != nil {
		return e
	}
	for _, c := range containers {
		id := c.Config.Labels["cxz.project"]
		if id == "" || !c.State.Running {
			continue
		}
		mounted := false
		for _, m := range c.Mounts {
			if m.Destination == "/cxz/state" {
				mounted = true
			}
		}
		if !mounted {
			continue
		}
		b, e := dockerx.Run(ctx, "exec", c.ID, "stat", "-c", "%u", "/cxz/state/data")
		if e != nil {
			return e
		}
		user := strings.TrimSpace(string(b))
		if _, e = dockerx.Run(ctx, "exec", "--user", user, c.ID, tool(p), "--format", "json", "version"); e != nil {
			return fmt.Errorf("project %s cannot run target binary: %w", id, e)
		}
		if _, e = dockerx.Run(ctx, "exec", "--user", user, c.ID, tool(p), "_use-project", "validate", "/cxz/state/data", p.Generation, p.Version); e != nil {
			return e
		}
		seen := false
		for _, prior := range tx.Projects {
			if prior.Container == c.ID {
				seen = true
			}
		}
		if seen {
			continue
		}
		tx.Projects = append(tx.Projects, Project{ID: id, Container: c.ID, User: user})
	}
	return nil
}
func checkImage(ctx context.Context, p versionpin.Pin) error {
	b, e := dockerx.Run(ctx, "run", "--rm", p.Image, "--format", "json", "version")
	if e != nil {
		return e
	}
	var v struct{ Version, Revision string }
	if json.Unmarshal(b, &v) != nil || v.Version != p.Version || v.Revision != p.Revision {
		return fmt.Errorf("manager image differs from the selected release")
	}
	return nil
}
func Apply(ctx context.Context, root string, tx *Transaction, out io.Writer) error {
	v, p := tx.Installation, tx.Pin
	if tx.Phase == "ready" {
		return nil
	}
	reservation, e := dockerx.Inspect(ctx, helperName(v.Owner))
	if e != nil {
		return e
	}
	if reservation.Config.Labels["cxz.owner"] != v.Owner || reservation.Config.Labels["cxz.role"] != "use" || reservation.Config.Labels["cxz.use"] != p.Generation {
		return fmt.Errorf("installation reservation identity changed")
	}
	if e = policy(ctx, v, p, "save"); e != nil {
		return e
	}
	if tx.Phase == "prepared" {
		old, e := dockerx.Owned(ctx, tx.OldManager, v.Owner, "")
		if e != nil {
			return e
		}
		if old.State.Running {
			if _, e = dockerx.Run(ctx, "stop", "--time", "10", old.ID); e != nil {
				return e
			}
		}
		// Include projects admitted while artifacts were being staged.
		if e = inventory(ctx, tx); e != nil {
			return e
		}
		tx.Phase = "stopping"
		if e = save(root, *tx); e != nil {
			return e
		}
	}
	if tx.Phase == "stopping" {
		for _, project := range tx.Projects {
			if _, e = dockerx.Owned(ctx, project.Container, v.Owner, project.ID); e != nil {
				return e
			}
			if _, e = dockerx.Run(ctx, "exec", "--user", project.User, project.Container, tool(p), "_use-project", "stop", "/cxz/state/data", p.Generation, p.Version); e != nil {
				return e
			}
		}
		tx.Phase = "installing"
		if e = save(root, *tx); e != nil {
			return e
		}
	}
	if tx.Phase == "installing" {
		old, e := dockerx.Owned(ctx, tx.OldManager, v.Owner, "")
		if e != nil {
			return e
		}
		backup := v.Container + "-before-use-" + p.Generation
		if strings.TrimPrefix(old.Name, "/") != backup {
			if _, e = dockerx.Run(ctx, "rename", old.ID, backup); e != nil {
				return e
			}
		}
		// installer recreates only the manager and publishes tools atomically.
		if e = installer.InstallLocked(ctx, root, v.WorkspaceRoot, p.Image, true, out); e != nil {
			return fmt.Errorf("manager installation failed; previous manager retained as %s; retry cxz use %s: %w", backup, p.Selection(), e)
		}
		tx.Phase = "resuming"
		if e = save(root, *tx); e != nil {
			return e
		}
	}
	if tx.Phase == "resuming" {
		for _, project := range tx.Projects {
			if _, e = dockerx.Owned(ctx, project.Container, v.Owner, project.ID); e != nil {
				return e
			}
			if _, e = dockerx.Run(ctx, "exec", "--user", project.User, project.Container, tool(p), "_use-project", "resume", "/cxz/state/data", p.Generation, p.Version); e != nil {
				return e
			}
		}
		p.Ready = true
		if e = policy(ctx, v, p, "save"); e != nil {
			return e
		}
		tx.Phase = "ready"
		if e = save(root, *tx); e != nil {
			return e
		}
	}
	return nil
}
func Finish(ctx context.Context, root string, tx *Transaction) error {
	if tx == nil {
		return nil
	}
	tx.Phase = "complete"
	if e := save(root, *tx); e != nil {
		return e
	}
	v := tx.Installation
	c, e := dockerx.Inspect(ctx, helperName(v.Owner))
	if e != nil {
		return e
	}
	if c.Config.Labels["cxz.owner"] != v.Owner || c.Config.Labels["cxz.use"] != tx.Pin.Generation {
		return fmt.Errorf("reservation changed")
	}
	_, e = dockerx.Run(ctx, "rm", c.ID)
	return e
}
func validateDatabase(root string) error {
	path := filepath.Join(root, "cxz.db")
	if _, e := os.Stat(path); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	db, e := sql.Open("sqlite3", (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String())
	if e != nil {
		return e
	}
	defer db.Close()
	var version int
	if e = db.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	if version > 1 {
		return fmt.Errorf("target release cannot read database schema %d", version)
	}
	return nil
}
func ValidateProject(root string) error {
	if e := validateDatabase(root); e != nil {
		return e
	}

	if _, e := workspace.LoadRuntime(root); e != nil {
		return e
	}
	file, e := os.CreateTemp(root, ".use-preflight-")
	if e != nil {
		return e
	}
	file.Close()
	return os.Remove(file.Name())
}

// Policy runs in an isolated volume helper, including when the manager is down.
func Policy(root, action string, p versionpin.Pin) error {
	switch action {
	case "validate":
		return validateDatabase(root)
	case "save":
		if e := versionpin.Save(root, p); e != nil {
			return e
		}
		if !p.Ready {
			for _, name := range []string{"run/update-lease.json", "manager-update.json"} {
				if e := archive(root, name, p.Generation); e != nil {
					return e
				}
			}
		}
		return nil
	case "clear":
		return versionpin.Clear(root)
	}
	return fmt.Errorf("invalid policy action")
}
