package tui

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/api"
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
	// The secret is the request, so the copy made for the call is cleared with
	// it rather than left for the collector.
	in := &api.PutSecretFileInput{Project: project.Id, Session: session, Secret: body}
	defer clear(in.Secret)
	reply, err := s.client.PutSecretFile(ctx, in)
	if err != nil {
		return "", err
	}
	return reply.Path, nil
}

func (s managerSecrets) DeleteSecret(_, ctx context.Context, project *api.Project, path string) error {
	if project == nil || project.Id == "" {
		return fmt.Errorf("project unavailable")
	}
	_, err := s.client.DeleteSecretFile(ctx, &api.DeleteSecretFileInput{Project: project.Id, Path: path})
	return err
}
