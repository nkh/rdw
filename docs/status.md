# rdw implementation status

## Test summary

- 317 tests passing across 16 tested packages
- 67.2% total statement coverage
- 11/11 selftest checks pass (`rdw selftest`)

## Package coverage

| Package | Coverage | Notes |
|---------|----------|-------|
| internal/session | 98.3% | TargetID, ScrollbackBuffer, Manager, BookmarkStore |
| internal/kvstore | 95%+ | Store, SQLite persistence |
| internal/control | 100% | Inline control sequence parser |
| internal/bindings | 100% | 32 actions, vim-like defaults |
| internal/auth | 90.6% | SHA-256 tokens, expiry, revocation |
| internal/layout | 100% | YAML schema, Parse, LoadFile, ParseResizeArg |
| internal/pipeline | 93% | Line relay, filter chain, sinks |
| internal/router | — | TargetID→pipeline map, bookmark stores |
| internal/pipe | — | Client relay, reconnect queue |
| internal/mirror | — | FileSync, CmdSync, Tee |
| internal/format | — | text/json/yaml/markdown/csv/image |
| internal/highlight | — | Regex profile store |
| internal/export | — | Markdown+assets bundle |
| internal/discovery | — | Multi-server registry |
| internal/selftest | 80% | 11 in-process smoke checks |
| internal/server | 60.9% | HTTP/WS server, full REST API, browser SPA |

## Implemented phases

### Phase 1 — HTTP/WebSocket server
- WebSocket hub with RECONNECT marker and reconnect queue
- `rdw-v1` sub-protocol
- Ping/pong keepalive
- Unix domain socket at `$XDG_RUNTIME_DIR/rdw/<id>.sock`
- Discovery register/deregister on start/stop

### Phase 2 — Session and router
- TargetID→pipeline routing, AllowUnassigned mode
- Session manager: windows, panes (max 64), active window
- Layout apply wired to session

### Phase 3 — REST API `/api/v1/`
- Auth middleware, rate limiter (10 req/min unauth), loopback admin guard
- All window, pane, layout, KV, token, admin, stream endpoints
- Pipe relay (Unix socket preferred, HTTP fallback)

### Phase 4 — Browser SPA
- CSS grid pane layout, dark theme, CSS variables
- Gutter drag-to-resize
- Full ANSI 16/256/true-colour parser
- 10,000-line per-pane DOM scrollback
- 32-action keyboard dispatch with two-key sequences
- Normal/swap/search mode state machine
- Search overlay with regex and n/N navigation
- Right-click context menu
- Window header bar with click-to-focus

### Phase 5 — Formatters
- text, json, yaml, markdown, csv, image
- `GET /api/v1/formatters`, `POST /api/v1/panes/{id}/format`

### Phase 6 — KV persistence
- SQLite via mattn/go-sqlite3, WAL journal
- `--kv-persist <path>`, `--restore`
- Persist on every set/delete, load on restore

### Phase 7 — Stream mirroring and open-browser
- `--forward-to-file` (append sink)
- `--forward-to-cmd` (sh -c subprocess stdin)
- `--open-browser` (xdg-open / open / rundll32)

### Polish
- Scrollback bookmarks (per-pane, named, sorted by line index)
- Regex highlight profiles (validated at store time, served to browser)
- Pane label/rename via `PATCH /api/v1/panes/{id}`
- `rdw selftest` 11-check in-process suite

## Not yet implemented

- gotty terminal pane integration (restricted-user sandbox)
- bash-rd `--forward rd/both` relay compatibility
- Focus cycle automation (wall-screen rotation)
- Man pages (`man/rdw.1`)
- Goreleaser cross-platform release config
- CSV table sort (client-side JS, server sends HTML structure)
