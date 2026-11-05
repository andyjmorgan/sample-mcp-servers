// mcpprobe is a tiny client for exercising the sample servers by hand. It is
// not built into any image.
//
//	mcpprobe -url https://mcp-dice.donkeywork.dev/mcp -tool roll -args '{"count":3,"sides":20}' \
//	  -token "$TOKEN"
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type headerTransport struct {
	headers map[string]string
	base    http.RoundTripper
}

func (t headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range t.headers {
		r.Header.Set(k, v)
	}
	return t.base.RoundTrip(r)
}

func main() {
	url := flag.String("url", "http://localhost:8080/mcp", "MCP endpoint")
	tool := flag.String("tool", "", "tool to call (empty: just list tools)")
	args := flag.String("args", "{}", "tool arguments as JSON")
	token := flag.String("token", "", "bearer token")
	apiKey := flag.String("api-key", "", "X-Api-Key value")
	basic := flag.String("basic", "", "HTTP Basic credentials as user:password")
	progress := flag.Bool("progress", false, "send a progress token and print progress events")
	flag.Parse()

	headers := map[string]string{}
	if *token != "" {
		headers["Authorization"] = "Bearer " + *token
	}
	if *apiKey != "" {
		headers["X-Api-Key"] = *apiKey
	}
	if *basic != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(*basic))
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "mcpprobe", Version: "1.0.0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, r *mcp.ProgressNotificationClientRequest) {
			fmt.Printf("progress %v/%v %s\n", r.Params.Progress, r.Params.Total, r.Params.Message)
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   *url,
		HTTPClient: &http.Client{Transport: headerTransport{headers, http.DefaultTransport}},
	}, nil)
	if err != nil {
		fail("connect: %v", err)
	}
	defer sess.Close()
	if ir := sess.InitializeResult(); ir != nil {
		fmt.Printf("server %s %s, protocol %s\n", ir.ServerInfo.Name, ir.ServerInfo.Version, ir.ProtocolVersion)
	}

	if *tool == "" {
		tools, err := sess.ListTools(ctx, nil)
		if err != nil {
			fail("list tools: %v", err)
		}
		for _, t := range tools.Tools {
			fmt.Printf("- %s: %s\n", t.Name, t.Description)
		}
		return
	}

	var a map[string]any
	if err := json.Unmarshal([]byte(*args), &a); err != nil {
		fail("bad -args: %v", err)
	}
	params := &mcp.CallToolParams{Name: *tool, Arguments: a}
	if *progress {
		params.SetProgressToken("mcpprobe")
	}
	res, err := sess.CallTool(ctx, params)
	if err != nil {
		fail("call: %v", err)
	}
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			fmt.Println(t.Text)
		}
	}
	if res.IsError {
		os.Exit(1)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
