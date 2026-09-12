package lifecycle

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/lesomnus/cxz/internal/projectref"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/slug"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
)

func validName(name string) error {
	if len(name) > 200 || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n\t\x00") {
		return status.Error(codes.InvalidArgument, "display name must be nonempty, at most 200 bytes and contain no control whitespace")
	}
	return nil
}

// Caller holds shared.mu; the generated unique index is the final arbiter.
func (s Layer) alias(ctx context.Context, id, name, requested string) (string, error) {
	explicit := requested != ""
	candidate := requested
	if !explicit {
		candidate = projectref.ShortAlias(name)
	}
	candidate, err := slug.ParseAlias(candidate)
	if err != nil {
		return "", err
	}
	base := candidate
	sum := sha256.Sum256([]byte(id))
	fingerprint := fmt.Sprintf("%x", sum)
	for n := 4; n <= 32; n += 4 {
		p, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: resource.ProjectRef_builder{Alias: &candidate}.Build(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
		if status.Code(err) == codes.NotFound {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		if p.GetRuntimeId() == id {
			return candidate, nil
		}
		if explicit {
			return "", status.Error(codes.AlreadyExists, "project alias is already in use")
		}
		candidate = base + "-" + fingerprint[:n]
	}
	return "", status.Error(codes.AlreadyExists, "cannot allocate a unique project alias")
}
