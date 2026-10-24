package networkservices

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
)

const LockFile = "network-services.lock"

func manager(ctx context.Context, name, owner string) (dockerx.Container, error) {
	v, err := dockerx.Owned(ctx, name, owner, "")
	if err == nil && (v.Config.Labels["cxz.role"] != "daemon" || v.Config.Image == "") {
		err = fmt.Errorf("invalid manager identity")
	}
	return v, err
}

// LockHost shares the manager-volume lock with automatic reconciliation. The
// short-lived holder has no Docker socket or host directory mounts, and exits
// when its stdin closes (including if the calling frontend dies). A manager
// replacement cannot release it prematurely. Host install.lock is never opened
// by a container process.
func LockHost(ctx context.Context, name, owner string) (context.Context, func(), error) {
	v, err := manager(ctx, name, owner)
	if err != nil {
		return nil, nil, err
	}
	volume := ""
	for _, mount := range v.Mounts {
		if mount.Destination == "/var/lib/cxz" && mount.Type == "volume" {
			volume = mount.Name
		}
	}
	if volume == "" {
		return nil, nil, fmt.Errorf("manager has no named state volume")
	}
	holder := "cxz-network-lock-" + core.ID()
	command := `exec flock -w 15 -E 73 /state/` + LockFile + ` sh -c 'if [ -f /state/network-services/web.json ] || [ -f /state/network-services/remote.json ]; then printf "locked pending\n"; else printf "locked\n"; fi; cat >/dev/null'`
	work, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(work, "docker", "run", "--rm", "-i", "--name", holder,
		"--label", "cxz.owner="+owner, "--label", "cxz.role=network-lock",
		"--network", "none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--mount", "type=volume,source="+volume+",target=/state", "--entrypoint", "/bin/sh", v.Config.Image, "-c", command)
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		input.Close()
		return nil, nil, err
	}
	var logs bytes.Buffer
	cmd.Stderr = &logs
	if err = cmd.Start(); err != nil {
		cancel()
		input.Close()
		return nil, nil, err
	}
	done := make(chan error, 1)
	needsRecovery := false
	go func() {
		line, err := bufio.NewReader(output).ReadString('\n')
		needsRecovery = strings.TrimSpace(line) == "locked pending"
		if err == nil && strings.TrimSpace(line) != "locked" && !needsRecovery {
			err = fmt.Errorf("invalid lock holder acknowledgement")
		}
		done <- err
	}()
	select {
	case err = <-done:
	case <-ctx.Done():
		cancel()
		err = ctx.Err()
		<-done
	case <-time.After(20 * time.Second):
		cancel()
		err = fmt.Errorf("network service lock timed out")
		<-done
	}
	exited := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, output); _ = cmd.Wait(); cancel(); close(exited) }()
	cleanup := func() {
		input.Close()
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			cancel()
			<-exited
		}
		cancel()
		recovery, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = dockerx.Run(recovery, "rm", "-f", holder)
	}
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("network service lock: %w: %.1000s", err, logs.String())
	}
	// Recovery executes in the manager, which already has Docker access. The
	// holder itself keeps no socket; the host has not changed any service yet.
	if needsRecovery {
		if _, err = dockerx.Run(work, "exec", name, "/usr/local/bin/cxz", "_recover-network-services", "/var/lib/cxz", name, owner); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("recover interrupted network service update (start manager if stopped): %w", err)
		}
	}
	return work, cleanup, nil
}
