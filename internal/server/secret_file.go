package server

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A secret file needs the Docker engine the project container runs on, which a
// project runtime does not have. It refuses rather than answering emptily: a
// caller that was told nothing went wrong would leave the secret unwritten and
// the redaction pointing at a file that is not there.
func (s *Server) requireManagerSecrets() error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "writing a secret file requires an installed host manager")
	}
	return nil
}

func (s *Server) PutSecretFile(ctx context.Context, r *api.PutSecretFileInput) (*api.SecretFileReply, error) {
	if err := s.requireManagerSecrets(); err != nil {
		return nil, err
	}
	return s.manager.PutSecretFile(ctx, r)
}

func (s *Server) DeleteSecretFile(ctx context.Context, r *api.DeleteSecretFileInput) (*api.SecretFileReply, error) {
	if err := s.requireManagerSecrets(); err != nil {
		return nil, err
	}
	return s.manager.DeleteSecretFile(ctx, r)
}
