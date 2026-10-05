package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/enroll"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

func enrollCommands() xli.Commands {
	return xli.Commands{
		{Name: "enroll", Brief: "Obtain this client's certificate for a connection, over its SSH channel or by hand",
			Synop: "An SSH connection already authenticates both ends, so a certificate obtained over it needs no\nother ceremony. Without SSH, --csr prints a request to sign on the host and --certificate stores\nthe result; the private key is generated here and never travels either way.",
			Args:  arg.Args{&arg.String{Name: "NAME"}},
			Flags: flg.Flags{
				&flg.String{Name: "label", Brief: "Name the installation records for this client (default: this host's name)", Default: remoteDefault("")},
				&flg.String{Name: "address", Brief: "Address to dial the relay on (default: the one the host reports)", Default: remoteDefault("")},
				&flg.Switch{Name: "csr", Brief: "Print a certificate request for the manual path and store the key", Default: remoteSwitch(false)},
				&flg.String{Name: "certificate", Brief: "Signed certificate file, completing the manual path", Default: remoteDefault("")},
				&flg.String{Name: "ca", Brief: "Installation root certificate file, completing the manual path", Default: remoteDefault("")},
				&flg.Switch{Name: "no-start", Brief: "Fail instead of starting a relay that is not running", Default: remoteSwitch(false)},
			},
			Handler: xli.OnRun(connectionEnroll)},
		{Name: "check", Brief: "Connect to a connection's relay with the stored certificate and report what answers", Args: arg.Args{&arg.String{Name: "NAME"}}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			_, state, err := connectionSettings(c)
			if err != nil {
				return err
			}
			name := arg.MustGet[string](c, "NAME")
			stored, ok, err := enroll.Load(state, name)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("connection %s has no client certificate; run cxz connection enroll %s", name, name)
			}
			if !stored.Usable() {
				return fmt.Errorf("the certificate for %s expired on %s; run cxz connection enroll %s", name, stored.Expires.UTC().Format("2006-01-02"), name)
			}
			conn, err := transport.DialMutual(stored.Address, stored.Identity)
			if err != nil {
				return err
			}
			defer conn.Close()
			if err = reach(ctx, conn); err != nil {
				return fmt.Errorf("mtls://%s did not answer: %w", stored.Address, err)
			}
			fmt.Fprintf(c.Writer, "mtls://%s answered; certificate %s expires %s\n", stored.Address, stored.Serial, stored.Expires.UTC().Format("2006-01-02"))
			return nil
		})},
		{Name: "forget", Brief: "Delete this client's stored certificate for a connection", Args: arg.Args{&arg.String{Name: "NAME"}}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			_, state, err := connectionSettings(c)
			if err != nil {
				return err
			}
			name := arg.MustGet[string](c, "NAME")
			if err = enroll.Forget(state, name); err != nil {
				return err
			}
			fmt.Fprintf(c.ErrWriter, "%s: identity deleted; the certificate stays valid until it expires or is revoked on the host\n", name)
			return nil
		})},
	}
}

func connectionEnroll(ctx context.Context, c *xli.Command, _ xli.Next) error {
	cfg, state, err := connectionSettings(c)
	if err != nil {
		return err
	}
	name := arg.MustGet[string](c, "NAME")
	entry, known := cfg.Connections.Entry(name)
	if !known {
		return fmt.Errorf("unknown connection %q; use cxz connection ls", name)
	}
	endpoint, err := transport.ParseEndpoint(entry.Resolve(state).Target)
	if err != nil {
		return err
	}
	address := flg.MustGet[string](c, "address")
	if address == "" && endpoint.Scheme == "mtls" {
		address = endpoint.Address
	}

	if flg.MustGet[bool](c, "csr") {
		csr, err := enroll.Request(state, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.ErrWriter, "%s: key stored in %s; sign this request on the host with cxz expose sign --label NAME\n", name, enroll.Dir(state, name))
		_, err = c.Writer.Write(csr)
		return err
	}
	if path := flg.MustGet[string](c, "certificate"); path != "" {
		cert, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		caPath := flg.MustGet[string](c, "ca")
		if caPath == "" {
			return fmt.Errorf("--certificate also needs --ca: the root is what this client verifies the relay against")
		}
		ca, err := os.ReadFile(caPath)
		if err != nil {
			return err
		}
		if address == "" {
			return fmt.Errorf("--address is required when the connection's target is not mtls://host:port")
		}
		stored, err := enroll.Complete(state, name, cert, ca, address)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.ErrWriter, "%s: enrolled for mtls://%s; certificate %s expires %s\n", name, stored.Address, stored.Serial, stored.Expires.UTC().Format("2006-01-02"))
		return nil
	}
	if flg.MustGet[string](c, "ca") != "" {
		return fmt.Errorf("--ca completes the manual path and needs --certificate")
	}
	if endpoint.Scheme != "ssh" {
		return fmt.Errorf("connection %s has no ssh channel to enroll over; use --csr and --certificate, or add an ssh:// connection to the same host", name)
	}
	_, err = enroll.Over(ctx, sshRunner(endpoint), state, name, endpoint.Address, enroll.Options{
		Label:   flg.MustGet[string](c, "label"),
		Address: address,
		Start:   !flg.MustGet[bool](c, "no-start"),
		Out:     c.ErrWriter,
	})
	return err
}

// sshRunner runs one cxz command on the SSH host. It is the trust anchor of the
// automatic path: ssh has already verified the host key and this user, so a
// certificate that comes back through it was issued by the installation we
// meant to ask.
func sshRunner(e transport.Endpoint) enroll.Runner {
	return func(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
		call, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(call, "ssh", e.SSHCommand(args...)...)
		cmd.Stdin = bytes.NewReader(stdin)
		var diagnostics bytes.Buffer
		cmd.Stderr = &diagnostics
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("ssh %s: %w: %.2000s", strings.Join(args, " "), err, strings.TrimSpace(diagnostics.String()))
		}
		return out, nil
	}
}
