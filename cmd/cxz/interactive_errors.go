//go:build !windows

package main

import (
	"context"
	"errors"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/configtrust"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
	"google.golang.org/grpc/status"
)

func exitOnError(c *xli.Command) bool {
	for c != nil {
		if value, _ := flg.Get[bool](c, "exit-on-error"); value {
			return true
		}
		if !c.HasParent() {
			break
		}
		c = c.Parent()
	}
	return false
}
func interactiveErrors(c *xli.Command) bool {
	if exitOnError(c) || !terminal(c) {
		return false
	}
	if detach, _ := flg.Get[bool](c, "no-attach"); detach {
		return false
	}
	if format, _ := flg.Get[string](c, "format"); format == "json" {
		return false
	}
	output, ok := c.ErrWriter.(*os.File)
	return ok && term.IsTerminal(output.Fd())
}
func handleCommandError(ctx context.Context, c *xli.Command, err error) error {
	var shown *displayedError
	if err == nil || errors.As(err, &shown) || ctx.Err() != nil || !interactiveErrors(c) {
		return err
	}
	_, displayErr := tui.InlineError(ctx, c.ReadCloser, c.ErrWriter, status.Convert(err).Message(), false)
	if displayErr != nil {
		return err
	}
	return &displayedError{err}
}
func withConfigTrust(ctx context.Context, r *api.ProjectRequest, interactive bool,
	open func(context.Context, *api.ProjectRequest) (*api.Session, error),
	confirm func(context.Context, error) (bool, error),
) (*api.Session, error) {
	s, err := open(ctx, r)
	if err == nil || r.TrustConfig || !interactive || !configtrust.IsRequired(err) {
		return s, err
	}
	yes, promptErr := confirm(ctx, err)
	if promptErr != nil {
		return nil, promptErr
	}
	if !yes {
		return nil, &displayedError{err}
	}
	r.TrustConfig = true
	return open(ctx, r)
}
func trustPrompt(c *xli.Command) func(context.Context, error) (bool, error) {
	return func(ctx context.Context, err error) (bool, error) {
		return tui.InlineError(ctx, c.ReadCloser, c.ErrWriter, status.Convert(err).Message(), true)
	}
}

func bindErrorFlags(c *xli.Command) {
	if c.Flags.Get("exit-on-error") == nil {
		c.Flags = append(c.Flags, &flg.Switch{Name: "exit-on-error", Alias: 'x', Brief: "Return errors immediately without interactive error recovery", Default: ptr(false)})
	}
	for _, child := range c.Commands {
		bindErrorFlags(child)
	}
}
