package webui

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
)

func TestContainerPathValidation(t *testing.T) {
	for _, source := range []string{"/", "/etc/hosts", "~", "~/notes.txt", "/workspace/../tmp/file"} {
		if err := containerPath(source, false); err != nil {
			t.Fatal(source, err)
		}
	}
	for _, source := range []string{"README.md", "src/a b.txt"} {
		if err := containerPath(source, true); err != nil {
			t.Fatal(source, err)
		}
		if status.Code(containerPath(source, false)) != codes.InvalidArgument {
			t.Fatal("relative directory accepted", source)
		}
	}
	for _, source := range []string{"", "~other/file", "/foo\x00bar", "/foo\nbar", strings.Repeat("a", 4097)} {
		if status.Code(containerPath(source, true)) != codes.InvalidArgument {
			t.Fatal("invalid path accepted", source)
		}
	}
}
