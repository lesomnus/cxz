package kube

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/coder/websocket"
)

// The streaming protocol: every message's first byte names its channel.
// v5 adds the close message, which is how stdin reaches end of file.
const (
	protocolV5 = "v5.channel.k8s.io"
	protocolV4 = "v4.channel.k8s.io"

	channelStdin  = 0
	channelStdout = 1
	channelStderr = 2
	channelStatus = 3
	channelResize = 4
	channelClose  = 255
)

// TerminalSize is a TTY's size in character cells.
type TerminalSize struct {
	Width  uint16 `json:"Width"`
	Height uint16 `json:"Height"`
}

type ExecOptions struct {
	Pod, Container string
	Command        []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	TTY            bool
	// Resize, when set, delivers terminal size changes for a TTY.
	Resize <-chan TerminalSize
}

// ExitError is a command that ran and exited non-zero.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return "command exited with status " + strconv.Itoa(e.Code) }

// Exec runs a command in a running container and streams its input and output.
func (c *Client) Exec(ctx context.Context, o ExecOptions) error {
	if len(o.Command) == 0 {
		return fmt.Errorf("exec: no command")
	}
	q := url.Values{}
	for _, arg := range o.Command {
		q.Add("command", arg)
	}
	if o.Container != "" {
		q.Set("container", o.Container)
	}
	q.Set("stdin", strconv.FormatBool(o.Stdin != nil))
	q.Set("stdout", strconv.FormatBool(o.Stdout != nil))
	// A TTY merges stderr into stdout; asking for both is rejected.
	q.Set("stderr", strconv.FormatBool(o.Stderr != nil && !o.TTY))
	q.Set("tty", strconv.FormatBool(o.TTY))
	server, err := url.Parse(c.cfg.Server)
	if err != nil {
		return err
	}
	switch server.Scheme {
	case "https":
		server.Scheme = "wss"
	case "http":
		server.Scheme = "ws"
	}
	server.Path = strings.TrimRight(server.Path, "/") + c.Path("pods", o.Pod) + "/exec"
	server.RawQuery = q.Encode()
	token, err := c.cfg.token()
	if err != nil {
		return err
	}
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	conn, resp, err := websocket.Dial(ctx, server.String(), &websocket.DialOptions{HTTPClient: c.streamingClient(), HTTPHeader: header, Subprotocols: []string{protocolV5, protocolV4}})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("exec %s: %w (HTTP %d)", o.Pod, err, resp.StatusCode)
		}
		return fmt.Errorf("exec %s: %w", o.Pod, err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(-1)
	v5 := conn.Subprotocol() == protocolV5
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if o.Stdin != nil {
		go func() {
			buf := make([]byte, 32<<10)
			for {
				n, err := o.Stdin.Read(buf)
				if n > 0 {
					if conn.Write(ctx, websocket.MessageBinary, append([]byte{channelStdin}, buf[:n]...)) != nil {
						return
					}
				}
				if err != nil {
					if v5 {
						_ = conn.Write(ctx, websocket.MessageBinary, []byte{channelClose, channelStdin})
					}
					return
				}
			}
		}()
	}
	if o.TTY && o.Resize != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case size, ok := <-o.Resize:
					if !ok {
						return
					}
					b, _ := json.Marshal(size)
					if conn.Write(ctx, websocket.MessageBinary, append([]byte{channelResize}, b...)) != nil {
						return
					}
				}
			}
		}()
	}
	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			var closed websocket.CloseError
			if errors.As(err, &closed) && closed.Code == websocket.StatusNormalClosure {
				return nil
			}
			return fmt.Errorf("exec %s: %w", o.Pod, err)
		}
		if len(msg) == 0 {
			continue
		}
		data := msg[1:]
		switch msg[0] {
		case channelStdout:
			if o.Stdout != nil {
				if _, err = o.Stdout.Write(data); err != nil {
					return err
				}
			}
		case channelStderr:
			if o.Stderr != nil {
				if _, err = o.Stderr.Write(data); err != nil {
					return err
				}
			}
		case channelStatus:
			return execStatus(data)
		}
	}
}

// execStatus is the Status the server sends when the command ends.
func execStatus(b []byte) error {
	var s struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Reason  string `json:"reason"`
		Details struct {
			Causes []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"causes"`
		} `json:"details"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("exec: unreadable status: %w", err)
	}
	if s.Status == "Success" {
		return nil
	}
	if s.Reason == "NonZeroExitCode" {
		for _, cause := range s.Details.Causes {
			if cause.Reason == "ExitCode" {
				if code, err := strconv.Atoi(cause.Message); err == nil {
					return &ExitError{Code: code}
				}
			}
		}
	}
	return fmt.Errorf("exec: %s", s.Message)
}

// streamingClient is the client's TLS without its request timeout: an exec
// lasts as long as the command does.
func (c *Client) streamingClient() *http.Client {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(c.cfg.CA) > 0 {
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(c.cfg.CA)
		tlsConfig.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Transport: transport}
}
