package server

import (
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Writing a secret file needs the engine the project container runs on, which a
// project runtime does not have. It refuses rather than answering emptily: a
// caller told that nothing went wrong would leave a redaction pointing at a
// file nobody wrote.
func TestSecretFileRefusesWithoutAManager(t *testing.T) {
	s := &Server{}
	if _, err := s.PutSecretFile(t.Context(), &api.PutSecretFileInput{Project: "p", Secret: []byte("x")}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.DeleteSecretFile(t.Context(), &api.DeleteSecretFileInput{Project: "p", Path: "/x"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
}
