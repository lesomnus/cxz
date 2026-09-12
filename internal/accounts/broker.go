package accounts

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

const BrokerSocket = "/cxz/tools/auth-broker.sock"

// Token is an internal, memory-only protocol value, never a resource or event.
type Token struct {
	AccessToken string  `json:"accessToken"`
	AccountID   string  `json:"chatgptAccountId"`
	PlanType    *string `json:"chatgptPlanType"`
}
type Grant struct {
	Socket, Capability, Project, Account, Binding, AccountID string
}
type tokenRequest struct {
	PreviousAccountID string `json:"previousAccountId"`
	Refresh           bool   `json:"refresh"`
}
type brokered struct{}

func (brokered) Info() BackendInfo {
	return BackendInfo{BrokeredAccessToken, "project", "account-login", "central-codex"}
}
func (brokered) Binding(project, account string) (BindingSpec, error) {
	if err := Validate(account, "codex"); err != nil {
		return BindingSpec{}, err
	}
	if project == "" {
		return BindingSpec{}, fmt.Errorf("project required for token supply authorization")
	}
	return BindingSpec{BindingID(project, account, BrokeredAccessToken), "project", "central/accounts/" + account}, nil
}
func (brokered) Login(ctx context.Context, r LoginRequest) error { return CentralLogin(ctx, r) }
func (brokered) Check(root, account string) error {
	_, err := ReadGrant(root, account)
	return err
}
func (b brokered) Launch(root, account string, env []string) (LaunchAuth, error) {
	if err := b.Check(root, account); err != nil {
		return LaunchAuth{}, err
	}
	// External mode must never pick up a persisted managed grant or API key.
	if _, err := os.Lstat(filepath.Join(Config(root, account), "auth.json")); !os.IsNotExist(err) {
		return LaunchAuth{}, fmt.Errorf("brokered profile must not contain auth.json")
	}
	return LaunchAuth{Env: Environment(env, root, account, "codex"), Args: []string{"-c", `cli_auth_credentials_store="ephemeral"`, "-c", `model_provider="openai"`}, ConfigDir: Config(root, account)}, nil
}
func centralRoot(root string) string { return filepath.Join(root, "central") }
func centralLock(root, account string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Join(centralRoot(root), "run"), 0700); err != nil {
		return nil, err
	}
	if err := Prepare(centralRoot(root), account, "codex"); err != nil {
		return nil, err
	}
	return core.Lock(filepath.Join(Dir(centralRoot(root), account), "broker.lock"))
}
func decodeToken(raw []byte) (Token, error) {
	var v struct {
		Tokens struct {
			Access  string `json:"access_token"`
			Account string `json:"account_id"`
			Refresh string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Tokens.Access == "" || v.Tokens.Account == "" || v.Tokens.Refresh == "" {
		return Token{}, fmt.Errorf("central account requires managed Codex subscription login")
	}
	return Token{AccessToken: v.Tokens.Access, AccountID: v.Tokens.Account}, nil
}

// Workspace/account ID alone may be shared by multiple organization members.
// Pin the user subject as well, without treating decoded claims as verification.
// The official Codex login is responsible for authenticating the credential.
func credentialSubject(raw []byte) (string, error) {
	var v struct {
		Tokens struct {
			ID string `json:"id_token"`
		} `json:"tokens"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return "", fmt.Errorf("invalid central credential")
	}
	parts := strings.Split(v.Tokens.ID, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("central login requires an identity token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid central identity token")
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Sub == "" {
		return "", fmt.Errorf("central identity token has no subject")
	}
	token, err := decodeToken(raw)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token.AccountID+"\x00"+claims.Sub))), nil
}
func CentralToken(root, account string) (Token, error) {
	raw, err := Credential(centralRoot(root), account, "codex")
	if err != nil {
		return Token{}, err
	}
	subject, err := credentialSubject(raw)
	if err != nil {
		return Token{}, err
	}
	var pinned string
	pin, err := os.ReadFile(filepath.Join(Dir(centralRoot(root), account), "subject.json"))
	if err != nil || json.Unmarshal(pin, &pinned) != nil || pinned != subject {
		return Token{}, fmt.Errorf("central account identity does not match its login pin")
	}
	return decodeToken(raw)
}
func CentralLogin(ctx context.Context, r LoginRequest) error {
	lock, err := centralLock(r.Root, r.Account)
	if err != nil {
		return fmt.Errorf("account authentication busy")
	}
	defer lock.Close()
	originalRoot := r.Root
	// Keep an immutable subject pin separate from rotating vendor credentials.
	pinPath := filepath.Join(Dir(centralRoot(originalRoot), r.Account), "subject.json")
	var pinned string
	if raw, e := os.ReadFile(pinPath); e == nil {
		if json.Unmarshal(raw, &pinned) != nil || pinned == "" {
			return fmt.Errorf("invalid account subject pin")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if pinned == "" {
		if existing, e := Credential(centralRoot(originalRoot), r.Account, "codex"); e == nil {
			pinned, err = credentialSubject(existing)
			if err != nil {
				return err
			}
		}
	}
	r.Root = centralRoot(r.Root)
	r.Workspace = Dir(r.Root, r.Account)
	r.ValidateCredential = func(raw []byte) error {
		subject, e := credentialSubject(raw)
		if e != nil {
			return e
		}
		if pinned != "" && subject != pinned {
			return fmt.Errorf("login belongs to a different ChatGPT account; register a separate cxz account")
		}
		return nil
	}
	if err = (projectLocalOAuth{Codex}).Login(ctx, r); err != nil {
		return err
	}
	raw, err := Credential(centralRoot(originalRoot), r.Account, "codex")
	if err != nil {
		return err
	}
	subject, err := credentialSubject(raw)
	if err != nil {
		return err
	}
	return core.WriteJSON(pinPath, subject)
}
func grantPath(root, account string) string { return filepath.Join(Dir(root, account), "broker.json") }
func ReadGrant(root, account string) (Grant, error) {
	if err := Validate(account, "codex"); err != nil {
		return Grant{}, err
	}
	var g Grant
	raw, err := os.ReadFile(grantPath(root, account))
	if err != nil {
		return g, fmt.Errorf("central account has not been connected to this project")
	}
	if json.Unmarshal(raw, &g) != nil || g.Account != account || g.Capability == "" || g.AccountID == "" || g.Socket == "" {
		return g, fmt.Errorf("invalid token supply grant")
	}
	_, err = ResolveBinding("codex", BrokeredAccessToken, g.Project, account, g.Binding)
	return g, err
}
func InstallGrant(root string, g Grant) error {
	if _, err := ResolveBinding("codex", BrokeredAccessToken, g.Project, g.Account, g.Binding); err != nil {
		return err
	}
	if g.Socket != BrokerSocket || len(g.Capability) != 64 || g.AccountID == "" {
		return fmt.Errorf("invalid token supply grant")
	}
	if err := Prepare(root, g.Account, "codex"); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(Config(root, g.Account), "auth.json")); !os.IsNotExist(err) {
		return fmt.Errorf("managed credentials must not be installed in a brokered project profile")
	}
	return core.WriteJSON(grantPath(root, g.Account), g)
}
func capabilityPath(root, capability string) string {
	return filepath.Join(root, "central", "grants", fmt.Sprintf("%x.json", sha256.Sum256([]byte(capability))))
}
func IssueGrant(root, project, account string) (Grant, error) {
	token, err := CentralToken(root, account)
	if err != nil {
		return Grant{}, err
	}
	spec, err := (brokered{}).Binding(project, account)
	if err != nil {
		return Grant{}, err
	}
	// Reuse a project/account capability on retries and manager restart.
	path := filepath.Join(root, "central", "bindings", spec.ID+".json")
	var g Grant
	if raw, e := os.ReadFile(path); e == nil {
		if json.Unmarshal(raw, &g) != nil || g.AccountID != token.AccountID || g.Project != project || g.Account != account || g.Binding != spec.ID || len(g.Capability) != 64 {
			return g, fmt.Errorf("central binding identity mismatch")
		}
	} else if !os.IsNotExist(e) {
		return g, e
	} else {
		var secret [32]byte
		if _, err = rand.Read(secret[:]); err != nil {
			return g, err
		}
		g = Grant{BrokerSocket, fmt.Sprintf("%x", secret), project, account, spec.ID, token.AccountID}
	}
	for _, dir := range []string{filepath.Dir(path), filepath.Dir(capabilityPath(root, g.Capability))} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return g, err
		}
	}
	if err = core.WriteJSON(capabilityPath(root, g.Capability), g); err != nil {
		return g, err
	}
	return g, core.WriteJSON(path, g)
}

// StartBroker exposes only bearer-capability-scoped access tokens over a Unix
// socket. The shared tools mount is read-only; no credential files live there.
func StartBroker(root, socket string, refresh func(context.Context, string) error) (io.Closer, error) {
	if st, err := os.Lstat(socket); err == nil {
		if st.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("broker path is not a socket")
		}
		if err = os.Remove(socket); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(socket, 0666); err != nil {
		ln.Close()
		return nil, err
	}
	srv := &http.Server{ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 9 * time.Second, IdleTimeout: 5 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fail := func() { http.Error(w, "central authentication unavailable or unauthorized", http.StatusForbidden) }
		if r.Method != "POST" || r.URL.Path != "/token" {
			fail()
			return
		}
		capability := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(capability) != 64 {
			fail()
			return
		}
		var g Grant
		raw, err := os.ReadFile(capabilityPath(root, capability))
		if err != nil || json.Unmarshal(raw, &g) != nil || g.Capability != capability {
			fail()
			return
		}
		var req tokenRequest
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req) != nil || (req.PreviousAccountID != "" && req.PreviousAccountID != g.AccountID) {
			fail()
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		var lock *os.File
		for {
			lock, err = centralLock(root, g.Account)
			if err == nil {
				break
			}
			select {
			case <-ctx.Done():
				fail()
				return
			case <-time.After(25 * time.Millisecond):
			}
		}
		defer lock.Close()
		if req.Refresh {
			if err = refresh(ctx, g.Account); err != nil {
				fail()
				return
			}
		}
		token, err := CentralToken(root, g.Account)
		if err != nil || token.AccountID != g.AccountID {
			fail()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(token)
	})
	go srv.Serve(ln)
	return srv, nil
}
func FetchToken(ctx context.Context, root, account, project, binding, previous string, refresh bool) (Token, error) {
	g, err := ReadGrant(root, account)
	if err != nil {
		return Token{}, err
	}
	if g.Project != project || g.Binding != binding || (previous != "" && previous != g.AccountID) {
		return Token{}, fmt.Errorf("token request account/binding mismatch")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", g.Socket)
	}}
	defer transport.CloseIdleConnections()
	body, _ := json.Marshal(tokenRequest{previous, refresh})
	req, err := http.NewRequestWithContext(ctx, "POST", "http://broker/token", bytes.NewReader(body))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.Capability)
	res, err := (&http.Client{Transport: transport, Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("central authentication unavailable")
	}
	defer res.Body.Close()
	var token Token
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 1024*1024)).Decode(&token) != nil || token.AccountID != g.AccountID || token.AccessToken == "" {
		return Token{}, fmt.Errorf("central authentication failed; check account login")
	}
	return token, nil
}
