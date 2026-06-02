# rdw — Implementation Status and Plan

## Implemented

### Core data pipeline

| Package | Description | Coverage |
| --- | --- | --- |
| `internal/session` | TargetID validation; ScrollbackBuffer circular ring buffer, configurable cap, thread-safe | 100% |
| `internal/kvstore` | Session-scoped KV store; namespace prefixes; 64 KB per-value / 64 MB total limits; thread-safe | 98% |
| `internal/control` | Inline control sequence parser; all 8 prefixes; KVPairs parser; verbatim passthrough | 100% |
| `internal/pipeline` | Line relay; filter chain (max 8 stages); timestamp toggle; base64 decode; KV dispatch; multiple sinks; context cancellation | 93% |
| `internal/auth` | SHA-256 hashed tokens; expiry; per-pane scope; revocation; concurrent-safe | 91% |
| `internal/config` | YAML loader; full field validation; sane production defaults | 76% |
| `internal/layout` | YAML layout schema (windows, panes, split, size, group, private); Parse; LoadFile; ParseResizeArg | 100% |
| `internal/bindings` | 32 named actions; vim-like defaults; Merge / Validate / Lookup / JSON | 100% |
| `internal/discovery` | Server registry (JSON file); Register / Deregister / PruneStale; Resolve with auto-detect and helpful error | 58% |
| `internal/selftest` | In-process smoke suite covering all core packages | 80% |

134 tests, 76.1% overall statement coverage.

### CLI scaffold

All commands defined with correct flags via cobra. Every command is stubbed
and prints "not yet implemented". The flag surface matches the requirements
exactly:

- `rdw server start / stop / list`
- `rdw pipe --id --layout --window --forward --allow-unassigned --forward-to-file --forward-to-cmd`
- `rdw window create / close / rename / focus / list`
- `rdw pane split / resize / zoom / swap / close`
- `rdw layout apply / save / list`
- `rdw kv set / get / delete`
- `rdw token create / revoke` (scaffold in requirements; not yet in cmd)
- `rdw group hide / show / focus / kill` (scaffold in requirements; not yet in cmd)
- `rdw save pane / window / all` (scaffold in requirements; not yet in cmd)
- `rdw selftest` — functional
- `rdw completion bash` — functional
- Global `--port` / `--config` on every command

### Documentation

- `README.md` — project overview, quick start, full command reference
- `docs/requirements.md` — complete functional requirements specification
- `docs/manual.md` — 23-section user manual

### Not started

Everything that runs: the HTTP server, WebSocket layer, REST API, browser UI,
formatters, persistence, and export. The CLI commands are all stubs.

---

## Plan

### Phase 1 — HTTP server and WebSocket transport

The foundation. Nothing else can be tested end-to-end until this exists.

- `internal/server` package
  - HTTP server lifecycle: start, stop, graceful shutdown, OS signal handling
  - `/api/v1/ping` — enables `discovery.Resolve` to work
  - WebSocket hub: connection registry, broadcast to all clients, sub-protocol `rdw-v1`
  - Per-connection token validation middleware
  - Unix socket listener for owner-privilege CLI authentication
  - Wire `internal/pipeline` sinks into WebSocket broadcast
  - Wire `internal/discovery.Register` / `Deregister` to server start/stop
- `rdw server start` — functional (no longer stubbed)
- `rdw server stop` — functional
- Tests: server lifecycle, ping endpoint, WebSocket connect/disconnect/broadcast,
  Unix socket auth, token middleware

### Phase 2 — Session and router

Live session state management.

- `internal/router` — Target ID to pipeline map; create / destroy pipelines;
  unassigned-target error vs allow-unassigned creation
- `internal/session.Manager` — owns windows, panes, router; enforces pane limits (max 64)
- Wire `rdw pipe` to an actual TCP/HTTP stream relay to the server
- Wire layout loading to session creation: `rdw layout apply` and `rdw pipe --layout` become functional
- Tests: routing, unassigned error, layout activation, pane limit enforcement

### Phase 3 — REST API

Full command parity at `/api/v1/`.

- Middleware: token auth, rate limiting (10 req/min unauthenticated), loopback guard for admin
- Window endpoints: create, close, rename, focus, list
- Pane endpoints: split, resize, zoom, swap, close
- Layout endpoints: apply, save, list
- KV endpoints: set, get, delete, list
- Token endpoints: create, revoke
- Stream ingest: POST `/api/v1/stream/:id` as HTTP alternative to piping
- Tests: auth middleware, each endpoint happy path and error paths, rate limiter,
  token scope enforcement

### Phase 4 — Browser UI

The visible product. All assets compiled into the binary.

- Window header bar: renders window list, highlights active window, click to switch
- Pane grid: split/resize layout engine
- WebSocket client: connect, sub-protocol negotiation, reconnect with queue flush on `RECONNECT` marker
- Scrollback renderer: ANSI 24-bit color passthrough, line-by-line streaming
- Keyboard binding layer: reads `bindings.JSON()` from server, dispatches all 32 actions
- Interactive layout editing: split prompts, rename prompts, resize drag, swap mode
- Search overlay: `/` opens, `n`/`N` navigate matches, scoped to pane / window / session
- Mouse support: gutter drag resize, header click, double-click zoom, right-click context menu

### Phase 5 — Formatters

Specialised pane renderers.

- `json` — interactive collapsible tree with syntax highlighting
- `yaml` — interactive collapsible tree
- `markdown` — compile and render HTML
- `csv` / `tsv` — sortable interactive grid
- `image` — decode `b64:`-encoded PNG / JPG / SVG and display inline

### Phase 6 — Persistence and export

- KV persistence: wire `--kv-persist` flag to a SQLite backend
  (config field exists; no write path implemented yet)
- Session restore: `--restore` on server start reloads saved layout and KV state
- Export: `rdw save` writes Markdown bundle with `assets/` subfolder

### Phase 7 — Remaining CLI wiring

Wire all currently-stubbed commands to the live server via the REST API:

- `rdw window *`, `rdw pane *`, `rdw layout *`, `rdw kv *`
- `rdw token create / revoke`
- `rdw group hide / show / focus / kill`
- `rdw save pane / window / all`

### Phase 8 — Polish

- `gotty` terminal pane integration with mandatory restricted-user enforcement
- bash-rd `--forward rd / both` relay
- Stream mirroring (`--forward-to-file`, `--forward-to-cmd`)
- Focus cycle automation for wall-screen / dashboard use
- Scrollback bookmarks
- Regex highlight profiles
- Man page generation (`man/rdw.1`)
- `rdw server start --open-browser` (platform detection: xdg-open / open / start)
