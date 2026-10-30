# sample-mcp-servers

Five tiny MCP servers for testing an MCP gateway. Each one speaks exactly one protocol version, runs stateless (`Stateless: true`, so only the gateway deals with `Mcp-Session-Id`), and sits behind Keycloak OAuth. MCP clients register through OAuth Client ID Metadata Documents (CIMD).

| Server | Tool | Protocol | Transport | Extra auth | Public endpoint |
|---|---|---|---|---|---|
| timeserver | `now` — `15:04:05 MST`, optional IANA `timezone` | 2025-03-26 | SSE | — | `https://mcp-time.donkeywork.dev/mcp` |
| dateserver | `today` — `2026-10-30 (Friday)`, optional `timezone` | 2025-06-18 | plain JSON (`JSONResponse: true`) | — | `https://mcp-date.donkeywork.dev/mcp` |
| uuidserver | `generate` — 1–20 v4 UUIDs (`count`) | 2024-11-05 | SSE | `X-Api-Key` | `https://mcp-uuid.donkeywork.dev/mcp` |
| diceserver | `roll` — `count` 1–100, `sides` 2–1000 | 2026-07-28 (no initialize) | SSE | — | `https://mcp-dice.donkeywork.dev/mcp` |
| countserver | `count` — 1..`n` with `delay_ms` pauses, `notifications/progress` when a progress token is sent | 2026-07-28 | SSE | — | `https://mcp-count.donkeywork.dev/mcp` |

A server pinned to 2026-07-28 answers a legacy `initialize` with `2025-11-25`, and the client is expected to disconnect. That's the go-sdk behaviour, and it's the case a translating gateway has to handle.

## Auth

Every request to `/mcp` needs `Authorization: Bearer <Keycloak access token>`. The token must:

- be signed by the `Agents` realm (`https://auth.donkeywork.dev/realms/Agents`)
- have the server's endpoint URL in `aud`
- carry the server's scope (`mcp:time`, `mcp:date`, `mcp:uuid`, `mcp:dice`, `mcp:count`)

An unauthenticated request gets `401` with `WWW-Authenticate: Bearer resource_metadata="…", scope="…"`. The Protected Resource Metadata at `/.well-known/oauth-protected-resource/mcp` points clients at Keycloak. There, a CIMD client (Claude Code, Claude Desktop, VS Code) requests the scope. An audience mapper on that client scope then puts the endpoint URL into `aud`.

The uuidserver also requires `X-Api-Key` when `API_KEY` is set. That models a gateway injecting a header.

## Configuration

| Env | Meaning |
|---|---|
| `PORT` | listen port, default `8080` |
| `PUBLIC_URL` | external base URL; the resource is `<PUBLIC_URL>/mcp` |
| `OAUTH_ISSUER` | realm issuer; when unset, OAuth is off (local dev) |
| `OAUTH_JWKS_URL` | JWKS location, default `<issuer>/protocol/openid-connect/certs` |
| `OAUTH_SCOPE` | required scope, advertised in PRM and `WWW-Authenticate` |
| `API_KEY` | when set, requests must also send a matching `X-Api-Key` |

## Build and run

```bash
go run ./cmd/timeserver                       # no auth, http://localhost:8080/mcp
go run ./cmd/mcpprobe -url http://localhost:8080/mcp -tool now -args '{"timezone":"Europe/Dublin"}'
go run ./cmd/mcpprobe -url http://localhost:8080/mcp -tool count -args '{"n":5}' -progress   # countserver

./build.sh                                    # build + push all images to 192.168.0.140:5555/sample-mcp-servers/<server>
```

`cmd/mcpprobe` is a small test client (`-token`, `-api-key`, `-progress`). It isn't shipped in any image.

Deployment manifests live in the lab GitOps repo under `clusters/attic/applications/sample-mcp-servers/`.
