package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/enroll"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"google.golang.org/grpc"
)

// Transport reports how a named connection will be reached, from what this
// client holds rather than from a connection attempt -- `connection ls` must
// not connect, and the TUI needs the answer before it does.
//
// An ssh connection that has enrolled is an mtls connection: ssh remains the
// channel that got the certificate, and would get the next one, but it is not
// what carries the conversation.
func connectionTransport(state, name, target string) (scheme, detail string) {
	e, err := transport.ParseEndpoint(target)
	if err != nil {
		return "", "unusable target"
	}
	switch e.Scheme {
	case "mtls", "ssh":
		stored, ok, err := enroll.Load(state, name)
		switch {
		case err != nil:
			return e.Scheme, "identity unreadable"
		case !ok:
			if e.Scheme == "mtls" {
				return "mtls", "not enrolled; cxz connection enroll " + name
			}
			return "ssh", "not enrolled"
		case !stored.Usable():
			return "mtls", "expired " + stored.Expires.UTC().Format("2006-01-02")
		default:
			return "mtls", "expires " + stored.Expires.UTC().Format("2006-01-02")
		}
	case "tcp":
		return "tcp", "plaintext; token"
	default:
		return e.Scheme, ""
	}
}

// openConnection reaches a named connection the best way it can, in one place
// for both the single-endpoint path and the multi-connection one.
//
// The order is deliberate: use the certificate if there is a usable one, obtain
// one over ssh if that is what the target offers, and fall back to ssh itself
// rather than failing. Nothing that works today stops working because this
// exists.
func openConnection(ctx context.Context, state, name string, entry settings.Connection, enrolling bool, out io.Writer) (string, *grpc.ClientConn, error) {
	target := entry.Resolve(state).Target
	e, err := transport.ParseEndpoint(target)
	if err != nil {
		return "", nil, err
	}
	switch e.Scheme {
	case "mtls":
		stored, ok, err := enroll.Load(state, name)
		if err != nil {
			return "", nil, err
		}
		if !ok {
			return "", nil, fmt.Errorf("connection %s has no client certificate; run cxz connection enroll %s", name, name)
		}
		if !stored.Usable() {
			return "", nil, fmt.Errorf("the certificate for %s expired on %s; run cxz connection enroll %s", name, stored.Expires.UTC().Format("2006-01-02"), name)
		}
		conn, err := transport.DialMutual(address(stored, e), stored.Identity)
		return "mtls", conn, err
	case "ssh":
		if conn := mutualOverSSH(ctx, state, name, e, enrolling, out); conn != nil {
			return "mtls", conn, nil
		}
		conn, err := transport.DialEndpoint(target, "", nil)
		return "ssh", conn, err
	case "tcp":
		token, err := transport.ReadToken(entry.Resolve(state).TokenFile)
		if err != nil {
			return "", nil, err
		}
		conn, err := transport.DialEndpoint(target, token, nil)
		return "tcp", conn, err
	}
	conn, err := dialConnection(state, target, "")
	return e.Scheme, conn, err
}

func address(stored enroll.Stored, e transport.Endpoint) string {
	if stored.Address != "" {
		return stored.Address
	}
	return e.Address
}

// mutualOverSSH returns a working mutual-TLS connection to an ssh target, or
// nil to say "use ssh". It is allowed to enroll and to start a stopped relay,
// because both are things the person could do by hand through the same channel
// they just authorized -- but it reports each one, and gives up quietly rather
// than turning a reachable host into an error.
func mutualOverSSH(ctx context.Context, state, name string, e transport.Endpoint, enrolling bool, out io.Writer) *grpc.ClientConn {
	stored, ok, err := enroll.Load(state, name)
	if err != nil {
		fmt.Fprintf(out, "%s: stored identity unreadable (%v); using ssh\n", name, err)
		return nil
	}
	run := sshRunner(e)
	if !ok || !stored.Usable() {
		if !enrolling {
			return nil
		}
		if ok {
			fmt.Fprintf(out, "%s: the client certificate expired on %s; enrolling again over ssh\n", name, stored.Expires.UTC().Format("2006-01-02"))
		}
		host, err := e.SSHDialHost(ctx)
		if err != nil {
			fmt.Fprintf(out, "%s: %v; using ssh\n", name, err)
			return nil
		}
		stored, err = enroll.Over(ctx, run, state, name, host, enroll.Options{Start: true, Out: out})
		if err != nil {
			fmt.Fprintf(out, "%s: %v; using ssh\n", name, err)
			return nil
		}
	}
	conn, err := transport.DialMutual(address(stored, e), stored.Identity)
	if err != nil {
		fmt.Fprintf(out, "%s: %v; using ssh\n", name, err)
		return nil
	}
	if reach(ctx, conn) == nil {
		return conn
	}
	conn.Close()
	if !enrolling {
		return nil
	}
	// Two things stop a stored address from answering, and the channel that is
	// still up can repair both: the relay is gone, which is the ordinary state
	// after a host reboot that removed the container, or the address is not one
	// this client can dial -- an identity enrolled before ssh aliases were
	// resolved holds a name only ssh knows how to look up.
	host, err := e.SSHDialHost(ctx)
	if err != nil {
		fmt.Fprintf(out, "%s: %v; using ssh\n", name, err)
		return nil
	}
	if stored.Address, err = relayAddress(ctx, run, state, name, host, stored.Address, out); err != nil {
		fmt.Fprintf(out, "%s: %v; using ssh\n", name, err)
		return nil
	}
	if conn, err = transport.DialMutual(stored.Address, stored.Identity); err != nil {
		fmt.Fprintf(out, "%s: %v; using ssh\n", name, err)
		return nil
	}
	if err = reach(ctx, conn); err != nil {
		conn.Close()
		fmt.Fprintf(out, "%s: the relay did not answer (%v); using ssh\n", name, err)
		return nil
	}
	return conn
}

// relayAddress re-derives where a connection's relay is, and records it when it
// is not where the stored identity says: a client whose address stopped working
// repairs itself over ssh instead of asking a person to enroll again for a
// certificate that is still perfectly valid.
func relayAddress(ctx context.Context, run enroll.Runner, state, name, host, current string, out io.Writer) (string, error) {
	address, err := enroll.Address(ctx, run, host, enroll.Options{Start: true, Out: out})
	if err != nil {
		return "", err
	}
	if address == current {
		return current, nil
	}
	if err = enroll.Rehost(state, name, address); err != nil {
		return "", err
	}
	fmt.Fprintf(out, "%s: %s did not answer; the relay is at %s and this connection now dials it there\n", name, current, address)
	return address, nil
}

// checkRelay answers the two questions a person has about a relay, in the one
// place that can answer them: is something listening, and does this identity
// reach the manager through it. Without an identity it stops at the first,
// which is all a readiness check may assume -- the handshake needs a
// certificate, and a relay has no reason to hold one of its own clients'.
func checkRelay(ctx context.Context, address, identity string, out io.Writer) error {
	if address == "" {
		return fmt.Errorf("--address host:port is required")
	}
	if identity == "" {
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var dialer net.Dialer
		c, err := dialer.DialContext(call, "tcp", address)
		if err != nil {
			return err
		}
		return c.Close()
	}
	id := transport.Identity{}
	for file, target := range map[string]*[]byte{"client.crt": &id.Certificate, "client.key": &id.Key, "ca.crt": &id.CA} {
		b, err := os.ReadFile(filepath.Join(identity, file))
		if err != nil {
			return err
		}
		*target = b
	}
	conn, err := transport.DialMutual(address, id)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = reach(ctx, conn); err != nil {
		return err
	}
	fmt.Fprintf(out, "mtls://%s answered\n", address)
	return nil
}

// reach is the one request that decides whether a transport works. The remote
// surface is the same either way, so any call proves it; List is the one the
// client makes first anyway.
func reach(ctx context.Context, conn *grpc.ClientConn) error {
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := resourceclient.New(conn).List(call, &api.Empty{})
	return err
}
