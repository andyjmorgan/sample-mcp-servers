// hashserver speaks MCP 2025-06-18 with plain JSON responses and is protected
// by an X-Api-Key header alone (set API_KEY). Tools: sha256, base64_encode,
// base64_decode.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type textIn struct {
	Text string `json:"text" jsonschema:"The input text."`
}

type sha256Out struct {
	Hex string `json:"hex"`
}

type base64Out struct {
	Result string `json:"result"`
}

func main() {
	mcpserve.Run(mcpserve.Spec{
		Name:            "hashserver",
		Version:         "1.0.0",
		ProtocolVersion: "2025-06-18",
		JSONResponse:    true,
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, &mcp.Tool{Name: "sha256", Description: "SHA-256 digest of the text, hex encoded."},
				func(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, sha256Out, error) {
					sum := sha256.Sum256([]byte(in.Text))
					out := sha256Out{Hex: hex.EncodeToString(sum[:])}
					return mcpserve.Text(out.Hex), out, nil
				})
			mcp.AddTool(s, &mcp.Tool{Name: "base64_encode", Description: "Base64 (standard, padded) encode the text."},
				func(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, base64Out, error) {
					out := base64Out{Result: base64.StdEncoding.EncodeToString([]byte(in.Text))}
					return mcpserve.Text(out.Result), out, nil
				})
			mcp.AddTool(s, &mcp.Tool{Name: "base64_decode", Description: "Decode standard base64 text back to UTF-8."},
				func(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, base64Out, error) {
					b, err := base64.StdEncoding.DecodeString(in.Text)
					if err != nil {
						return mcpserve.ErrResult("not valid base64: %v", err), base64Out{}, nil
					}
					out := base64Out{Result: string(b)}
					return mcpserve.Text(out.Result), out, nil
				})
		},
	})
}
