package transport

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/lesomnus/cxz/internal/pki"
	"google.golang.org/grpc"
)

// Identity is a client's half of a mutual-TLS connection: the certificate this
// installation signed for it, the key that never left it, and the root it pins
// the relay against. All three are PEM, and none of them is a bearer token --
// the key proves possession rather than being presented.
type Identity struct {
	Certificate, Key, CA []byte
}

func (i Identity) Complete() bool {
	return len(i.Certificate) > 0 && len(i.Key) > 0 && len(i.CA) > 0
}

// DialMutual connects to a relay by address. There is no name to verify and
// none is sent: the peer is accepted for being signed by the pinned root, which
// is what lets this work on a network that hands out addresses by lease.
func DialMutual(address string, id Identity) (*grpc.ClientConn, error) {
	if !id.Complete() {
		return nil, fmt.Errorf("incomplete client identity: certificate, key and installation root are all required")
	}
	creds, err := pki.ClientCredentials(id.Certificate, id.Key, id.CA)
	if err != nil {
		return nil, err
	}
	return grpc.NewClient("passthrough:///"+address, grpc.WithTransportCredentials(creds), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(24*1024*1024)), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}))
}

// ExposeMutual is the relay the installation runs for its own clients. Unlike
// the plaintext surface there is no token: a connection that completes the
// handshake has already proven it holds a certificate this installation signed
// and has not revoked, so every method is reached with no second credential to
// manage.
func ExposeMutual(ctx context.Context, root, listen, serverDir string, ready io.Writer) error {
	creds, err := pki.ServerCredentials(serverDir)
	if err != nil {
		return err
	}
	conn, err := Dial(root)
	if err != nil {
		return err
	}
	defer conn.Close()
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	defer ln.Close()
	g := proxyServer(conn, nil, grpc.Creds(creds))
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			g.Stop()
		case <-done:
		}
	}()
	if ready != nil {
		fmt.Fprintf(ready, "cxz relay: mtls://%s (client certificate required)\n", ln.Addr())
	}
	err = g.Serve(ln)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
