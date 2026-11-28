package lifecycle

import (
	"bufio"
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/projectid"
	"io"
	"net"
	"os"
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

type loginRuntime struct {
	*fixture
	cancelled chan struct{}
	// reports is the identity an accepted login claims to have reached; pinned
	// collects what each request was told to require.
	reports string
	pinned  []string
}

func (f *loginRuntime) LoginSession(ctx context.Context, r accounts.SessionLogin) (accounts.SessionLoginResult, error) {
	none := accounts.SessionLoginResult{}
	if r.Project != projectid.Canonical("project") || r.Account != "work1" {
		return none, fmt.Errorf("wrong login target")
	}
	f.pinned = append(f.pinned, r.Identity)
	fmt.Fprintln(r.Output, "https://example.invalid/login")
	switch r.Key {
	case "without-input":
		return accounts.SessionLoginResult{Identity: f.reports}, nil
	case "cancel":
		<-ctx.Done()
		close(f.cancelled)
		return none, ctx.Err()
	case "input":
		code, err := bufio.NewReader(r.Input).ReadString('\n')
		if err != nil || code != "fixture#state\n" {
			return none, fmt.Errorf("unexpected login input")
		}
		return accounts.SessionLoginResult{Identity: f.reports}, nil
	case "invalid-frame":
		<-ctx.Done()
		return none, ctx.Err()
	}
	return none, fmt.Errorf("unexpected login key")
}

type loginTestWriter func([]byte) (int, error)

func (w loginTestWriter) Write(p []byte) (int, error) { return w(p) }

func TestSessionLoginRPCInputExitCancellationAndPrivacy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &loginRuntime{fixture: &fixture{p: &api.Project{Id: "project", Name: "workspace", Workspace: "/workspace", State: "running"}}, cancelled: make(chan struct{})}
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: "/workspace"}.Build()); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"claude", "codex"} {
		alias := "work1"
		if agent == "codex" {
			alias = "codex"
		}
		if _, err := stack.Account().Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Agent: agent, AuthBackend: accounts.ProjectLocalOAuth}.Build()); err != nil {
			t.Fatal(err)
		}
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
	var before, after int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit").Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"input", "without-input", "cancel"} {
		in, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		request, abort := context.WithCancel(ctx)
		seen := false
		_, err = c.LoginSession(request, accounts.SessionLogin{Project: "project", Account: "work1", Key: key, Input: in, Output: loginTestWriter(func(p []byte) (int, error) {
			seen = strings.Contains(string(p), "https://example.invalid/login")
			if key == "cancel" {
				abort()
			} else if key == "input" {
				if _, err := io.WriteString(w, "fixture#state\n"); err != nil {
					return 0, err
				}
			}
			return len(p), nil
		})})
		w.Close()
		abort()
		if !seen || (key != "cancel" && err != nil) || (key == "cancel" && status.Code(err) != codes.Canceled) {
			t.Fatalf("%s: seen %v, %v", key, seen, err)
		}
	}
	select {
	case <-f.cancelled:
	case <-ctx.Done():
		t.Fatal("remote cancellation did not reach runtime")
	}
	// These requests must fail without starting login or writing any audit data.
	for _, alias := range []string{"codex", "missing"} {
		_, err := c.LoginSession(ctx, accounts.SessionLogin{Project: "project", Account: alias, Key: "without-input", Input: io.NopCloser(strings.NewReader("")), Output: io.Discard})
		if err == nil {
			t.Fatal("unsupported/missing account accepted", alias)
		}
	}
	stream, err := resource.NewProjectServiceClient(conn).SessionLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(resource.ProjectLoginRequest_builder{Ref: projectRef("project"), Account: accountRef("work1"), SessionKey: ptr("invalid-frame")}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(resource.ProjectLoginRequest_builder{Account: accountRef("codex"), Input: []byte("secret")}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); status.Code(err) != codes.InvalidArgument {
		t.Fatal("login input could retarget account", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit").Scan(&after); err != nil || before != after {
		t.Fatal("login stream entered resource audit", before, after, err)
	}
}
