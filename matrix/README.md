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
| `deploy.go` | `LocalDeploy`: local `deploy`; each deployment publishes a fresh random-id directory under `data/<site-id>` (previous releases kept, like the real server) |
| `site.go` | `SiteHandler`: serves `data/<project>` at `http://<project>.<domain>/` |
| `cmd/matrix` | Entry point (cobra CLI): stdio or streamable HTTP |
| `cmd/deploy-server` | Site server (cobra CLI): subdomain hosting for deployed sites |

## Run

```sh
# Mock mode (offline, deterministic)
go run ./cmd/matrix --mode mock

# Proxy mode: forward to the real matrix server (high fidelity)
MATRIX_URL=http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message \
MATRIX_SK=sk_... \
go run ./cmd/matrix            # mode=auto picks proxy when URL+SK are set

# Streamable HTTP on :8080
go run ./cmd/matrix --http :8080 --mode mock

# Local deploy: every deployment publishes a fresh random-id site under ./data
# (--domain makes website_url an absolute http://<site-id>.<domain>/ URL)
go run ./cmd/matrix --mode mock --data-dir ./data --workspace-dir /workspace --domain localhost
```

## Site server (deploy-server)

Serves what `deploy` wrote into `--data-dir`: each published site
(a random id, like the real server's per-deployment subdomain) becomes a
second-level domain.

```sh
# Serve ./data at http://<project>.localhost/ (apex lists all sites)
go run ./cmd/deploy-server --data-dir ./data --domain localhost --http :8080

# Or let the environment pick the port (all servers honor $PORT):
PORT=8080 go run ./cmd/deploy-server --data-dir ./data
PORT=8080 go run ./cmd/matrix --mode mock   # serves HTTP; no $PORT = stdio

# Rewrite every served index.html with a snippet (inject-proxy's HTML
# injection, extracted into the reusable rewrite package):
go run ./cmd/deploy-server --data-dir ./data --inject-html '<script src="/probe.js"></script>'
go run ./cmd/deploy-server --data-dir ./data --inject ./inject-ball.html   # file form wins
```

- `http://<site-id>.<domain>/` serves `data-dir/<site-id>/` (static files)
- the bare apex (`http://localhost/`) lists all published sites
- unknown sites, wrong domains and deeper subdomains are 404s
- with `--inject`/`--inject-html`, requests that resolve to a directory's
  `index.html` are served with the snippet injected before `</body>`
  (idempotent; explicit `/index.html` URLs and other files behave exactly
  like plain static serving)
- run it alongside `cmd/matrix --data-dir <same-dir>` for the full
deploy → serve loop (`*.localhost` resolves to loopback in modern
browsers; for other domains point the wildcard DNS record at this host)


## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--mode` | `auto` | `auto` \| `proxy` \| `mock` |
| `--url` | `$MATRIX_URL` | Real matrix MCP HTTP endpoint |
| `--token` | `$MATRIX_SK` | `?sk=` token |
| `--source` | `$MATRIX_SOURCE` or `hermes` | `?source=` label (server whitelists `openclaw`, `hermes`) |
| `--http` | empty | Address for streamable HTTP (e.g. `:8080`); defaults to `$PORT`, empty = stdio |
| `--timeout` | `5m` | Upstream request timeout (proxy mode) |
| `--data-dir` | `$MATRIX_DATA_DIR` | When set, `deploy` copies the dist directory into `data-dir/<random-id>` locally (a fresh site per deployment, mirroring the real server) instead of forwarding/mocking; `deploy-server` serves it |
| `--workspace-dir` | `$MATRIX_WORKSPACE` or `/workspace` | Sandbox root; `deploy` rejects a `dist_dir` outside it, like the real server |
| `--domain` | `$MATRIX_DOMAIN` | Apex domain for deploy `website_url` subdomains; empty = relative `/data/<site-id>/` URL |

## Verify

```sh
go test ./... -v
```

Tests spin up the server over stdio with a real go-sdk client and check:

- `tools/list` returns 22 tools; every input schema is a JSON object
- `tools/call` on `batch_web_search` / `get_voice_list` returns JSON output
- invalid input (missing `dist_dir`) yields a tool error, not a panic
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
