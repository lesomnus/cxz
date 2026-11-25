//go:build linux

package kube

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// ExecCommand is `docker exec` for a project pod, run by the manager in place
// of the docker CLI: `_kube-exec [-i] [-t] [--user U] [--workdir W] [--env K=V]... POD CMD...`.
// User, directory and environment are applied inside the pod by _run-as.
func ExecCommand(args []string) error {
	fs := flag.NewFlagSet("_kube-exec", flag.ContinueOnError)
	interactive := fs.Bool("i", false, "attach stdin")
	tty := fs.Bool("t", false, "allocate a terminal")
	name := fs.String("user", "", "user")
	workdir := fs.String("workdir", "", "working directory")
	var env multi
	fs.Var(&env, "env", "KEY=VALUE")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 2 {
		return fmt.Errorf("_kube-exec: need a pod and a command")
	}
	pod, command := rest[0], rest[1:]
	if *name != "" || *workdir != "" || len(env) > 0 {
		wrapped := []string{RuntimeBinary, "_run-as"}
		if *name != "" {
			wrapped = append(wrapped, "--user", *name)
		}
		if *workdir != "" {
			wrapped = append(wrapped, "--workdir", *workdir)
		}
		for _, kv := range env {
			wrapped = append(wrapped, "--env", kv)
		}
		command = append(append(wrapped, "--"), command...)
	}
	cfg, err := Load()
	if err != nil {
		return err
	}
	c, err := New(cfg)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	o := ExecOptions{Pod: pod, Command: command, Stdout: os.Stdout, Stderr: os.Stderr, TTY: *tty}
	if *interactive {
		o.Stdin = os.Stdin
	}
	if *tty && term.IsTerminal(int(os.Stdin.Fd())) {
		state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err == nil {
			defer term.Restore(int(os.Stdin.Fd()), state)
		}
		sizes := make(chan TerminalSize, 1)
		o.Resize = sizes
		send := func() {
			if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
				select {
				case sizes <- TerminalSize{Width: uint16(w), Height: uint16(h)}:
				default:
				}
			}
		}
		send()
		winch := make(chan os.Signal, 1)
		signal.Notify(winch, syscall.SIGWINCH)
		defer signal.Stop(winch)
		go func() {
			for range winch {
				send()
			}
		}()
	}
	err = c.Exec(ctx, o)
	var exit *ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.Code)
	}
	return err
}

func init() { Helpers["_kube-exec"] = ExecCommand }
