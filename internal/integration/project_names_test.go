package integration

import (
	"context"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/server"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectNames(t *testing.T) {
	t.Setenv("CXZ_OWNER", "")
	t.Setenv("CXZ_PROJECT_ID", "")
	root, err := os.MkdirTemp("", "cxz-names-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := func() func() {
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- server.Run(runCtx, root, "unused", "") }()
		return func() {
			stop()
			if err := <-done; err != nil {
				t.Error(err)
			}
		}
	}
	stop := start()
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	conn, err := server.Dial(root)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	for i := 0; i < 100; i++ {
		_, err = client.ResolveProject(ctx, "not-yet")
		if status.Code(err) == codes.NotFound {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	work1 := filepath.Join(root, "one", "my-web-app")
	work2 := filepath.Join(root, "two", "my-web-app")
	for _, p := range []string{work1, work2} {
		if err = os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	p, err := client.AddProject(ctx, work1, "My Web App", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "My Web App" || p.Alias != "mwa" {
		t.Fatalf("default name/alias: %v", p)
	}
	second, err := client.AddProject(ctx, work2, "My Web App", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(second.Alias, "mwa-") || second.Alias == p.Alias {
		t.Fatalf("collision not disambiguated: %v", second)
	}
	if _, err = client.ResolveProject(ctx, "My Web App"); status.Code(err) != codes.InvalidArgument {
		t.Fatal("ambiguous name accepted", err)
	}
	newName, newAlias := "Production Web", " WEB "
	renamed, err := client.SetProject(ctx, p.Alias, &newName, &newAlias)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != newName || renamed.Alias != "web" || renamed.Id != p.Id {
		t.Fatalf("rename changed identity: %v", renamed)
	}
	if _, err = client.ResolveProject(ctx, "mwa"); status.Code(err) != codes.NotFound {
		t.Fatal("old alias retained", err)
	}
	duplicate := "web"
	if _, err = client.SetProject(ctx, second.Id, nil, &duplicate); status.Code(err) != codes.AlreadyExists {
		t.Fatal("duplicate alias accepted", err)
	}
	invalid := "bad alias"
	if _, err = client.SetProject(ctx, second.Id, nil, &invalid); status.Code(err) != codes.InvalidArgument {
		t.Fatal("invalid alias accepted", err)
	}
	// Concurrent callers cannot claim the same explicit alias for two projects.
	claims := make(chan error, 2)
	for _, dir := range []string{"race-one", "race-two"} {
		path := filepath.Join(root, dir)
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		go func() { _, err := client.AddProject(ctx, path, "Concurrent", "race", ""); claims <- err }()
	}
	success, conflict := 0, 0
	for range 2 {
		err := <-claims
		if err == nil {
			success++
		} else if status.Code(err) == codes.AlreadyExists {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("alias uniqueness race: success=%d conflict=%d", success, conflict)
	}
	// API refs, not only CLI-side lookup, resolve by alias.
	svc := resource.NewProjectServiceClient(conn)
	alias := "web"
	ref := resource.ProjectRef_builder{Alias: &alias}.Build()
	got, err := svc.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref}.Build())
	if err != nil || got.GetRuntimeId() != p.Id {
		t.Fatal("generated alias ref failed", err)
	}
	// A raw patch still cannot set operational state, nor bypass optimistic locking.
	if _, err = svc.Patch(ctx, resource.ProjectPatchRequest_builder{Ref: ref, Name: &newName}.Build()); status.Code(err) != codes.InvalidArgument {
		t.Fatal("unversioned patch accepted", err)
	}
	if _, err = svc.Patch(ctx, resource.ProjectPatchRequest_builder{Ref: ref, Status: resource.ProjectStatus_builder{State: "running"}.Build()}.Build()); status.Code(err) != codes.PermissionDenied {
		t.Fatal("status patch allowed", err)
	}
	stop()
	stop = nil
	stop = start()
	var restoredID string
	for i := 0; i < 100; i++ {
		restored, e := client.ResolveProject(ctx, "web")
		if e == nil {
			restoredID = restored.Id
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if restoredID != p.Id {
		t.Fatal("name/alias did not survive restart")
	}
	again, err := client.AddProject(ctx, work1, "", "", "")
	if err != nil || again.Alias != "web" || again.Name != newName {
		t.Fatalf("registration reset metadata: %v %v", again, err)
	}
}
