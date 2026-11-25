package telemetry

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestForwardSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				line, _ := bufio.NewReader(c).ReadString('\n')
				c.Write([]byte("echo " + line))
			}()
		}
	}()
	dir, err := os.MkdirTemp("/tmp", "cxz-fwd-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "telemetry", "collector.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closer, err := ForwardSocket(ctx, socket, "tcp://"+ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	if b, err := os.ReadFile(filepath.Join(dir, "telemetry", "enabled")); err != nil || string(b) != "1\n" {
		t.Fatal("telemetry was not marked enabled", err)
	}
	c, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte("hello\n"))
	got, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || got != "echo hello\n" {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"cxz-telemetry:4317", "http://x:1", "tcp://nohost"} {
		if _, err = ForwardSocket(ctx, filepath.Join(dir, "x.sock"), bad); err == nil {
			t.Errorf("collector %q accepted", bad)
		}
	}
}
