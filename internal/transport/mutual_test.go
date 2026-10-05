package transport

import (
	"context"
	"strings"
	"testing"
)

// Without an identity there is nothing to present, and the error has to name
// the command that produces one rather than failing as a transport fault.
func TestMutualEndpointWithoutAnIdentity(t *testing.T) {
	_, err := DialEndpoint("mtls://127.0.0.1:7349", "", nil)
	if err == nil || !strings.Contains(err.Error(), "connection enroll") {
		t.Fatal(err)
	}
	if _, err = DialEndpoint("mtls://127.0.0.1:7349", "", &Identity{Certificate: []byte("x")}); err == nil {
		t.Fatal("accepted an incomplete identity")
	}
}

func TestMutualSchemeIsConfidential(t *testing.T) {
	ctx := WithScheme(WithRemote(context.Background()), "mtls")
	if !Confidential(ctx) {
		t.Fatal("an mtls connection is not treated as confidential")
	}
	if Confidential(WithScheme(WithRemote(context.Background()), "tcp")) {
		t.Fatal("plaintext tcp is treated as confidential")
	}
}
