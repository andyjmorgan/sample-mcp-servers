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

## Why two surfaces

MCP clients are uneven. Some implement the resources capability and show
resources in their own UI. Others only know how to call tools. A gateway in
front of this server has to cope with both, so the server exposes the same
catalogue through both doors:

| Surface | List | Read |
|---|---|---|
| Resources | ` + "`resources/list`" + ` and ` + "`resources/templates/list`" + ` | ` + "`resources/read`" + ` |
| Tools | ` + "`list_resources`" + ` | ` + "`read_resource`" + ` |

The tool results carry the document in ` + "`structuredContent`" + ` and again as a
JSON text block, so a text-only client sees the same data as a structured one.

## What a gateway should do with this

A gateway that merges several servers behind one front door has to answer
three questions for every resource request:

1. Which server owns the URI? This server owns everything under
   ` + "`sample://docs/`" + ` and the ` + "`sample://greet/{name}`" + ` template. A merged
   listing tags each entry with the owning server in ` + "`_meta`" + `.
2. Is the content text or binary? Every document here is text. The MIME type
   tells the client how to render it: markdown, plain text or JSON.
3. Can the listing be cached? Yes. Nothing here changes while the process
   runs, and the server does not send ` + "`listChanged`" + ` notifications.

## Documents

- ` + "`sample://docs/readme`" + ` this file
- ` + "`sample://docs/getting-started`" + ` a walkthrough of the two surfaces
- ` + "`sample://docs/faq`" + ` questions that come up when testing a gateway
- ` + "`sample://docs/config`" + ` a small JSON document for the JSON rendering path
- ` + "`sample://docs/license`" + ` a plain-text notice

## Authentication

The server validates a bearer token issued by the lab Keycloak realm and
expects the ` + "`mcp:resources`" + ` audience. Without a token it answers 401 with a
` + "`WWW-Authenticate`" + ` header that points at the protected resource metadata, so
a client can discover the authorization server and register itself.
`,
	},
	{
		resource: &mcp.Resource{URI: "sample://docs/getting-started", Name: "getting-started", Title: "Getting Started",
			Description: "A short walkthrough of listing and reading resources.", MIMEType: "text/markdown"},
		text: `# Getting started

This walkthrough reads one document three ways so you can compare what each
path returns. The content is identical; only the envelope differs.

## 1. List what is available

Over the resources surface:

` + "```" + `json
{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}
` + "```" + `

The result lists five documents, each with a URI, name, title, description and
MIME type. Templates live in a separate call:

` + "```" + `json
{"jsonrpc":"2.0","id":2,"method":"resources/templates/list","params":{}}
` + "```" + `

Over the tools surface, call ` + "`list_resources`" + ` with no arguments. The result
includes a ` + "`size`" + ` for each document in bytes.

## 2. Pick a URI

Static documents have fixed URIs, for example ` + "`sample://docs/faq`" + `.

Templated resources take a value in the URI. The template
` + "`sample://greet/{name}`" + ` expands to ` + "`sample://greet/Ada`" + `, which renders a
greeting for Ada. Any name works; the server does not validate it beyond
rejecting an empty one.

## 3. Read it

Over the resources surface:

` + "```" + `json
{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"sample://docs/faq"}}
` + "```" + `

The result has a ` + "`contents`" + ` array with one entry carrying ` + "`uri`" + `,
` + "`mimeType`" + ` and ` + "`text`" + `.

Over the tools surface, call ` + "`read_resource`" + ` with ` + "`{\"uri\": \"sample://docs/faq\"}`" + `.
The tool returns the same three fields in ` + "`structuredContent`" + `.

## 4. Read it through a gateway

Behind a gateway the URI does not change. The gateway finds the owning server
from its cached listings, forwards the read, and returns the upstream result
unchanged. If the URI is not in any cached listing the gateway refreshes the
listings once before giving up with a not-found error.

## Things to try

- Read ` + "`sample://docs/config`" + ` and check the MIME type is ` + "`application/json`" + `.
- Read ` + "`sample://greet/Grace`" + ` and confirm the template resolved.
- Read ` + "`sample://docs/missing`" + ` and confirm you get a resource-not-found error
  rather than an empty result.
- Read the same document over both surfaces and diff the text.
`,
	},
	{
		resource: &mcp.Resource{URI: "sample://docs/faq", Name: "faq", Title: "FAQ",
			Description: "Frequently asked questions about the fixture.", MIMEType: "text/markdown"},
		text: `# FAQ

## Content

**Do the resources change?** No. They are compiled into the binary. A rebuild
is the only way to change them, which makes the server a stable fixture for
cache tests.

**Is there a size limit?** Every document is under 8 KB. The config document
advertises a ` + "`maxDocumentBytes`" + ` limit, but that value is itself fixture data,
not something the server enforces.

**Can I subscribe to updates?** No. Nothing ever changes, so there is nothing
to subscribe to. The server does not advertise the ` + "`subscribe`" + ` capability
and will answer ` + "`resources/subscribe`" + ` with method not found.

**What MIME types are used?** Markdown for the prose documents, plain text for
the licence and the greeting, JSON for the config document. That covers the
three rendering paths a client is likely to have.

## Surfaces

**Why are there tools as well?** Some MCP clients do not implement resources
yet. The tools expose the same content so a gateway can be tested either way.

**Do the tools return the same bytes?** Yes. ` + "`read_resource`" + ` calls the same
lookup as ` + "`resources/read`" + ` and copies the text and MIME type into its result.

**Are the tool results structured?** Yes. Both tools declare an output schema
and return ` + "`structuredContent`" + `. The text block is the same JSON serialized,
as the specification asks of tools that return structured content.

## Gateway behaviour

**How does a gateway know which server owns a URI?** From its cached
listings. An exact match on a listed URI wins. Failing that, the longest
template prefix (everything before the first ` + "`{`" + `) wins. Failing that, the
gateway refreshes its listings and tries once more.

**What happens on the merged route?** Listings from every server are
concatenated and each entry is tagged with the owning server's slug in
` + "`_meta`" + `. URIs are not rewritten, because a URI is the resource's identity.

**What if two servers list the same URI?** The first server in slug order
wins. This fixture only lists URIs under ` + "`sample://`" + `, so clashes with other
servers are unlikely unless another fixture copies it.

## Authentication

**Which scope do I need?** ` + "`mcp:resources`" + `. Tokens without that audience are
rejected with 401.

**Can I use an API key instead?** Not on this server. The fetch and search
servers accept an ` + "`X-Api-Key`" + ` header; this one is OAuth only so that the
gateway's OAuth path gets exercised.
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
  "limits": { "maxDocumentBytes": 8192, "templates": 1 }
}
`,
	},
	{
		resource: &mcp.Resource{URI: "sample://docs/license", Name: "license", Title: "License",
			Description: "Plain-text licence notice for the sample content.", MIMEType: "text/plain"},
		text: `Sample content for MCP gateway testing.

This text is provided as-is, with no warranty, and may be copied freely.
It exists only so that a gateway has a text/plain resource to pass through.

Permission is granted to anyone obtaining a copy of this fixture and its
associated documents to use, copy, modify, merge, publish and distribute
them without restriction, provided that this notice is kept with any copy.

The fixture is a test aid. It is not a product, it carries no support
commitment, and nothing in it should be taken as advice. If it breaks,
you keep both pieces.

Third-party notices

The server is built with the Model Context Protocol Go SDK and the Go
standard library. Each is distributed under its own licence, which is
not changed by this notice.

Contact

Questions about the fixture belong with the gateway it was written to
test, not with the fixture itself.
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
