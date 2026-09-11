package installer

import (
	"context"
	"github.com/lesomnus/cxz/internal/transport"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedInstallRetainsIdentity(t *testing.T) {
	root, bin := t.TempDir(), t.TempDir()
	// Every Docker operation fails: the locator must still survive and be reused.
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("DOCKER_HOST", "")
	if Install(context.Background(), root, root, "fixture:v1", false, io.Discard) == nil {
		t.Fatal("expected failure")
	}
	a, err := transport.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if Install(context.Background(), root, root, "fixture:v1", false, io.Discard) == nil {
		t.Fatal("expected failure")
	}
	b, err := transport.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if a.Owner == "" || a != b {
		t.Fatalf("retry lost identity: %+v %+v", a, b)
	}
}

func TestEndpointRejectedBeforeMutation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DOCKER_HOST", "ssh://not-supported")
	// Use shared path so path validation doesn't mask endpoint validation.
	if Install(context.Background(), root, "/workspaces", "fixture:v1", true, io.Discard) == nil {
		t.Fatal("accepted endpoint")
	}
	if _, err := transport.Load(root); !os.IsNotExist(err) {
		t.Fatal("invalid endpoint persisted installation")
	}
}
