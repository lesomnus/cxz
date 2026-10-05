package webconfig

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lesomnus/cxz/internal/core"
)

// DefaultPort is the gateway's port wherever it is not configured: the
// foreground server, the published container, and the tunnel's own default.
const DefaultPort = 7350

// DefaultPath is the configuration a command reads when none is named.
func DefaultPath(root string) string { return filepath.Join(root, "web.json") }

// TokenPath holds the browser's sign-in secret, separate from every provider
// credential. It is deliberately beside the configuration rather than inside
// it: the configuration is printed and copied, and this is not.
func TokenPath(root string) string { return filepath.Join(root, "web-token") }

// Default is the configuration a desktop needs and nothing more. Loopback
// plaintext asks for no certificate, no trust store and no hostname, and the
// browser still treats the origin as secure, so there is nothing left to
// configure before a browser can open it.
func Default(root string) Config {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(DefaultPort))
	return Config{Listen: address, Origin: "http://" + address, TokenFile: TokenPath(root)}
}

// Generate writes the default configuration and a token, and reports the files
// it created. It never overwrites either one: a token already pasted into a
// browser keeps working, and a configuration someone wrote by hand is theirs.
func Generate(root string) (Config, []string, error) {
	cfg := Default(root)
	var created []string
	if _, err := os.Stat(cfg.TokenFile); errors.Is(err, fs.ErrNotExist) {
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return cfg, created, err
		}
		if err = core.WriteFile(cfg.TokenFile, []byte(hex.EncodeToString(secret)+"\n")); err != nil {
			return cfg, created, fmt.Errorf("write %s: %w", cfg.TokenFile, err)
		}
		created = append(created, cfg.TokenFile)
	} else if err != nil {
		return cfg, created, err
	}
	path := DefaultPath(root)
	// Written with the token named relatively so moving a state directory keeps
	// the pair together, while the returned value stays resolved for this run.
	written := cfg
	written.TokenFile = filepath.Base(cfg.TokenFile)
	if err := core.WriteJSON(path, written); err != nil {
		return cfg, created, fmt.Errorf("write %s: %w", path, err)
	}
	return cfg, append(created, path), nil
}
