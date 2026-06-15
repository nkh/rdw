# rdw development worklog

## Session 1 — May 29

**Requirements analysis and language selection**

Analysed the original bash-rd-based requirements gist. Identified vocabulary inconsistencies (Target ID character set, Filter vs Formatter conflation, undefined KV scope), structural problems (broken command table, missing protocol versioning, informal security language), and ambiguities (backward-compat scope, zero-copy as functional requirement, binary encoding contract).

Produced 30 improvement suggestions. Rewrote the requirements document incorporating suggestions 2–9, 11, 13–15, 17–18, 20, 23–26, 28, 30. Key additions: WebSocket sub-protocol `rdw-v1`, REST base path `/api/v1/`, default port 7681, Unix socket CLI auth, SHA-256 token hashing, gotty restricted-user sandboxing, KV scoping rules, SQLite persistence flag, base64 binary encoding contract, `v:` verbatim escape, `rdw selftest` CI smoke command.

Evaluated Go vs Rust. Selected Go: goroutine-per-pane stream model, single-binary embedding, mature standard library HTTP/WebSocket, fast iteration. Rust reserved for future if memory constraints emerge.

## Session 2 — May 29 (continued)

**Project scaffold and dependency proxy**

`proxy.golang.org` unavailable in the build environment. Built a local Go module proxy using `codeload.github.com` and `raw.githubusercontent.com` to repackage dependencies as valid Go module zip format. Dependencies proxied: cobra, pflag, gorilla/websocket, yaml.v3, testify, go-spew, go-difflib, mousetrap, cpuguy83/go-md2man, russross/blackfriday, golang.org/x/net.

Established directory layout. Implemented and tested the first six packages with 100% or near-100% coverage:

- `internal/session` — TargetID validation (`[a-zA-Z0-9_][a-zA-Z0-9_ -]*`, max 64), ScrollbackBuffer (circular ring, thread-safe, configurable cap)
- `internal/kvstore` — session-scoped KV store, colon-namespaced keys, value/total size limits
- `internal/control` — inline control sequence parser (`kv:`, `f:`, `b64:`, `ts:`, `v:`, `hl:`, `bm:`, `sc:`), KVPairs decoder, verbatim passthrough
- `internal/pipeline` — line relay, filter chain (max 8 stages), timestamp toggle, base64 decode, KV dispatch, multiple sinks, context cancellation
- `internal/auth` — SHA-256 hashed tokens, expiry, per-pane scope, revocation, concurrent-safe
- `internal/config` — YAML loader over defaults, full validation

Wrote README.md and docs/requirements.md. Pushed 10 commits to https://github.com/nkh/rdw.

## Session 3 — May 31

**Infrastructure packages**

- `internal/layout` — YAML layout schema (WindowSpec, PaneSpec, schema_version gate), Parse, LoadFile, ParseResizeArg. Validates split direction and size.
- `internal/bindings` — 32 named actions, vim-like default binding set (zero key conflicts), Merge/Validate/Lookup/JSON. Key conflict `r` between layout.reload and pane.rename found and resolved in testing.
- `internal/selftest` — in-process smoke suite, `rdw selftest` command
- `internal/discovery` — multi-server registry at `$XDG_CACHE_HOME/rdw/servers.json`, Register/Deregister/PruneStale, Resolve with live HTTP probe

CLI scaffold: all commands defined with correct flags via cobra. Full cobra tree: `server`, `pipe`, `kv`, `token`, `window`, `pane`, `layout`, `group`, `save`, `selftest`, `completion`.

wrote docs/status.md with phase plan.

## Session 4 — Jun 1

**Multi-server, window model, bindings, layout language**

New requirements incorporated:

- `--port/-p` persistent global flag on all commands
- Server discovery: auto-detect running instance, fall back to default port 7681, error if ambiguous with list
- Windows are server-managed views within one browser page (not browser tabs). Window header bar added.
- `rdw pipe --layout` and `--window` flags: named layout applied on pipe start
- 32 keyboard actions with vim-like defaults, configurable via YAML, validated at startup, served as JSON to browser
- Layout description language documented in docs/manual.md

Wrote docs/manual.md (23 sections): architecture, layout language field reference with 6 annotated examples, interactive editing guide, complete 32-action binding table, control sequence reference, KV namespacing, security model, troubleshooting, annotated config.yaml.

## Session 5 — Jun 2–3

**Phase 1: HTTP/WebSocket server**

- `internal/server/hub.go` — WebSocket hub, per-connection send channel, token revocation closes live connections, RECONNECT marker on reconnect
- `internal/server/ws.go` — gorilla/websocket write/read pumps, ping/pong keepalive, `rdw-v1` sub-protocol
- `internal/server/unix.go` — Unix domain socket at `$XDG_RUNTIME_DIR/rdw/<id>.sock`, 0600 permissions, owner auth
- `internal/server/middleware.go` — token auth middleware, rate limiter (10 req/min unauthenticated), loopback guard for admin endpoints
- `internal/server/server.go` — HTTP server lifecycle, graceful shutdown, signal handling, discovery register/deregister

**Phase 2: Router and Session Manager**

- `internal/router` — TargetID-to-pipeline map, Register/Get/Deregister/Route/Stream, AllowUnassigned mode, broadcast sink
- `internal/session.Manager` — ordered window list, active window, AddPane (max 64), RemovePane, FindPane, Snapshot (empty array not null)

**Phase 3: REST API**

Full `/api/v1/` surface (all endpoints authenticated, rate-limited on unauth routes, loopback-only admin):

- `GET /api/v1/ping`, `GET /api/v1/session`
- `POST /api/v1/stream/:id` — HTTP stream ingest
- Windows: create, close, rename, focus, list
- Panes: split, zoom, resize, close, swap
- Layout: apply, save, list
- KV: set, get, delete, list with prefix filter
- Tokens: create, revoke, list
- Admin: connections

**Phase 3: pipe relay**

`internal/pipe` — client-side relay: prefers Unix socket (owner auth), falls back to HTTP Bearer token. Reconnect queue buffers up to 1000 lines, flushes in order on RECONNECT, drops oldest on overflow.

All CLI commands wired to live server via REST or Unix socket. `rdw selftest` extended to 11 in-process checks covering server ping.

244 tests, 67.3% coverage.

## Session 6 — Jun 5–7

**Phase 4: Browser SPA** (`internal/server/frontend.go`, ~1200 lines)

Complete single-page application embedded in the binary:

- CSS dark theme with CSS variables; pane grid via CSS `grid-template-columns/rows`, rebuilt on every `layout_update` WebSocket message
- Gutter drag-to-resize: `mousedown` on `.gutter-v/.gutter-h`, tracks delta as percentage, updates grid live
- ANSI colour parser: full 16/256/true-colour; escape sequences → inline CSS `color:`/`background:` spans
- Scrollback: per-pane DOM ring buffer capped at 10,000 lines, auto-scroll when within 40px of bottom
- WebSocket client: `rdw-v1` sub-protocol, auto-reconnect every 2s, client-side reconnect queue flushed on RECONNECT marker
- 32-action keyboard dispatch: reads `/api/v1/bindings` on startup, single-key and two-key sequences (`g t`, `g g`)
- Mode state machine: normal / swap / search
- Search overlay: `/` opens, regex on pane text, `n`/`N` navigate, `scrollIntoView`
- Right-click context menu: zoom, split, rename, clear, close
- Window header bar: rendered list, active highlighted, click to switch

Additional endpoints and packages:

- `GET /api/v1/bindings` — serves binding map as JSON
- `internal/export` — `Bundle()` writes Markdown + `assets/` with decoded images; `POST /api/v1/export/pane|window|all`
- Group/swap endpoints: `POST /api/v1/groups/{name}/kill|hide|show|focus`, `POST /api/v1/panes/{id}/swap`
- `Pipeline.Scrollback()` accessor for export path

270 tests, 68.5% coverage, 11/11 selftest.

## Session 7 — Jun 8–10

**Phase 5: Formatters** (`internal/format`)

Six formatters, each implementing `Formatter` interface (`Format([]string) (string, error)`):

- `text` — HTML-escaped `<pre>`, ANSI handled client-side
- `json` — per-line parse + pretty-print + token-level CSS classes (key, string, number, literal, punct)
- `yaml` — multi-document split on `---`, canonical re-marshal, key/value CSS classes
- `markdown` — single-pass renderer: headings, bold, italic, inline code, code fences, lists, blockquotes, links
- `csv` — auto-detect CSV/TSV delimiter, header row as `<th>`, data rows as `<td>`
- `image` — base64 decode (standard + URL-safe), magic-byte MIME detection (PNG/JPEG/GIF/SVG/WebP), inline `<img>`

Formatter endpoints: `GET /api/v1/formatters`, `POST /api/v1/panes/{id}/format`.

**Phase 6: KV SQLite persistence** (`internal/kvstore/persist.go`)

- `OpenDB(path)` — opens or creates SQLite DB with WAL journal
- `Load(store)` — reads all rows into in-memory store on `--restore`
- `Persist(k, v)` — upsert on every `PUT /api/v1/kv/{key}`
- `Remove(k)` — delete on every `DELETE /api/v1/kv/{key}`
- `--kv-persist <path>` and `--restore` wired into `server.Options`
- `kvDB` field on `Server`, opened at `Run` start, closed on shutdown

**Phase 7: Stream mirroring** (`internal/mirror`)

- `FileSync(path)` — append-open sink
- `CmdSync(cmdStr)` — `sh -c` subprocess, stdin pipe as sink
- `Tee(r, sink)` — goroutine copies every read to sink without buffering the main path
- `--forward-to-file` and `--forward-to-cmd` wired into `runPipe`

**Open-browser** (`internal/browser`)

- `Open(url)` — platform dispatch: `xdg-open` (Linux), `open` (macOS), `rundll32` (Windows)
- `--open-browser` flag wired in `runServerStart`

331 tests, 70.1% coverage, 11/11 selftest.

## Session 8 — Jun 11 (restored after reset)

**Scrollback bookmarks** (`internal/session/bookmark.go`)

- `BookmarkStore` — named bookmarks keyed by string, stores `{name, line_index, created_at}`
- `Add`, `Remove`, `Get`, `All` (sorted by `line_index`), `Len`
- Integrated into `Router`: one `BookmarkStore` per registered pipeline, cleaned up on deregister
- API: `GET /api/v1/panes/{id}/bookmarks`, `PUT /api/v1/panes/{id}/bookmarks/{name}`, `DELETE /api/v1/panes/{id}/bookmarks/{name}`

**Regex highlight profiles** (`internal/highlight`)

- `Store` — named `Profile` set, each profile is an ordered list of `{pattern, class}` rules
- `Add` validates every regex at store time; rejects empty name or class
- `Remove`, `Get`, `All`, `Len`
- Held in `Server.highlights`
- API: `GET /api/v1/highlights`, `PUT /api/v1/highlights/{name}`, `DELETE /api/v1/highlights/{name}`

**Pane rename**

- `Label` field added to `session.PaneState`
- `PATCH /api/v1/panes/{id}` sets `pane.Label` and broadcasts `layout_update`

317 tests, 67.2% coverage, 11/11 selftest.

## Session 9 — Jun 15 (restoration complete)

**gotty terminal pane** (`internal/terminal`)

- `Manager` tracks active subprocesses and allocates ports from a base offset
- `Launch(id, cmd)` runs `ttyd --once su -s /bin/sh nobody -c cmd`; falls back to socat for environments without ttyd; fails loudly if neither is available
- `Kill(id)` stops the process and frees the slot
- `POST /api/v1/panes/{id}/terminal` — launches and returns `{id, port, url}`
- `DELETE /api/v1/panes/{id}/terminal` — kills the subprocess

**Focus cycle automation** (`internal/cycle`)

- `Cycle` holds an ordered window list and a dwell interval
- `Run(ctx)` sends `Event{Window}` on a channel, advances index on each tick, closes channel on cancel
- Wired into server as `POST /api/v1/cycle/start` and `POST /api/v1/cycle/stop`
- Start accepts `{windows: [...], interval_ms: N}`; cancels any existing cycle before starting

**bash-rd `--forward` compatibility**

- `--forward rd|rdw|both` flag now wired in `runPipe`
- `rd` and `both` modes tee stdin through `mirror.CmdSync("rd")` before relaying to rdw

**Man page** (`man/rdw.1`)

- Full groff man page covering all commands, options, control sequences, auth model, environment variables, and file paths

**Goreleaser** (`.goreleaser.yaml`)

- Linux + macOS, amd64 + arm64
- Archives include README, docs/, man/rdw.1
- Homebrew tap stub for `nkh/homebrew-tap`

**CSV table sort** (frontend.go)

- Event-delegated click handler on `rdw-csv-table` `<th>` elements
- Numeric vs lexicographic detection via `parseFloat`
- Toggle ascending/descending on repeated click; `sort-asc`/`sort-desc` CSS class markers

327 tests, 11/11 selftest, all items from "Not yet implemented" resolved.
