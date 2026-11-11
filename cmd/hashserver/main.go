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
		Instructions:    "Hashes and encodes text: sha256 returns a hex digest, base64_encode and base64_decode convert standard padded base64. All results are deterministic, so do not call twice for the same input.",
		JSONResponse:    true,
		Register: func(s *mcp.Server) {
			mcp.AddTool(s, mcpserve.Tool("sha256", "SHA-256 Hash", "SHA-256 digest of the text, hex encoded.", true),
				func(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, sha256Out, error) {
					sum := sha256.Sum256([]byte(in.Text))
					out := sha256Out{Hex: hex.EncodeToString(sum[:])}
					return mcpserve.Text(out.Hex), out, nil
				})
			mcp.AddTool(s, mcpserve.Tool("base64_encode", "Base64 Encode", "Base64 (standard, padded) encode the text.", true),
				func(_ context.Context, _ *mcp.CallToolRequest, in textIn) (*mcp.CallToolResult, base64Out, error) {
					out := base64Out{Result: base64.StdEncoding.EncodeToString([]byte(in.Text))}
					return mcpserve.Text(out.Result), out, nil
				})
			mcp.AddTool(s, mcpserve.Tool("base64_decode", "Base64 Decode", "Decode standard base64 text back to UTF-8.", true),
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
