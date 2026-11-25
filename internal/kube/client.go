package kube

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FieldManager is how cxz's server-side applies are attributed.
const FieldManager = "cxz"

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) (*Client, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(cfg.CA) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.CA) {
			return nil, fmt.Errorf("kube: no certificate in the cluster CA")
		}
		tlsConfig.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &Client{cfg: cfg, http: &http.Client{Transport: transport, Timeout: 60 * time.Second}}, nil
}

func (c *Client) Namespace() string { return c.cfg.Namespace }

// Error is a Status the API server answered with.
type Error struct {
	Code    int
	Reason  string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("kubernetes: %s (%d %s)", e.Message, e.Code, e.Reason)
}

func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == http.StatusNotFound
}

func IsConflict(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == http.StatusConflict
}

// Path is a namespaced resource in the client's namespace: Path("pods", name),
// or Path("pods") for the collection. Core resources only.
func (c *Client) Path(resource string, name ...string) string {
	p := "/api/v1/namespaces/" + url.PathEscape(c.cfg.Namespace) + "/" + resource
	if len(name) > 0 && name[0] != "" {
		p += "/" + url.PathEscape(name[0])
	}
	return p
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, contentType string, body []byte, out any) error {
	u := strings.TrimRight(c.cfg.Server, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	token, err := c.cfg.token()
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var status struct {
			Message string `json:"message"`
			Reason  string `json:"reason"`
		}
		_ = json.Unmarshal(b, &status)
		if status.Message == "" {
			status.Message = strings.TrimSpace(string(b))
		}
		return &Error{Code: resp.StatusCode, Reason: status.Reason, Message: status.Message}
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// Get reads one object into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, "", nil, out)
}

// List reads a collection, narrowed by a label selector when one is given.
func (c *Client) List(ctx context.Context, path, selector string, out any) error {
	q := url.Values{}
	if selector != "" {
		q.Set("labelSelector", selector)
	}
	return c.do(ctx, http.MethodGet, path, q, "", nil, out)
}

// Apply creates or updates obj by server-side apply. The object must carry
// apiVersion, kind and metadata.name. Fields cxz stops sending are removed.
func (c *Client) Apply(ctx context.Context, path string, obj, out any) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	q := url.Values{"fieldManager": {FieldManager}, "force": {"true"}}
	return c.do(ctx, http.MethodPatch, path, q, "application/apply-patch+yaml", b, out)
}

// Delete removes an object; an object already gone is not an error.
func (c *Client) Delete(ctx context.Context, path string) error {
	body, _ := json.Marshal(map[string]any{"kind": "DeleteOptions", "apiVersion": "v1", "propagationPolicy": "Background"})
	err := c.do(ctx, http.MethodDelete, path, nil, "application/json", body, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}
