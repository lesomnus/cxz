package configtrust

import (
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestWrappedAndLegacyTrustErrors(t *testing.T) {
	original := &Required{Findings: []string{"  - /services/dev/privileged: privileged container requested"}}
	wrapped := fmt.Errorf("configuration %q: %w", "/project/compose.yaml", original)
	wire := status.Convert(wrapped).Err()
	if status.Code(wire) != codes.FailedPrecondition || !IsRequired(wire) {
		t.Fatal(wire)
	}
	if !IsRequired(status.Error(codes.Unknown, wrapped.Error())) {
		t.Fatal("old manager unsupported")
	}
	for _, err := range []error{nil, fmt.Errorf("--trust-config"), status.Error(codes.PermissionDenied, original.Error()), status.Error(codes.Unknown, "devcontainer configuration requires explicit trust: broken")} {
		if IsRequired(err) {
			t.Fatal("false trust prompt", err)
		}
	}
}
