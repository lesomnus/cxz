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
	r, e := LoadRuntime(root)
	if e != nil {
		return e
	}
	bin := r.Claude
	args := []string{"auth", "login"}
	key := "CLAUDE_CONFIG_DIR"
	if kind == "codex" {
		bin = r.Codex
		args = []string{"login", "--device-auth"}
		key = "CODEX_HOME"
	} else if kind != "claude" {
		return fmt.Errorf("unknown agent")
	}
	if bin == "" {
		return fmt.Errorf("agent not provisioned; run cxz up --agent %s", kind)
	}
	cfg := filepath.Join(root, "agents", kind)
	if e = os.MkdirAll(cfg, 0700); e != nil {
		return e
	}
	return syscall.Exec(bin, append([]string{bin}, args...), append(os.Environ(), key+"="+cfg))
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
