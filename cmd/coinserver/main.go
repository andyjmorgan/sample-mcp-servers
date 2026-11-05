// coinserver speaks MCP 2025-11-25 and is protected by a static
// Authorization: Bearer token (set BEARER_TOKEN; no OAuth). Tools: flip, pick,
// random_int.
package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type flipIn struct {
	Count int `json:"count,omitempty" jsonschema:"Number of coins to flip, 1-100. Defaults to 1."`
}

type flipOut struct {
	Flips []string `json:"flips"`
	Heads int      `json:"heads"`
	Tails int      `json:"tails"`
}

type pickIn struct {
	Options []string `json:"options" jsonschema:"The options to choose from, 1-50 entries."`
}

type pickOut struct {
	Choice string `json:"choice"`
	Index  int    `json:"index"`
}

type randomIntIn struct {
	Min int `json:"min,omitempty" jsonschema:"Lowest value, inclusive. Defaults to 0."`
	Max int `json:"max,omitempty" jsonschema:"Highest value, inclusive. Defaults to 100."`
}

type randomIntOut struct {
	Value int `json:"value"`
}

func main() {
	mcpserve.Run(mcpserve.Spec{
		Name:            "coinserver",
		Version:         "1.0.0",
		ProtocolVersion: "2025-11-25",
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, &mcp.Tool{Name: "flip", Description: "Flip 1-100 coins."},
				func(_ context.Context, _ *mcp.CallToolRequest, in flipIn) (*mcp.CallToolResult, flipOut, error) {
					n := in.Count
					if n == 0 {
						n = 1
					}
					if n < 1 || n > 100 {
						return mcpserve.ErrResult("count must be between 1 and 100, got %d", in.Count), flipOut{}, nil
					}
					out := flipOut{Flips: make([]string, n)}
					for i := range out.Flips {
						if rand.IntN(2) == 0 {
							out.Flips[i] = "heads"
							out.Heads++
						} else {
							out.Flips[i] = "tails"
							out.Tails++
						}
					}
					return mcpserve.Text(fmt.Sprintf("%s (%d heads, %d tails)", strings.Join(out.Flips, ", "), out.Heads, out.Tails)), out, nil
				})
			mcp.AddTool(s, &mcp.Tool{Name: "pick", Description: "Pick one option at random from a list of 1-50."},
				func(_ context.Context, _ *mcp.CallToolRequest, in pickIn) (*mcp.CallToolResult, pickOut, error) {
					if len(in.Options) < 1 || len(in.Options) > 50 {
						return mcpserve.ErrResult("options must have 1-50 entries, got %d", len(in.Options)), pickOut{}, nil
					}
					i := rand.IntN(len(in.Options))
					out := pickOut{Choice: in.Options[i], Index: i}
					return mcpserve.Text(out.Choice), out, nil
				})
			mcp.AddTool(s, &mcp.Tool{Name: "random_int", Description: "A uniformly random integer in [min, max] (defaults 0-100)."},
				func(_ context.Context, _ *mcp.CallToolRequest, in randomIntIn) (*mcp.CallToolResult, randomIntOut, error) {
					lo, hi := in.Min, in.Max
					if lo == 0 && hi == 0 {
						hi = 100
					}
					if hi < lo {
						return mcpserve.ErrResult("max (%d) must be >= min (%d)", hi, lo), randomIntOut{}, nil
					}
					out := randomIntOut{Value: lo + rand.IntN(hi-lo+1)}
					return mcpserve.Text(fmt.Sprint(out.Value)), out, nil
				})
		},
	})
}
