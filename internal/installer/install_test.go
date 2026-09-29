package installer

import (
	"context"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/versionpin"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailedInstallRetainsIdentity(t *testing.T) {
	root, bin := t.TempDir(), t.TempDir()
	// Image preflight succeeds; resource operations fail. Identity must survive.
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nif [ \"$1\" = image ]; then exit 0; fi\nexit 1\n"), 0700); err != nil {
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

// Stop at image preflight: selecting the image must not reuse a channel's
// historical digest when the caller explicitly asks to recreate the manager.
func TestRecreateChannelBuildsCurrentExecutable(t *testing.T) {
	for _, tc := range []struct {
		name, channel, image, want string
		recreate                   bool
	}{
		{"edge recreate", "edge", "", "info", true},
		{"stable recreate", "stable", "", "info", true},
		{"edge ordinary install", "edge", "", "image inspect fixture:old", false},
		{"release pin", "", "", "image inspect fixture:old", true},
		{"explicit image", "edge", "fixture:new", "image inspect fixture:new", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, bin := t.TempDir(), t.TempDir()
			log := filepath.Join(bin, "calls")
			t.Setenv("CXZ_TEST_DOCKER_LOG", log)
			t.Setenv("DOCKER_HOST", "")
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CXZ_TEST_DOCKER_LOG\"\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := versionpin.Save(root, versionpin.Pin{Ready: true, Version: "v0.1.0", Channel: tc.channel, Image: "fixture:old"}); err != nil {
				t.Fatal(err)
			}
			if err := Install(t.Context(), root, root, tc.image, tc.recreate, io.Discard); err == nil {
				t.Fatal("expected preflight failure")
			}
			b, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(b), tc.want) {
				t.Fatalf("Docker calls %q; want prefix %q", b, tc.want)
			}
			if _, err := transport.Load(root); !os.IsNotExist(err) {
				t.Fatal("preflight changed installation", err)
			}
		})
	}
}
