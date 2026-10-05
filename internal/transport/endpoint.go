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
	"sync"

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
func WithLocal(ctx context.Context) context.Context {
	return context.WithValue(ctx, remoteKey{}, false)
}
func IsRemote(ctx context.Context) bool { v, _ := ctx.Value(remoteKey{}).(bool); return v }

type schemeKey struct{}

// WithScheme records how the client reached the daemon. Remoteness answers one
// question -- can this client reach the engine itself -- and the scheme answers
// another that it cannot: whether the link is one a secret may travel over. A
// ssh connection is encrypted by ssh; the exposed TCP surface is authenticated
// plaintext meant for a tunnel, and cannot be told apart from a local client on
// the daemon side, so the client is where that decision has to be made.
func WithScheme(ctx context.Context, scheme string) context.Context {
	return context.WithValue(ctx, schemeKey{}, scheme)
}
func Scheme(ctx context.Context) string { v, _ := ctx.Value(schemeKey{}).(string); return v }

// Confidential reports whether the connection keeps what crosses it from the
// network: a local client never puts it there, ssh encrypts it, and mtls is TLS
// to a peer whose certificate this installation signed.
func Confidential(ctx context.Context) bool {
	return !IsRemote(ctx) || Scheme(ctx) == "ssh" || Scheme(ctx) == "mtls"
}
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
	if u.Scheme == "unix" || u.Scheme == "local" {
		if u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(u.Path, "\x00\r\n") {
			return e, fmt.Errorf("invalid local endpoint")
		}
		if u.Scheme == "unix" && !strings.HasPrefix(u.Path, "/") {
			return e, fmt.Errorf("unix endpoint requires an absolute socket path")
		}
		e.Address = u.Path
		return e, nil
	}
	if u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return e, fmt.Errorf("endpoint must use ssh://[user@]host[:port], mtls://host:port or tcp://host:port")
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
	case "tcp", "mtls":
		if u.User != nil || len(q) != 0 {
			return e, fmt.Errorf("%s endpoint cannot contain credentials or query options", u.Scheme)
		}
		host, port, err := net.SplitHostPort(u.Host)
		if err != nil || host == "" {
			return e, fmt.Errorf("%s endpoint requires host:port", u.Scheme)
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return e, fmt.Errorf("invalid %s port", u.Scheme)
		}
	default:
		return e, fmt.Errorf("unsupported endpoint scheme; use ssh://, mtls://, tcp://, local:// or unix:///path")
	}
	return e, nil
}

func shellArgument(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }

// SSHCommand runs one cxz command on the SSH host, with the same options and
// the same remote binary and state the connection itself uses. Enrollment goes
// through here: the channel that already authenticates both ends is what makes
// a certificate handed back over it trustworthy.
func (e Endpoint) SSHCommand(args ...string) []string {
	options := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"}
	if e.Port != "" {
		options = append(options, "-p", e.Port)
	}
	if e.User != "" {
		options = append(options, "-l", e.User)
	}
	command := shellArgument(e.Binary)
	if e.State != "" {
		command += " --state " + shellArgument(e.State)
	}
	for _, a := range args {
		command += " " + shellArgument(a)
	}
	return append(options, "--", e.Address, command)
}

func (e Endpoint) SSHArguments() []string { return e.SSHCommand("_connect") }

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

// DialEndpoint opens a connection to one endpoint. The credential it needs
// depends on the scheme: a token for the plaintext TCP surface, a client
// certificate for mtls, and nothing for ssh, which authenticates itself.
func DialEndpoint(raw, token string, id *Identity) (*grpc.ClientConn, error) {
	e, err := ParseEndpoint(raw)
	if err != nil {
		return nil, err
	}
	if e.Scheme == "mtls" {
		if id == nil {
			return nil, fmt.Errorf("mtls connections require an enrolled client certificate; run cxz connection enroll")
		}
		return DialMutual(e.Address, *id)
	}
	if e.Scheme == "tcp" {
		if token == "" {
			return nil, fmt.Errorf("TCP connections require an authentication token")
		}
		return Remote("passthrough:///"+e.Address, token)
	}
	if e.Scheme == "local" {
		return nil, fmt.Errorf("local:// requires a client state directory")
	}
	if e.Scheme == "unix" {
		return grpc.NewClient("passthrough:///cxz-unix", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(24*1024*1024)), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", e.Address)
		}))
	}
	return grpc.NewClient("passthrough:///cxz-ssh", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(24*1024*1024)), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cmd := exec.Command("ssh", e.SSHArguments()...)
		diagnostics := &connectionDiagnostics{}
		cmd.Stderr = diagnostics
		conn, err := commandConnection(cmd)
		if err != nil {
			return nil, err
		}
		return &diagnosticConnection{Conn: conn, diagnostics: diagnostics}, nil
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

// Capture a bounded tail of SSH diagnostics for the connection error instead of
// printing asynchronous reconnect failures into the frontend's terminal.
type connectionDiagnostics struct {
	mu   sync.Mutex
	tail []byte
}

func (d *connectionDiagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(p)
	if len(p) > 2048 {
		p = p[len(p)-2048:]
	}
	d.tail = append(d.tail, p...)
	if len(d.tail) > 2048 {
		d.tail = d.tail[len(d.tail)-2048:]
	}
	return n, nil
}
func (d *connectionDiagnostics) String() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return strings.TrimSpace(string(d.tail))
}

type diagnosticConnection struct {
	net.Conn
	diagnostics *connectionDiagnostics
}

func (c *diagnosticConnection) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if err != nil {
		if detail := c.diagnostics.String(); detail != "" {
			err = fmt.Errorf("SSH: %s: %w", detail, err)
		}
	}
	return n, err
}
