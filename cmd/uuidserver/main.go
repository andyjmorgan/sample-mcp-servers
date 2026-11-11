// uuidserver speaks the oldest version, MCP 2024-11-05, and expects an
// injected X-Api-Key header (set API_KEY). Tool: generate.
package main

import (
	"context"
	"strings"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type generateIn struct {
	Count int `json:"count,omitempty" jsonschema:"How many v4 UUIDs to return, 1-20. Defaults to 1."`
}

type generateOut struct {
	UUIDs []string `json:"uuids"`
}

func main() {
	mcpserve.Run(mcpserve.Spec{
		Name:            "uuidserver",
		Version:         "1.0.0",
		ProtocolVersion: "2024-11-05",
		Instructions:    "Generates random version 4 UUIDs. Ask for the exact count you need (1 to 20) in one call rather than calling repeatedly; every call returns fresh values.",
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, mcpserve.Tool("generate", "Generate UUIDs", "Generate 1-20 random (v4) UUIDs.", false),
				func(_ context.Context, _ *mcp.CallToolRequest, in generateIn) (*mcp.CallToolResult, generateOut, error) {
					n := in.Count
					if n == 0 {
						n = 1
					}
					if n < 1 || n > 20 {
						return mcpserve.ErrResult("count must be between 1 and 20, got %d", in.Count), generateOut{}, nil
					}
					out := generateOut{UUIDs: make([]string, n)}
					for i := range out.UUIDs {
						out.UUIDs[i] = uuid.NewString()
					}
					return mcpserve.Text(strings.Join(out.UUIDs, "\n")), out, nil
				})
		},
	})
}
