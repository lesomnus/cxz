package server

import (
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Aux belongs to the manager: the controller that runs it holds the account
// profiles and the rolling context for every session on the installation. A
// project runtime has neither, so it refuses rather than answering emptily --
// an empty answer would read as "no summaries" instead of "wrong server".
func TestAuxRefusesWithoutAManager(t *testing.T) {
	s := &Server{}
	ctx := t.Context()
	for name, err := range map[string]error{
		"run":    must(s.AuxRun(ctx, &api.AuxRunInput{SessionId: "s", Kinds: []string{"summary"}})),
		"status": must(s.AuxStatus(ctx, &api.AuxStatusInput{SessionId: "s"})),
		"prefer": must(s.AuxPrefer(ctx, &api.AuxPreferInput{SessionId: "s"})),
		"cancel": must(s.AuxCancel(ctx, &api.AuxCancelInput{SessionId: "s"})),
		"forget": must(s.AuxForget(ctx, &api.AuxForgetInput{SessionId: "s"})),
		"config": must(s.AuxConfig(ctx, &api.Empty{})),
		"set":    must(s.AuxSetConfig(ctx, &api.AuxSetConfigInput{})),
		"models": must(s.AuxModels(ctx, &api.AuxModelsInput{Account: "work"})),
		"login":  must(s.AuxLoginInfo(ctx, &api.AuxLoginInfoInput{Account: "work"})),
	} {
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func must[T any](_ T, err error) error { return err }
