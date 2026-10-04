// Package webconfig holds the shared foreground and installed web configuration.
package webconfig

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/webui"
)

type Config struct {
	Listen      string `json:"listen"`
	Origin      string `json:"origin"`
	Certificate string `json:"tls_cert"`
	Key         string `json:"tls_key"`
	TokenFile   string `json:"access_token_file"`
}

func Load(path string, optional bool) (Config, error) {
	c := Config{Listen: "127.0.0.1:7350"}
	f, err := os.Open(path)
	if optional && os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 65536))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, fmt.Errorf("web config: %w", err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("web config: expected one JSON object")
	}
	c.Resolve(filepath.Dir(path))
	return c, nil
}

func (c *Config) Resolve(base string) {
	for _, p := range []*string{&c.Certificate, &c.Key, &c.TokenFile} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}
}

func (c Config) Runtime() (webui.Config, error) {
	token, err := transport.ReadToken(c.TokenFile)
	if err != nil {
		return webui.Config{}, err
	}
	v := webui.Config{Listen: c.Listen, Origin: c.Origin, Certificate: c.Certificate, Key: c.Key, Token: token}
	if err = v.Validate(); err != nil {
		return v, err
	}
	if _, err = tls.LoadX509KeyPair(c.Certificate, c.Key); err != nil {
		return v, fmt.Errorf("web TLS: %w", err)
	}
	for _, p := range []string{c.Certificate, c.Key, c.TokenFile} {
		if strings.ContainsAny(p, ",\n\r") {
			return v, fmt.Errorf("unsupported web file path")
		}
	}
	return v, nil
}
