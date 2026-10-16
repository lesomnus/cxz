package attachmenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lesomnus/cxz/internal/assets"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Client struct {
	HTTP   *http.Client
	Origin string
	// Header is what this link authenticates with, when the link does not
	// authenticate itself. A relay on the plaintext surface reads the same
	// `Authorization: Bearer` that link's calls carry; mutual TLS presents a
	// certificate in the handshake and leaves this empty.
	Header    http.Header
	supported atomic.Bool
	mu        sync.Mutex
	active    map[*http.Request]context.CancelFunc
	closed    bool
}

// Supported probes before consuming src. Failed uploads are never retried on
// another transport: a disconnect can happen after immutable publication.
func (c *Client) Supported(ctx context.Context) (bool, error) {
	if c.supported.Load() {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodHead, c.Origin+Prefix, nil)
	if err != nil {
		return false, err
	}
	response, err := c.do(r)
	if err != nil {
		if ctx.Err() == nil && (errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
			return false, nil
		}
		if ctx.Err() != nil {
			return false, status.FromContextError(ctx.Err()).Err()
		}
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusNotImplemented {
		return false, nil
	}
	if response.StatusCode != http.StatusNoContent || response.Header.Get(ProtocolHeader) != "1" {
		return false, fmt.Errorf("attachment endpoint discovery: HTTP %d", response.StatusCode)
	}
	c.supported.Store(true)
	return true, nil
}

func (c *Client) UploadAttachment(ctx context.Context, in assets.Upload, src io.Reader) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	address := c.Origin + Prefix + url.PathEscape(in.SessionID) + "?" + url.Values{"run": {in.RunID}, "name": {in.Name}, "size": {strconv.FormatInt(in.Size, 10)}}.Encode()
	body := &uploadBody{Reader: src, ctx: ctx}
	published := false
	defer func() {
		if !published {
			cancel()
			_ = body.Close()
		}
	}()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, address, body)
	if err != nil {
		return "", err
	}
	// Chunked HTTP lets storage inspect EOF and reject excess bytes before
	// publication, even if a caller's io.Reader disagrees with the declared size.
	r.ContentLength = -1
	r.Header.Set("Content-Type", "application/octet-stream")
	r.Header.Set("Expect", "100-continue")
	response, err := c.do(r)
	if err != nil {
		if ctx.Err() != nil {
			return "", status.FromContextError(ctx.Err()).Err()
		}
		if errors.Is(err, context.Canceled) {
			return "", status.FromContextError(context.Canceled).Err()
		}
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Code    int32
			Message string
		}
		if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure) != nil || failure.Code <= 0 || failure.Code > int32(codes.Unauthenticated) {
			return "", fmt.Errorf("attachment upload failed: HTTP %d", response.StatusCode)
		}
		return "", status.Error(codes.Code(failure.Code), failure.Message)
	}
	var reply struct{ Path string }
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&reply); err != nil {
		return "", err
	}
	if reply.Path == "" || reply.Path[0] != '/' {
		return "", fmt.Errorf("upload completed without an attachment path")
	}
	published = true
	return reply.Path, nil
}
func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	for _, cancel := range c.active {
		cancel()
	}
	c.mu.Unlock()
	if transport, ok := c.HTTP.Transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
}

func (c *Client) do(r *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(r.Context())
	request := r.Clone(ctx)
	// Set here rather than per call site, so discovery and the upload present
	// the same credential: a probe that is accepted and a POST that is not
	// would read as an endpoint that does not exist.
	for name, values := range c.Header {
		request.Header[name] = values
	}
	if body, ok := r.Body.(*uploadBody); ok {
		body.ctx = ctx
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		cancel()
		return nil, status.Error(codes.Canceled, "attachment connection closed")
	}
	if c.active == nil {
		c.active = make(map[*http.Request]context.CancelFunc)
	}
	c.active[request] = cancel
	c.mu.Unlock()
	stopBody := context.AfterFunc(ctx, func() {
		if request.Body != nil {
			_ = request.Body.Close()
		}
	})
	var once sync.Once
	finish := func() {
		once.Do(func() { stopBody(); cancel(); c.mu.Lock(); delete(c.active, request); c.mu.Unlock() })
	}
	client := *c.HTTP
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		cancel()
		if request.Body != nil {
			_ = request.Body.Close()
		}
		finish()
		return nil, err
	}
	response.Body = &responseBody{ReadCloser: response.Body, finish: finish}
	return response, nil
}

type responseBody struct {
	io.ReadCloser
	finish func()
}

func (r *responseBody) Close() error { err := r.ReadCloser.Close(); r.finish(); return err }

type uploadBody struct {
	io.Reader
	ctx context.Context
	eof atomic.Bool
}

func (b *uploadBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.eof.Store(true)
	}
	return n, err
}

func (b *uploadBody) Close() error {
	if b.ctx.Err() != nil && !b.eof.Load() {
		if closer, ok := b.Reader.(io.Closer); ok {
			return closer.Close()
		}
	}
	return nil
}
