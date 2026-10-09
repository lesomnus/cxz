package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/transport"
	"google.golang.org/grpc"
)

// What the manager was asked for, recorded as the call it became.
type secretCall struct {
	call             string
	project, session string
	path             string
	secret           []byte
}
type secretClient struct {
	api.SessionsClient
	seen []secretCall
	path string
}

func (c *secretClient) PutSecretFile(_ context.Context, in *api.PutSecretFileInput, _ ...grpc.CallOption) (*api.SecretFileReply, error) {
	c.seen = append(c.seen, secretCall{
		call: "put", project: in.Project, session: in.Session,
		secret: append([]byte(nil), in.Secret...),
	})
	return &api.SecretFileReply{Path: c.path}, nil
}

func (c *secretClient) DeleteSecretFile(_ context.Context, in *api.DeleteSecretFileInput, _ ...grpc.CallOption) (*api.SecretFileReply, error) {
	c.seen = append(c.seen, secretCall{call: "delete", project: in.Project, path: in.Path})
	return &api.SecretFileReply{}, nil
}

// Who writes the secret file depends on the connection, and the one case that
// must never happen is a secret put on the wire in the clear.
func TestSecretStoreFollowsTheConnection(t *testing.T) {
	m := &model{wisp: nil}
	local := m.secretStore(transport.WithLocal(context.Background()))
	if local == nil {
		t.Fatal("a local client was refused its own helper")
	}
	ssh := m.secretStore(transport.WithScheme(transport.WithRemote(context.Background()), "ssh"))
	if _, ok := ssh.(managerSecrets); !ok {
		t.Fatalf("an ssh client did not route through the manager: %T", ssh)
	}
	tcp := m.secretStore(transport.WithScheme(transport.WithRemote(context.Background()), "tcp"))
	if tcp != nil {
		t.Fatalf("an exposed TCP client was handed a secret store: %T", tcp)
	}
	// A remote connection of unknown scheme is not assumed to be safe.
	if got := m.secretStore(transport.WithRemote(context.Background())); got != nil {
		t.Fatalf("an unidentified remote connection was handed a secret store: %T", got)
	}
}

// The manager is asked for the same two operations the local helper performs,
// and the reply's path is what the message ends up carrying.
func TestManagerSecretsRoundTrip(t *testing.T) {
	c := &secretClient{path: "/cxz/secrets/ab12/secret"}
	store := managerSecrets{client: c}
	project := &api.Project{Id: "project", ContainerId: "container", RemoteUser: "hypnos"}
	path, err := store.PutSecret(context.Background(), context.Background(), project, "session/run", []byte("hunter2"))
	if err != nil || path != c.path {
		t.Fatal(path, err)
	}
	if err = store.DeleteSecret(context.Background(), context.Background(), project, path); err != nil {
		t.Fatal(err)
	}
	if len(c.seen) != 2 || c.seen[0].call != "put" || c.seen[1].call != "delete" {
		t.Fatalf("manager saw %+v", c.seen)
	}
	if string(c.seen[0].secret) != "hunter2" || c.seen[0].session != "session/run" {
		t.Fatalf("put did not carry the secret and its scope: %+v", c.seen[0])
	}
	if c.seen[1].path != c.path || len(c.seen[1].secret) != 0 {
		t.Fatalf("delete carried the wrong fields: %+v", c.seen[1])
	}
	// Only one of the two calls can carry a secret at all, which is why they
	// are two calls: the delete has no field to put one in.
	if _, ok := any(&api.DeleteSecretFileInput{}).(interface{ GetSecret() []byte }); ok {
		t.Fatal("the delete request has somewhere to put a secret")
	}
	// A project the client cannot name is refused before anything is sent.
	if _, err = store.PutSecret(context.Background(), context.Background(), nil, "s", []byte("x")); err == nil {
		t.Fatal("accepted a secret with no project")
	}
	if len(c.seen) != 2 {
		t.Fatal("a refused request still reached the manager")
	}
	if !strings.Contains(c.path, "/cxz/secrets/") {
		t.Fatal("fixture path is not a secret path")
	}
}
