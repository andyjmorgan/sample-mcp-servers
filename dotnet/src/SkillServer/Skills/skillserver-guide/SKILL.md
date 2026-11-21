---
name: skillserver-guide
description: How to discover, verify and read the skills this server publishes over the MCP Skills extension (SEP-2640). Use when testing a gateway or client against skillserver.
license: Apache-2.0
metadata:
  author: sample-mcp-servers
  version: "1.0"
---

# skillserver guide

This server publishes Agent Skills over MCP. Every file of every skill is a
resource under `skill://`, read with `resources/read`. Two extension methods
sit on top:

- `skills/list` returns one entry per skill.
- `skills/get` returns the entry for one skill, given the URI of its `SKILL.md`.

The server does not declare `directoryRead`, so it does not implement
`resources/directory/read`. A skill's manifest already lists every file.

Each entry carries the skill's frontmatter verbatim and a manifest of its
files with a `sha256:` digest and byte size per file. Verify every file you
read against the manifest. See `references/protocol.md` for the cases this
server is built to exercise.

## The catalogue

| Skill | What it tests |
|---|---|
| `skill://skillserver-guide/SKILL.md` | this skill; no prefix, contains a nested skill |
| `skill://skillserver-guide/verify-digests/SKILL.md` | a nested skill, listed flat |
| `skill://anthropic/*/SKILL.md` | five skills from anthropics/skills under an `anthropic/` prefix |
| `skill://clock/SKILL.md` | a generated skill whose `resources` is `"dynamic"` |

`skill://anthropic/theme-factory/theme-showcase.pdf` is binary and comes back
as a base64 `blob` rather than `text`.
