package auxiliary

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"io"
	"os"
	"path/filepath"
	"time"
)

type HelperInput struct {
	Input  Input
	Binary string
	Grant  *accounts.Grant
}

func Serve(ctx context.Context, root string, r io.Reader, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var q HelperInput
	if e := json.NewDecoder(io.LimitReader(r, 128<<10)).Decode(&q); e != nil {
		return e
	}
	p := q.Input.Profile
	if e := accounts.Validate(p.Account, p.Agent); e != nil {
		return e
	}
	if e := accounts.Prepare(root, p.Account, p.Agent); e != nil {
		return e
	}
	lock, e := core.Lock(filepath.Join(accounts.Dir(root, p.Account), "login.lock"))
	if e != nil {
		return fmt.Errorf("auxiliary account is busy with login or another job")
	}
	defer lock.Close()
	if q.Grant != nil {
		if p.Agent != "codex" || p.Backend != accounts.BrokeredAccessToken || q.Grant.Scope != "auxiliary" || q.Grant.Account != p.Account {
			return fmt.Errorf("wrong auxiliary grant")
		}
		if e = accounts.InstallGrant(root, *q.Grant); e != nil {
			return e
		}
	}
	backend, e := accounts.Resolve(p.Agent, p.Backend)
	if e != nil {
		return e
	}
	auth, e := backend.Launch(root, p.Account, os.Environ())
	if e != nil {
		return fmt.Errorf("auxiliary account %s needs login: run cxz ai login %s on the Manager host", p.Account, p.Account)
	}
	dir, e := os.MkdirTemp("", "cxz-auxiliary-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	var token func(string, bool) (accounts.Token, error)
	if p.Backend == accounts.BrokeredAccessToken {
		token = func(previous string, refresh bool) (accounts.Token, error) {
			return accounts.FetchAuxiliaryToken(ctx, root, p.Account, previous, refresh)
		}
	}
	out, e := Provider(ctx, q.Binary, dir, auth, q.Input, token)
	if e != nil {
		return e
	}
	return json.NewEncoder(w).Encode(out)
}
func Login(ctx context.Context, root, bin string, p Profile, r io.Reader, w, errw io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	if p.Backend != accounts.ProjectLocalOAuth {
		return fmt.Errorf("use cxz account login %s for central Codex authentication", p.Account)
	}
	b, e := accounts.Resolve(p.Agent, p.Backend)
	if e != nil {
		return e
	}
	return b.Login(ctx, accounts.LoginRequest{Root: root, Account: p.Account, Workspace: "/tmp", Binary: bin, Env: os.Environ(), Input: r, Output: w, Error: errw})
}
