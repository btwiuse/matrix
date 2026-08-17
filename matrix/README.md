# matrix — MiniMax matrix MCP 复刻

High-fidelity replica of the MiniMax **matrix** MCP server, built on the
[modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk).

Serves the exact same 22 tools with byte-identical names, descriptions and
input schemas as the real matrix MCP server, verified against the live
backend (`DIFFS: 0`).

## Layout

| File | Role |
|---|---|
| `schema.json` | Verbatim `tools/list` captured from the real matrix server (embedded at build time) |
| `types.go` | Typed input structs for all 22 tools (interface layer) |
| `handler.go` | `Handler` interface: 22 methods, one per tool, `Output = []byte` |
| `server.go` | `mcp.Server` assembly; registers all 22 tools via `mcp.AddTool` |
| `proxy.go` | `ProxyHandler`: forwards every call to the real matrix HTTP endpoint |
| `mock.go` | `MockHandler`: deterministic offline responses (same output shapes) |
| `deploy.go` | `LocalDeploy`: local `deploy`; each deployment publishes a fresh random-id directory under `data/<site-id>` (previous releases kept, like the real server); random 431-prefixed `website_id`, absolute `website_url` without trailing slash, Python-style JSON text, per-case `isError` flags |
| `envelope.go` | `EnvelopeRewriter`: HTTP-layer envelope patches go-sdk cannot express (`display_data`, explicit `isError:false`, tools/list `required` restore) |
| `site.go` | `SiteHandler`: serves `data/<project>` at `http://<project>.<domain>/` (optional index.html rewrite) |
| `router.go` | `Router`: dispatches by Host — site subdomains vs the MCP endpoint |
| `cmd/matrix` | Single entry point (cobra CLI): MCP + site hosting on one HTTP listener |
| `rewrite/` | Shared HTML injection core (also used by inject-proxy) |

## Run

```sh
# Mock mode (offline, deterministic) — HTTP on :8080 ($PORT honored)
go run ./cmd/matrix --mode mock

# Proxy mode: forward to the real matrix server (high fidelity)
MATRIX_URL=http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message \
MATRIX_SK=sk_... \
go run ./cmd/matrix            # mode=auto picks proxy when URL+SK are set

# Local deploy + site hosting: every deployment publishes a fresh random-id
# site, served by the same process at http://<site-id>.localhost (MCP stays
# reachable on the listen address or at http://localhost:PORT/mcp/...)
go run ./cmd/matrix --mode mock --data-dir ./data --workspace-dir /workspace --domain localhost

# Rewrite every served index.html with a snippet (inject-proxy's HTML injection)
go run ./cmd/matrix --data-dir ./data --inject-html '<script src="/probe.js"></script>'
```

## Site hosting (Host routing)

With `--data-dir` the same process that serves MCP also hosts what `deploy`
published: each site (a random id, like the real server's per-deployment
subdomain) becomes a second-level domain. One listener, dispatch by Host:

- `http://<site-id>.<domain>/` serves `data-dir/<site-id>/` (static files)
- the bare apex (`http://localhost/`) lists all published sites
- everything else is the MCP endpoint: the listen address itself
  (`http://127.0.0.1:PORT/`), apex paths like `http://localhost:PORT/mcp/...`,
  and any hostname outside the site namespace
- unknown sites, wrong domains and deeper subdomains are 404s (the site
  namespace owns every subdomain)
- with `--inject`/`--inject-html`, `index.html` (directory roots, explicit
  `/index.html` URLs and SPA fallbacks) is served with the snippet injected
  before `</body>` (idempotent); other files are served untouched
- missing extensionless paths fall back to `index.html` (SPA fallback, like
  the real gateway); missing paths with an extension, missing directories
  and directories without `index.html` are 404s (no directory listings)
- `deploy` output matches the real server: random 15-digit `website_id` with
  the `431` prefix, `website_url` without trailing slash, Python-style JSON
  in `content[0].text`, `display_data` in the result envelope, `isError`
  false for the soft missing-dist error and true for path validation errors

`*.localhost` resolves to loopback in modern browsers; for other domains
point the wildcard DNS record at this host.


## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--mode` | `auto` | `auto` \| `proxy` \| `mock` |
| `--url` | `$MATRIX_URL` | Real matrix MCP HTTP endpoint |
| `--token` | `$MATRIX_SK` | `?sk=` token |
| `--source` | `$MATRIX_SOURCE` or `hermes` | `?source=` label (server whitelists `openclaw`, `hermes`) |
| `--http` | `$PORT` or `:8080` | Listen address for the single HTTP listener (MCP + sites) |
| `--timeout` | `5m` | Upstream request timeout (proxy mode) |
| `--data-dir` | `$MATRIX_DATA_DIR` | When set, `deploy` copies the dist directory into `data-dir/<random-id>` locally (a fresh site per deployment, mirroring the real server) instead of forwarding/mocking, and the same process hosts the sites by Host; empty = no local hosting |
| `--workspace-dir` | `$MATRIX_WORKSPACE` or `/workspace` | Sandbox root; `deploy` rejects a `dist_dir` outside it, like the real server |
| `--domain` | `$MATRIX_DOMAIN` or `localhost` | Apex domain for deploy `website_url` and site hosting (subdomain dispatch) |
| `--inject` / `--inject-html` | empty | HTML snippet (file wins over inline) injected into every served `index.html` |

## Verify

```sh
go test ./... -v
```

Tests spin up the server over streamable HTTP with a real go-sdk client and check:

- `tools/list` returns 22 tools; every input schema is a JSON object
- `tools/call` on all 22 tools (mock) returns the expected JSON/markdown output
- invalid input (missing required field) yields a tool error, not a panic
  (`deploy` is deliberately lenient: a missing `dist_dir` defaults to
  `<workspace>/dist`, like the real server)
- Host routing: sites and MCP on one listener, deployed site served with the
  injected snippet end to end; raw envelopes carry `display_data` and the
  explicit `isError` flag
- `ProxyHandler` reaches the real matrix server (skipped when unreachable)

## Design notes

- **Fidelity**: schemas come from `schema.json`, not from Go reflection, so
  `tools/list` matches the real server exactly.
- **Interface layer**: `Handler` is the seam between the MCP server and
  tool implementations; swap in proxy / mock / your own backend.
- **`arguments` quirk**: the real server crashes on `arguments: null`, so the
  proxy always sends `{}` for parameterless tools (`get_voice_list`, ...).
- **Outputs**: passed through as raw JSON text (`Output = []byte`), exactly
  like the real server's `content[0].text`.
