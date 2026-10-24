package networkservices

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

type Service struct {
	Container    string `json:"container"`
	Image        string `json:"image,omitempty"`
	State        string `json:"state"`
	NeedsRefresh bool   `json:"needs_refresh"`
	Error        string `json:"error,omitempty"`
}
type Report struct {
	CheckedAt    time.Time `json:"checked_at"`
	ManagerImage string    `json:"manager_image,omitempty"`
	Web          Service   `json:"web"`
	Relay        Service   `json:"relay"`
	Error        string    `json:"error,omitempty"`
}

func serviceName(name, role string) string {
	if role == "web" {
		return name + "-web"
	}
	return name + "-remote"
}
func ownedService(ctx context.Context, name, owner, role string) (dockerx.Container, bool, error) {
	v, err := dockerx.Inspect(ctx, serviceName(name, role))
	if dockerx.Missing(err) {
		return v, false, nil
	}
	if err != nil {
		return v, false, err
	}
	if v.Config.Labels["cxz.owner"] != owner || v.Config.Labels["cxz.role"] != role || v.Config.Labels["cxz.project"] != "" {
		return v, true, fmt.Errorf("unowned %s container occupies installation name", role)
	}
	return v, true, nil
}
func snapshot(ctx context.Context, name, owner, role, image string) Service {
	s := Service{Container: serviceName(name, role), State: "not installed"}
	v, exists, err := ownedService(ctx, name, owner, role)
	if err != nil {
		s.State = "unknown"
		s.Error = err.Error()
		return s
	}
	if !exists {
		return s
	}
	s.Image = v.Config.Image
	s.State = "stopped"
	if v.State.Running {
		s.State = "running"
		s.NeedsRefresh = v.Config.Image != image
	}
	return s
}
func Inspect(ctx context.Context, name, owner string) Report {
	r := Report{CheckedAt: time.Now()}
	v, err := manager(ctx, name, owner)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.ManagerImage = v.Config.Image
	r.Web = snapshot(ctx, name, owner, "web", r.ManagerImage)
	r.Relay = snapshot(ctx, name, owner, "remote", r.ManagerImage)
	return r
}
func (r Report) Failure() error {
	var failures []error
	for _, message := range []string{r.Error, r.Web.Error, r.Relay.Error} {
		if message != "" {
			failures = append(failures, errors.New(message))
		}
	}
	return errors.Join(failures...)
}

// Refresh uses the installed container's full Docker configuration as desired
// configuration. This also upgrades legacy installations with no manager-side
// copy of the host's settings, without reading or writing the host directory.
// Deliberately stopped/removed services are never started or recreated.
func Refresh(ctx context.Context, root, name, owner string) (Report, error) {
	r := Inspect(ctx, name, owner)
	if err := r.Failure(); err != nil {
		return r, err
	}
	if !r.Web.NeedsRefresh && !r.Relay.NeedsRefresh && !pending(root) {
		return r, nil
	}
	lock, err := core.Lock(filepath.Join(root, LockFile))
	if err != nil {
		r.Error = "network service refresh busy: " + err.Error()
		return r, r.Failure()
	}
	defer lock.Close()
	if err = RecoverLocked(ctx, root, name, owner); err != nil {
		r.Error = "network service recovery: " + err.Error()
		return r, r.Failure()
	}
	// The manager or host configuration may have changed while acquiring the lock.
	r = Inspect(ctx, name, owner)
	if err = r.Failure(); err != nil {
		return r, err
	}
	for _, role := range []string{"web", "remote"} {
		target := &r.Web
		if role == "remote" {
			target = &r.Relay
		}
		if !target.NeedsRefresh {
			continue
		}
		err = replace(ctx, root, name, owner, role, r.ManagerImage)
		*target = snapshot(ctx, name, owner, role, r.ManagerImage)
		if err != nil {
			target.Error = fmt.Sprintf("%s refresh: %v", role, err)
		}
	}
	r.CheckedAt = time.Now()
	return r, r.Failure()
}
func webOrigin(v dockerx.Container) (*url.URL, error) {
	for i, arg := range v.Config.Cmd {
		if arg == "--origin" && i+1 < len(v.Config.Cmd) {
			u, err := url.Parse(v.Config.Cmd[i+1])
			if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
				return u, nil
			}
		}
	}
	return nil, fmt.Errorf("installed gateway has no valid origin")
}
func ready(ctx context.Context, id, role string, origin *url.URL) error {
	var err error
	for {
		if role == "web" {
			_, err = dockerx.Run(ctx, "exec", id, "curl", "--silent", "--show-error", "--fail", "--insecure", "--max-time", "2", "--header", "Host: "+origin.Host, origin.Scheme+"://127.0.0.1:7350/")
		} else {
			_, err = dockerx.Run(ctx, "exec", id, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_expose-check", "--address", "127.0.0.1:7349")
		}
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("service readiness: %w: %v", ctx.Err(), err)
		case <-time.After(200 * time.Millisecond):
		}
	}
}
func replace(ctx context.Context, root, name, owner, role, image string) error {
	old, exists, err := ownedService(ctx, name, owner, role)
	if err != nil || !exists || !old.State.Running || old.Config.Image == image {
		return err
	}
	var origin *url.URL
	if role == "web" {
		origin, err = webOrigin(old)
		if err != nil {
			return err
		}
	}
	command := "_web-serve"
	if role == "remote" {
		command = "_expose-serve"
		if _, err = dockerx.Run(ctx, "exec", name, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_pki", "server"); err != nil {
			return fmt.Errorf("prepare relay certificate: %w", err)
		}
	}
	if _, err = dockerx.Run(ctx, "run", "--rm", "--entrypoint", "/usr/local/bin/cxz", image, command, "--help"); err != nil {
		return fmt.Errorf("target image does not support %s: %w", role, err)
	}
	canonical := serviceName(name, role)
	backup := canonical + "-previous-" + core.ID()[:12]
	// Persist intent before creating/moving either container, so a killed
	// manager can recover even between Docker success and acknowledgement.
	tx := transaction{Owner: owner, Name: canonical, Role: role, OldID: old.ID, NewName: canonical + "-next-" + core.ID()[:12], Image: image}
	if err = saveTransaction(root, tx); err != nil {
		return err
	}
	next, err := dockerx.CloneOwned(ctx, old.ID, tx.NewName, owner, role, image)
	if err != nil {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if recoverErr := recoverService(recovery, root, name, owner, role); recoverErr != nil {
			return fmt.Errorf("%w; recovery: %v", err, recoverErr)
		}
		return err
	}
	tx.NewID = next
	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if e := recoverService(recovery, root, name, owner, role); e != nil {
			return fmt.Errorf("%w; rollback: %v", cause, e)
		}
		return cause
	}
	if err = saveTransaction(root, tx); err != nil {
		return rollback(err)
	}
	if _, err = dockerx.Run(ctx, "rename", old.ID, backup); err != nil {
		return rollback(err)
	}
	if _, err = dockerx.Run(ctx, "stop", "--time", "10", old.ID); err != nil {
		return rollback(err)
	}
	if _, err = dockerx.Run(ctx, "rename", next, canonical); err != nil {
		return rollback(err)
	}
	if _, err = dockerx.Run(ctx, "start", next); err != nil {
		return rollback(err)
	}
	probe, cancel := context.WithTimeout(ctx, 20*time.Second)
	err = ready(probe, next, role, origin)
	cancel()
	if err != nil {
		return rollback(err)
	}
	if _, err = dockerx.Run(ctx, "rm", old.ID); err != nil {
		return fmt.Errorf("service updated; remove retained container %s: %w", backup, err)
	}
	return discardTransaction(root, role)
}
