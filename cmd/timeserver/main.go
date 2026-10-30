// timeserver speaks only MCP 2025-03-26, so a gateway in front of it always
// has to translate. Tool: now.
package main

import (
	"context"
	"time"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type nowIn struct {
	Timezone string `json:"timezone,omitempty" jsonschema:"IANA timezone, e.g. Europe/Dublin. Defaults to UTC."`
}

func main() {
	mcpserve.Run(mcpserve.Spec{
		Name:            "timeserver",
		Version:         "1.0.0",
		ProtocolVersion: "2025-03-26",
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, &mcp.Tool{Name: "now", Description: "Current time as 15:04:05 MST, in an optional IANA timezone."},
				func(_ context.Context, _ *mcp.CallToolRequest, in nowIn) (*mcp.CallToolResult, any, error) {
					loc := time.UTC
					if in.Timezone != "" {
						l, err := time.LoadLocation(in.Timezone)
						if err != nil {
							return mcpserve.ErrResult("unknown timezone %q", in.Timezone), nil, nil
						}
						loc = l
					}
					return mcpserve.Text(time.Now().In(loc).Format("15:04:05 MST")), nil, nil
				})
		},
	})
}
