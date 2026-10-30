package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/networkservices"
	"os"
	"time"
)

func updateInternal(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "_recover-network-services", "_refresh-network-services":
		if len(args) != 4 {
			return true, fmt.Errorf("invalid network service arguments")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if args[0] == "_recover-network-services" {
			return true, networkservices.RecoverLocked(ctx, args[1], args[2], args[3])
		}
		report, err := networkservices.Refresh(ctx, args[1], args[2], args[3])
		if encodeErr := json.NewEncoder(os.Stdout).Encode(report); err == nil {
			err = encodeErr
		}
		return true, err

	case "_publish-tools":
		return true, cxzupdate.PublishTools()
	case "_update-runtime-start":
		if len(args) != 4 {
			return true, fmt.Errorf("invalid runtime child arguments")
		}
		return true, cxzupdate.RuntimeChild(args[1], args[2], args[3])
	case "_recover-runtime":
		if len(args) != 2 {
			return true, fmt.Errorf("invalid runtime recovery arguments")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		return true, cxzupdate.RecoverRuntime(ctx, args[1])
	case "_update-policy":
		if len(args) != 3 {
			return true, fmt.Errorf("invalid update policy arguments")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, e := localUpdateCommand(ctx, args[1], args[2])
		if e != nil {
			return true, e
		}
		_, e = os.Stdout.Write(b)
		return true, e
	case "_rollout-apply":
		// root, manager container, owner, forced. The caller is the frontend,
		// which is the side that knows the container and owner.
		if len(args) != 5 {
			return true, fmt.Errorf("invalid rollout arguments")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		b, e := applyRollout(ctx, args[1], args[2], args[3], args[4] == "force")
		if e != nil {
			return true, e
		}
		_, e = os.Stdout.Write(b)
		return true, e
	case "_maintenance":
		if len(args) != 4 {
			return true, fmt.Errorf("invalid maintenance arguments")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		h, e := cxzupdate.Call(ctx, args[1], args[2], args[3])
		if e != nil {
			return true, e
		}
		return true, json.NewEncoder(os.Stdout).Encode(h)
	case "_update-runtime":
		if len(args) != 4 {
			return true, fmt.Errorf("invalid runtime update arguments")
		}
		b, e := os.ReadFile(args[3])
		if e != nil {
			return true, e
		}
		var r cxzupdate.Release
		if e = json.Unmarshal(b, &r); e != nil {
			return true, e
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		return true, cxzupdate.ReplaceRuntime(ctx, args[1], args[2], r)
	case "_update-manager":
		if len(args) != 2 {
			return true, fmt.Errorf("invalid manager update arguments")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return true, cxzupdate.ReplaceManager(ctx, args[1])
	}
	return false, nil
}
