//go:build !windows && !js

package taskmcp

import (
	"context"
	"fmt"
	"io"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/mcpruntime"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func init() {
	mcpruntime.Register("cxz_tasks", "cxz Tasks", Instructions, func(ctx context.Context, root string, session core.Session, c io.ReadWriteCloser) error {
		conn, err := transport.Dial(root)
		if err != nil {
			return err
		}
		defer conn.Close()
		s, err := New(conn, Binding{Project: session.ProjectID, Session: session.ID, Run: mcpruntime.RunID(ctx)})
		if err != nil {
			return err
		}
		s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(callCtx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				if method == "tools/call" {
					launch, err := mcpruntime.ReadLaunch(root, session.ID)
					if err != nil || launch.RunID != mcpruntime.RunID(ctx) {
						result := &mcp.CallToolResult{}
						result.SetError(fmt.Errorf("Task MCP launch changed; restart the agent"))
						return result, nil
					}
				}
				return next(callCtx, method, req)
			}
		})
		return s.Run(ctx, &mcp.IOTransport{Reader: c, Writer: c})
	})
}
