//go:build !windows

package memorylib

import (
	"context"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpruntime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
)

func init() {
	mcpruntime.Register("cxz_memory", "cxz Memory", Instructions, func(ctx context.Context, root string, session core.Session, c io.ReadWriteCloser) error {
		store := New(root, session)
		if _, e := store.Do(ctx, Request{Action: "init"}); e != nil {
			return e
		}
		return MCPServer(store).Run(ctx, &mcp.IOTransport{Reader: c, Writer: c})
	})
}
