// resourceserver speaks only MCP 2026-07-28 (stateless) and serves the same
// documents on two surfaces: native MCP resources (resources/list,
// resources/read, resources/templates/list) and two tools (list_resources,
// read_resource) for clients that only speak tools.
package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type doc struct {
	resource *mcp.Resource
	text     string
}

// docs is the single catalogue behind both surfaces.
var docs = []doc{
	{
		resource: &mcp.Resource{URI: "sample://docs/readme", Name: "readme", Title: "Read Me",
			Description: "What this server is and how its two surfaces relate.", MIMEType: "text/markdown"},
		text: `# resourceserver

A fixture MCP server for gateway testing. Every document here is static and
can be read two ways:

- **resources/read** with the document's URI, for clients that support MCP resources.
- **read_resource** (a tool) with the same URI, for clients that only call tools.

Both return the same bytes and the same MIME type.
`,
	},
	{
		resource: &mcp.Resource{URI: "sample://docs/getting-started", Name: "getting-started", Title: "Getting Started",
			Description: "A short walkthrough of listing and reading resources.", MIMEType: "text/markdown"},
		text: `# Getting started

1. List what is available: ` + "`resources/list`" + ` or the ` + "`list_resources`" + ` tool.
2. Pick a URI, for example ` + "`sample://docs/faq`" + `.
3. Read it: ` + "`resources/read`" + ` or ` + "`read_resource`" + ` with ` + "`uri`" + `.

Templated resources (` + "`resources/templates/list`" + `) take a value in the URI:
` + "`sample://greet/Ada`" + ` renders a greeting for Ada.
`,
	},
	{
		resource: &mcp.Resource{URI: "sample://docs/faq", Name: "faq", Title: "FAQ",
			Description: "Frequently asked questions about the fixture.", MIMEType: "text/markdown"},
		text: `# FAQ

**Do the resources change?** No. They are compiled into the binary.

**Is there a size limit?** Every document is under a few KB.

**Can I subscribe to updates?** No. Nothing ever changes, so there is nothing to subscribe to.

**Why are there tools as well?** Some MCP clients do not implement resources yet.
The tools expose the same content so a gateway can be tested either way.
`,
	},
	{
		resource: &mcp.Resource{URI: "sample://docs/config", Name: "config", Title: "Sample Config",
			Description: "A small JSON configuration document.", MIMEType: "application/json"},
		text: `{
  "service": "resourceserver",
  "protocolVersion": "2026-07-28",
  "stateless": true,
  "surfaces": ["resources", "tools"],
  "limits": { "maxDocumentBytes": 4096, "templates": 1 }
}
`,
	},
	{
		resource: &mcp.Resource{URI: "sample://docs/license", Name: "license", Title: "License",
			Description: "Plain-text licence notice for the sample content.", MIMEType: "text/plain"},
		text: `Sample content for MCP gateway testing.

This text is provided as-is, with no warranty, and may be copied freely.
It exists only so that a gateway has a text/plain resource to pass through.
`,
	},
}

const (
	greetTemplate = "sample://greet/{name}"
	greetPrefix   = "sample://greet/"
)

var greet = &mcp.ResourceTemplate{URITemplate: greetTemplate, Name: "greet", Title: "Greeting",
	Description: "A one-line greeting for the name in the URI, e.g. sample://greet/Ada.", MIMEType: "text/plain"}

// read resolves a URI against the static documents and the greet template.
func read(uri string) (text, mimeType string, ok bool) {
	for _, d := range docs {
		if d.resource.URI == uri {
			return d.text, d.resource.MIMEType, true
		}
	}
	if raw, found := strings.CutPrefix(uri, greetPrefix); found && raw != "" && !strings.Contains(raw, "/") {
		name, err := url.PathUnescape(raw)
		if err != nil || strings.TrimSpace(name) == "" {
			return "", "", false
		}
		return fmt.Sprintf("Hello, %s! This greeting was rendered from %s.\n", name, greetTemplate), greet.MIMEType, true
	}
	return "", "", false
}

func readHandler(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	text, mt, ok := read(req.Params.URI)
	if !ok {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: mt, Text: text}}}, nil
}

type resourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
	Size        int64  `json:"size"`
}

type templateInfo struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
}

type listResourcesOut struct {
	Resources []resourceInfo `json:"resources"`
	Templates []templateInfo `json:"templates"`
}

type readResourceIn struct {
	URI string `json:"uri" jsonschema:"Resource URI, e.g. sample://docs/readme or sample://greet/Ada."`
}

type readResourceOut struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType"`
	Text     string `json:"text"`
}

func register(s *mcp.Server) {
	for _, d := range docs {
		d.resource.Size = int64(len(d.text))
		s.AddResource(d.resource, readHandler)
	}
	s.AddResourceTemplate(greet, readHandler)

	mcp.AddTool(s, mcpserve.Tool("list_resources", "List Resources", "List the available documents and URI templates. Same catalogue as resources/list plus resources/templates/list.", true),
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listResourcesOut, error) {
			var out listResourcesOut
			var lines []string
			for _, d := range docs {
				r := d.resource
				out.Resources = append(out.Resources, resourceInfo{URI: r.URI, Name: r.Name, Title: r.Title, Description: r.Description, MIMEType: r.MIMEType, Size: int64(len(d.text))})
				lines = append(lines, fmt.Sprintf("%s (%s) - %s: %s", r.URI, r.MIMEType, r.Title, r.Description))
			}
			out.Templates = []templateInfo{{URITemplate: greet.URITemplate, Name: greet.Name, Title: greet.Title, Description: greet.Description, MIMEType: greet.MIMEType}}
			lines = append(lines, fmt.Sprintf("%s (%s, template) - %s: %s", greet.URITemplate, greet.MIMEType, greet.Title, greet.Description))
			return mcpserve.Text(strings.Join(lines, "\n")), out, nil
		})

	mcp.AddTool(s, mcpserve.Tool("read_resource", "Read Resource", "Read a document or templated resource by URI. Returns the same content as resources/read.", true),
		func(_ context.Context, _ *mcp.CallToolRequest, in readResourceIn) (*mcp.CallToolResult, readResourceOut, error) {
			text, mt, ok := read(in.URI)
			if !ok {
				return mcpserve.ErrResult("resource not found: %q (call list_resources for valid URIs)", in.URI), readResourceOut{}, nil
			}
			return mcpserve.Text(text), readResourceOut{URI: in.URI, MIMEType: mt, Text: text}, nil
		})
}

var spec = mcpserve.Spec{
	Name:            "resourceserver",
	Version:         "1.0.0",
	ProtocolVersion: "2026-07-28",
	Instructions: "Serves five static documents under sample://docs/ (markdown, JSON and plain text) and one template, " +
		"sample://greet/{name}, on two equivalent surfaces. Clients with resource support can use resources/list, " +
		"resources/templates/list and resources/read. Clients that only call tools can use list_resources for the same " +
		"catalogue and read_resource with a uri for the same content and MIME type. Pick whichever your client supports.",
	Register: register,
}

func main() { mcpserve.Run(spec) }
