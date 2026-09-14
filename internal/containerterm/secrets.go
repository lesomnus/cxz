package containerterm

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/wisp"
)

// Secret requests never use manager RPCs, attachments, argv or environment.
func (p *WispPool) secret(lifetime, ctx context.Context, project *api.Project, request wisp.Request) (wisp.Response, error) {
	// Ownership of secret bytes transfers to this call.
	started := false
	defer func() {
		if !started {
			clear(request.Secret)
		}
	}()
	c, err := p.client(lifetime, ctx, project)
	if err != nil {
		return wisp.Response{}, err
	}
	if !c.secrets {
		return wisp.Response{}, fmt.Errorf("wisp secret support unavailable; update project runtime")
	}
	select {
	case <-ctx.Done():
		return wisp.Response{}, ctx.Err()
	case <-c.done:
		return wisp.Response{}, fmt.Errorf("wisp disconnected")
	case c.gate <- struct{}{}:
	}
	result := make(chan wisp.Response, 1)
	started = true
	go func() {
		defer clear(request.Secret)
		defer func() { <-c.gate }()
		var r wisp.Response
		if c.enc.Encode(request) != nil || c.dec.Decode(&r) != nil {
			c.stop()
			r = wisp.Response{Error: "secret transport disconnected"}
		}
		result <- r
	}()
	select {
	case <-ctx.Done():
		c.stop()
		return wisp.Response{}, ctx.Err()
	case r := <-result:
		if r.Error != "" {
			return r, fmt.Errorf("%s", r.Error)
		}
		if !r.Done {
			return r, fmt.Errorf("invalid secret response")
		}
		return r, nil
	}
}

func (p *WispPool) PutSecret(lifetime, ctx context.Context, project *api.Project, session string, body []byte) (string, error) {
	if len(body) == 0 || len(body) > wisp.MaxSecretBytes {
		return "", fmt.Errorf("secret must contain 1–65536 bytes")
	}
	if _, err := p.secret(lifetime, ctx, project, wisp.Request{Operation: "secret/check"}); err != nil {
		return "", err
	}
	// Own the buffer until the transport goroutine finishes, including cancellation.
	copyBody := append([]byte(nil), body...)
	r, err := p.secret(lifetime, ctx, project, wisp.Request{Operation: "secret/put", Session: session, Secret: copyBody})
	// The encoder may still be running after cancellation; its buffer is cleared there.
	return r.SecretPath, err
}
func (p *WispPool) DeleteSecret(lifetime, ctx context.Context, project *api.Project, path string) error {
	_, err := p.secret(lifetime, ctx, project, wisp.Request{Operation: "secret/delete", Path: path})
	return err
}

func (p *WispPool) ClearSecrets(lifetime, ctx context.Context, project *api.Project, session string) error {
	_, err := p.secret(lifetime, ctx, project, wisp.Request{Operation: "secret/clear", Session: session})
	return err
}
