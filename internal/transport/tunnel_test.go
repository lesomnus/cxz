package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTunnelArguments(t *testing.T) {
	e, err := ParseEndpoint("ssh://alice@work:2222?state=%2Fprivate&binary=custom-cxz")
	if err != nil {
		t.Fatal(err)
	}
	args, err := e.TunnelArguments(7443, 7350)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-N", "-T", "-S", "none", "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-p", "2222", "-l", "alice", "-L", "127.0.0.1:7443:127.0.0.1:7350", "--", "work"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("%q", args)
	}
	for _, ports := range [][2]int{{0, 7350}, {7350, -1}, {65536, 7350}} {
		if _, err = e.TunnelArguments(ports[0], ports[1]); err == nil {
			t.Fatal("invalid ports accepted", ports)
		}
	}
	e.Scheme = "tcp"
	if _, err = e.TunnelArguments(7350, 7350); err == nil {
		t.Fatal("non-SSH accepted")
	}
}

func TestTunnelProcessHelper(t *testing.T) {
	mode := os.Getenv("CXZ_TEST_TUNNEL_HELPER")
	if mode == "" {
		return
	}
	if mode == "fail" {
		fmt.Fprintln(os.Stderr, "fixture bind failed")
		os.Exit(17)
	}
	fmt.Fprintln(os.Stdout, "ready")
	time.Sleep(time.Minute)
	os.Exit(0)
}

type tunnelReadyWriter struct{ ready chan struct{} }

func (w tunnelReadyWriter) Write(p []byte) (int, error) {
	select {
	case w.ready <- struct{}{}:
	default:
	}
	return len(p), nil
}
func TestTunnelProcessCancellationAndFailure(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"fail", "wait"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestTunnelProcessHelper$")
			cmd.Env = append(os.Environ(), "CXZ_TEST_TUNNEL_HELPER="+mode)
			var stderr bytes.Buffer
			if mode == "fail" {
				err = runTunnelProcess(ctx, cmd, io.Discard, &stderr)
				if err == nil || !strings.Contains(err.Error(), "SSH tunnel failed") || !strings.Contains(stderr.String(), "fixture bind failed") {
					t.Fatalf("%v %s", err, &stderr)
				}
				return
			}
			ready := make(chan struct{}, 1)
			done := make(chan error, 1)
			go func() { done <- runTunnelProcess(ctx, cmd, tunnelReadyWriter{ready}, &stderr) }()
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal("helper did not start")
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("tunnel process survived cancellation")
			}
		})
	}
}
