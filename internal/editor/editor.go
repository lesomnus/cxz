// Package editor runs an optional, pinned browser IDE in a project container.
// The Manager owns Docker; the gateway only receives an authenticated byte tunnel.
package editor

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
)

const Version = "1.109.5"
const Address = "127.0.0.1:7351"
const MaxArchive = 160 << 20

var projectPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func BasePath(project string) (string, error) {
	if !projectPattern.MatchString(project) {
		return "", fmt.Errorf("invalid editor project")
	}
	return "/editor/" + project, nil
}

func Digest(arch string) (string, error) {
	switch arch {
	case "amd64":
		return "b433bf4f0227321a7014d8460d10a8f958adc0f45aa79bd889e84e65e8f88363", nil
	case "arm64":
		return "36d9c14036489b63de84ebace837fcacf7e60e669a0dc715802c5443684ea4dc", nil
	default:
		return "", fmt.Errorf("browser editor supports Linux amd64/arm64, got %s", arch)
	}
}

func ArchiveURL(arch string) (string, error) {
	if _, err := Digest(arch); err != nil {
		return "", err
	}
	if arch == "amd64" {
		arch = "x64"
	}
	name := "openvscode-server-v" + Version
	return "https://github.com/gitpod-io/openvscode-server/releases/download/" + name + "/" + name + "-linux-" + arch + ".tar.gz", nil
}

type Result struct {
	Workspace string `json:"workspace"`
	Token     string `json:"token"`
}
type Runtime interface {
	Editor(context.Context, string) (Result, error)
	OpenEditorTunnel(context.Context, string) (io.ReadWriteCloser, error)
}

// Tunnel is private stdio-to-loopback transport, not a general destination proxy.
func Tunnel(ctx context.Context, in io.Reader, out io.Writer) error {
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", Address)
	if err != nil {
		return err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	go func() {
		io.Copy(c, in)
		if tcp, ok := c.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	_, err = io.Copy(out, c)
	return err
}
