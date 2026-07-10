# rdw architecture

## Overview

rdw is a single-binary daemon. It has three operational roles that may run in
the same process or in separate processes:

- server — the persistent HTTP/WebSocket daemon
- client — any process that pipes output via `rdw pipe`
- browser — the embedded SPA that renders panes

```
process stdout
      |
      v
  rdw pipe  ──── Unix socket ────┐
                                  │
                           ┌──────┴──────┐
                           │   Server    │
                           │             │
                    Router │  Session    │  KV Store
                    ┌──────┤  Manager   ├──────────┐
                    │      │             │          │
                 Pipeline  │  Hub        │        SQLite
                    │      └──────┬──────┘        (opt)
                    │             │
                    └────> WebSocket broadcast
                                  │
                           Browser SPA
```

## Data flow

1. The source process writes to stdout. The shell pipe connects that to `rdw pipe`'s stdin.
2. `rdw pipe` opens a Unix socket to the server (falls back to HTTP). It sends one JSON frame per line.
3. The server's `Router` receives each line. It looks up the `Pipeline` registered for that `TargetID`.
4. The `Pipeline` applies the filter chain, dispatches inline control sequences, appends to the `ScrollbackBuffer`, and calls every registered sink.
5. The broadcast sink serialises a `{type:"line", target_id:"...", line:"..."}` JSON frame and sends it to the WebSocket `Hub`.
6. The `Hub` fans the frame out to every connected browser.
7. The browser SPA appends the line to the appropriate pane's DOM ring buffer.

## Key packages

### internal/session

`TargetID` is a validated identifier `[a-zA-Z0-9_][a-zA-Z0-9_ -]*` (max 64 chars).

`ScrollbackBuffer` is a fixed-capacity circular ring backed by a slice. It is
the source of truth for export and formatter operations.

`Manager` owns the ordered window list, the active window index, and the KV
store reference. All mutations are guarded by a single `sync.RWMutex`. Windows
hold slices of `*PaneState`; panes are never shared between windows.

`BookmarkStore` is a per-pane named-position store, held in the Router
alongside the pipeline.

### internal/router

`Router` maps `TargetID → *pipeline.Pipeline` and `TargetID → *BookmarkStore`.
It holds a single `PipelineSink` function (the WebSocket broadcast) and passes
it to every pipeline it creates.

`SetControlHandler` registers a callback that the pipeline calls for every
recognised control sequence that is not KV or verbatim. The server uses this
to dispatch `bm:`, `hl:`, and `sc:` sequences.

### internal/pipeline

`Pipeline` is the per-pane processing unit. It is goroutine-safe. Processing
order for each line:

1. `control.Parse` — check for a control sequence prefix
2. If verbatim (`v:`): strip prefix, continue as content
3. If `b64:`: preserve prefix, continue as content (browser decodes)
4. If KV (`=:`): write to KV store, stop
5. Else: call the control handler and stop
6. Apply filter chain (max 8 external transformers)
7. Optionally prepend RFC-3339 timestamp (`t:` toggle)
8. `maybeDecodeBase64` — decode `b64:` prefix to raw bytes for export
9. Append to `ScrollbackBuffer`
10. Call all sinks

### internal/server

`Server` aggregates all subsystems. It is constructed by `New()` and started
by `Run(ctx)`. Shutdown is triggered by context cancellation or SIGINT/SIGTERM.

`Hub` is the WebSocket connection registry. It runs a single goroutine that
serialises writes to avoid concurrent send races.

`routes()` registers all HTTP handlers on a fresh `http.ServeMux`. Middleware
is composed via wrapper functions: `authed`, `unauth`, `admin`. The middleware
stack is: loopback guard (admin only) → rate limiter (unauth only) → token
auth (authed only) → handler.

### internal/control

The parser checks multi-char prefixes (`b64:`, `bm:`, `hl:`, `sc:`) first,
then single-char prefixes (`v:`, `q:`, `s:`, `c:`, `t:`, `f:`, `r:`, `=:`).

### internal/kvstore

The in-memory store is a `map[Key]string` guarded by a `sync.RWMutex`.
Keys follow the same character rules as TargetID and support a `prefix:` namespace
convention (`window:name:key` or `pane:id:key`).

When `--kv-persist` is set, each `Set` and `Delete` also writes to SQLite via
`kvstore.DB`. On `--restore`, `DB.Load` populates the in-memory store before
the server begins accepting connections.

### internal/format

Six formatters implement `Formatter.Format([]string) (string, error)`. They
produce HTML strings consumed by `POST /api/v1/panes/{id}/format`. The browser
may also trigger formatting via the right-click context menu.

### internal/pipe

`Relay` reads lines from an `io.Reader`, sends each to the server as a JSON
frame, and maintains a reconnect queue (default 1000 lines). On reconnect the
queue is flushed in order; overflow drops the oldest entry silently. The
`RECONNECT` marker frame sent by the server on WebSocket reconnect causes the
browser to request a catch-up scrollback replay.

## Authentication

Every request to authenticated endpoints must carry `Authorization: Bearer <token>`.
Tokens are stored SHA-256 hashed; the plain-text value is returned once at
creation. Token scope, expiry, and per-pane restrictions are enforced in
`internal/auth`.

The Unix socket at `$XDG_RUNTIME_DIR/rdw/<session-id>.sock` (mode 0600)
accepts unauthenticated connections from the owning user. `rdw pipe` prefers
this path.

## WebSocket protocol

Sub-protocol: `rdw-v1`. Frame format: newline-delimited JSON objects.

Server-to-browser message types:

| type | payload fields | trigger |
| --- | --- | --- |
| `line` | `target_id`, `line` | new output line |
| `layout_update` | `windows`, `active` | any layout change |
| `RECONNECT` | — | client reconnect |
| `highlight_set` | `target_id`, `profile` | `hl:` control sequence |
| `scrollback_ctl` | `target_id`, `action` | `sc:` control sequence |

## Port and discovery

The default port is 7681. Every CLI command accepts `--port / -p`. When
`--port 0` is given, `discovery.Resolve(0)` probes the registry at
`$XDG_CACHE_HOME/rdw/servers.json` and falls back to the default port.
`PruneStale` removes entries whose server process is no longer alive.

## Concurrency model

- One goroutine per WebSocket connection (read pump + write pump)
- One goroutine per active `Pipeline` (line processing loop)
- Hub runs one goroutine (fan-out loop)
- Discovery registry is file-locked; the in-process registry is mutex-guarded
- Session Manager: single `sync.RWMutex` over the window slice
- Router: single `sync.RWMutex` over the pipeline and bookmark maps
