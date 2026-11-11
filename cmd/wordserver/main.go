// wordserver speaks MCP 2025-03-26 and is protected by HTTP Basic auth
// (set BASIC_USER and BASIC_PASSWORD). Tools: random_word, reverse, word_count.
package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var words = []string{
	"anvil", "badger", "cobalt", "donkey", "ember", "fjord", "glacier", "harbour", "iris", "juniper",
	"kestrel", "lantern", "meadow", "nimbus", "orchard", "pebble", "quartz", "raven", "saffron", "thistle",
	"umber", "velvet", "willow", "xenon", "yarrow", "zephyr",
}

type randomWordIn struct {
	Count int `json:"count,omitempty" jsonschema:"How many words to return, 1-20. Defaults to 1."`
}

type randomWordOut struct {
	Words []string `json:"words"`
}

type textIn struct {
	Text string `json:"text" jsonschema:"The text to operate on."`
}

type reverseOut struct {
	Reversed string `json:"reversed"`
}

type wordCountOut struct {
	Words      int `json:"words"`
	Characters int `json:"characters"`
}

func main() {
	mcpserve.Run(mcpserve.Spec{
		Name:            "wordserver",
		Version:         "1.0.0",
		ProtocolVersion: "2025-03-26",
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, mcpserve.Tool("random_word", "Random Words", "Return 1-20 random English words.", false),
				func(_ context.Context, _ *mcp.CallToolRequest, in randomWordIn) (*mcp.CallToolResult, randomWordOut, error) {
					n := in.Count
					if n == 0 {
						n = 1
					}
					if n < 1 || n > 20 {
						return mcpserve.ErrResult("count must be between 1 and 20, got %d", in.Count), randomWordOut{}, nil
					}
					out := randomWordOut{Words: make([]string, n)}
					for i := range out.Words {
						out.Words[i] = words[rand.IntN(len(words))]
					}
					return mcpserve.Text(strings.Join(out.Words, " ")), out, nil
				})
			mcp.AddTool(s, mcpserve.Tool("reverse", "Reverse Text", "Reverse a string (rune-aware).", true),
				func(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, reverseOut, error) {
					r := []rune(in.Text)
					for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
						r[i], r[j] = r[j], r[i]
					}
					out := reverseOut{Reversed: string(r)}
					return mcpserve.Text(out.Reversed), out, nil
				})
			mcp.AddTool(s, mcpserve.Tool("word_count", "Word Count", "Count the words and characters in a string.", true),
				func(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, wordCountOut, error) {
					out := wordCountOut{Words: len(strings.Fields(in.Text)), Characters: len([]rune(in.Text))}
					return mcpserve.Text(fmt.Sprintf("%d words, %d characters", out.Words, out.Characters)), out, nil
				})
		},
	})
}
