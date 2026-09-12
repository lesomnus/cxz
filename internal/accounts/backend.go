package accounts

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/lesomnus/cxz/internal/core"
)

type AgentKind string

const (
	Claude              AgentKind = "claude"
	Codex               AgentKind = "codex"
	ProjectLocalOAuth             = "project-local-oauth"
	BrokeredAccessToken           = "brokered-access-token"
	APIKey                        = "api-key"
)

type BackendInfo struct {
	ID           string `json:"id"`
	Scope        string `json:"scope"`
	Workflow     string `json:"workflow"`
	RefreshOwner string `json:"refresh_owner"`
}
type AgentInfo struct {
	Kind           AgentKind     `json:"kind"`
	DefaultBackend string        `json:"default_backend"`
	Backends       []BackendInfo `json:"backends"`
}
type LoginRequest struct {
	Root, Account, Workspace, Binary string
	Env                              []string
	Input                            io.Reader
	Output, Error                    io.Writer
	ValidateCredential               func([]byte) error
}
type LaunchAuth struct {
	Env, Args []string
	ConfigDir string
}
type BindingSpec struct{ ID, Scope, CredentialRef string }

// Backend owns the whole authentication lifecycle, not just token retrieval.
// Secrets are never returned to the resource layer or stored in its audit log.
type Backend interface {
	Info() BackendInfo
	Binding(project, account string) (BindingSpec, error)
	Login(context.Context, LoginRequest) error
	Check(root, account string) error
	Launch(root, account string, env []string) (LaunchAuth, error)
}

type registration struct {
	defaultBackend string
	backends       map[string]func() Backend
}

var registry = map[AgentKind]registration{
	Claude: {ProjectLocalOAuth, map[string]func() Backend{ProjectLocalOAuth: func() Backend { return projectLocalOAuth{Claude} }}},
	Codex:  {BrokeredAccessToken, map[string]func() Backend{ProjectLocalOAuth: func() Backend { return projectLocalOAuth{Codex} }, BrokeredAccessToken: func() Backend { return brokered{} }}},
}

func Catalog() []AgentInfo {
	out := []AgentInfo{}
	for _, kind := range []AgentKind{Claude, Codex} {
		r := registry[kind]
		var ids []string
		for id := range r.backends {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		info := AgentInfo{Kind: kind, DefaultBackend: r.defaultBackend}
		for _, id := range ids {
			info.Backends = append(info.Backends, r.backends[id]().Info())
		}
		out = append(out, info)
	}
	return out
}

// Defaulting is restricted to profile creation, never to launch/recovery.
func Select(agent, requested string) (Backend, error) {
	if requested == "" {
		requested = registry[AgentKind(agent)].defaultBackend
	}
	return Resolve(agent, requested)
}
func Resolve(agent, id string) (Backend, error) {
	kind := AgentKind(agent)
	r, ok := registry[kind]
	if !ok {
		return nil, fmt.Errorf("unknown agent kind %q", agent)
	}
	factory, ok := r.backends[id]
	if !ok {
		return nil, fmt.Errorf("auth backend %q is not supported for %s", id, agent)
	}
	return factory(), nil
}
func BindingID(project, account, backend string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(project+"\x00"+account+"\x00"+backend)))
}

type projectLocalOAuth struct{ agent AgentKind }

func (b projectLocalOAuth) Info() BackendInfo {
	return BackendInfo{ProjectLocalOAuth, "project", "project-login", "agent"}
}
func (b projectLocalOAuth) Binding(project, account string) (BindingSpec, error) {
	if err := Validate(account, string(b.agent)); err != nil {
		return BindingSpec{}, err
	}
	if project == "" {
		return BindingSpec{}, fmt.Errorf("project-local-oauth requires a project")
	}
	return BindingSpec{BindingID(project, account, ProjectLocalOAuth), "project", "accounts/" + account}, nil
}
func ResolveBinding(agent, backend, project, account, id string) (Backend, error) {
	b, err := Resolve(agent, backend)
	if err != nil {
		return nil, err
	}
	expected, err := b.Binding(project, account)
	if err != nil {
		return nil, err
	}
	if expected.ID != id {
		return nil, fmt.Errorf("auth binding does not match project/account/backend")
	}
	return b, nil
}
func (b projectLocalOAuth) Check(root, account string) error {
	_, err := Credential(root, account, string(b.agent))
	return err
}
func (b projectLocalOAuth) Launch(root, account string, env []string) (LaunchAuth, error) {
	if err := b.Check(root, account); err != nil {
		return LaunchAuth{}, err
	}
	out := LaunchAuth{Env: Environment(env, root, account, string(b.agent)), ConfigDir: Config(root, account)}
	if b.agent == Codex {
		out.Args = []string{"-c", `cli_auth_credentials_store="file"`, "-c", `model_provider="openai"`}
	}
	return out, nil
}
func (b projectLocalOAuth) Login(ctx context.Context, r LoginRequest) error {
	if err := Validate(r.Account, string(b.agent)); err != nil {
		return err
	}
	if r.Binary == "" || r.Workspace == "" {
		return fmt.Errorf("project runtime and agent executable required")
	}
	lock, err := core.Lock(filepath.Join(r.Root, "run", fmt.Sprintf("workspace-%x.lock", sha256.Sum256([]byte(r.Workspace)))))
	if err != nil {
		return fmt.Errorf("stop the active project session before login: %w", err)
	}
	defer lock.Close()
	if err = Prepare(r.Root, r.Account, string(b.agent)); err != nil {
		return err
	}
	loginLock, err := core.Lock(filepath.Join(Dir(r.Root, r.Account), "login.lock"))
	if err != nil {
		return err
	}
	defer loginLock.Close()
	staging, err := os.MkdirTemp(Dir(r.Root, r.Account), "login-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err = Prepare(staging, r.Account, string(b.agent)); err != nil {
		return err
	}
	args := []string{"auth", "login"}
	if b.agent == Codex {
		args = []string{"-c", `cli_auth_credentials_store="file"`, "login", "--device-auth"}
	}
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Dir = Dir(staging, r.Account)
	cmd.Env = Environment(r.Env, staging, r.Account, string(b.agent))
	cmd.Stdin = r.Input
	cmd.Stdout = r.Output
	cmd.Stderr = r.Error
	if b.agent == Claude && terminalInput(r.Input) {
		err = runClaudeLogin(ctx, cmd, r.Input, r.Error)
	} else {
		err = cmd.Run()
	}
	if err != nil {
		return err
	}
	credential, err := Credential(staging, r.Account, string(b.agent))
	if err != nil {
		return err
	}
	if r.ValidateCredential != nil {
		if err := r.ValidateCredential(credential); err != nil {
			return err
		}
	}
	return Install(r.Root, r.Account, string(b.agent), credential)
}
