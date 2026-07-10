# Concepts

## The pipeline

```
process stdout
      |
      v
  rdw pipe  ──── Unix socket ────┐
                                  │
                           ┌──────┴──────┐
                           │   Server    │
                     Router│  Session    │  KV Store
                    ┌──────┤  Manager   ├──────────┐
                    │      │             │          │
                 Pipeline  │  Hub        │        SQLite
                    │      └──────┬──────┘        (opt)
                    │             │
                    └────> WebSocket broadcast
                                  │
                           Browser SPA
```

Each source process writes to its stdout. The shell pipe connects that to `rdw pipe`'s stdin. `rdw pipe` sends each line to the server, which routes it to the correct pipeline, processes it, appends it to the scrollback buffer, and broadcasts it to all connected browsers.

## Target IDs

Every pane has a **Target ID** — an alphanumeric identifier used for routing. The Target ID is permanent for the lifetime of a session; the display **title** is a separate human-readable label that can change at any time.

Pattern: `[a-zA-Z0-9_][a-zA-Z0-9_ -]*` · Max length: 64 characters

```sh
make build 2>&1 | rdw pipe --id build --title "CI Build"
```

## Windows and panes

Windows are server-managed views within a single browser page — not browser tabs. Each window contains one or more panes. Keyboard shortcuts (`g t`, `g T`) switch between windows; `s` and `v` split panes.

## Filters vs formatters

**Filters** run server-side on every incoming line before it reaches the scrollback. They transform (or drop) lines. A filter is an external shell command. The current KV snapshot is injected into filter subprocesses as environment variables.

**Formatters** run on demand against the stored scrollback to produce HTML for display. They are read-only — they do not affect what is stored. Built-in formatters: text, json, yaml, markdown, csv, image. User-defined formatters are external commands producing HTML.

## The KV store

A session-scoped key-value store shared across all panes. Any stream can write to it via the `=:` control sequence. Filters and formatters read it as environment variables. Optional SQLite persistence across restarts.

## Control sequences

Lines beginning with a recognised prefix are intercepted server-side and acted on rather than displayed. See [Control Sequences](control.md) for the full list.
