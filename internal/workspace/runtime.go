package workspace

import (
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transport"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func Login(root, kind string) error {
	return fmt.Errorf("project-wide login is disabled; use cxz account login ACCOUNT from the installed client")
}
func LoadRuntime(root string) (Runtime, error) {
	var r Runtime
	b, e := os.ReadFile(filepath.Join(filepath.Dir(root), "runtime.json"))
	if e == nil {
		e = json.Unmarshal(b, &r)
	}
	return r, e
}
func Boot(root string) error {
	if e := core.Prepare(root); e != nil {
		return e
	}
	if c, e := net.DialTimeout("unix", transport.Socket(root), time.Second); e == nil {
		c.Close()
		return nil
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	log, e := os.OpenFile(filepath.Join(root, "runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	cmd := exec.Command(exe, "--state", root, "_project")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		return e
	}
	return cmd.Process.Release()
}
