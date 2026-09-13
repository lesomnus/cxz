package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/lesomnus/cxz/api"
)

func sessionProgress(ctx context.Context, out io.Writer, work func() error) error {
	started := time.Now()
	done := make(chan error, 1)
	go func() { done <- work() }()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	frame := 0
	print := func() {
		fmt.Fprintf(out, "\r\x1b[2K%c cxz: preparing agent session · %s", frames[frame%len(frames)], time.Since(started).Round(time.Second))
		frame++
	}
	print()
	for {
		select {
		case err := <-done:
			message := "session connected"
			if err != nil {
				message = "session preparation needs attention"
			}
			fmt.Fprintf(out, "\r\x1b[2Kcxz: %s · %s\n", message, time.Since(started).Round(time.Second))
			return err
		case <-tick.C:
			print()
		// Wait for the operation to finish even on cancellation so its caller
		// never observes a concurrently written session result.
		case <-ctx.Done():
			err := <-done
			fmt.Fprint(out, "\r\x1b[2K")
			return err
		}
	}
}

func provisionDescription(step string) string {
	switch step {
	case "inventory":
		return "Checking existing containers and ownership"
	case "recreate":
		return "Removing the old container (workspace and named volumes retained)"
	case "configuration":
		return "Reading and validating devcontainer configuration"
	case "resources":
		return "Preparing volumes, network and runtime tools"
	case "devcontainer-up":
		return "Starting devcontainer: image pull/build and lifecycle hooks"
	case "agent-tools":
		return "Installing/verifying the selected agent binary"
	case "github-cli":
		return "Installing/verifying GitHub CLI and injecting host credentials"
	case "runtime-boot":
		return "Starting the workspace runtime"
	case "runtime-ready":
		return "Checking workspace runtime readiness"
	case "session":
		return "Connecting the agent session"
	case "ready":
		return "Workspace ready"
	default:
		return "Waiting for manager provisioning status"
	}
}

// Status is sampled from persisted server checkpoints, not a guessed percentage.
// Only stderr is used; --no-attach structured stdout stays machine-readable.
func prepareWithProgress(ctx context.Context, out io.Writer, resolve func(context.Context) (*api.Project, error), open func() error, interval time.Duration) error {
	started := time.Now()
	fmt.Fprintln(out, "cxz: configuration checks passed; requesting workspace preparation")
	done := make(chan error, 1)
	go func() { done <- open() }()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	last := ""
	lastPrinted := started
	for {
		select {
		case err := <-done:
			state := "workspace ready"
			if err != nil {
				state = "workspace preparation failed"
			}
			fmt.Fprintf(out, "cxz: [%s] %s\n", time.Since(started).Round(time.Second), state)
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			poll, cancel := context.WithTimeout(ctx, 2*time.Second)
			p, err := resolve(poll)
			cancel()
			message := "Waiting for manager provisioning status"
			if err == nil && p != nil {
				message = provisionDescription(p.ProvisionStep)
			}
			if message != last || time.Since(lastPrinted) >= 10*time.Second {
				fmt.Fprintf(out, "cxz: [%s] %s\n", time.Since(started).Round(time.Second), message)
				last, lastPrinted = message, time.Now()
			}
		}
	}
}
