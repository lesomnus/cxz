package workspace

import (
	"os"
	"syscall"
	"testing"
)

func ownerUID(t *testing.T, info os.FileInfo) uint32 {
	t.Helper()
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("no owner information on this platform")
	}
	return st.Uid
}
