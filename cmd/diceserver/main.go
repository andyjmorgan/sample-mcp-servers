// diceserver speaks only MCP 2026-07-28 (stateless, no initialize) and is
// authorized with an Authorization: Bearer token. Tool: roll.
package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type rollIn struct {
	Count int `json:"count,omitempty" jsonschema:"Number of dice, 1-100. Defaults to 1."`
	Sides int `json:"sides,omitempty" jsonschema:"Sides per die, 2-1000. Defaults to 6."`
}

type rollOut struct {
	Rolls []int `json:"rolls"`
	Total int   `json:"total"`
}

func main() {
	mcpserve.Run(mcpserve.Spec{
		Name:            "diceserver",
		Version:         "1.0.0",
		ProtocolVersion: "2026-07-28",
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, &mcp.Tool{Name: "roll", Description: "Roll count dice with the given number of sides (count 1-100, sides 2-1000)."},
				func(_ context.Context, _ *mcp.CallToolRequest, in rollIn) (*mcp.CallToolResult, rollOut, error) {
					count, sides := in.Count, in.Sides
					if count == 0 {
						count = 1
					}
					if sides == 0 {
						sides = 6
					}
					if count < 1 || count > 100 {
						return mcpserve.ErrResult("count must be between 1 and 100, got %d", in.Count), rollOut{}, nil
					}
					if sides < 2 || sides > 1000 {
						return mcpserve.ErrResult("sides must be between 2 and 1000, got %d", in.Sides), rollOut{}, nil
					}
					out := rollOut{Rolls: make([]int, count)}
					parts := make([]string, count)
					for i := range out.Rolls {
						out.Rolls[i] = rand.IntN(sides) + 1
						out.Total += out.Rolls[i]
						parts[i] = fmt.Sprint(out.Rolls[i])
					}
					return mcpserve.Text(fmt.Sprintf("%dd%d: %s (total %d)", count, sides, strings.Join(parts, ", "), out.Total)), out, nil
				})
		},
	})
}
