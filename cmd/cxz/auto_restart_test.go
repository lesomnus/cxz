package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("CXZ_TEST_FRONTEND") == "1" {
		if len(os.Args) == 2 && os.Args[1] == "_build-info" {
			_ = json.NewEncoder(os.Stdout).Encode(cxzupdate.Build{Revision: strings.Repeat("b", 40), Platform: runtime.GOOS + "/" + runtime.GOARCH, Protocol: cxzupdate.Protocol, Schema: cxzupdate.Schema})
			os.Exit(0)
		}
		path, _ := os.Executable()
		data, _ := os.ReadFile(path)
		old := bytes.HasSuffix(data, []byte("old-build-fixture"))
		if !old && os.Getenv("CXZ_TEST_FRONTEND_FAIL") == "1" {
			os.Exit(42)
		}
		root := os.Getenv("CXZ_TEST_FRONTEND_ROOT")
		_ = core.WriteJSON(cxzupdate.FrontendFile(root, "ready"), map[string]bool{"old": old})
		if old {
			_ = os.WriteFile(filepath.Join(root, "restored"), []byte("yes"), 0600)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestFrontendStartupRollbackAndSharedInstallation(t *testing.T) {
	for _, mode := range []string{"success", "rollback", "already-installed"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("CXZ_TEST_FRONTEND", "1")
			t.Setenv("CXZ_TEST_FRONTEND_ROOT", root)
			t.Setenv("CXZ_FRONTEND_SLOT", "fixture")
			if mode == "rollback" {
				t.Setenv("CXZ_TEST_FRONTEND_FAIL", "1")
			}
			path, e := os.Executable()
			if e != nil {
				t.Fatal(e)
			}
			data, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			candidate, target := filepath.Join(root, "candidate.exe"), filepath.Join(root, "cxz.exe")
			old := append(append([]byte{}, data...), []byte("old-build-fixture")...)
			if mode == "already-installed" {
				old = data
			}
			for p, b := range map[string][]byte{candidate: data, target: old} {
				if e = os.WriteFile(p, b, 0755); e != nil {
					t.Fatal(e)
				}
			}
			previous := filepath.Join(root, "cxz.previous.exe")
			if mode == "already-installed" {
				if e = os.WriteFile(previous, []byte("keep backup"), 0700); e != nil {
					t.Fatal(e)
				}
			}
			hash := sha256.Sum256(data)
			r := cxzupdate.Release{Revision: strings.Repeat("b", 40), Sequence: 1, Protocol: cxzupdate.Protocol, Schema: cxzupdate.Schema, Image: "ghcr.io/lesomnus/cxz@sha256:" + strings.Repeat("c", 64), Assets: map[string]cxzupdate.Asset{}}
			for _, p := range []string{"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
				name := "cxz-" + r.Revision + "-" + strings.ReplaceAll(p, "/", "-")
				if strings.HasPrefix(p, "windows/") {
					name += ".exe"
				}
				r.Assets[p] = cxzupdate.Asset{Name: name, SHA256: hex.EncodeToString(hash[:])}
			}
			if e = cxzupdate.Save(root, cxzupdate.State{Release: &r}); e != nil {
				t.Fatal(e)
			}
			restart := &cxzupdate.Restart{Client: cxzupdate.Client{Root: root, Executable: target}, Candidate: candidate, Release: r}
			next, e := runFrontendReplacement(restart, os.Environ(), make(chan os.Signal))
			if e != nil || next != nil {
				t.Fatal(next, e)
			}
			installed, e := os.ReadFile(target)
			if e != nil {
				t.Fatal(e)
			}
			if mode == "rollback" {
				if !bytes.Equal(installed, old) {
					t.Fatal("old binary not restored")
				}
				if _, e = os.Stat(filepath.Join(root, "restored")); e != nil {
					t.Fatal("old frontend did not restart")
				}
				s, _ := cxzupdate.Load(root)
				if s.State != "rolled_back" || s.FailedRevision != r.Revision {
					t.Fatal(s)
				}
			} else if !bytes.Equal(installed, data) {
				t.Fatal("new binary not installed")
			}
			if mode != "rollback" {
				s, _ := cxzupdate.Load(root)
				if s.State != "healthy" {
					t.Fatalf("short-lived initialized frontend not committed: %+v", s)
				}
			}
			if mode == "already-installed" {
				b, e := os.ReadFile(previous)
				if e != nil || string(b) != "keep backup" {
					t.Fatal("second TUI overwrote rollback backup")
				}
			}
		})
	}
}
