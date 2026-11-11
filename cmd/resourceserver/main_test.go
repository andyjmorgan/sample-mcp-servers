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

func TestResources(t *testing.T) {
	ctx, sess := connect(t)
	list, err := sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Resources) != len(docs) {
		t.Fatalf("got %d resources, want %d", len(list.Resources), len(docs))
	}
	mimes := map[string]bool{}
	for _, r := range list.Resources {
		mimes[r.MIMEType] = true
		if r.Title == "" || r.Description == "" || r.Size == 0 || r.Size > 4096 {
			t.Errorf("resource %s: %+v", r.URI, r)
		}
	}
	for _, m := range []string{"text/markdown", "application/json", "text/plain"} {
		if !mimes[m] {
			t.Errorf("no %s resource", m)
		}
	}

	tl, err := sess.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tl.ResourceTemplates) != 1 || tl.ResourceTemplates[0].URITemplate != greetTemplate {
		t.Fatalf("templates: %+v", tl.ResourceTemplates)
	}

	rr, err := sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "sample://docs/config"})
	if err != nil {
		t.Fatal(err)
	}
	if c := rr.Contents[0]; c.MIMEType != "application/json" || !json.Valid([]byte(c.Text)) {
		t.Errorf("config: %+v", c)
	}

	rr, err = sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "sample://greet/Ada%20Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	if c := rr.Contents[0]; c.MIMEType != "text/plain" || !strings.Contains(c.Text, "Hello, Ada Lovelace!") {
		t.Errorf("greet: %+v", c)
	}

	if _, err := sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "sample://docs/missing"}); err == nil {
		t.Error("missing resource should fail")
	}
}

func TestToolsMatchResources(t *testing.T) {
	ctx, sess := connect(t)
	lr, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_resources"})
	if err != nil || lr.IsError {
		t.Fatalf("list_resources: %v %+v", err, lr)
	}
	var lo listResourcesOut
	b, _ := json.Marshal(lr.StructuredContent)
	if err := json.Unmarshal(b, &lo); err != nil || len(lo.Resources) != len(docs) || len(lo.Templates) != 1 {
		t.Fatalf("list_resources structured: %s", b)
	}

	for _, uri := range []string{"sample://docs/readme", "sample://docs/license", "sample://greet/Bob"} {
		tr, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "read_resource", Arguments: map[string]any{"uri": uri}})
		if err != nil || tr.IsError {
			t.Fatalf("read_resource %s: %v %+v", uri, err, tr)
		}
		var ro readResourceOut
		b, _ := json.Marshal(tr.StructuredContent)
		if err := json.Unmarshal(b, &ro); err != nil {
			t.Fatal(err)
		}
		rr, err := sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatal(err)
		}
		c := rr.Contents[0]
		if ro.Text != c.Text || ro.MIMEType != c.MIMEType || tr.Content[0].(*mcp.TextContent).Text != c.Text {
			t.Errorf("%s: tool %+v vs resources/read %+v", uri, ro, c)
		}
	}

	bad, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "read_resource", Arguments: map[string]any{"uri": "sample://nope"}})
	if err != nil || !bad.IsError {
		t.Errorf("unknown uri should be a tool error: %v %+v", err, bad)
	}
}
