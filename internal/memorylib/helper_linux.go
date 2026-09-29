package memorylib

import (
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"syscall"
)

// Only called by the disposable helper process, never by the manager or supervisor.
func helperIdentity(root string, s core.Session) error {
	if os.Geteuid() != 0 {
		return nil
	}
	st, e := os.Stat(core.Dir(root, s.ID))
	if e != nil {
		return e
	}
	owner := st.Sys().(*syscall.Stat_t)
	if e = syscall.Setgroups(nil); e != nil {
		return e
	}
	if e = syscall.Setgid(int(owner.Gid)); e != nil {
		return e
	}
	return syscall.Setuid(int(owner.Uid))
}
