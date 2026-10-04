package transport

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
)

// TunnelArguments forwards only a client loopback port to the SSH host's loopback.
// It never executes cxz (or any other command) on the remote host.
func (e Endpoint) TunnelArguments(localPort, remotePort int) ([]string, error) {
	if e.Scheme != "ssh" {
		return nil, fmt.Errorf("tunnel requires an SSH connection (ssh://)")
	}
	for _, p := range []int{localPort, remotePort} {
		if p < 1 || p > 65535 {
			return nil, fmt.Errorf("tunnel ports must be between 1 and 65535")
		}
	}
	args := []string{"-N", "-T", "-S", "none", "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"}
	if e.Port != "" {
		args = append(args, "-p", e.Port)
	}
	if e.User != "" {
		args = append(args, "-l", e.User)
	}
	args = append(args, "-L", "127.0.0.1:"+strconv.Itoa(localPort)+":127.0.0.1:"+strconv.Itoa(remotePort), "--", e.Address)
	return args, nil
}

func Tunnel(ctx context.Context, e Endpoint, localPort, remotePort int, out, errout io.Writer) error {
	args, err := e.TunnelArguments(localPort, remotePort)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "ssh", args...)
	return runTunnelProcess(ctx, cmd, out, errout)
}
func runTunnelProcess(ctx context.Context, cmd *exec.Cmd, out, errout io.Writer) error {
	cmd.Stdout, cmd.Stderr = out, errout
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("SSH tunnel failed (check SSH authentication, host key and local port): %w", err)
	}
	return nil
}
