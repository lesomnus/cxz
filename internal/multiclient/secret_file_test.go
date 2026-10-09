package multiclient

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
)

type secretDaemon struct {
	daemon
	project string
	secret  string
	calls   int
}

func (d *secretDaemon) PutSecretFile(_ context.Context, in *api.PutSecretFileInput, _ ...grpc.CallOption) (*api.SecretFileReply, error) {
	d.calls++
	d.project, d.secret = in.Project, string(in.Secret)
	return &api.SecretFileReply{Path: "/cxz/secrets/ab/secret"}, nil
}

// The project says which connection writes the file, read from a field rather
// than parsed out of a payload -- and the caller's own request is left intact,
// secret included, because it may have to be retried.
func TestSecretFileRoutesByProject(t *testing.T) {
	a, b := &secretDaemon{}, &secretDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	in := &api.PutSecretFileInput{Project: "b::P", Secret: []byte("hunter2")}
	if _, err := c.PutSecretFile(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if a.calls != 0 || b.project != "P" || b.secret != "hunter2" {
		t.Fatal("wrong manager, project or secret", a.calls, b.project, b.secret)
	}
	if in.Project != "b::P" || string(in.Secret) != "hunter2" {
		t.Fatal("the caller's request was mutated", in.Project, string(in.Secret))
	}
}
