package projectspec_test

import (
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/projectspec"
)

func valid() projectspec.Spec {
	return projectspec.Spec{
		Owner: "owner", Project: "project", Image: "image",
		Env: map[string]string{"CXZ_STATE": projectspec.StateDir},
		Mounts: []projectspec.Mount{
			{Type: projectspec.Volume, Source: "state", Subpath: "projects/p/state", Target: "/cxz/state"},
			{Type: projectspec.Volume, Source: "tools", Target: "/cxz/tools", ReadOnly: true},
			{Type: projectspec.Bind, Source: "/host/assets", Target: "/cxz/assets", ReadOnly: true},
		},
		Workspace: "/workspace", Start: []string{"/cxz/tools/cxz", "_boot"},
	}
}

func TestValidate(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*projectspec.Spec){
		"no owner":          func(s *projectspec.Spec) { s.Owner = "" },
		"relative path":     func(s *projectspec.Spec) { s.Workspace = "workspace" },
		"no start":          func(s *projectspec.Spec) { s.Start = nil },
		"state redefined":   func(s *projectspec.Spec) { s.Env["CXZ_STATE"] = "/elsewhere" },
		"empty source":      func(s *projectspec.Spec) { s.Mounts[0].Source = "" },
		"bind with subpath": func(s *projectspec.Spec) { s.Mounts[2].Subpath = "x" },
		"no state mount":    func(s *projectspec.Spec) { s.Mounts = s.Mounts[1:] },
		"duplicate target":  func(s *projectspec.Spec) { s.Mounts = append(s.Mounts, s.Mounts[1]) },
		"unknown type":      func(s *projectspec.Spec) { s.Mounts[1].Type = "tmpfs" },
	} {
		s := valid()
		edit(&s)
		if s.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestMountString(t *testing.T) {
	m := valid().Mounts
	if got := m[0].String(); got != "type=volume,source=state,target=/cxz/state,volume-subpath=projects/p/state" {
		t.Fatal(got)
	}
	if got := m[2].String(); got != "type=bind,source=/host/assets,target=/cxz/assets,readonly" {
		t.Fatal(got)
	}
}

func TestUnexpressedIn(t *testing.T) {
	got := projectspec.UnexpressedIn(map[string]any{"image": "x", "features": map[string]any{}, "postStartCommand": "x", "remoteUser": "u"})
	var keys []string
	for _, u := range got {
		keys = append(keys, u.Key)
		if u.Why == "" || u.Instead == "" {
			t.Errorf("%s: says neither why nor what to do", u.Key)
		}
	}
	if strings.Join(keys, ",") != "features,postStartCommand" {
		t.Fatal(keys)
	}
	if len(projectspec.UnexpressedIn(map[string]any{"image": "x", "containerEnv": map[string]any{}, "remoteUser": "u", "workspaceFolder": "/w"})) != 0 {
		t.Fatal("an image-only configuration was refused")
	}
}
