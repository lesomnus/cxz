package server

import (
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The shared engine belongs to the host, which a project runtime is inside of
// rather than on. Each operation refuses by itself: one handler used to refuse
// the whole envelope, so the reason was the same sentence whatever was asked.
func TestEngineOperationsNeedAManager(t *testing.T) {
	s := &Server{}
	if _, err := s.SaveEngine(t.Context(), &api.SaveEngineInput{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.StartEngine(t.Context(), &api.StartEngineInput{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.StopEngine(t.Context(), &api.Empty{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.PruneEngine(t.Context(), &api.Empty{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.EngineStatus(t.Context(), &api.Empty{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.GetEngineInfo(t.Context(), &api.Empty{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	// The channel and the pin are the installation's files, which only a
	// manager has, even though the build itself is the same one.
	if _, err := s.GetInstallationVersion(t.Context(), &api.Empty{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
}
