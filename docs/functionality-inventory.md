# rdw functionality inventory

Every feature in rdw, whether it is Unix-like, and what could replace it.

Unix-like rating:
- **YES** — follows Unix principles; composable, single-purpose, transparent
- **PARTIAL** — some Unix alignment but carries extra concerns
- **NO** — violates Unix principles; better done outside rdw

---

## Transport

| Feature | Description | Unix-like | Shell replacement |
| --- | --- | --- | --- |
| `rdw pipe --id ID` | Read stdin, relay lines to named pane | PARTIAL | Is essentially `netcat`/`socat` to a named socket; Unix but `--layout`, `--filter`, `--title` flags corrupt it |
| `--forward-to-file` | Mirror stream to a file/FIFO | NO | `tee logfile \| rdw pipe` |
| `--forward-to-cmd` | Mirror stream to a shell command | NO | `tee >(cmd) \| rdw pipe` |
| `--forward rd\|both` | Also send stream to bash-rd | NO | `tee >(rd -c ID) \| rdw pipe` |
| `--layout NAME` on pipe | Apply layout before streaming | NO | `rdw layout apply NAME && prog \| rdw pipe` |
| `--title TEXT` on pipe | Set pane title on connect | NO | `rdw pane rename ID TITLE && prog \| rdw pipe` |
| `--filter CMD` on pipe | Attach filter stage | NO | `prog \| CMD \| rdw pipe` |
| Reconnect queue (1000 lines) | Buffer lines when server unreachable | PARTIAL | Useful but belongs in a thin retry wrapper, not the relay |
| `rdw send FILE` | Send file to pane (auto-detect type) | PARTIAL | Convenience over `base64 FILE \| rdw pipe`; hides composition |
| Hybrid binary reader (`image:end`) | Sentinel-framed binary in text stream | NO | `base64 img.png \| rdw pipe`; sentinel framing is a homemade protocol |

---

## Server core

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| HTTP server (port 7681) | Serves REST API and SPA | YES | Core function; cannot be composed away |
| WebSocket hub (`rdw-v1`) | Pushes lines to browsers | YES | Core function |
| Unix socket auth | Owner-only local access | YES | Core Unix pattern |
| Multi-server discovery | Registry at `~/.cache/rdw/servers.json` | YES | Analogous to PID files |
| `--network-expose` | Bind to all interfaces | YES | Standard server flag |
| `GET /api/v1/ping` | Health check | YES | Standard |
| `GET /api/v1/ws` | WebSocket upgrade | YES | Core function |
| `POST /api/v1/stream/{id}` | HTTP line ingest | YES | HTTP alternative to Unix socket |

---

## Session and layout

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| Windows (named groups of panes) | Server-managed tmux-like views | YES | Core function; this is what makes rdw a browser tool not just `netcat` |
| `rdw window create/close/rename/focus/list` | Window management | YES | Core function |
| `rdw pane split/resize/zoom/swap/close` | Pane geometry | YES | Core function |
| `rdw pane rename ID TITLE` | Set pane display title | YES | Could be a KV key (`kv:pane.ID.title=TEXT`) but dedicated command is cleaner |
| Layout YAML files | Declarative window/pane arrangement | YES | Config-as-data; Unix-like |
| `rdw layout apply/save/list` | Layout management | YES | Core function |
| `rdw layout apply` on `rdw pipe` | Apply layout at pipe time | NO | `rdw layout apply NAME && prog \| rdw pipe` |
| Pane groups (hide/show/focus/kill) | Batch pane operations by group name | PARTIAL | Useful but could be a naming convention (`ID_group`) plus a shell loop |
| `rdw group hide/show/focus/kill` | Group actions | PARTIAL | Shell loop over `rdw pane close` for each group member |
| Gutter drag-to-resize | Browser UI resize | YES | Core browser function |
| Scrollback buffer (10,000 lines) | Stores all received lines per pane | NO | Violates formatter authority; should be a thin ring buffer for reconnect only |
| Session restore (`--restore`) | Reload KV and layout on server start | PARTIAL | Useful; the KV part is fine; layout restore is config-file territory |

---

## Key-value store

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| In-memory KV per session | `key=value` store shared across panes | YES | Core bash-rd concept; correct |
| `=:key=value` inline control sequence | Write KV from stream | YES | Core bash-rd concept |
| `rdw kv set/get/delete/list` | KV management CLI | YES | Thin wrapper over REST API |
| `GET/PUT/DELETE /api/v1/kv` | KV REST API | YES | Correct |
| KV injection into filters | Env vars in filter subprocess | YES | Correct; bash-rd faithful |
| KV injection into user formatters | Env vars in formatter subprocess | YES | Correct |
| KV in built-in formatters | Built-in formatters read KV keys | NO | Built-in formatters should not exist (see formatters section) |
| SQLite persistence (`--kv-persist`) | Persist KV across restarts | PARTIAL | Useful; analogous to a dot-file |
| KV namespace convention (`window:N:key`) | Informal namespacing by prefix | YES | Convention, not enforcement; correct |

---

## Filtering

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| `--filter CMD` on `rdw pipe` | Attach per-line shell command filter | NO | `prog \| CMD \| rdw pipe`; the pipe IS the filter chain |
| `POST /api/v1/panes/{id}/filters` | Register filter via API | NO | Same; filter belongs in the shell before `rdw pipe` |
| `CmdFilter` (long-lived subprocess) | Keeps filter process alive per pane | NO | If filter is in the shell, no server-side filter needed |
| Filter chain (max 8 stages) | Multiple filter stages per pipeline | NO | Unlimited stages in the shell: `prog \| f1 \| f2 \| f3 \| rdw pipe` |
| KV injection into filters | Pass KV snapshot as env to filter | PARTIAL | Correct idea but wrong layer; env injection should happen in the shell |

---

## Formatters

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| Built-in `text` formatter | ANSI passthrough | NO | Should not exist; the formatter is user code |
| Built-in `json` formatter | JSON syntax highlight | NO | `prog \| python3 -m json.tool \| rdw pipe` or user formatter |
| Built-in `yaml` formatter | YAML highlight | NO | User formatter |
| Built-in `markdown` formatter | Markdown to HTML | NO | User formatter (`pandoc`, `cmark`) |
| Built-in `csv` formatter | CSV to sortable table | NO | User formatter |
| Built-in `image` formatter | base64 → `<img>` | NO | User formatter |
| On-demand formatting (`POST /api/v1/panes/{id}/format`) | Apply formatter to scrollback | NO | If formatter runs per-line there is nothing to apply on demand |
| User-defined formatter (external command) | Shell script → HTML per invocation | YES | This is the correct model; the only formatter type that should exist |
| `rdw formatter register/unregister/list` | Manage user formatters | YES | Reasonable; analogous to a plugin registry |
| Formatter save/restore on `image:`/`svg:` | Server saves active formatter during binary block | NO | Side-effect hidden from user; user should set formatter explicitly |
| Formatter registry in config (`formatters:` block) | Pre-register formatters at server start | YES | Reasonable config-as-data |
| Sandboxed iframe for user formatter output | Output wrapped in `<iframe sandbox>` | YES | Correct security model |

---

## Control sequences

| Sequence | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| `v:` verbatim | Prevent interpretation | YES | Necessary escape hatch |
| `=:key=value` KV write | Write to session KV | YES | Core bash-rd concept |
| `f:NAME` formatter switch | Change active formatter | YES | Core; streamlined way to switch display mode |
| `b64:DATA` base64 passthrough | Binary in text stream | PARTIAL | Better than `image:` sentinel; `base64 \| rdw pipe` is cleaner |
| `bm:NAME` bookmark | Mark line in scrollback | NO | Relies on scrollback; remove with scrollback |
| `hl:PROFILE` highlight | Apply highlight profile | PARTIAL | Belongs in the formatter, not in a control sequence |
| `sc:clear\|top\|bottom` | Scrollback control | NO (clear) / YES (top/bottom) | clear: remove with scrollback; top/bottom: useful browser scroll |
| `title:NAME` | Set pane title | YES | Composable; producer can name its pane |
| `image:` / `image:end` sentinel | Binary image framing | NO | `base64 img \| rdw pipe`; fragile homemade protocol |
| `svg:` / `svg:end` sentinel | SVG framing | NO | SVG is text; just pipe it with `f:svg` set |
| `scale:MODE` | Image scaling hint | PARTIAL | Belongs in the formatter's HTML output, not a separate sequence |
| `t:` timestamp toggle | Prepend timestamp | NO | `prog \| ts \| rdw pipe` (`ts` from moreutils) |
| `c:` clear scrollback | Clear display | NO | Remove with scrollback; `sc:clear` covers the browser side |
| `q:` quit server | Stop server from stream | NO | `rdw server stop`; a stream should not control the server |

---

## Authentication and security

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| Bearer token auth | SHA-256 hashed tokens per session | PARTIAL | Over-engineered for local use; correct for network use |
| `rdw token create/revoke/list` | Token management | PARTIAL | Necessary for network use; not for local |
| Per-pane/window token scope | Tokens restricted to specific panes | PARTIAL | Fine-grained but complex |
| Unix socket 0600 (no token) | Owner-only local access | YES | Core Unix pattern; correct |
| Admin-local-only loopback guard | Restrict admin API to loopback | YES | Correct network hygiene |
| `--no-auth` flag | Disable token auth | YES | Correct development mode |
| Rate limiting (10 req/min unauth) | Protect unauthenticated endpoints | YES | Reasonable for network service |
| Admin token (`--admin-token`) | Separate token for `/admin` page | YES | Correct separation of concerns |

---

## Browser SPA

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| Embedded SPA in binary | HTML/CSS/JS compiled into Go binary | NO | Serve from a configurable directory; embedded is the default |
| ANSI colour parser (16/256/truecolour) | Decode escape sequences to CSS | YES | Core display function |
| Per-pane DOM scrollback (10k lines) | Ring buffer of recent lines in browser | PARTIAL | 10k is too large; 200–500 for reconnect is enough |
| Gutter drag-to-resize | Mouse resize of panes | YES | Core browser UI |
| 32-action keyboard dispatch | Vim-like pane/window navigation | YES | Core browser UI |
| Normal/swap/search mode | Modal keyboard input | YES | Core browser UI |
| Window header bar (click-to-focus) | Window tab strip | YES | Core browser UI |
| CSV column sort (client-side JS) | Click table header to sort | NO | Belongs in the formatter's HTML (e.g. `datatables.net`) |
| Highlight profile rendering | Apply regex CSS classes to lines | NO | Belongs in the formatter |
| `image_render` / `svg_render` WS messages | Server-pushed image display | NO | Remove with sentinel framing |
| `pane_scale` WS message | Server-pushed scaling hint | NO | Belongs in formatter HTML |
| `formatter_set` WS message | Notify browser of formatter change | NO | Remove with built-in formatters |
| `scrollback_ctl` WS message | clear/top/bottom commands | PARTIAL | clear: remove; top/bottom: keep as scroll hints |
| Double-click pane header to rename | Inline title edit | YES | Core browser UI |
| Right-click context menu | Zoom/split/rename/close | YES | Core browser UI |

---

## Introspection

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| `rdw status` | Full server snapshot | YES | Core; necessary for a daemon |
| `rdw status pane ID` | Per-pane detail | YES | Core; necessary |
| `GET /api/v1/status` | Status REST endpoint | YES | Core |
| `/admin` page | Browser introspection dashboard | YES | Correct separate page |
| Admin token auth | Separate access for introspection | YES | Correct |
| `GET /api/v1/admin/connections` | Active WS connection count | YES | Core |
| `rdw selftest` | Smoke test suite | YES | Correct; CI-friendly |

---

## Scrollback and export

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| Scrollback buffer (10k lines per pane) | Server stores all received lines | NO | `prog \| tee logfile \| rdw pipe`; storage is `tee`'s job |
| `rdw save pane/window/all` | Export scrollback to Markdown | NO | Remove with scrollback; use `tee` + your editor |
| `POST /api/v1/export/*` | Export REST endpoints | NO | Same |
| ANSI stripping in export | Remove escape sequences from stored lines | NO | `cat logfile \| sed 's/\x1b\[[0-9;]*m//g'` |
| Image assets in export | Decode base64 images into files | NO | `base64 -d` |
| Scrollback bookmarks | Named line indices | NO | Remove with scrollback |
| `bm:` / bookmark API | Create/list/delete bookmarks | NO | Remove with scrollback |

---

## Advanced / peripheral features

| Feature | Description | Unix-like | Replacement |
| --- | --- | --- | --- |
| Focus cycle (`rdw cycle start/stop/status`) | Auto-rotate window focus | YES | Core browser function for dashboards |
| Highlight profiles (`rdw highlight`) | Named regex → CSS class rules | NO | Belongs in user formatter; formatter writes `<span class="x">` |
| `GET/PUT/DELETE /api/v1/highlights` | Highlight REST API | NO | Same |
| Terminal panes (`rdw pane terminal`) | Launch gotty/socat shell in pane | NO | rdw wraps `ttyd`; use `ttyd` directly in a browser tab |
| KV persistence (SQLite, `--kv-persist`) | Persist KV across restarts | PARTIAL | Reasonable; like a dot-file |
| `rdw send FILE` | File to pane with type detection | PARTIAL | Convenience; hides `base64`/`cat` pipeline |
| Mirror (`--forward-to-file/cmd`) | Tee the stream | NO | `tee` |
| `--forward rd` bash-rd compat | Tee to bash-rd | NO | `tee >(rd -c ID)` |
| `rdw pane zoom` | Full-window zoom on a pane | YES | Core browser UI |
| `rdw pane swap` | Exchange pane positions | YES | Core browser UI |
| Bindings config (`bindings:` in YAML) | Remap keyboard shortcuts | YES | Correct |
| `GET /api/v1/bindings` | Serve binding map to browser | YES | Correct |

---

## Summary counts

| Rating | Count | Implication |
| --- | --- | --- |
| YES — Unix-like | ~40 | Keep as-is |
| PARTIAL — mixed | ~20 | Simplify or make optional |
| NO — should be outside rdw | ~35 | Remove or move to shell / user code |

Of the ~35 NO items, the largest clusters are:

1. **Built-in formatters** (6 items) — all should be user code
2. **Scrollback and export** (7 items) — storage is `tee`'s job
3. **Filter chain server-side** (5 items) — filtering is the shell's job
4. **Transport flags on `rdw pipe`** (5 items) — mirroring, forwarding, layout apply, title, filter belong in the shell
5. **Sentinel framing** (2 items) — `image:end`, `svg:end` — use standard `base64`
6. **Browser-side rendering that belongs in formatter** (4 items) — highlight, CSV sort, image_render, pane_scale
