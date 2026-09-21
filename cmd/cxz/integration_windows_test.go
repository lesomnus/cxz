package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lesomnus/xli/xlitest"
	"golang.org/x/sys/windows"
)

func TestWindowsTerminalFragmentLaunchArgumentsAndReregistration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Fragments", "cxz", "cxz.json")
	executable := filepath.Join(root, "Programs 한글 with spaces & (dev)", "cxz.exe")
	state := filepath.Join(root, "state 한글 with spaces", "settings")
	for _, current := range []string{executable, filepath.Join(root, "new location", "cxz.exe")} {
		if err := writeTerminalFragment(path, current, state); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var fragment terminalFragment
		if err := json.Unmarshal(data, &fragment); err != nil || len(fragment.Profiles) != 1 {
			t.Fatal("duplicate or unreadable profiles", err)
		}
		profile := fragment.Profiles[0]
		args, err := windows.DecomposeCommandLine(profile.Commandline)
		if err != nil || !reflect.DeepEqual(args, []string{current, "--state", state}) {
			t.Fatal("Windows launch arguments changed", args, err)
		}
		if profile.GUID != terminalProfileGUID || profile.Name != "cxz" || profile.StartingDirectory != "%USERPROFILE%" {
			t.Fatal("profile identity changed on reregistration", profile)
		}
	}
}

func TestWindowsDesktopCommandsHelpAndInvalidTargetDoNotInstall(t *testing.T) {
	state := filepath.Join(t.TempDir(), "absent-state")
	for _, args := range [][]string{{"self-install", "--help"}, {"windows-install", "--help"}, {"integration", "--help"}, {"integration", "add", "--help"}, {"integration", "remove", "--help"}, {"integration", "ls", "--help"}} {
		if got := xlitest.Run(t, newRoot(state), args...); got.Err != nil {
			t.Fatal(args, got)
		}
	}
	got := xlitest.Run(t, newRoot(state), "integration", "add", "invalid-target")
	if got.Err == nil || !strings.Contains(got.Err.Error(), "unknown integration") {
		t.Fatal(got)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("desktop commands unexpectedly initialized daemon/client state", err)
	}
}
