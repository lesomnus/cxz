package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"os"
	"time"
)

func updateInternal(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "_publish-tools":
		return true, cxzupdate.PublishTools()
	case "_update-runtime-start":
		if len(args) != 4 {
			return true, fmt.Errorf("invalid runtime child arguments")
		}
		return true, cxzupdate.RuntimeChild(args[1], args[2], args[3])
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
