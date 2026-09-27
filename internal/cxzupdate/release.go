// Package cxzupdate implements cxz release discovery and durable update state.
// It never restarts an agent: server-side callers own readiness and transactions.
package cxzupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

const Protocol = 1
const Schema = 1
const Interval = 24 * time.Hour
const IdlePeriod = 5 * time.Minute
const ReleaseURL = "https://github.com/lesomnus/cxz/releases/download/edge/cxz-update.json"

var revisionPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Revision is set for image builds that do not carry VCS metadata.
var Revision string

type Build struct {
	Revision string `json:"revision"`
	Platform string `json:"platform"`
	Protocol int    `json:"protocol"`
	Schema   int    `json:"schema"`
	Dirty    bool   `json:"dirty"`
	PID      int    `json:"pid"`
}

func Current() Build {
	b := Build{Revision: Revision, Platform: runtime.GOOS + "/" + runtime.GOARCH, Protocol: Protocol, Schema: Schema, PID: os.Getpid()}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, v := range info.Settings {
			if v.Key == "vcs.revision" && b.Revision == "" {
				b.Revision = v.Value
			}
			if v.Key == "vcs.modified" {
				b.Dirty = v.Value == "true"
			}
		}
	}
	return b
}
func (b Build) Managed() bool { return revisionPattern.MatchString(b.Revision) && !b.Dirty }

type Config struct {
	Enabled *bool `json:"enabled,omitempty"`
}

func (c Config) Active() bool { return c.Enabled == nil || *c.Enabled }

type Asset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}
type Release struct {
	Revision  string           `json:"revision"`
	Ancestors []string         `json:"ancestors,omitempty"`
	Sequence  int64            `json:"sequence"`
	Protocol  int              `json:"protocol"`
	Schema    int              `json:"schema"`
	Image     string           `json:"image"`
	Assets    map[string]Asset `json:"assets"`
}

func (r Release) Validate() error {
	if !revisionPattern.MatchString(r.Revision) || r.Sequence <= 0 || r.Protocol != Protocol || r.Schema != Schema {
		return fmt.Errorf("release requires a compatible protocol and state schema")
	}
	if !strings.HasPrefix(r.Image, "ghcr.io/lesomnus/cxz@sha256:") || !hashPattern.MatchString(strings.TrimPrefix(r.Image, "ghcr.io/lesomnus/cxz@sha256:")) {
		return fmt.Errorf("invalid immutable manager image")
	}
	for _, ancestor := range r.Ancestors {
		if !revisionPattern.MatchString(ancestor) {
			return fmt.Errorf("invalid main ancestry")
		}
	}
	for _, platform := range []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
		a := r.Assets[platform]
		name := "cxz-" + r.Revision + "-" + strings.ReplaceAll(platform, "/", "-")
		if strings.HasPrefix(platform, "windows/") {
			name += ".exe"
		}
		if a.Name != name || !hashPattern.MatchString(a.SHA256) {
			return fmt.Errorf("invalid release asset for %s", platform)
		}
	}
	return nil
}
func Fetch(ctx context.Context) (Release, error) {
	var r Release
	b, e := download(ctx, ReleaseURL, 1<<20)
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return r, e
	}
	return r, r.Validate()
}
func download(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return nil, e
	}
	c := &http.Client{Timeout: 5 * time.Minute}
	resp, e := c.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("release download: HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e == nil && int64(len(b)) > limit {
		e = fmt.Errorf("release too large")
	}
	return b, e
}
func executableName(platform string) string {
	if strings.HasPrefix(platform, "windows/") {
		return "cxz.exe"
	}
	return "cxz"
}
func Path(root string, r Release, platform string) string {
	return filepath.Join(root, "releases", r.Revision, strings.ReplaceAll(platform, "/", "-"), executableName(platform))
}
func Verify(path, checksum string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	if hex.EncodeToString(h.Sum(nil)) != checksum {
		return fmt.Errorf("release checksum mismatch")
	}
	return nil
}
func Stage(ctx context.Context, root string, r Release, platform string) (string, error) {
	if e := r.Validate(); e != nil {
		return "", e
	}
	a, ok := r.Assets[platform]
	if platform != "linux/amd64" && platform != "linux/arm64" && platform != "windows/amd64" && platform != "windows/arm64" {
		ok = false
	}
	if !ok {
		return "", fmt.Errorf("unsupported platform %s", platform)
	}
	path := Path(root, r, platform)
	if e := Verify(path, a.SHA256); e == nil {
		return path, nil
	}
	// Never overwrite an existing immutable path, including an unexpectedly modified file.
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		return "", fmt.Errorf("immutable release path exists but is invalid: %s", path)
	}
	b, e := download(ctx, "https://github.com/lesomnus/cxz/releases/download/edge/"+a.Name, 512<<20)
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != a.SHA256 {
		return "", fmt.Errorf("release checksum mismatch")
	}
	if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".stage-")
	if e != nil {
		return "", e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, e = f.Write(b); e != nil {
		return "", e
	}
	if e = f.Chmod(0755); e != nil {
		return "", e
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	// Link provides create-if-absent semantics; concurrent downloaders cannot replace a running image.
	if e = os.Link(f.Name(), path); e != nil {
		if e = Verify(path, a.SHA256); e != nil {
			return "", e
		}
	}
	return path, core.SyncDir(filepath.Dir(path))
}
func CheckBinary(ctx context.Context, path string, r Release, platform string) error {
	if e := r.Validate(); e != nil {
		return e
	}
	if e := Verify(path, r.Assets[platform].SHA256); e != nil {
		return e
	}
	b, e := exec.CommandContext(ctx, path, "_build-info").Output()
	if e != nil {
		return e
	}
	var v Build
	if json.Unmarshal(b, &v) != nil || v.Revision != r.Revision || v.Platform != platform || v.Protocol != r.Protocol || v.Schema != r.Schema || v.Dirty {
		return fmt.Errorf("candidate build identity mismatch")
	}
	return nil
}

// ValidBinary restricts persisted supervisor targets to immutable releases.
func ValidBinary(path string) bool {
	p := strings.Split(strings.TrimPrefix(path, "/cxz/tools/cxz-builds/releases/"), "/")
	return strings.HasPrefix(path, "/cxz/tools/cxz-builds/releases/") && len(p) == 3 && revisionPattern.MatchString(p[0]) && (p[1] == "linux-amd64" || p[1] == "linux-arm64") && p[2] == "cxz"
}

type State struct {
	CheckedAt       time.Time `json:"checked_at"`
	FailedAt        time.Time `json:"failed_at,omitempty"`
	FailedRevision  string    `json:"failed_revision,omitempty"`
	Release         *Release  `json:"release,omitempty"`
	State           string    `json:"state"`
	Reason          string    `json:"reason,omitempty"`
	AppliedSequence int64     `json:"applied_sequence,omitempty"`
	Running         Build     `json:"running"`
}

func StatePath(root string) string { return filepath.Join(root, "cxz-update.json") }
func Load(root string) (State, error) {
	var s State
	b, e := os.ReadFile(StatePath(root))
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
func Save(root string, s State) error { return core.WriteJSON(StatePath(root), s) }
func Check(ctx context.Context, root string, force bool) (State, error) {
	s, e := Load(root)
	if e != nil {
		return s, e
	}
	s.Running = Current()
	if !force && time.Since(s.CheckedAt) < Interval {
		return s, nil
	}
	r, e := Fetch(ctx)
	s.CheckedAt = time.Now()
	if e != nil {
		s.Reason = e.Error()
		_ = Save(root, s)
		return s, e
	}
	if r.Sequence < s.AppliedSequence || (s.Release != nil && r.Sequence < s.Release.Sequence) {
		s.Reason = "older publication ignored"
		return s, Save(root, s)
	}
	s.Release = &r
	s.State = "discovered"
	s.Reason = ""
	if r.Revision == s.Running.Revision {
		s.State = "healthy"
		s.AppliedSequence = r.Sequence
	}
	return s, Save(root, s)
}
func (s State) RetryAllowed() bool {
	return s.Release != nil && (s.FailedRevision != s.Release.Revision || time.Since(s.FailedAt) >= Interval)
}

// A clean local build can be newer than edge. Publication sequence alone cannot
// order a previously unseen executable, so require main ancestry as well.
func (r Release) CanReplace(b Build) bool {
	if r.Revision == b.Revision {
		return true
	}
	for _, rev := range r.Ancestors {
		if rev == b.Revision {
			return true
		}
	}
	return false
}
