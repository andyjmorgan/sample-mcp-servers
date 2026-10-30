// dateserver speaks MCP 2025-06-18 and answers with plain JSON instead of an
// SSE stream (JSONResponse). Tool: today.
package main

import (
	"context"
	"time"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type todayIn struct {
	Timezone string `json:"timezone,omitempty" jsonschema:"IANA timezone, e.g. Europe/Dublin. Defaults to UTC."`
}

func main() {
	mcpserve.Run(mcpserve.Spec{
		Name:            "dateserver",
		Version:         "1.0.0",
		ProtocolVersion: "2025-06-18",
		JSONResponse:    true,
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, &mcp.Tool{Name: "today", Description: "Today's date as 2006-01-02 (Monday), in an optional IANA timezone."},
				func(_ context.Context, _ *mcp.CallToolRequest, in todayIn) (*mcp.CallToolResult, any, error) {
					loc := time.UTC
					if in.Timezone != "" {
						l, err := time.LoadLocation(in.Timezone)
						if err != nil {
							return mcpserve.ErrResult("unknown timezone %q", in.Timezone), nil, nil
						}
						loc = l
					}
					return mcpserve.Text(time.Now().In(loc).Format("2006-01-02 (Monday)")), nil, nil
				})
		},
	})
}
