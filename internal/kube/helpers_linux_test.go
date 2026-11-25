//go:build linux

package kube

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

func TestPrepare(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "secret-runtime.json")
	if err := os.WriteFile(src, []byte(`{"Token":"t"}`), 0600); err != nil {
		t.Fatal(err)
	}
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	tools, state := filepath.Join(dir, "tools"), filepath.Join(dir, "state")
	if err = Prepare([]string{"--copy-self", tools}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(tools, "cxz")); err != nil || st.Mode().Perm() != 0755 {
		t.Fatal("this executable was not installed", st, err)
	}
	if err = Prepare([]string{"--runtime", src, "--own", me.Username, filepath.Join(state, "runtime.json"), state, filepath.Join(dir, "workspace")}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(state, "runtime.json")); err != nil || string(b) != `{"Token":"t"}` {
		t.Fatal("the runtime manifest was not copied", string(b), err)
	}
	if _, err = os.Stat(filepath.Join(dir, "workspace")); err != nil {
		t.Fatal("a directory to own was not created", err)
	}
	if err = Prepare([]string{"--runtime", src}); err == nil {
		t.Fatal("--runtime without a destination was accepted")
	}
	if err = Prepare([]string{"--own", "no-such-user-cxz", dir}); err == nil {
		t.Fatal("an unknown user was accepted")
	}
}

func TestRunAsRejects(t *testing.T) {
	if err := RunAs([]string{"--user", "no-such-user-cxz", "--", "true"}); err == nil {
		t.Fatal("an unknown user was accepted")
	}
	if err := RunAs([]string{"--env", "NOEQUALS", "--", "true"}); err == nil {
		t.Fatal("a malformed --env was accepted")
	}
	if err := RunAs([]string{}); err == nil {
		t.Fatal("no command was accepted")
	}
}

func TestPrepareCopiesBesideItself(t *testing.T) {
	dir := t.TempDir()
	extra := filepath.Join(dir, "buildctl")
	if err := os.WriteFile(extra, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	tools := filepath.Join(dir, "tools")
	if err := Prepare([]string{"--copy-self", tools, "--copy", extra, "--copy", filepath.Join(dir, "absent")}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(tools, "buildctl")); err != nil || st.Mode().Perm() != 0755 {
		t.Fatal("buildctl was not installed beside cxz", st, err)
	}
	if _, err := os.Stat(filepath.Join(tools, "absent")); !os.IsNotExist(err) {
		t.Fatal("a file that does not exist was installed")
	}
}
