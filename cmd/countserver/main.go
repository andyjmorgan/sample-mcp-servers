// countserver speaks MCP 2026-07-28 and streams: it counts 1..n with a pause
// between steps and, when the caller passes a progress token, emits
// notifications/progress events on the response's SSE stream. Tool: count.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type countIn struct {
	N       int  `json:"n,omitempty" jsonschema:"Count from 1 to n, 1-50. Defaults to 5."`
	DelayMs *int `json:"delay_ms,omitempty" jsonschema:"Pause between steps in milliseconds, 0-5000. Defaults to 500."`
}

func main() {
	mcpserve.Run(mcpserve.Spec{
		Name:            "countserver",
		Version:         "1.0.0",
		ProtocolVersion: "2026-07-28",
		Instructions:    "Counts from 1 to n slowly, reporting progress along the way. It exists to exercise progress notifications and long-running calls; keep n small (under 20) unless the user is testing progress.",
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, mcpserve.Tool("count", "Count to N", "Count from 1 to n with a pause between steps, reporting progress when a progress token is supplied.", true),
				func(ctx context.Context, req *mcp.CallToolRequest, in countIn) (*mcp.CallToolResult, any, error) {
					n, delay := in.N, 500
					if n == 0 {
						n = 5
					}
					if in.DelayMs != nil {
						delay = *in.DelayMs
					}
					if n < 1 || n > 50 {
						return mcpserve.ErrResult("n must be between 1 and 50, got %d", in.N), nil, nil
					}
					if delay < 0 || delay > 5000 {
						return mcpserve.ErrResult("delay_ms must be between 0 and 5000, got %d", delay), nil, nil
					}
					token := req.Params.GetProgressToken()
					steps := make([]string, 0, n)
					for i := 1; i <= n; i++ {
						if i > 1 {
							select {
							case <-ctx.Done():
								return nil, nil, ctx.Err()
							case <-time.After(time.Duration(delay) * time.Millisecond):
							}
						}
						steps = append(steps, fmt.Sprint(i))
						if token != nil {
							err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
								ProgressToken: token,
								Progress:      float64(i),
								Total:         float64(n),
								Message:       fmt.Sprintf("counted %d of %d", i, n),
							})
							if err != nil {
								return nil, nil, err
							}
						}
					}
					return mcpserve.Text(strings.Join(steps, " ")), nil, nil
				})
		},
	})
}
