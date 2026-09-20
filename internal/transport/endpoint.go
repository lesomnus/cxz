package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Endpoint struct {
	Scheme, Address, User, Port, State, Binary string
}

type remoteKey struct{}

func WithRemote(ctx context.Context) context.Context {
	return context.WithValue(ctx, remoteKey{}, true)
}
func IsRemote(ctx context.Context) bool { v, _ := ctx.Value(remoteKey{}).(bool); return v }
func LocalOnly(ctx context.Context, operation string) error {
	if IsRemote(ctx) {
		return fmt.Errorf("%s requires host-local Docker access; run cxz on the daemon host", operation)
	}
	return nil
}

var sshHost = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.:-]*$`)
var sshUser = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`)

func ParseEndpoint(raw string) (Endpoint, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Endpoint{}, fmt.Errorf("invalid remote endpoint")
	}
	e := Endpoint{Scheme: u.Scheme, Address: u.Host, Binary: "cxz"}
	if u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return e, fmt.Errorf("endpoint must use ssh://[user@]host[:port] or tcp://host:port")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return e, fmt.Errorf("invalid endpoint query")
	}
	switch u.Scheme {
	case "ssh":
		e.Address = u.Hostname()
		e.Port = u.Port()
		if !sshHost.MatchString(e.Address) && net.ParseIP(e.Address) == nil {
			return e, fmt.Errorf("invalid SSH host")
		}
		if u.User != nil {
			if _, set := u.User.Password(); set {
				return e, fmt.Errorf("SSH passwords are not accepted in endpoints; use ssh-agent or SSH config")
			}
			e.User = u.User.Username()
			if !sshUser.MatchString(e.User) {
				return e, fmt.Errorf("invalid SSH user")
			}
		}
		if e.Port != "" {
			n, err := strconv.Atoi(e.Port)
			if err != nil || n < 1 || n > 65535 {
				return e, fmt.Errorf("invalid SSH port")
			}
		}
		for key, values := range q {
			if len(values) != 1 || values[0] == "" || strings.ContainsAny(values[0], "\x00\r\n") {
				return e, fmt.Errorf("invalid SSH option")
			}
			switch key {
			case "state":
				e.State = values[0]
			case "binary":
				e.Binary = values[0]
			default:
				return e, fmt.Errorf("unknown SSH endpoint option %q", key)
			}
		}
	case "tcp":
		if u.User != nil || len(q) != 0 {
			return e, fmt.Errorf("TCP endpoint cannot contain credentials or query options; use --token-file")
		}
		host, port, err := net.SplitHostPort(u.Host)
		if err != nil || host == "" {
			return e, fmt.Errorf("TCP endpoint requires host:port")
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return e, fmt.Errorf("invalid TCP port")
		}
	default:
		return e, fmt.Errorf("unsupported endpoint scheme; use ssh:// or tcp://")
	}
	return e, nil
}

func shellArgument(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }
func (e Endpoint) SSHArguments() []string {
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"}
	if e.Port != "" {
		args = append(args, "-p", e.Port)
	}
	if e.User != "" {
		args = append(args, "-l", e.User)
	}
	command := shellArgument(e.Binary)
	if e.State != "" {
		command += " --state " + shellArgument(e.State)
	}
	command += " _connect"
	return append(args, "--", e.Address, command)
}

func ReadToken(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("TCP connections require --token-file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(b))
	if len(token) < 32 || len(b) > 4096 || strings.ContainsAny(token, " \t\r\n\x00") {
		return "", fmt.Errorf("token file must contain a single token of 32–4096 bytes")
	}
	return token, nil
}

func DialEndpoint(raw, token string) (*grpc.ClientConn, error) {
	e, err := ParseEndpoint(raw)
	if err != nil {
		return nil, err
	}
	if e.Scheme == "tcp" {
		if token == "" {
			return nil, fmt.Errorf("TCP connections require an authentication token")
		}
		return Remote("passthrough:///"+e.Address, token)
	}
	return grpc.NewClient("passthrough:///cxz-ssh", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(24*1024*1024)), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cmd := exec.Command("ssh", e.SSHArguments()...)
		cmd.Stderr = os.Stderr
		return commandConnection(cmd)
	}))
}

func commandConnection(cmd *exec.Cmd) (net.Conn, error) {
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return nil, err
	}
	return &pipeConn{Reader: out, Writer: in, cmd: cmd}, nil
}

func LocalConnection(ctx context.Context, root string) (net.Conn, error) {
	install, err := Load(root)
	if err == nil {
		return commandConnection(exec.Command("docker", "exec", "-i", install.Container, "/usr/local/bin/cxz", "--state", "/var/lib/cxz", "_bridge"))
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", Socket(root))
}

// ConnectBridge runs on the SSH host and understands host-side installations.
// Unlike _bridge, it also reaches managers running inside an owned container.
func ConnectBridge(ctx context.Context, root string, in io.Reader, out io.Writer) error {
	c, err := LocalConnection(ctx, root)
	if err != nil {
		return err
	}
	defer c.Close()
	done := make(chan error, 2)
	go func() { _, err := io.Copy(c, in); done <- err }()
	go func() { _, err := io.Copy(out, c); done <- err }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
