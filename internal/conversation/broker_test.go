package conversation

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/resource"
)

func TestRegistryAuthorizationAndProjectFilter(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "registry.sock")
	id := uuid.NewString()
	srv, err := StartRegistry(socket, func(_ context.Context, q RegistryRequest, token string) bool {
		return token == "project-token" && q.Project == "p" && q.Caller == "caller"
	}, func(_ context.Context, project string) ([]Session, error) {
		return []Session{{ID: id, Alias: "seal", RuntimeID: "internal", ProjectID: "p"}, {ID: uuid.NewString(), Alias: "other", ProjectID: "other"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	got, err := RemoteRegistry(t.Context(), socket, "project-token", RegistryRequest{"p", "caller"})
	if err != nil || len(got) != 1 || got[0].ID != id || got[0].RuntimeID != "internal" {
		t.Fatal(got, err)
	}
	for _, q := range []RegistryRequest{{"other", "caller"}, {"p", "other"}} {
		if _, err = RemoteRegistry(t.Context(), socket, "project-token", q); err == nil {
			t.Fatal("unscoped request accepted")
		}
	}
	if _, err = RemoteRegistry(t.Context(), socket, "bad", RegistryRequest{"p", "caller"}); err == nil {
		t.Fatal("bad token accepted")
	}
}
func TestCanonicalRegistryPagination(t *testing.T) {
	yes := true
	alias := "seal"
	id := uuid.New()
	calls := 0
	list := func(_ context.Context, r *resource.SessionListRequest) (*resource.SessionListResponse, error) {
		calls++
		if r.GetFilters()[0].GetProject().GetRuntimeId() != "p" {
			t.Fatal("missing project filter")
		}
		next := "next"
		if r.GetAfter() != "" {
			next = ""
			alias = "otter"
		}
		return resource.SessionListResponse_builder{Next: next, Items: []*resource.Session{resource.Session_builder{Id: id[:], Alias: alias, Listed: yes, RuntimeId: "runtime", Project: resource.Project_builder{RuntimeId: "p"}.Build()}.Build(), resource.Session_builder{Id: id[:], Alias: "other", Listed: yes, Project: resource.Project_builder{RuntimeId: "other"}.Build()}.Build()}}.Build(), nil
	}
	got, err := RegistryFromResources(t.Context(), "p", list)
	if err != nil || calls != 2 || len(got) != 2 || got[0].ID != id.String() {
		t.Fatal(got, err)
	}
}
