# sample-mcp-servers

Thirteen tiny MCP servers for testing an MCP gateway: ten in Go (`cmd/`) and three in C# (`dotnet/`). Each one speaks exactly one protocol version and runs stateless (`Stateless: true`, so only the gateway deals with `Mcp-Session-Id`). Seven sit behind Keycloak OAuth, where MCP clients register through OAuth Client ID Metadata Documents (CIMD). The other three model the non-OAuth auth schemes a gateway has to inject on a client's behalf: HTTP Basic, `X-Api-Key`, and a static `Authorization: Bearer` token.

| Server | Tool | Protocol | Transport | Extra auth | Public endpoint |
|---|---|---|---|---|---|
| timeserver | `now` — `15:04:05 MST`, optional IANA `timezone` | 2025-03-26 | SSE | — | `https://mcp-time.donkeywork.dev/mcp` |
| dateserver | `today` — `2026-10-30 (Friday)`, optional `timezone` | 2025-06-18 | plain JSON (`JSONResponse: true`) | — | `https://mcp-date.donkeywork.dev/mcp` |
| uuidserver | `generate` — 1–20 v4 UUIDs (`count`) | 2024-11-05 | SSE | `X-Api-Key` | `https://mcp-uuid.donkeywork.dev/mcp` |
| diceserver | `roll` — `count` 1–100, `sides` 2–1000 | 2026-07-28 (no initialize) | SSE | — | `https://mcp-dice.donkeywork.dev/mcp` |
| countserver | `count` — 1..`n` with `delay_ms` pauses, `notifications/progress` when a progress token is sent | 2026-07-28 | SSE | — | `https://mcp-count.donkeywork.dev/mcp` |
| wordserver | `random_word` (1–20 words), `reverse` (`text`), `word_count` (`text`) | 2025-03-26 | SSE | HTTP Basic only | `https://mcp-word.donkeywork.dev/mcp` |
| hashserver | `sha256`, `base64_encode`, `base64_decode` (all take `text`) | 2025-06-18 | plain JSON | `X-Api-Key` only | `https://mcp-hash.donkeywork.dev/mcp` |
| coinserver | `flip` (`count` 1–100), `pick` (`options` 1–50), `random_int` (`min`,`max`) | 2026-07-28 | SSE | static Bearer only | `https://mcp-coin.donkeywork.dev/mcp` |
| promptserver | MCP prompts `summarize`, `translate`, `code_review`, `daily_standup`; same content via tools `list_prompts`, `get_prompt` (`name`, `arguments`) | 2026-07-28 | SSE | — | `https://mcp-prompts.donkeywork.dev/mcp` |
| resourceserver | MCP resources `sample://docs/{readme,getting-started,faq,config,license}` + template `sample://greet/{name}`; same content via tools `list_resources`, `read_resource` (`uri`) | 2026-07-28 | SSE | — | `https://mcp-resources.donkeywork.dev/mcp` |

### C# servers (`dotnet/`)

These use the official C# SDK 2.4.0, because it ships the Skills and Tasks extensions and the Go SDK doesn't yet. Each is pinned to 2026-07-28, stateless, and behind Keycloak OAuth like the Go servers.

| Server | Surface | Protocol | Extra auth | Public endpoint |
|---|---|---|---|---|
| skillserver | Skills extension (`io.modelcontextprotocol/skills`): `skills/list`, `skills/get`, every skill file as a `skill://` resource. Same catalogue via tools `list_skills`, `read_skill_file` (`uri`). | 2026-07-28 | — | `https://mcp-skills.donkeywork.dev/mcp` |
| taskserver | Tasks extension (`io.modelcontextprotocol/tasks`): `generate_report`, `count` (cancellable), `fail_job` (ends failed), `crash_job` (ends with an error result), `approve_job` (parks in `input_required`), `batch_export` (task required), `add` (never a task) | 2026-07-28 | — | `https://mcp-tasks.donkeywork.dev/mcp` |
| elicitationserver | `order_pizza`: asks for anything missing from the order with a form elicitation over multi round-trip requests (`input_required` + `requestState`), up to two rounds | 2026-07-28 | — | `https://mcp-elicitation.donkeywork.dev/mcp` |

The skillserver catalogue is five Apache-2.0 skills from [anthropics/skills](https://github.com/anthropics/skills) (commit `e4bb5d9`) under `skill://anthropic/`, each with its `LICENSE.txt`: internal-comms, mcp-builder, theme-factory (includes a binary PDF), webapp-testing and brand-guidelines. Two skills are written for this repo: `skillserver-guide`, which contains the nested skill `verify-digests`, and a generated `clock` skill whose entry carries `"resources": "dynamic"`.

taskserver keeps tasks in one in-memory store, so it must run as a single replica.

```bash
cd dotnet
dotnet test tests/SampleMcp.Tests                    # all three servers end to end over HTTP
PORT=8080 dotnet run --project src/SkillServer        # no auth, http://localhost:8080/mcp
```

A server pinned to 2026-07-28 answers a legacy `initialize` with `2025-11-25`, and the client is expected to disconnect. That's the go-sdk behaviour, and it's the case a translating gateway has to handle.

## Auth

### OAuth / CIMD (time, date, uuid, dice, count, prompts, resources)

Every request to `/mcp` needs `Authorization: Bearer <Keycloak access token>`. The token must:

- be signed by the `Agents` realm (`https://auth.donkeywork.dev/realms/Agents`)
- have the server's endpoint URL in `aud`
- carry the server's scope (`mcp:time`, `mcp:date`, `mcp:uuid`, `mcp:dice`, `mcp:count`, `mcp:prompts`, `mcp:resources`, and for the C# servers `mcp:skills`, `mcp:tasks`, `mcp:elicitation`)

An unauthenticated request gets `401` with `WWW-Authenticate: Bearer resource_metadata="…", scope="…"`. The Protected Resource Metadata at `/.well-known/oauth-protected-resource/mcp` points clients at Keycloak. There, a CIMD client (Claude Code, Claude Desktop, VS Code) requests the scope. An audience mapper on that client scope then puts the endpoint URL into `aud`.

The uuidserver also requires `X-Api-Key` when `API_KEY` is set. That models a gateway injecting a header on top of OAuth.

### Non-OAuth schemes (word, hash, coin)

These three have no Keycloak in front of them; the credential is a static secret the gateway (or the user) supplies.

| Server | Scheme | Env | Rejection |
|---|---|---|---|
| wordserver | `Authorization: Basic base64(user:password)` | `BASIC_USER`, `BASIC_PASSWORD` | `401` + `WWW-Authenticate: Basic realm="wordserver"` |
| hashserver | `X-Api-Key: <key>` | `API_KEY` | `401` plain text |
| coinserver | `Authorization: Bearer <token>` (opaque, not a JWT) | `BEARER_TOKEN` | `401` + `WWW-Authenticate: Bearer error="invalid_token"` |

`BEARER_TOKEN` and `OAUTH_ISSUER` are mutually exclusive (the server exits if both are set). The layers compose, so any server can be given any combination of `API_KEY`, `BASIC_*` and one bearer scheme.

## Configuration

| Env | Meaning |
|---|---|
| `PORT` | listen port, default `8080` |
| `PUBLIC_URL` | external base URL; the resource is `<PUBLIC_URL>/mcp` |
| `OAUTH_ISSUER` | realm issuer; when unset, OAuth is off (local dev) |
| `OAUTH_JWKS_URL` | JWKS location, default `<issuer>/protocol/openid-connect/certs` |
| `OAUTH_SCOPE` | required scope, advertised in PRM and `WWW-Authenticate` |
| `API_KEY` | when set, requests must also send a matching `X-Api-Key` |
| `BASIC_USER` / `BASIC_PASSWORD` | when set, requests must send matching HTTP Basic credentials |
| `BEARER_TOKEN` | when set, requests must send `Authorization: Bearer <token>` (static; excludes `OAUTH_ISSUER`) |

## Build and run

```bash
go run ./cmd/timeserver                       # no auth, http://localhost:8080/mcp
go run ./cmd/mcpprobe -url http://localhost:8080/mcp -tool now -args '{"timezone":"Europe/Dublin"}'
go run ./cmd/mcpprobe -url http://localhost:8080/mcp -tool count -args '{"n":5}' -progress   # countserver

BASIC_USER=alice BASIC_PASSWORD=s3cret go run ./cmd/wordserver
go run ./cmd/mcpprobe -basic alice:s3cret -tool random_word -args '{"count":3}'
API_KEY=k123 go run ./cmd/hashserver
go run ./cmd/mcpprobe -api-key k123 -tool sha256 -args '{"text":"abc"}'
BEARER_TOKEN=tok456 go run ./cmd/coinserver
go run ./cmd/mcpprobe -token tok456 -tool flip -args '{"count":5}'

./build.sh                                    # build + push all images (Go and C#) to 192.168.0.140:5555/sample-mcp-servers/<server>
```

`cmd/mcpprobe` is a small test client (`-token`, `-api-key`, `-basic user:pass`, `-progress`). It isn't shipped in any image.

Deployment manifests live in the lab GitOps repo under `clusters/attic/applications/sample-mcp-servers/`.
