// promptserver speaks only MCP 2026-07-28 (stateless) and serves the same
// prompt catalogue on two surfaces: native MCP prompts (prompts/list,
// prompts/get) and two tools (list_prompts, get_prompt) for clients that only
// speak tools.
package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/andyjmorgan/sample-mcp-servers/internal/mcpserve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type message struct {
	Role mcp.Role `json:"role" jsonschema:"user or assistant"`
	Text string   `json:"text"`
}

type promptDef struct {
	prompt *mcp.Prompt
	render func(args map[string]string) []message
}

// prompts is the single catalogue behind both surfaces.
var prompts = []promptDef{
	{
		prompt: &mcp.Prompt{
			Name:        "summarize",
			Title:       "Summarize Text",
			Description: "Ask for a summary of a piece of text, optionally in a given style.",
			Arguments: []*mcp.PromptArgument{
				{Name: "text", Title: "Text", Description: "The text to summarize.", Required: true},
				{Name: "style", Title: "Style", Description: "Summary style, e.g. bullet points, one sentence, executive. Defaults to a short paragraph."},
			},
		},
		render: func(a map[string]string) []message {
			style := a["style"]
			if style == "" {
				style = "a short paragraph"
			}
			return []message{{Role: "user", Text: fmt.Sprintf("Summarize the following text as %s.\n\n%s", style, a["text"])}}
		},
	},
	{
		prompt: &mcp.Prompt{
			Name:        "translate",
			Title:       "Translate Text",
			Description: "Ask for a translation of a piece of text into a target language.",
			Arguments: []*mcp.PromptArgument{
				{Name: "text", Title: "Text", Description: "The text to translate.", Required: true},
				{Name: "language", Title: "Target Language", Description: "The language to translate into, e.g. French.", Required: true},
			},
		},
		render: func(a map[string]string) []message {
			return []message{{Role: "user", Text: fmt.Sprintf("Translate the following text into %s. Reply with the translation only.\n\n%s", a["language"], a["text"])}}
		},
	},
	{
		prompt: &mcp.Prompt{
			Name:        "code_review",
			Title:       "Code Review",
			Description: "Ask for a code review, optionally focused on one concern. Returns the request plus reviewer guidelines as a second message.",
			Arguments: []*mcp.PromptArgument{
				{Name: "code", Title: "Code", Description: "The code to review.", Required: true},
				{Name: "focus", Title: "Focus", Description: "What to focus on, e.g. security, performance, readability. Defaults to general correctness."},
			},
		},
		render: func(a map[string]string) []message {
			focus := a["focus"]
			if focus == "" {
				focus = "general correctness"
			}
			return []message{
				{Role: "user", Text: fmt.Sprintf("Please review this code, focusing on %s.\n\n```\n%s\n```", focus, a["code"])},
				{Role: "assistant", Text: "Reviewer guidelines: act as a senior engineer. List concrete problems first, most severe first, each with the line it applies to and a suggested fix. Do not restate the code. Say plainly if you find nothing wrong."},
			}
		},
	},
	{
		prompt: &mcp.Prompt{
			Name:        "daily_standup",
			Title:       "Daily Standup",
			Description: "Ask for a daily standup update in the usual three-question format. Takes no arguments.",
		},
		render: func(map[string]string) []message {
			return []message{{Role: "user", Text: "Help me write my daily standup update. Ask me, one at a time: what I did yesterday, what I am doing today, and whether anything is blocking me. Then format the answers as three short bullet lists."}}
		},
	},
}

func lookup(name string) (promptDef, bool) {
	for _, p := range prompts {
		if p.prompt.Name == name {
			return p, true
		}
	}
	return promptDef{}, false
}

// render validates the arguments and substitutes them into the prompt.
func render(name string, args map[string]string) (promptDef, []message, error) {
	p, ok := lookup(name)
	if !ok {
		names := make([]string, len(prompts))
		for i, d := range prompts {
			names[i] = d.prompt.Name
		}
		return p, nil, fmt.Errorf("unknown prompt %q (have %s)", name, strings.Join(names, ", "))
	}
	known := map[string]bool{}
	for _, a := range p.prompt.Arguments {
		known[a.Name] = true
		if a.Required && strings.TrimSpace(args[a.Name]) == "" {
			return p, nil, fmt.Errorf("prompt %q needs argument %q", name, a.Name)
		}
	}
	var extra []string
	for k := range args {
		if !known[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return p, nil, fmt.Errorf("prompt %q does not take argument(s) %s", name, strings.Join(extra, ", "))
	}
	return p, p.render(args), nil
}

type argInfo struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

type promptInfo struct {
	Name        string    `json:"name"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Arguments   []argInfo `json:"arguments"`
}

type listPromptsOut struct {
	Prompts []promptInfo `json:"prompts"`
}

type getPromptIn struct {
	Name      string            `json:"name" jsonschema:"Prompt name, as returned by list_prompts."`
	Arguments map[string]string `json:"arguments,omitempty" jsonschema:"Prompt arguments by name. Omit for prompts that take none."`
}

type getPromptOut struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Messages    []message `json:"messages"`
}

func register(s *mcp.Server) {
	for _, d := range prompts {
		s.AddPrompt(d.prompt, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			_, msgs, err := render(d.prompt.Name, req.Params.Arguments)
			if err != nil {
				return nil, err
			}
			res := &mcp.GetPromptResult{Description: d.prompt.Description}
			for _, m := range msgs {
				res.Messages = append(res.Messages, &mcp.PromptMessage{Role: m.Role, Content: &mcp.TextContent{Text: m.Text}})
			}
			return res, nil
		})
	}

	mcp.AddTool(s, mcpserve.Tool("list_prompts", "List Prompts", "List the available prompt templates with their titles, descriptions and arguments. Same catalogue as prompts/list.", true),
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listPromptsOut, error) {
			var out listPromptsOut
			var lines []string
			for _, d := range prompts {
				pi := promptInfo{Name: d.prompt.Name, Title: d.prompt.Title, Description: d.prompt.Description, Arguments: []argInfo{}}
				var args []string
				for _, a := range d.prompt.Arguments {
					pi.Arguments = append(pi.Arguments, argInfo{Name: a.Name, Title: a.Title, Description: a.Description, Required: a.Required})
					if a.Required {
						args = append(args, a.Name)
					} else {
						args = append(args, a.Name+"?")
					}
				}
				out.Prompts = append(out.Prompts, pi)
				lines = append(lines, fmt.Sprintf("%s(%s) - %s: %s", pi.Name, strings.Join(args, ", "), pi.Title, pi.Description))
			}
			return mcpserve.Text(strings.Join(lines, "\n")), out, nil
		})

	mcp.AddTool(s, mcpserve.Tool("get_prompt", "Get Prompt", "Render a prompt template with arguments. Returns the same messages as prompts/get.", true),
		func(_ context.Context, _ *mcp.CallToolRequest, in getPromptIn) (*mcp.CallToolResult, getPromptOut, error) {
			d, msgs, err := render(in.Name, in.Arguments)
			if err != nil {
				return mcpserve.ErrResult("%v", err), getPromptOut{}, nil
			}
			out := getPromptOut{Name: d.prompt.Name, Description: d.prompt.Description, Messages: msgs}
			parts := make([]string, len(msgs))
			for i, m := range msgs {
				parts[i] = fmt.Sprintf("[%s]\n%s", m.Role, m.Text)
			}
			return mcpserve.Text(strings.Join(parts, "\n\n")), out, nil
		})
}

var spec = mcpserve.Spec{
	Name:            "promptserver",
	Version:         "1.0.0",
	ProtocolVersion: "2026-07-28",
	Instructions: "Serves four prompt templates (summarize, translate, code_review, daily_standup) on two equivalent surfaces. " +
		"Clients with prompt support can use prompts/list and prompts/get. Clients that only call tools can use list_prompts " +
		"to see the same catalogue and get_prompt (name plus an arguments map) to get the same rendered messages. " +
		"Both surfaces return identical content; pick whichever your client supports.",
	Register: register,
}

func main() { mcpserve.Run(spec) }
