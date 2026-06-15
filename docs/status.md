# rdw implementation status

## Summary

327 tests, 11/11 selftest checks pass, all planned phases complete.

## Package inventory

| Package | Role |
|---------|------|
| internal/session | TargetID, ScrollbackBuffer, Manager, BookmarkStore |
| internal/kvstore | KV store, SQLite persistence |
| internal/control | Inline control sequence parser |
| internal/bindings | 32 named actions, vim-like defaults |
| internal/auth | SHA-256 tokens, expiry, revocation |
| internal/config | YAML loader, validation |
| internal/layout | Window/pane schema, YAML parse |
| internal/pipeline | Line relay, filter chain, sinks |
| internal/router | TargetID→pipeline, bookmark stores |
| internal/pipe | Client relay, reconnect queue |
| internal/mirror | FileSync, CmdSync, Tee |
| internal/format | text/json/yaml/markdown/csv/image formatters |
| internal/highlight | Regex highlight profile store |
| internal/export | Markdown+assets bundle |
| internal/discovery | Multi-server registry |
| internal/browser | Cross-platform URL open |
| internal/terminal | gotty/socat terminal pane launcher |
| internal/cycle | Focus cycle rotation |
| internal/selftest | 11 in-process smoke checks |
| internal/server | HTTP/WS server, full REST API, browser SPA |

## Implemented

- HTTP/WebSocket server with `rdw-v1` sub-protocol, RECONNECT marker, reconnect queue
- Unix socket auth at `$XDG_RUNTIME_DIR/rdw/<id>.sock`
- Multi-server discovery and `--port` flag on all commands
- Session manager: windows, panes (max 64), active window
- Full REST API at `/api/v1/` — windows, panes, KV, tokens, layout, export, groups, stream, cycle, terminal, highlights, bookmarks, formatters
- Browser SPA: CSS grid layout, gutter drag-to-resize, ANSI 16/256/true-colour, 10k-line scrollback, 32-action keyboard dispatch, swap/search modes, right-click menu, window header bar, CSV sort
- Formatters: text, json, yaml, markdown, csv, image
- KV SQLite persistence (`--kv-persist`, `--restore`)
- Stream mirroring (`--forward-to-file`, `--forward-to-cmd`)
- bash-rd compat (`--forward rd|rdw|both`)
- Open-browser (`--open-browser`, xdg-open/open/rundll32)
- Scrollback bookmarks (per-pane, named, sorted)
- Regex highlight profiles (validated at store time)
- Pane label rename
- gotty/socat terminal pane with restricted-user sandbox
- Focus cycle automation with configurable dwell interval
- Man page (`man/rdw.1`)
- Goreleaser config (Linux + macOS, amd64 + arm64)
