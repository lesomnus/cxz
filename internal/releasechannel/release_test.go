package releasechannel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundtrip func(*http.Request) (*http.Response, error)

func (f roundtrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestStableUsesSemverAcrossPages(t *testing.T) {
	calls := 0
	c := &http.Client{Transport: roundtrip(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `[{"tag_name":"v0.9.0"},{"tag_name":"v0.10.0"},{"tag_name":"v8.0.0-rc.1"},{"tag_name":"edge"},{"tag_name":"v9.0.0","draft":true},{"tag_name":"v10.0.0","prerelease":true}]`
		if calls == 1 {
			body = "[" + strings.Repeat(`{"tag_name":"v0.1.0"},`, 99) + `{"tag_name":"v0.2.0"}]`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	got, e := latestStable(context.Background(), c)
	if e != nil || got != "v0.10.0" || calls != 2 {
		t.Fatal(got, calls, e)
	}
}
func TestManifestRejectsMixedAndUnsafeArtifacts(t *testing.T) {
	r := Release{Version: "source-" + strings.Repeat("a", 12), Tag: "edge", Revision: strings.Repeat("a", 40), Sequence: 1, Protocol: 1, Schema: StateSchema, Image: "ghcr.io/lesomnus/cxz@sha256:" + strings.Repeat("b", 64), Assets: map[string]Asset{}}
	for _, p := range []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
		name := "cxz-" + r.Revision + "-" + strings.ReplaceAll(p, "/", "-")
		if strings.HasPrefix(p, "windows") {
			name += ".exe"
		}
		r.Assets[p] = Asset{name, strings.Repeat("c", 64)}
	}
	if e := r.Validate(); e != nil {
		t.Fatal(e)
	}
	r.Schema = 1
	if r.Validate() == nil {
		t.Fatal("pre-retention schema accepted as compatible")
	}
	r.Schema = StateSchema
	r.Version = "source-" + strings.Repeat("d", 12)
	if r.Validate() == nil {
		t.Fatal("mixed revision accepted")
	}
	r.Version = "v0.1.2"
	r.Tag = "v0.1.2"
	if e := r.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"../cxz", "https://example.test/cxz", fmt.Sprintf("cxz-%s-linux-amd64", strings.Repeat("d", 40))} {
		r.Assets["linux/amd64"] = Asset{bad, strings.Repeat("c", 64)}
		if r.Validate() == nil {
			t.Fatal(bad)
		}
	}
}
