package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connect(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ts := httptest.NewServer(mcpserve.NewHandler(spec))
	t.Cleanup(ts.Close)
	ctx := context.Background()
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	sess, err := c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	return ctx, sess
}

func TestPrompts(t *testing.T) {
	ctx, sess := connect(t)
	list, err := sess.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Prompts) != 4 {
		t.Fatalf("got %d prompts, want 4", len(list.Prompts))
	}
	for _, p := range list.Prompts {
		if p.Title == "" || p.Description == "" {
			t.Errorf("prompt %s missing title/description", p.Name)
		}
	}

	res, err := sess.GetPrompt(ctx, &mcp.GetPromptParams{Name: "translate", Arguments: map[string]string{"text": "hello", "language": "French"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].Role != "user" {
		t.Fatalf("translate messages: %+v", res.Messages)
	}
	if txt := res.Messages[0].Content.(*mcp.TextContent).Text; !strings.Contains(txt, "French") || !strings.Contains(txt, "hello") {
		t.Errorf("arguments not substituted: %q", txt)
	}

	res, err = sess.GetPrompt(ctx, &mcp.GetPromptParams{Name: "code_review", Arguments: map[string]string{"code": "x := 1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 2 || res.Messages[1].Role != "assistant" {
		t.Fatalf("code_review should return request + preamble: %+v", res.Messages)
	}

	if _, err := sess.GetPrompt(ctx, &mcp.GetPromptParams{Name: "summarize"}); err == nil {
		t.Error("summarize without text should fail")
	}
	if _, err := sess.GetPrompt(ctx, &mcp.GetPromptParams{Name: "daily_standup"}); err != nil {
		t.Errorf("daily_standup: %v", err)
	}
}

func TestToolsMatchPrompts(t *testing.T) {
	ctx, sess := connect(t)
	lr, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_prompts"})
	if err != nil || lr.IsError {
		t.Fatalf("list_prompts: %v %+v", err, lr)
	}
	var lo listPromptsOut
	b, _ := json.Marshal(lr.StructuredContent)
	if err := json.Unmarshal(b, &lo); err != nil || len(lo.Prompts) != 4 {
		t.Fatalf("list_prompts structured: %s", b)
	}

	args := map[string]string{"text": "a long story", "style": "one sentence"}
	gr, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "get_prompt", Arguments: map[string]any{"name": "summarize", "arguments": args}})
	if err != nil || gr.IsError {
		t.Fatalf("get_prompt: %v %+v", err, gr)
	}
	var got getPromptOut
	b, _ = json.Marshal(gr.StructuredContent)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	pr, err := sess.GetPrompt(ctx, &mcp.GetPromptParams{Name: "summarize", Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != len(pr.Messages) || got.Messages[0].Text != pr.Messages[0].Content.(*mcp.TextContent).Text {
		t.Errorf("tool and prompts/get differ: %s vs %+v", b, pr.Messages)
	}
	if txt := gr.Content[0].(*mcp.TextContent).Text; !strings.Contains(txt, "one sentence") {
		t.Errorf("text content: %q", txt)
	}

	bad, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "get_prompt", Arguments: map[string]any{"name": "nope"}})
	if err != nil || !bad.IsError {
		t.Errorf("unknown prompt should be a tool error: %v %+v", err, bad)
	}
}
