package projectref

import (
	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestShortAlias(t *testing.T) {
	for name, want := range map[string]string{"cld": "cld", "webapi": "webapi", "my-web-app": "mwa", "Observability/Platform": "op", "reallylongsingleword": "really", "한국어": "p", "123project": "p-123pro"} {
		if got := ShortAlias(name); got != want {
			t.Errorf("%q: %q != %q", name, got, want)
		}
	}
}
func TestResolve(t *testing.T) {
	ps := []*api.Project{{Id: "one", Workspace: "/one", Name: "Same Name", Alias: "sn"}, {Id: "two", Workspace: "/two", Name: "Same Name", Alias: "other"}, {Id: "foreign", Name: "sn", State: "foreign"}}
	for _, handle := range []string{"one", "/one", "sn", " SN "} {
		p, err := Resolve(ps, handle)
		if err != nil || p.Id != "one" {
			t.Fatalf("%q: %v %v", handle, p, err)
		}
	}
	if _, err := Resolve(ps, "Same Name"); status.Code(err) != codes.InvalidArgument {
		t.Fatal("ambiguous name accepted", err)
	}
	if _, err := Resolve(ps, "foreign"); status.Code(err) != codes.NotFound {
		t.Fatal("foreign target accepted", err)
	}
}
