package telemetry

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ForwardSocket makes the collector socket exist where a project expects it,
// carrying each connection to a collector reached over TCP: a project pod on
// Kubernetes shares no volume with the collector. What crosses it is the same
// signed OTLP the socket carries; the collector still checks every batch.
//
// It also marks telemetry enabled, which on Docker the manager does in the
// shared tools directory.
func ForwardSocket(ctx context.Context, socket, collector string) (io.Closer, error) {
	address, ok := strings.CutPrefix(collector, "tcp://")
	if !ok {
		return nil, fmt.Errorf("telemetry collector must be tcp://host:port, not %q", collector)
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		return nil, fmt.Errorf("telemetry collector: %w", err)
	}
	dir := filepath.Dir(socket)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(dir, "enabled"), []byte("1\n"), 0644); err != nil {
		ln.Close()
		return nil, err
	}
	go func() {
		for {
			local, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer local.Close()
				dial, cancel := context.WithTimeout(ctx, 10*time.Second)
				remote, err := (&net.Dialer{}).DialContext(dial, "tcp", address)
				cancel()
				if err != nil {
					return
				}
				defer remote.Close()
				done := make(chan struct{}, 2)
				go func() { _, _ = io.Copy(remote, local); done <- struct{}{} }()
				go func() { _, _ = io.Copy(local, remote); done <- struct{}{} }()
				<-done
			}()
		}
	}()
	go func() { <-ctx.Done(); ln.Close() }()
	return ln, nil
}
