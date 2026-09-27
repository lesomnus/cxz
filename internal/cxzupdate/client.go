package cxzupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/versionpin"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type clientKey struct{}
type Client struct{ Root, Executable string }

func WithClient(ctx context.Context, root string) context.Context {
	// Remember the executable pathname before another frontend can rename it.
	exe, e := os.Executable()
	if e != nil {
		return ctx
	}
	root, e = filepath.Abs(root)
	if e != nil {
		return ctx
	}
	return context.WithValue(ctx, clientKey{}, Client{root, exe})
}
func ClientFrom(ctx context.Context) (Client, bool) {
	c, ok := ctx.Value(clientKey{}).(Client)
	return c, ok
}
func Policy(root string) (Config, error) {
	if p, e := versionpin.Load(root); e != nil {
		return Config{}, e
	} else if p.Pinned() || (p.Version != "" && !p.Ready) {
		disabled := false
		return Config{Enabled: &disabled}, nil
	}
	var c Config
	b, e := os.ReadFile(filepath.Join(root, "cxz-update-policy.json"))
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func SetPolicy(root string, enabled bool) error {
	if enabled {
		if e := versionpin.Check(root); e != nil {
			return e
		}
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	return core.WriteJSON(filepath.Join(root, "cxz-update-policy.json"), Config{Enabled: &enabled})
}
func ClientCandidate(ctx context.Context, c Client) (State, string, error) {
	policy, e := Policy(c.Root)
	if e != nil {
		return State{}, "", e
	}
	if !policy.Active() {
		return State{State: "paused", Running: Current()}, "", nil
	}
	if !Current().Managed() {
		return State{State: "unmanaged", Reason: "development/modified build; automatic replacement disabled", Running: Current()}, "", nil
	}
	if e = os.MkdirAll(c.Root, 0700); e != nil {
		return State{}, "", e
	}
	lock, e := core.Lock(filepath.Join(c.Root, "self-update.lock"))
	if e != nil {
		return State{}, "", e
	}
	defer lock.Close()
	s, e := Check(ctx, c.Root, false)
	if e != nil {
		return s, "", e
	}
	if s.Release == nil || s.Release.Revision == Current().Revision || !s.RetryAllowed() {
		return s, "", nil
	}
	if !s.Release.CanReplace(Current()) {
		s.State = "waiting"
		s.Reason = "selected channel cannot replace the running build; automatic downgrade refused"
		_ = Save(c.Root, s)
		return s, "", nil
	}
	path, e := Stage(ctx, filepath.Join(c.Root, "cxz"), *s.Release, Current().Platform)
	if e == nil {
		e = CheckBinary(ctx, path, *s.Release, Current().Platform)
	}
	if e != nil {
		s.State = "failed"
		s.Reason = e.Error()
		s.FailedAt = time.Now()
		s.FailedRevision = s.Release.Revision
		_ = Save(c.Root, s)
		return s, "", e
	}
	s.State = "staged"
	s.Reason = "waiting for an idle frontend"
	if e = Save(c.Root, s); e != nil {
		return s, "", e
	}
	return s, path, nil
}

type Resume struct {
	Revision, Session, Project, Connection string
	Position                               float64
}

func ResumePath(root string) string { return FrontendFile(root, "resume") }
func TakeResume(root string) (Resume, error) {
	var r Resume
	b, e := os.ReadFile(ResumePath(root))
	if os.IsNotExist(e) {
		return r, nil
	}
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return r, e
	}
	if r.Revision != Current().Revision {
		return Resume{}, nil
	}
	return r, os.Remove(ResumePath(root))
}

// Restart is consumed only after Bubble Tea restores terminal modes and all
// connection defers have run. It is not a user-facing command failure.
type Restart struct {
	Client    Client
	Candidate string
	Release   Release
	Resume    Resume
}

func (r *Restart) Error() string {
	return fmt.Sprintf("cxz frontend restart to %s", r.Release.Revision)
}

// Per-launcher files prevent concurrent TUIs from consuming each other's state.
func FrontendFile(root, kind string) string {
	slot := os.Getenv("CXZ_FRONTEND_SLOT")
	if slot == "" {
		slot = fmt.Sprint(os.Getpid())
	}
	if strings.ContainsAny(slot, "/\\.") {
		slot = fmt.Sprint(os.Getpid())
	}
	return filepath.Join(root, "frontend-"+slot+"-"+kind+".json")
}
func FrontendReady(ctx context.Context) {
	if os.Getenv("CXZ_FRONTEND_SLOT") == "" {
		return
	}
	if c, ok := ClientFrom(ctx); ok {
		_ = core.WriteJSON(FrontendFile(c.Root, "ready"), Current())
	}
}
