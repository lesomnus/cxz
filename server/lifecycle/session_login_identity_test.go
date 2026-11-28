package lifecycle

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// The pin lives on the Account resource, which only a Manager owns: it reads it
// from there rather than from the request, writes down what a first login
// reached, and refuses to let whoever asked for the login name the pin their
// own login is then measured against.
func TestManagerSessionLoginPinsTheAccountItReached(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &loginRuntime{fixture: &fixture{p: &api.Project{Id: "project", Name: "workspace", Workspace: "/workspace", State: "running"}}, cancelled: make(chan struct{})}
	stack, err := Build(ctx, db, f, nil, Options{Manager: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: "/workspace"}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: "work1", Agent: "claude", AuthBackend: accounts.ProjectLocalOAuth}.Build()); err != nil {
		t.Fatal(err)
	}
	ln := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	resource.RegisterServer(g, stack)
	go g.Serve(ln)
	defer g.Stop()
	conn, err := grpc.NewClient("passthrough:///login", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := resourceclient.New(conn)

	stored := func() string {
		a, err := stack.Account().Get(ctx, resource.AccountGetRequest_builder{Ref: resource.AccountRef_builder{Alias: ptr("work1")}.Build(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			t.Fatal(err)
		}
		return a.GetLoginIdentity()
	}
	login := func(identity string) error {
		_, err := c.LoginSession(ctx, accounts.SessionLogin{Project: "project", Account: "work1", Key: "without-input", Identity: identity, Input: io.NopCloser(strings.NewReader("")), Output: io.Discard})
		return err
	}

	if stored() != "" {
		t.Fatal("a new account is pinned to nothing until it logs in")
	}
	// A client naming the pin would be naming what its own login is checked
	// against, which is the whole of the check.
	if err := login("forged"); status.Code(err) != codes.InvalidArgument {
		t.Fatal("client-supplied login identity accepted", err)
	}
	if len(f.pinned) != 0 {
		t.Fatal("refused request still reached the runtime", f.pinned)
	}

	f.reports = "account-one"
	if err := login(""); err != nil {
		t.Fatal(err)
	}
	if stored() != "account-one" {
		t.Fatal("first login was not pinned", stored())
	}
	if len(f.pinned) != 1 || f.pinned[0] != "" {
		t.Fatal("runtime was told to require something before anything was pinned", f.pinned)
	}

	// From here the stored pin is what the runtime is given to enforce, and a
	// login that reports the same account changes nothing.
	if err := login(""); err != nil {
		t.Fatal(err)
	}
	if len(f.pinned) != 2 || f.pinned[1] != "account-one" {
		t.Fatal("stored pin was not handed to the runtime", f.pinned)
	}
	if stored() != "account-one" {
		t.Fatal("pin changed without a different account", stored())
	}

	// Clearing it is how a person says a change of provider account is meant;
	// the next login pins whatever it reaches.
	pinned, err := stack.Account().Get(ctx, resource.AccountGetRequest_builder{Ref: resource.AccountRef_builder{Alias: ptr("work1")}.Build(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Account().Patch(ctx, resource.AccountPatchRequest_builder{Ref: resource.AccountRef_builder{Alias: ptr("work1")}.Build(), LoginIdentity: ptr(""), DateUpdated: pinned.GetDateUpdated()}.Build()); err != nil {
		t.Fatal(err)
	}
	if stored() != "" {
		t.Fatal("pin not cleared", stored())
	}
	f.reports = "account-two"
	if err := login(""); err != nil {
		t.Fatal(err)
	}
	if len(f.pinned) != 3 || f.pinned[2] != "" {
		t.Fatal("cleared pin was still required of the runtime", f.pinned)
	}
	if stored() != "account-two" {
		t.Fatal("login after clearing did not re-pin", stored())
	}
}
