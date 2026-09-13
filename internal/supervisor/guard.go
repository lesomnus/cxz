package supervisor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func guard(pgid int, log, profileLock *os.File) (func(), error) {
	read, write, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	exe, e := os.Executable()
	if e != nil {
		read.Close()
		write.Close()
		return nil, e
	}
	cmd := exec.Command(exe, "_guard", strconv.Itoa(pgid))
	cmd.ExtraFiles = []*os.File{read, profileLock}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		read.Close()
		write.Close()
		return nil, e
	}
	read.Close()
	return func() { write.Write([]byte("disarm\n")); write.Close(); cmd.Wait() }, nil
}

// Guard is an internal child entrypoint. FD 3 is owned exclusively by its
// supervisor. EOF means the owner died; a normal shutdown explicitly disarms it.
func Guard(pid string) error {
	pgid, e := strconv.Atoi(pid)
	if e != nil || pgid <= 1 {
		return fmt.Errorf("invalid agent process group")
	}
	f := os.NewFile(3, "supervisor-liveness")
	if f == nil {
		return fmt.Errorf("missing liveness pipe")
	}
	defer f.Close()
	// Keep the session profile lease until the old process group is canceled, even if
	// the supervisor died before a replacement daemon noticed it.
	lease := os.NewFile(4, "session-profile-lease")
	if lease != nil {
		defer lease.Close()
	}
	line, e := bufio.NewReader(f).ReadString('\n')
	if e == nil && line == "disarm\n" {
		return nil
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
