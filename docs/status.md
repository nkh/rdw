# rdw — Implementation Status and Plan

## Implemented

### Packages

| Package | Description | Coverage |
| --- | --- | --- |
| `internal/session` | TargetID validation; ScrollbackBuffer; Manager (windows, panes, layout) | 98.6% |
| `internal/kvstore` | Session-scoped KV store; namespace prefixes; size limits; thread-safe | 98% |
| `internal/control` | Inline control sequence parser; all 8 prefixes; KVPairs; verbatim | 100% |
| `internal/pipeline` | Line relay; filter chain; timestamp; base64 decode; KV dispatch; Scrollback() | 89.2% |
| `internal/auth` | SHA-256 hashed tokens; expiry; per-pane scope; revocation | 90.6% |
| `internal/config` | YAML loader; validation; defaults; Bindings field | 75.8% |
| `internal/layout` | YAML schema; Parse; LoadFile; ParseResizeArg | 100% |
| `internal/bindings` | 32 named actions; vim defaults; Merge/Validate/Lookup/JSON | 100% |
| `internal/discovery` | Server registry; Register/Deregister/PruneStale; Resolve | 58% |
| `internal/router` | Target ID to pipeline map; Route/Stream; AllowUnassigned | 85.5% |
| `internal/pipe` | Client-side stdin relay; Unix socket or HTTP; reconnect queue | 67.3% |
| `internal/export` | Markdown bundle; ANSI strip; image extraction to assets/ | 78% |
| `internal/server` | HTTP/WebSocket server; hub; middleware; full REST API; Unix socket; browser UI | 70.2% |
| `internal/selftest` | In-process smoke suite: 11 checks | 75.2% |

270 tests · 68.5% overall statement coverage

### CLI — all commands functional

- `rdw server start/stop/list`
- `rdw pipe --id --layout --window`
- `rdw window create/close/rename/focus/list`
- `rdw pane split/resize/zoom/swap/close`
- `rdw layout apply/save/list`
- `rdw kv set/get/delete`
- `rdw token create/revoke/list`
- `rdw group hide/show/focus/kill`
- `rdw save pane/window/all`
- `rdw selftest` (11 checks, exits 0)
- `rdw completion bash`

### REST API — complete

All `/api/v1/` endpoints implemented and tested:

| Endpoint | Description |
| --- | --- |
| `GET /api/v1/ping` | Health probe (unauthenticated, rate-limited) |
| `GET /api/v1/ws` | WebSocket upgrade, sub-protocol `rdw-v1` |
| `GET /api/v1/session` | Session snapshot JSON |
| `GET /api/v1/bindings` | Keyboard bindings map (unauthenticated) |
| `POST /api/v1/stream/{id}` | Ingest one line for a target ID |
| `GET/POST/DELETE/PATCH /api/v1/windows/*` | Window CRUD + focus |
| `POST /api/v1/panes/{id}/split\|zoom\|resize\|swap` | Pane operations |
| `DELETE /api/v1/panes/{id}` | Close pane |
| `GET/POST /api/v1/layouts`, `POST /api/v1/layouts/{name}/apply` | Layout presets |
| `GET/PUT/DELETE /api/v1/kv/{key}` | KV store |
| `GET/POST/DELETE /api/v1/tokens/{id}` | Token management |
| `POST /api/v1/groups/{name}/hide\|show\|focus\|kill` | Group operations |
| `POST /api/v1/export/pane\|window\|all` | Markdown export |
| `GET /api/v1/admin/connections` | Admin (loopback only) |

### Browser UI — complete (Phase 4)

- Dark theme with CSS variables, monospace font stack
- Window header bar with named tabs, click-to-switch, active highlight
- Pane grid built from CSS grid; rebuilt on every `layout_update` message
- Gutter drag-to-resize (mouse) with live grid fraction update
- Per-pane header with zoom and close buttons
- ANSI 16/256/true-colour parser → inline CSS spans
- Scrollback per pane, DOM ring buffer capped at 10,000 lines, auto-scroll
- WebSocket client: `rdw-v1` sub-protocol, auto-reconnect, RECONNECT queue flush
- Modal state machine: normal | swap | search
- 32 keyboard actions dispatched via key table built from `/api/v1/bindings`
  - Two-key sequence support (e.g. `g t`, `g g`)
- Interactive layout editing: split/close/zoom/rename/swap via keyboard + mouse
- Search overlay: `/` opens, `n`/`N` navigate, scoped to pane or session
- Right-click context menu
- Session and bindings loaded on startup from REST API

### Export (Phase 6 partial)

- `internal/export` package: Markdown bundle with `assets/` directory
- ANSI stripping, base64 image extraction with magic-byte type detection
- Export endpoints in REST API and CLI wired

---

## Remaining work

### Phase 5 — Formatters

The pipeline's `f:` control sequence and `rdw json/yaml/markdown/csv` commands
exist as CLI stubs. The rendering engines are not yet built.

- `json` — interactive collapsible tree with syntax highlighting
- `yaml` — interactive collapsible tree
- `markdown` — compile and render HTML in pane
- `csv`/`tsv` — sortable interactive grid
- `image` — decode `b64:`-encoded PNG/JPG/SVG inline in pane

### Phase 6 — Persistence

- KV persistence: wire `--kv-persist` flag to a SQLite write path
  (config field exists, `--kv-persist` CLI flag exists, no write path yet)
- Session restore: `--restore` reloads saved layout and KV state on startup

### Phase 7 — Remaining polish

- Pane rename: `PATCH /api/v1/panes/{id}` endpoint
- Scrollback bookmarks (drop marker, navigate via bindings)
- Regex highlight profiles per pane
- bash-rd `--forward rd|both` relay in `rdw pipe`
- Stream mirroring: `--forward-to-file`, `--forward-to-cmd` in pipe relay
- Focus cycle automation (wall-screen rotation)

### Phase 8 — Delivery

- Man page generation (`man/rdw.1`)
- `rdw server start --open-browser` cross-platform (xdg-open / open / start)
- Replace deprecated `strings.Title` with `golang.org/x/text/cases`
- Goreleaser config for binary releases
