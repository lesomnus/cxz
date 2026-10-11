//go:build linux

package wisp

import (
	"net"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSpecialFilesInListing(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(root, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	entries := map[string]string{}
	if err := list(root, func(v any) error {
		for _, e := range v.(Response).Entries {
			entries[e.Name] = e.Kind
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if entries["pipe"] != "fifo" || entries["socket"] != "socket" {
		t.Fatal(entries)
	}
}
