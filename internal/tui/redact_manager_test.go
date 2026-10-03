package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/secretfile"
	"github.com/lesomnus/cxz/internal/transport"
	"google.golang.org/grpc"
)

type secretClient struct {
	api.SessionsClient
	seen []secretfile.Request
	path string
}

func (c *secretClient) Docker(_ context.Context, in *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	var r secretfile.Request
	if err := json.Unmarshal(in.Spec, &r); err != nil {
		return nil, err
	}
	c.seen = append(c.seen, r)
	if in.Action != "secret-file" {
		return nil, context.Canceled
	}
	b, _ := json.Marshal(secretfile.Reply{Path: c.path})
	return &api.Receipt{Status: string(b)}, nil
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
	if len(c.seen) != 2 || c.seen[0].Action != "put" || c.seen[1].Action != "delete" {
		t.Fatalf("manager saw %+v", c.seen)
	}
	if string(c.seen[0].Secret) != "hunter2" || c.seen[0].Session != "session/run" {
		t.Fatalf("put did not carry the secret and its scope: %+v", c.seen[0])
	}
	if c.seen[1].Path != c.path || len(c.seen[1].Secret) != 0 {
		t.Fatalf("delete carried the wrong fields: %+v", c.seen[1])
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
