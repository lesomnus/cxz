//go:build linux

package kube

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func init() {
	Helpers["_run-as"] = RunAs
	Helpers["_kube-prepare"] = Prepare
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// RunAs replaces this process with a command run as a user, in a directory,
// with extra environment: `_run-as [--user U] [--workdir W] [--env K=V]... -- CMD...`.
func RunAs(args []string) error {
	fs := flag.NewFlagSet("_run-as", flag.ContinueOnError)
	name := fs.String("user", "", "user to run as")
	workdir := fs.String("workdir", "", "working directory")
	var env multi
	fs.Var(&env, "env", "KEY=VALUE, repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	argv := fs.Args()
	if len(argv) == 0 {
		return fmt.Errorf("_run-as: no command")
	}
	environ := os.Environ()
	var cred *syscall.Credential
	if *name != "" {
		u, err := user.Lookup(*name)
		if err != nil {
			return fmt.Errorf("_run-as: %w", err)
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		if os.Getuid() != uid {
			groups := []uint32{}
			if ids, err := u.GroupIds(); err == nil {
				for _, id := range ids {
					if g, err := strconv.Atoi(id); err == nil && g != gid {
						groups = append(groups, uint32(g))
					}
				}
			}
			cred = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: groups}
		}
		environ = setEnv(environ, "HOME", u.HomeDir)
		environ = setEnv(environ, "USER", u.Username)
		environ = setEnv(environ, "LOGNAME", u.Username)
	}
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("_run-as: --env %q is not KEY=VALUE", kv)
		}
		environ = setEnv(environ, k, v)
	}
	path := argv[0]
	if !strings.Contains(path, "/") {
		resolved, err := lookPath(path, getEnv(environ, "PATH"))
		if err != nil {
			return fmt.Errorf("_run-as: %w", err)
		}
		path = resolved
	}
	if cred == nil {
		if *workdir != "" {
			if err := os.Chdir(*workdir); err != nil {
				return fmt.Errorf("_run-as: %w", err)
			}
		}
		return syscall.Exec(path, argv, environ)
	}
	// Changing a running Go process's user means changing every thread's, and
	// that does not reliably finish in a process started by a container's
	// exec. A child is given the user between fork and exec instead, and this
	// process stays only to pass on signals and the exit status.
	cmd := exec.Command(path, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Env = environ
	cmd.Dir = *workdir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
	signals := make(chan os.Signal, 8)
	signal.Notify(signals)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("_run-as: %w", err)
	}
	go func() {
		for sig := range signals {
			if sig == syscall.SIGCHLD || sig == syscall.SIGURG {
				continue
			}
			_ = cmd.Process.Signal(sig)
		}
	}()
	err := cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			os.Exit(128 + int(status.Signal()))
		}
		os.Exit(exit.ExitCode())
	}
	return err
}

func setEnv(env []string, key, value string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}

func getEnv(env []string, key string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}

func lookPath(file, path string) (string, error) {
	if path == "" {
		path = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	for _, dir := range filepath.SplitList(path) {
		p := filepath.Join(dir, file)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			return p, nil
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

// Prepare readies a project pod's volume before its runtime starts:
//
//	_kube-prepare [--copy-self DIR] [--runtime SRC DST] [--own USER PATH...]
//
// --copy-self installs this cxz as DIR/cxz, so the runtime is always the
// manager's own build. --runtime copies the runtime manifest from its Secret.
// --own gives the directories to the user the runtime runs as; it does not
// descend, so a large workspace costs nothing.
func Prepare(args []string) error {
	fs := flag.NewFlagSet("_kube-prepare", flag.ContinueOnError)
	copySelf := fs.String("copy-self", "", "directory to install this executable into")
	runtime := fs.String("runtime", "", "runtime manifest to copy; the destination is the next argument")
	owner := fs.String("own", "", "user to give the remaining paths to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if *copySelf != "" {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		if err = os.MkdirAll(*copySelf, 0755); err != nil {
			return err
		}
		if err = copyFile(self, filepath.Join(*copySelf, "cxz"), 0755); err != nil {
			return err
		}
	}
	if *runtime != "" {
		if len(rest) == 0 {
			return errors.New("_kube-prepare: --runtime needs a destination")
		}
		if err := os.MkdirAll(filepath.Dir(rest[0]), 0755); err != nil {
			return err
		}
		if err := copyFile(*runtime, rest[0], 0644); err != nil {
			return err
		}
		rest = rest[1:]
	}
	if *owner != "" {
		u, err := user.Lookup(*owner)
		if err != nil {
			return fmt.Errorf("_kube-prepare: %w", err)
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		for _, p := range rest {
			if err = os.MkdirAll(p, 0755); err != nil {
				return err
			}
			if err = os.Lchown(p, uid, gid); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile replaces dst atomically, so a running process keeps its old file.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	next := dst + ".next"
	out, err := os.OpenFile(next, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = os.Chmod(next, mode); err != nil {
		return err
	}
	return os.Rename(next, dst)
}
