package server

import (
	"context"
	"encoding/json"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectconfig"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Devcontainer(_ context.Context, r *api.DevcontainerInput) (*api.Receipt, error) {
	if s.manager == nil {
		return nil, status.Error(codes.FailedPrecondition, "devcontainer settings require an installed host manager")
	}
	var spec projectconfig.Spec
	if err := json.Unmarshal(r.Spec, &spec); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid devcontainer settings")
	}
	if err := projectconfig.Save(s.manager.Root, spec); err != nil {
		return nil, err
	}
	return &api.Receipt{Status: "Devcontainer settings saved; applies to new or explicitly recreated project containers"}, nil
}
