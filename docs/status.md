# rdw implementation status

## Summary

378 tests · 11/11 selftest · `go vet` clean

## Packages

| Package | Role |
| --- | --- |
| internal/session | TargetID, ScrollbackBuffer, Manager, BookmarkStore |
| internal/kvstore | KV store, SQLite persistence |
| internal/control | Control sequence parser (all prefixes) |
| internal/bindings | 32 named actions, vim-like defaults |
| internal/auth | SHA-256 tokens, expiry, revocation |
| internal/config | YAML loader, UserFormatters |
| internal/layout | Window/pane schema, YAML parse |
| internal/pipeline | Line relay, filter chain + KV injection, sinks |
| internal/router | TargetID→pipeline, bookmark stores |
| internal/pipe | Client relay, reconnect queue, hybrid binary reader |
| internal/mirror | FileSync, CmdSync, Tee |
| internal/format | text/json/yaml/markdown/csv/image + CmdFormatter |
| internal/highlight | Regex highlight profile store |
| internal/export | Markdown+assets bundle |
| internal/discovery | Multi-server registry |
| internal/browser | Cross-platform URL open |
| internal/terminal | gotty/socat terminal pane launcher |
| internal/cycle | Focus cycle rotation |
| internal/selftest | 11 in-process smoke checks |
| internal/server | HTTP/WS server, REST API, browser SPA, /admin page |

## Feature inventory

### Data pipeline
- Line relay (text + hybrid binary/image/svg mode)
- Filter chain (max 8 stages), KV injected into filter subprocesses
- User-defined formatters (external command, KV injected, sandboxed iframe)
- Six built-in formatters: text, json, yaml, markdown, csv, image
- Control sequences: v: =: f: b64: bm: hl: sc: t: c: q: image: svg: scale: title:
- Stream mirroring (--forward-to-file, --forward-to-cmd, --forward rd)
- Scrollback buffer (circular, configurable cap, bookmark-addressable)

### Server
- HTTP/WebSocket server with rdw-v1 sub-protocol
- REST API at /api/v1/ (all endpoints)
- Unix socket auth ($XDG_RUNTIME_DIR/rdw/<id>.sock)
- Multi-server discovery and --port flag
- Token auth (SHA-256 hashed, expiry, per-pane/window scope)
- KV store (session-scoped, namespaced, SQLite persistence)
- /admin introspection page (separate admin token)
- rdw status / rdw status pane CLI

### Browser SPA
- CSS grid layout, dark theme, gutter drag-to-resize
- ANSI 16/256/true-colour parser
- 10,000-line per-pane scrollback ring buffer
- 32-action keyboard dispatch (vim-like), two-key sequences
- Pane title display, double-click inline edit
- image_render, svg_render (inline SVG, interactive)
- scale:fit/fill/native per image/SVG block
- Formatter-set WebSocket message, highlight_set, scrollback_ctl
- Normal/swap/search mode state machine
- Right-click context menu, window header bar
- CSV column sort (client-side)
- Focus cycle (window header auto-focus)

### CLI (rdw commands)
server, pipe (--filter, --title, --forward, --forward-to-file, --forward-to-cmd),
send, window, pane (split/resize/zoom/swap/rename/close),
kv, token, layout, group, save, formatter, status, cycle, selftest, completion
