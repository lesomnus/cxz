//go:build linux

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/kube"
)

// _agent-ensure KIND installs the selected agent CLI into a project pod's own
// /cxz/tools and prints its path. A Docker project shares the manager's tools
// volume; a pod does not, so it fetches for itself.
func agentEnsure(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: _agent-ensure claude|codex")
	}
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	musl, _ := filepath.Glob("/lib/ld-musl-*.so.1")
	bin, err := distribution.Ensure(context.Background(), "/cxz/tools", args[0], arch, len(musl) > 0)
	if err != nil {
		return err
	}
	fmt.Println(bin)
	return nil
}

func init() { kube.Helpers["_agent-ensure"] = agentEnsure }
