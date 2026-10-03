package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/secretfile"
)

// managerSecrets writes a secret through the manager, for a client that cannot
// reach the engine itself. The file still ends up in the same host tmpfs
// written by the same helper; what changes is who runs the helper.
//
// Only a connection that keeps the secret off the network gets here: a local
// client writes the file itself, ssh encrypts what it carries, and the exposed
// TCP surface is authenticated plaintext, so it is refused instead.
type managerSecrets struct{ client api.SessionsClient }

func (s managerSecrets) PutSecret(_, ctx context.Context, project *api.Project, session string, body []byte) (string, error) {
	if project == nil || project.Id == "" {
		return "", fmt.Errorf("project unavailable")
	}
	reply, err := s.call(ctx, secretfile.Request{Action: "put", Project: project.Id, Session: session, Secret: body})
	return reply.Path, err
}

func (s managerSecrets) DeleteSecret(_, ctx context.Context, project *api.Project, path string) error {
	if project == nil || project.Id == "" {
		return fmt.Errorf("project unavailable")
	}
	_, err := s.call(ctx, secretfile.Request{Action: "delete", Project: project.Id, Path: path})
	return err
}

func (s managerSecrets) call(ctx context.Context, r secretfile.Request) (secretfile.Reply, error) {
	var reply secretfile.Reply
	spec, err := json.Marshal(r)
	if err != nil {
		return reply, err
	}
	// The request holds the secret itself, so the buffer is cleared as soon as
	// the call is over rather than left for the collector.
	defer clear(spec)
	out, err := s.client.Docker(ctx, &api.DockerInput{Action: "secret-file", Spec: spec})
	if err != nil {
		return reply, err
	}
	if err = json.Unmarshal([]byte(out.Status), &reply); err != nil {
		return reply, err
	}
	return reply, nil
}
