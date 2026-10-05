// Package pki issues the certificates a cxz installation uses to authenticate
// its own clients. It is deliberately small: one self-signed root per
// installation, kept where the manager keeps its state, signing exactly two
// kinds of leaf -- the relay that puts the manager on a network, and the
// clients allowed to reach it.
//
// There is no hostname in any of this. A LAN host is reached by whatever
// address it has today, which a DHCP lease is free to change, so verification
// checks that a peer's certificate chains to this installation's root and
// carries the right extended key usage, and nothing about the name it was
// reached by. The extended key usage is what keeps a client certificate from
// being presented as the relay.
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"google.golang.org/grpc/credentials"
)

const (
	// A root this small is pinned by the clients that use it, so its life is
	// bounded by the installation rather than by a policy about public trust.
	rootLife = 10 * 365 * 24 * time.Hour
	// A client certificate is a bearer credential: whoever holds it holds the
	// installation. It expires so that a copy left on a machine someone no
	// longer uses stops working without anyone having to notice.
	ClientLife = 30 * 24 * time.Hour
	serverLife = 365 * 24 * time.Hour
)

// Record is what the installation remembers about a certificate it signed.
// Enough to report it and to revoke it; nothing that would let it be reissued.
type Record struct {
	Serial  string    `json:"serial"`
	Label   string    `json:"label"`
	Issued  time.Time `json:"issued"`
	Expires time.Time `json:"expires"`
}

type Authority struct {
	dir  string
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	raw  []byte // DER, for writing chains without re-encoding
}

func caPath(dir string) (string, string) {
	return filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")
}

// EnsureCA loads the installation's root, creating it only when it is absent.
// It is never replaced: every client certificate and every pin a client stored
// chains to this key, so regenerating it would silently lock out every client
// that had been enrolled -- including across the manager replacement that an
// upgrade performs.
func EnsureCA(dir string) (*Authority, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	certPath, keyPath := caPath(dir)
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	switch {
	case certErr == nil && keyErr == nil:
		return load(dir, certPEM, keyPEM)
	case errors.Is(certErr, fs.ErrNotExist) && errors.Is(keyErr, fs.ErrNotExist):
	case certErr != nil:
		return nil, certErr
	default:
		return nil, keyErr
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "cxz installation root"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(rootLife),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	marshalled, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err = core.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: marshalled})); err != nil {
		return nil, err
	}
	if err = core.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &Authority{dir: dir, cert: cert, key: key, raw: der}, nil
}

func load(dir string, certPEM, keyPEM []byte) (*Authority, error) {
	certBlock, _ := pem.Decode(certPEM)
	keyBlock, _ := pem.Decode(keyPEM)
	if certBlock == nil || keyBlock == nil {
		return nil, fmt.Errorf("installation root is not PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return &Authority{dir: dir, cert: cert, key: key, raw: certBlock.Bytes}, nil
}

// CertificatePEM is what a client pins. It carries no private material, so it
// is the one file in here that may be copied anywhere.
func (a *Authority) CertificatePEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: a.raw})
}

func (a *Authority) Fingerprint() string { return fingerprint(a.cert) }

var labelPattern = func(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if r == '-' || r == '_' || r == '.' || r == '@' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// SignClient signs a client's own request. The key it proves possession of was
// generated on that client and never travels, so this takes a request and
// returns a certificate -- there is no path here that produces both halves of
// an identity in one place.
func (a *Authority) SignClient(csrPEM []byte, label string, life time.Duration) ([]byte, Record, error) {
	if !labelPattern(label) {
		return nil, Record{}, fmt.Errorf("client label must be 1-64 characters of letters, digits, dot, dash, underscore or @")
	}
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, Record{}, fmt.Errorf("expected a PEM certificate request")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, Record{}, err
	}
	if err = csr.CheckSignature(); err != nil {
		return nil, Record{}, fmt.Errorf("certificate request signature: %w", err)
	}
	// The request names nothing that is trusted: the subject, the extensions and
	// any requested usage are replaced by what this installation decided.
	serial, err := serialNumber()
	if err != nil {
		return nil, Record{}, err
	}
	if life <= 0 {
		life = ClientLife
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: label, OrganizationalUnit: []string{"cxz client"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(life),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.cert, csr.PublicKey, a.key)
	if err != nil {
		return nil, Record{}, err
	}
	record := Record{Serial: serial.String(), Label: label, Issued: now, Expires: template.NotAfter}
	if err = a.record(record); err != nil {
		return nil, Record{}, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), record, nil
}

func (a *Authority) record(r Record) error {
	records, err := a.Issued()
	if err != nil {
		return err
	}
	records = append(records, r)
	return core.WriteJSON(filepath.Join(a.dir, "issued.json"), records)
}

func (a *Authority) Issued() ([]Record, error) {
	b, err := os.ReadFile(filepath.Join(a.dir, "issued.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []Record
	if err = json.Unmarshal(b, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// ServerDir holds what the relay needs and nothing else. The relay is the one
// process on a network, so it never sees the root's private key: it is given a
// leaf, and the root's certificate only to verify clients with.
func ServerDir(dir string) string { return filepath.Join(dir, "server") }

// EnsureServer refreshes the relay's own material. The leaf is derived state --
// it is reissued whenever it is missing or close to expiry, and nothing is lost
// by throwing it away -- so the only durable secret remains the root.
func (a *Authority) EnsureServer() error {
	dir := ServerDir(a.dir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := core.WriteFile(filepath.Join(dir, "ca.crt"), a.CertificatePEM()); err != nil {
		return err
	}
	if b, err := os.ReadFile(filepath.Join(dir, "tls.crt")); err == nil {
		if block, _ := pem.Decode(b); block != nil {
			if leaf, err := x509.ParseCertificate(block.Bytes); err == nil && time.Until(leaf.NotAfter) > 30*24*time.Hour && leaf.Issuer.CommonName == a.cert.Subject.CommonName {
				return nil
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := serialNumber()
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "cxz relay", OrganizationalUnit: []string{"cxz server"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(serverLife),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.cert, &key.PublicKey, a.key)
	if err != nil {
		return err
	}
	marshalled, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err = core.WriteFile(filepath.Join(dir, "tls.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: marshalled})); err != nil {
		return err
	}
	return core.WriteFile(filepath.Join(dir, "tls.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func revokedPath(dir string) string { return filepath.Join(dir, "revoked") }

// Revoke writes the serial where the relay reads it. The list lives beside the
// relay's own material rather than with the root, because the relay has to read
// it on every handshake -- which is also why revoking takes effect without
// restarting anything.
func (a *Authority) Revoke(serial string) error {
	records, err := a.Issued()
	if err != nil {
		return err
	}
	known := false
	for _, r := range records {
		if r.Serial == serial {
			known = true
		}
	}
	if !known {
		return fmt.Errorf("no certificate with serial %s was issued here", serial)
	}
	if err = os.MkdirAll(ServerDir(a.dir), 0700); err != nil {
		return err
	}
	list, err := Revoked(ServerDir(a.dir))
	if err != nil {
		return err
	}
	if list[serial] {
		return nil
	}
	serials := make([]string, 0, len(list)+1)
	for s := range list {
		serials = append(serials, s)
	}
	serials = append(serials, serial)
	return core.WriteFile(revokedPath(ServerDir(a.dir)), []byte(strings.Join(serials, "\n")+"\n"))
}

func Revoked(dir string) (map[string]bool, error) {
	out := map[string]bool{}
	b, err := os.ReadFile(revokedPath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out[s] = true
		}
	}
	return out, nil
}

// NewRequest is run by a client. It returns a private key that stays on that
// machine and a request that is safe to move by any means, which is what makes
// the manual path safe to document.
func NewRequest() (keyPEM, csrPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "cxz client"}}, key)
	if err != nil {
		return nil, nil, err
	}
	marshalled, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: marshalled}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

func pool(caPEM []byte) (*x509.CertPool, error) {
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no certificate authority in the given PEM")
	}
	return p, nil
}

// verifyChain is the whole verification policy: chain to the pinned root, carry
// the usage this side of the connection requires, and not be revoked. No name
// is compared, deliberately -- see the package comment.
func verifyChain(roots *x509.CertPool, usage x509.ExtKeyUsage, revoked func() map[string]bool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("peer sent no certificate")
		}
		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return err
		}
		intermediates := x509.NewCertPool()
		for _, raw := range rawCerts[1:] {
			if c, err := x509.ParseCertificate(raw); err == nil {
				intermediates.AddCert(c)
			}
		}
		if _, err = leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
			return err
		}
		if revoked != nil && revoked()[leaf.SerialNumber.String()] {
			return fmt.Errorf("certificate %s is revoked", leaf.SerialNumber)
		}
		return nil
	}
}

// ClientCredentials presents this client's certificate and accepts only the
// relay of the installation that signed it.
func ClientCredentials(certPEM, keyPEM, caPEM []byte) (credentials.TransportCredentials, error) {
	identity, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("client identity: %w", err)
	}
	roots, err := pool(caPEM)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{identity},
		MinVersion:   tls.VersionTLS13,
		// Name verification is replaced, not skipped: VerifyPeerCertificate below
		// is what accepts the peer, and it is stricter about who signed it than a
		// hostname match would be.
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: verifyChain(roots, x509.ExtKeyUsageServerAuth, nil),
	}), nil
}

// ServerCredentials is the relay's side. The revocation list is read per
// handshake, so a revoked client stops being accepted without a restart.
func ServerCredentials(dir string) (credentials.TransportCredentials, error) {
	identity, err := tls.LoadX509KeyPair(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"))
	if err != nil {
		return nil, fmt.Errorf("relay identity: %w", err)
	}
	caPEM, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return nil, err
	}
	roots, err := pool(caPEM)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates:          []tls.Certificate{identity},
		MinVersion:            tls.VersionTLS13,
		ClientAuth:            tls.RequireAnyClientCert,
		VerifyPeerCertificate: verifyChain(roots, x509.ExtKeyUsageClientAuth, func() map[string]bool { list, _ := Revoked(dir); return list }),
	}), nil
}

// Expiry reports when a stored client certificate stops being accepted, so a
// client can say so before a connection fails.
func Expiry(certPEM []byte) (time.Time, string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, "", fmt.Errorf("client certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, "", err
	}
	return cert.NotAfter, cert.SerialNumber.String(), nil
}

// fingerprint is how a person compares the root they pinned with the root the
// host reports, over a channel that is not this one.
func fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	parts := make([]string, 0, len(sum))
	for _, v := range sum {
		parts = append(parts, fmt.Sprintf("%02x", v))
	}
	return "SHA256:" + strings.Join(parts, ":")
}

func serialNumber() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}
