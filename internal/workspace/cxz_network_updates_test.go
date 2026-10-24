//go:build !windows

package workspace

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/networkservices"
)

type unavailableReleaseTransport struct{}

func (unavailableReleaseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("release channel offline")
}

func TestNetworkServiceReconciliationIsRecordedBeforeChannelLookup(t *testing.T) {
	root := t.TempDir()
	if err := cxzupdate.SetPolicy(root, true); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `#!/bin/sh
if [ "$1" != inspect ]; then exit 1; fi
case "$2" in
manager) printf '%s' '[{"Id":"manager","Config":{"Image":"actual:new","Labels":{"cxz.owner":"owner","cxz.role":"daemon"}}}]';;
manager-web) printf '%s' '[{"Id":"web","Config":{"Image":"installed:old","Labels":{"cxz.owner":"owner","cxz.role":"web"}},"State":{"Running":true}}]';;
manager-remote) printf '%s' '[{"Id":"relay","Config":{"Image":"installed:old","Labels":{"cxz.owner":"owner","cxz.role":"remote"}},"State":{"Running":false}}]';;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	previous := http.DefaultTransport
	http.DefaultTransport = unavailableReleaseTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
	// Simulate a host operation: the rollout must record the contention and
	// live drift, then attempt the channel without losing the sibling report.
	lock, err := core.Lock(filepath.Join(root, networkservices.LockFile))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	m := &Manager{Root: root, Container: "manager", Owner: "owner"}
	m.cxzUpdateStep(t.Context())
	state, err := cxzupdate.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	r := state.NetworkServices
	if r == nil || r.ManagerImage != "actual:new" || !r.Web.NeedsRefresh || r.Relay.NeedsRefresh || !strings.Contains(r.Error, "busy") {
		t.Fatalf("live reconciliation lost: %+v", r)
	}
	if !strings.Contains(state.Reason, "release channel offline") {
		t.Fatal("channel lookup not reached", state.Reason)
	}
}
