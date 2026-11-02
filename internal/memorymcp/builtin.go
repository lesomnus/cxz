//go:build !windows && !js

package memorymcp

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpruntime"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
)

func init() {
	mcpruntime.Register("cxz_memory", "cxz Memory", Instructions, func(ctx context.Context, root string, session core.Session, c io.ReadWriteCloser) error {
		conn, err := transport.Dial(root)
		if err != nil {
			return err
		}
		defer conn.Close()
		run := mcpruntime.RunID(ctx)
		s, err := New(conn, Binding{session.ProjectID, session.ID, run})
		if err != nil {
			return err
		}
		conversationTools(s, root, session)
		s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(call context.Context, method string, r mcp.Request) (mcp.Result, error) {
				if method == "tools/call" {
					launch, e := mcpruntime.ReadLaunch(root, session.ID)
					if e != nil || launch.RunID != run {
						result := &mcp.CallToolResult{}
						result.SetError(fmt.Errorf("Memory MCP launch changed; restart the agent"))
						return result, nil
					}
				}
				return next(call, method, r)
			}
		})
		return s.Run(ctx, &mcp.IOTransport{Reader: c, Writer: c})
	})
}
