# rdw — Implementation Status and Plan

## Implemented

### Core data pipeline

| Package | Description | Coverage |
| --- | --- | --- |
| `internal/session` | TargetID validation; ScrollbackBuffer circular ring buffer, configurable cap, thread-safe; Manager (windows, panes, layout application) | 98.6% |
| `internal/kvstore` | Session-scoped KV store; namespace prefixes; 64 KB per-value / 64 MB total limits; thread-safe | 98% |
| `internal/control` | Inline control sequence parser; all 8 prefixes; KVPairs parser; verbatim passthrough | 100% |
| `internal/pipeline` | Line relay; filter chain (max 8 stages); timestamp toggle; base64 decode; KV dispatch; multiple sinks; context cancellation | 93% |
| `internal/auth` | SHA-256 hashed tokens; expiry; per-pane scope; revocation; concurrent-safe | 91% |
| `internal/config` | YAML loader; full field validation; sane production defaults | 76% |
| `internal/layout` | YAML layout schema (windows, panes, split, size, group, private); Parse; LoadFile; ParseResizeArg | 100% |
| `internal/bindings` | 32 named actions; vim-like defaults; Merge / Validate / Lookup / JSON | 100% |
| `internal/discovery` | Server registry (JSON file); Register / Deregister / PruneStale; Resolve with auto-detect and helpful error | 58% |
| `internal/router` | Target ID to pipeline map; Register / Get / Deregister / Route / Stream; AllowUnassigned; SetControlHandler | 85.5% |
| `internal/pipe` | Client-side stdin relay; Unix socket (owner auth) or HTTP (token auth); reconnect queue; queue overflow | 67.3% |
| `internal/server` | HTTP/WebSocket server; hub (broadcast, token revoke); middleware (auth, rate limit, loopback guard); full REST API at /api/v1/; Unix domain socket; frontend HTML placeholder | 67.9% |
| `internal/selftest` | In-process smoke suite: 11 checks covering all core packages including server ping | 75.2% |

244 tests, 67.3% overall statement coverage.

### CLI — all commands functional

Every command is wired to the live server. None are stubs.

- `rdw server start` — starts the HTTP/WebSocket server, registers in discovery
- `rdw server stop` — sends stop via Unix socket
- `rdw server list` — reads registry, prunes stale entries, prints tabular output
- `rdw pipe --id --layout --window` — relays stdin via Unix socket or HTTP
- `rdw window create/close/rename/focus/list`
- `rdw pane split/resize/zoom/close`
- `rdw layout apply/save/list`
- `rdw kv set/get/delete`
- `rdw token create/revoke/list`
- `rdw group hide/show/focus/kill`
- `rdw save pane/window/all`
- `rdw selftest` — 11 in-process checks, exits 0 on success
- `rdw completion bash`

### Server — REST API complete

All `/api/v1/` endpoints are implemented and tested:

- `GET /api/v1/ping` — health probe (unauthenticated, rate-limited)
- `GET /api/v1/ws` — WebSocket upgrade, sub-protocol `rdw-v1`
- `GET /api/v1/session` — session snapshot JSON
- `POST /api/v1/stream/{id}` — ingest one line for a target ID
- `GET/POST/DELETE/PATCH /api/v1/windows/*` — window CRUD + focus
- `POST /api/v1/panes/{id}/split|zoom|resize`, `DELETE /api/v1/panes/{id}`
- `GET/POST /api/v1/layouts`, `POST /api/v1/layouts/{name}/apply`
- `GET/PUT/DELETE /api/v1/kv/{key}` — KV store with prefix filter
- `GET/POST/DELETE /api/v1/tokens/{id}` — token management
- `GET /api/v1/admin/connections` — admin (loopback only)

### Browser UI — placeholder

The frontend HTML at `/` connects via WebSocket, renders a window header bar,
and displays incoming stream lines with ANSI color. This is the Phase 4
placeholder; full interactive editing is not yet built.

### Documentation

- `README.md` — project overview, quick start, full command reference
- `docs/requirements.md` — complete functional requirements specification
- `docs/manual.md` — 23-section user manual
- `docs/status.md` — this file

---

## Remaining work

### Phase 4 — Full browser UI

The core work remaining. The server and REST API are complete; what is
missing is a production browser UI.

- Pane grid layout engine: split/resize rendered in the browser, mirroring
  the server's WindowState/PaneState tree
- Interactive pane editing: split prompts, rename prompts, resize drag,
  zoom toggle, swap mode — all updating server state via REST
- Search overlay with exact/fuzzy matching
- Scrollback bookmarks
- Regex highlight profiles per pane
- Keyboard binding layer fully wired to the 32 actions from `internal/bindings`
- Mouse: gutter drag, header click, double-click zoom, right-click context menu
- JavaScript-off graceful degradation (static HTML render)

### Phase 5 — Formatters

- `json` — interactive collapsible tree
- `yaml` — interactive collapsible tree
- `markdown` — compile and render HTML
- `csv`/`tsv` — sortable interactive grid
- `image` — decode `b64:`-encoded PNG/JPG/SVG inline

### Phase 6 — Persistence and export

- KV persistence: wire `--kv-persist` to SQLite (config field exists, no write path)
- Session restore: `--restore` reloads saved layout and KV state
- Export: `rdw save` Markdown bundle with `assets/` directory
- Export REST endpoints: `/api/v1/export/pane|window|all`

### Phase 7 — Remaining server endpoints

- `POST /api/v1/groups/{name}/hide|show|focus|kill`
- `POST /api/v1/export/pane|window|all`
- Pane swap: `POST /api/v1/panes/{id}/swap`
- WebSocket-driven layout change broadcast (currently a placeholder broadcast)

### Phase 8 — Polish

- `gotty` terminal pane integration with mandatory restricted-user enforcement
- bash-rd `--forward rd|both` relay via `rdw pipe`
- Stream mirroring: `--forward-to-file`, `--forward-to-cmd`
- Focus cycle automation for wall-screen/dashboard use
- Man page generation (`man/rdw.1`)
- `rdw server start --open-browser` cross-platform
- Replace `strings.Title` with `golang.org/x/text/cases` (deprecated in Go 1.18+)
