package workspace

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

func TestAssetMountLive(t *testing.T) {
	if os.Getenv("CXZ_TEST_ASSET_MOUNTS") != "1" {
		t.Skip("set CXZ_TEST_ASSET_MOUNTS=1 for isolated Docker mount verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := "cxz-assets-test-" + core.ID()
	run := func(args ...string) []byte {
		t.Helper()
		out, err := dockerx.Run(ctx, args...)
		if err != nil {
			t.Fatalf("docker %v: %v", args, err)
		}
		return out
	}
	run("volume", "create", name)
	defer dockerx.Run(context.Background(), "volume", "rm", name)
	run("run", "-d", "--name", name, "--mount", "type=volume,source="+name+",target=/var/lib/cxz", "alpine:3.22", "sleep", "120")
	defer dockerx.Run(context.Background(), "rm", "-f", name)
	run("exec", name, "sh", "-c", "mkdir -p /var/lib/cxz/assets/exports/project/session/asset /var/lib/cxz/assets/exports/other /var/lib/cxz/assets/cas; printf attachment > /var/lib/cxz/assets/cas/blob; ln /var/lib/cxz/assets/cas/blob /var/lib/cxz/assets/exports/project/session/asset/report.txt; chmod 444 /var/lib/cxz/assets/cas/blob")
	manager, err := dockerx.Inspect(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	source, err := assetEnginePath("/var/lib/cxz/assets/exports/project", manager)
	if err != nil {
		t.Fatal(err)
	}
	output := run("run", "--rm", "--user", "12345:12345", "--mount", "type=bind,source="+source+",target=/cxz/assets,readonly", "alpine:3.22", "sh", "-c", "cat /cxz/assets/session/asset/report.txt; test ! -e /cxz/assets/other; test ! -e /cxz/assets/cas; if touch /cxz/assets/new 2>/dev/null; then exit 1; fi")
	if strings.TrimSpace(string(output)) != "attachment" {
		t.Fatal("agent could not read exported attachment", string(output))
	}
	output = run("run", "--rm", "--mount", "type=bind,source="+source+",target=/cxz/assets,readonly", "alpine:3.22", "sh", "-c", "if echo changed >> /cxz/assets/session/asset/report.txt 2>/dev/null; then exit 1; fi; cat /cxz/assets/session/asset/report.txt")
	if !strings.HasSuffix(strings.TrimSpace(string(output)), "attachment") {
		t.Fatal("root changed exported blob", string(output))
	}
}
