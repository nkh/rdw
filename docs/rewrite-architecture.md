# rdw rewrite — component architecture

## Premise

# [1] The fundamental insight: rdw is not one tool. It is a web layout server,
# a line relay, a KV store, a session manager, a browser SPA, an auth system,
# a formatter runner, and a filter chain. These are seven different tools that
# grew into one binary because it was convenient. The rewrite separates them.

# [2] The goal is not to make rdw smaller — it is to make each component
# independently useful, testable, and replaceable by a Unix user who does not
# want the full stack.

# [3] bash-rd's lesson: the server should be so small that a user can read and
# understand it in five minutes. Everything else is composition.

# [4] The rewrite targets two audiences: the developer who wants `rdw pipe`
# to just work, and the power user who wants to replace any layer with their
# own code. Both must be satisfied without compromise.

---

## Component map

```
┌─────────────────────────────────────────────────────────────────┐
│  USER INTERFACE LAYER                                           │
│                                                                 │
│  rdw-cli (Go)        rdw-compose (bash)     browser            │
│  thin REST client    argument → pipeline     SPA or custom      │
│  one flag → one      composition engine      via /rdw-api.js    │
│  REST call           (generates shell)                          │
└───────────────┬──────────────────┬───────────────┬─────────────┘
                │                  │               │
                ▼                  ▼               ▼
┌─────────────────────────────────────────────────────────────────┐
│  rdw-server (Go) — the only persistent process                  │
│                                                                 │
│  rdw-layout-server   rdw-kv-server   rdw-ws-hub                 │
│  window/pane mgmt    KV store        WebSocket fan-out          │
│  REST endpoints      REST + events   line broadcast             │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                ┌───────────────┴───────────────┐
                ▼                               ▼
┌───────────────────────┐         ┌─────────────────────────────┐
│  STREAM LAYER         │         │  DISPLAY LAYER              │
│                       │         │                             │
│  rdw-relay (Go/bash)  │         │  user formatters (any lang) │
│  stdin → socket       │         │  per-line, KV-injected      │
│  pure transport       │         │  HTML or text output        │
│                       │         │                             │
│  composed in shell:   │         │  rdw-fmt-json (bash/python) │
│  prog | fmt | relay   │         │  rdw-fmt-csv  (bash/awk)    │
└───────────────────────┘         │  rdw-fmt-md   (pandoc)      │
                                  └─────────────────────────────┘
```

# [5] Every box above is independently runnable. A user who only needs the
# relay and KV store does not need the layout server. A user who only needs
# the layout server does not need the KV store.

# [6] The server is the only mandatory process. Everything else is optional
# composition. This is exactly the Unix model.

---

## Component 1 — rdw-server (Go)

### What it is

The minimal persistent process. It manages:
- window/pane session state
- WebSocket connections to browsers
- the REST API surface
- auth (token + Unix socket)
- the KV store

### What it is NOT

# [7] rdw-server does not run formatters. It does not store scrollback. It does
# not filter lines. It does not mirror streams. It does not manage tokens in any
# way beyond checking them. These are other programs' concerns.

# [8] rdw-server's job is: receive a line on a socket, broadcast it to browsers
# watching that pane. That is it. Everything else is the user's pipeline.

### Internal structure

```go
// rdw-server exposes three sub-services, each independently testable.
// They share a process but nothing else — no shared global state.

type LayoutServer struct {
    // Windows, panes, groups. Pure session geometry.
    // Broadcasts layout_update over WebSocket on every mutation.
    // Knows nothing about line content.
}

type KVServer struct {
    // Key-value store with SQLite persistence option.
    // Emits kv_update WebSocket events on change.
    // Knows nothing about panes or lines.
}

type StreamHub struct {
    // Receives lines on POST /api/v1/stream/{id}
    // Broadcasts to WebSocket clients watching that pane.
    // Applies per-pane formatter if registered (calls external command).
    // Knows nothing about session geometry.
}
```

# [9] The three sub-services are separate Go packages with no imports between
# them. They are wired together only in main.go. This enforces separation and
# makes each testable without the others.

# [10] LayoutServer is stateful but has no I/O. KVServer has I/O (SQLite) but
# is single-purpose. StreamHub has I/O (sockets, WebSocket) but no state.
# Each maps cleanly to a Unix daemon that does one thing.

### REST API surface (reduced)

# [11] The current rdw has 50+ REST endpoints. The rewrite targets ~20.
# The difference is removed by moving functionality to the shell pipeline
# or to optional extension daemons.

**Keep (core):**
```
GET  /api/v1/ping
GET  /api/v1/ws                       WebSocket upgrade
POST /api/v1/stream/{id}              line ingest
GET  /api/v1/session                  session snapshot
GET  /api/v1/windows                  list windows
POST /api/v1/windows                  create window
DEL  /api/v1/windows/{name}
PATCH /api/v1/windows/{name}          rename
POST /api/v1/windows/{name}/focus
POST /api/v1/panes/{id}/split
POST /api/v1/panes/{id}/zoom
POST /api/v1/panes/{id}/resize
DEL  /api/v1/panes/{id}
PATCH /api/v1/panes/{id}              set title
GET  /api/v1/kv                       list keys
GET  /api/v1/kv/{key}
PUT  /api/v1/kv/{key}
DEL  /api/v1/kv/{key}
GET  /api/v1/layouts
POST /api/v1/layouts
POST /api/v1/layouts/{name}/apply
GET  /api/v1/status
GET  /api/v1/status/panes/{id}
```

**Remove (moved to shell/pipeline):**
```
POST /api/v1/panes/{id}/filters       → shell: prog | filter | relay
POST /api/v1/panes/{id}/format        → shell: relay runs formatter per-line
POST /api/v1/panes/{id}/bookmarks     → removed with scrollback
GET  /api/v1/highlights               → moved to user formatter
POST /api/v1/formatters               → user registers via KV or config file
POST /api/v1/export/*                 → removed; use tee
POST /api/v1/cycle/*                  → standalone rdw-cycle daemon
POST /api/v1/panes/{id}/terminal      → standalone rdw-terminal daemon
GET  /api/v1/tokens (management)      → rdw-auth daemon or config file
```

# [12] Removing 30 endpoints is not a loss of functionality. It is a transfer
# of responsibility to the correct layer. Every removed endpoint has a simpler
# shell equivalent.

# [13] The REST API becomes a stable, versioned contract at /api/v1/. Extension
# daemons (cycle, terminal, auth) speak the same API. A user writing a custom
# extension knows exactly what calls are available.

### Making the server more Unix-like

# [14] REST is not Unix. Unix tools communicate over pipes and files, not HTTP.
# But for a browser-integrated tool, HTTP is unavoidable. The Unix principle
# to apply here is: make every operation idempotent, composable via scripts,
# and auditable via logs.

# [15] Every REST endpoint is also available as a named Unix socket command.
# The socket accepts the same JSON as the HTTP API. This means `socat` or
# `nc` can drive rdw-server from the shell without curl.

# [16] Events (layout changes, KV changes, new lines) are published as
# newline-delimited JSON on a named pipe: /run/rdw/{id}/events. Any
# Unix tool can subscribe: `tail -f /run/rdw/{id}/events | jq .`

```sh
# Subscribe to all rdw events from the shell
tail -f /run/rdw/default/events | while IFS= read -r event; do
  type=$(echo "$event" | jq -r .type)
  case "$type" in
    kv_update) echo "KV changed: $(echo $event | jq -r .key)" ;;
    layout_update) echo "Layout changed" ;;
  esac
done
```

# [17] This is more Unix than a WebSocket subscription. WebSockets require a
# WebSocket client. `tail -f` requires nothing. Both the event pipe and the
# WebSocket are offered; they carry the same events.

---

## Component 2 — rdw-relay (Go, ~100 lines)

### What it is

A single-purpose binary: read stdin, send lines to rdw-server.

```go
// rdw-relay reads stdin line by line and sends each to the server.
// It has exactly three flags: --id, --port, --token.
// Nothing else.
func main() {
    id   := flag.String("id", "", "target pane")
    port := flag.Int("port", 7681, "server port")
    tok  := flag.String("token", "", "auth token")
    flag.Parse()
    relay(os.Stdin, *id, *port, *tok)
}
```

# [18] rdw-relay is so simple it could be a bash function. The Go binary is
# provided for performance (no shell overhead per line) but a bash equivalent
# works identically:

```sh
# rdw-relay.sh — the entire relay in bash
rdw_relay() {
  local id=$1 port=${2:-7681}
  while IFS= read -r line; do
    curl -sf -X POST "http://127.0.0.1:$port/api/v1/stream/$id" \
      -H 'Content-Type: application/json' \
      -d "{\"line\":$(printf '%s' "$line" | jq -Rs .)}" > /dev/null
  done
}
# Usage: prog | rdw_relay my-pane
```

# [19] The bash version is slower (one curl per line) but illustrates the
# principle: if a tool is so small it can be a shell function, it should not
# be a flag on a larger tool. `rdw pipe --filter` bloats the relay.
# `prog | filter | rdw-relay` composes the same result from smaller pieces.

# [20] rdw-relay connects via Unix socket first (no token needed for local
# owner) and falls back to HTTP. This is the only "smart" behaviour it has.

# [21] The reconnect queue stays in rdw-relay: it is a transport concern, not
# a pipeline concern. If the server is temporarily unavailable, rdw-relay
# buffers N lines and flushes on reconnect. The shell pipeline does not need
# to know this happened.

---

## Component 3 — rdw-compose (bash)

### What it is

This is the key innovation: a bash script that translates rdw-style arguments
into shell pipeline composition. It is the answer to the "burden on the user"
problem.

# [22] The user's burden: instead of `rdw pipe --filter 'grep ERROR' --id log`,
# they must write `prog | grep ERROR | rdw-relay --id log`. For power users this
# is natural. For others, it is friction.

# [23] rdw-compose eliminates that friction by being the composition engine.
# The user writes familiar rdw-style arguments; rdw-compose generates and
# executes the correct shell pipeline. No magic, no hidden behaviour — the
# generated pipeline is always printed before execution.

```bash
#!/usr/bin/env bash
# rdw-compose — translate rdw pipe arguments into shell pipeline
# Usage: rdw-compose [rdw-pipe-flags] [--dry-run]

set -euo pipefail

# [24] Parse arguments in order. Build a pipeline as an array of commands.
# The user sees the generated pipeline before it runs (--dry-run or always).

declare -a PIPELINE=()
declare -a BEFORE=()   # commands to run before the pipeline (setup)
ID=""
PORT=7681
TITLE=""
LAYOUT=""
FORWARD_FILE=""
FORWARD_CMD=""
FORWARD_RD=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --id)         ID=$2;           shift 2 ;;
    --port)       PORT=$2;         shift 2 ;;
    --title)      TITLE=$2;        shift 2 ;;
    --layout)     LAYOUT=$2;       shift 2 ;;
    --filter)     PIPELINE+=("$2"); shift 2 ;;
    --forward-to-file)  FORWARD_FILE=$2; shift 2 ;;
    --forward-to-cmd)   FORWARD_CMD=$2;  shift 2 ;;
    --forward)    FORWARD_RD=true; shift ;;
    --dry-run)    DRY_RUN=true;    shift ;;
    *)            echo "unknown: $1" >&2; exit 1 ;;
  esac
done

# [25] Setup commands run before the pipeline, not in it.
# These are idempotent: running them twice is safe.
[[ -n $LAYOUT ]] && BEFORE+=("rdw layout apply '$LAYOUT' --port $PORT")
[[ -n $TITLE  ]] && BEFORE+=("rdw pane rename '$ID' '$TITLE' --port $PORT")

# [26] Build the pipeline right-to-left. Start from the relay, add stages.
RELAY="rdw-relay --id '$ID' --port $PORT"

# [27] Mirroring: tee is the Unix tool for stream duplication.
# rdw-compose generates the correct tee invocation.
if [[ -n $FORWARD_FILE && -n $FORWARD_CMD ]]; then
  RELAY="tee '$FORWARD_FILE' >(${FORWARD_CMD}) | $RELAY"
elif [[ -n $FORWARD_FILE ]]; then
  RELAY="tee '$FORWARD_FILE' | $RELAY"
elif [[ -n $FORWARD_CMD ]]; then
  RELAY="tee >($FORWARD_CMD) | $RELAY"
fi

# [28] Forward to bash-rd: tee is again the right tool.
$FORWARD_RD && RELAY="tee >(rd -c '$ID' 2>/dev/null || true) | $RELAY"

# [29] Filters are just pipeline stages before the relay.
# They are composed right-to-left: last filter runs last.
STAGES=""
for filter in "${PIPELINE[@]}"; do
  STAGES="${STAGES:+$STAGES | }$filter"
done

FULL_PIPELINE="${STAGES:+$STAGES | }$RELAY"

# [30] Always show what will run. The user is never surprised.
echo "# rdw-compose generated pipeline:" >&2
for cmd in "${BEFORE[@]}"; do echo "#   $cmd" >&2; done
echo "#   stdin | $FULL_PIPELINE" >&2

$DRY_RUN && exit 0

# [31] Run setup commands first, then exec the pipeline.
for cmd in "${BEFORE[@]}"; do eval "$cmd"; done
exec bash -c "cat | $FULL_PIPELINE"
```

### Usage examples

```sh
# Simple pipe — identical to current rdw pipe --id
prog | rdw-compose --id log

# With filter — generates: prog | grep ERROR | rdw-relay --id log
prog | rdw-compose --id log --filter 'grep ERROR'

# With multiple filters
prog | rdw-compose --id log \
  --filter 'grep -v DEBUG' \
  --filter 'python3 highlight.py'

# With layout, title, mirroring
prog | rdw-compose --id build \
  --title "CI Build" \
  --layout ci.yaml \
  --filter 'grep -v DEBUG' \
  --forward-to-file /tmp/build.log

# Dry run: print the pipeline without executing
prog | rdw-compose --id build --filter 'grep ERROR' --dry-run
# Output:
# rdw-compose generated pipeline:
#   stdin | grep ERROR | rdw-relay --id 'build' --port 7681
```

# [32] The dry-run output is itself a valid shell command. The user can copy
# it, modify it, and run it directly. rdw-compose is transparent by design.

# [33] rdw-compose is not magic. It is a 60-line bash script that generates
# shell pipelines. It can be read in two minutes. A user who disagrees with
# any generated pipeline can just write the pipeline themselves. There is no
# lock-in.

# [34] rdw-compose covers 100% of the removed flags from `rdw pipe`:
# --filter, --forward-to-file, --forward-to-cmd, --forward, --layout, --title.
# The user experience is identical; the implementation is transparent.

---

## Component 4 — Per-line formatter model

### The interface

# [35] A formatter in the rewrite is any executable that:
# - reads one line from stdin
# - writes zero or more lines to stdout
# - receives the current KV as environment variables
# - writes HTML if registered as an HTML formatter; text otherwise

# [36] This is exactly bash-rd's model. The formatter is the user's code.
# rdw-server calls it; the user writes it.

### How rdw-server calls formatters

```go
// internal/stream/hub.go — simplified
func (h *Hub) ingestLine(paneID string, rawLine string) {
    // [37] If a formatter is registered for this pane, call it per-line.
    // The formatter's stdout is what gets broadcast to browsers.
    // If no formatter is registered, broadcast the raw line.
    if fmt := h.formatters[paneID]; fmt != nil {
        display, err := fmt.FormatLine(rawLine, h.kv.Snapshot())
        if err != nil || display == "" {
            return // formatter dropped the line
        }
        h.broadcast(paneID, display)
        h.ring[paneID].Append(display)
    } else {
        h.broadcast(paneID, rawLine)
        h.ring[paneID].Append(rawLine)
    }
}
```

# [38] The ring buffer stores the formatter's OUTPUT, not the raw input.
# This is important: on reconnect, the browser replays what it would have
# seen, not the raw data that needs formatting.

# [39] Ring buffer size: 200 lines per pane. Enough for reconnect. Not enough
# to be a log store. Users who need log storage use `tee`.

### Formatter registration

```sh
# Register a per-line text formatter (output replaces the line)
rdw formatter register log --cmd 'python3 highlight.py'

# Register an HTML formatter (output is wrapped in rdw-html div)
rdw formatter register api --cmd 'python3 jsoncolor.py' --html

# Register via KV (no separate command needed)
rdw kv set fmt.log.cmd  'python3 highlight.py'
rdw kv set fmt.log.html false
# rdw-server watches fmt.*.cmd keys and auto-registers formatters
```

# [40] KV-driven formatter registration is the most Unix-like approach.
# The KV store is the configuration channel. Setting a KV key triggers
# a side effect (formatter registration) on the server. This is analogous
# to how systemd unit files work: write a file, the daemon picks it up.

# [41] The formatter is registered once and stays alive between lines
# (long-lived mode, for efficiency) OR is called fresh per line (per-line
# mode, for KV freshness). The user chooses with a flag. See formatter-model-analysis.md.

### Formatter examples

```python
#!/usr/bin/env python3
# log-color.py — per-line HTML formatter
# KV: $LOG_PREFIX is injected from rdw kv set LOG_PREFIX "[PROD]"
import sys, html, os

prefix = os.environ.get('LOG_PREFIX', '')
line   = sys.stdin.readline().rstrip()

cls = 'err' if 'ERROR' in line else 'warn' if 'WARN' in line else 'info'
print(f'<span class="{cls}">{prefix} {html.escape(line)}</span>')
```

```sh
#!/bin/sh
# json-pretty.sh — per-line text formatter using standard jq
line=$(cat)
echo "$line" | jq . 2>/dev/null || echo "$line"
```

```awk
# count-errors.awk — stateful formatter (run as long-lived)
# Counts errors and prepends the running total
/ERROR/ { errors++ }
{ print "[errors:" errors "] " $0 }
```

# [42] The awk formatter is naturally stateful: awk keeps its variables
# across lines when run as a long-lived process. No special rdw support needed.
# `--stateful` flag keeps the process alive; `--per-line` (default) restarts it.

# [43] The KV injection into formatters is what makes them dynamic. When
# `rdw kv set LOG_PREFIX "[STAGING]"` is run mid-stream, the next per-line
# invocation picks it up. In long-lived mode, a SIGUSR1 or a re-read of a
# temp file is needed — but that is the formatter's problem, not rdw's.

---

## Component 5 — rdw-cli (Go, thin REST client)

### What it is

# [44] rdw-cli is what `rdw` currently is at the command layer — a thin
# wrapper that calls REST endpoints. But in the rewrite it is even thinner:
# no local logic, no argument-to-pipeline composition (that is rdw-compose's job).
# Every subcommand maps to exactly one REST call.

```go
// rdw window create ci  →  POST /api/v1/windows {"name":"ci"}
// rdw pane rename log "My Log"  →  PATCH /api/v1/panes/log {"title":"My Log"}
// rdw kv set foo bar  →  PUT /api/v1/kv/foo {"value":"bar"}
```

# [45] rdw-cli is essentially `curl` with knowledge of the API schema and
# the ability to read the token from config. It could be replaced entirely
# by a shell function:

```sh
rdw() {
  local token=$(cat ~/.config/rdw/token 2>/dev/null)
  local base="http://127.0.0.1:${RDW_PORT:-7681}/api/v1"
  case "$1 $2" in
    "window create") curl -sf -X POST "$base/windows" \
                       -H "Authorization: Bearer $token" \
                       -d "{\"name\":\"$3\"}" ;;
    "kv set")        curl -sf -X PUT "$base/kv/$3" \
                       -H "Authorization: Bearer $token" \
                       -d "{\"value\":\"$4\"}" ;;
    # ...
  esac
}
```

# [46] A bash rdw-cli is an interesting thought experiment. It would require
# no compilation, be readable by any Unix user, and be trivially extensible.
# The downside: no tab completion, no structured error messages, slower.
# Go rdw-cli is the right choice but the bash version should work identically
# for scripting purposes.

# [47] rdw-cli should support `--output json` to get raw JSON from any endpoint.
# This makes it composable with `jq`:
#   rdw status --output json | jq '.panes | keys[]'
#   rdw kv list --output json | jq '.keys[]'

---

## Dynamism in the rewrite

# [48] "rdw is dynamic" means: a running rdw session can be reconfigured
# without restart. Windows created, panes split, formatters changed, KV
# updated — all while streams are flowing. The rewrite must preserve this.

### Dynamic reconfiguration mechanisms

**1. REST API (same as current):**

# [49] The REST API is the primary dynamic interface. Any tool that can make
# HTTP requests can reconfigure a running rdw session. This is already Unix-
# compatible via curl. The rewrite makes it more accessible by reducing the
# API surface to ~20 well-named endpoints.

**2. KV-as-configuration:**

# [50] In the rewrite, KV is not just data — it is the primary configuration
# channel. Formatter registration, pane titles, highlight rules, and display
# preferences are all KV keys. The server watches specific key patterns and
# reacts to changes:

```sh
# Set formatter by writing to KV — no separate API call
rdw kv set fmt.log.cmd   'python3 highlight.py'
rdw kv set fmt.log.html  true
rdw kv set pane.log.title "Application Log"

# Highlight rules as KV
rdw kv set hl.errors.pattern 'ERROR'
rdw kv set hl.errors.class   'hl-error'
```

# [51] KV-as-configuration means a formatter script can reconfigure rdw
# by writing to KV: `echo "=:pane.log.title=Error Storm" | rdw-relay --id log`.
# The producer controls its own display metadata. This is the bash-rd model.

# [52] The server watches key prefixes (`fmt.`, `pane.`, `hl.`) and treats
# writes to those keys as configuration mutations. This is analogous to how
# procfs works: write to a file, the kernel changes behaviour.

**3. Event pipe (new):**

# [53] A named pipe at `/run/rdw/{id}/events` streams all server events as
# newline-delimited JSON. External daemons can subscribe and react:

```sh
# rdw-cycle: external daemon that reads events and drives window focus
tail -f /run/rdw/default/events \
  | grep '"type":"layout_update"' \
  | rdw-cycle --windows build,logs,metrics --interval 10
```

# [54] The event pipe makes rdw observable from the shell without a WebSocket
# client. Automation scripts can react to rdw state changes using standard
# shell tools (grep, jq, awk) without any rdw-specific knowledge.

**4. Unix socket commands:**

# [55] The Unix socket at `/run/rdw/{id}/rdw.sock` accepts JSON commands,
# one per line. Any tool can drive rdw without HTTP:

```sh
echo '{"action":"kv_set","key":"status","value":"running"}' \
  | socat - UNIX-CONNECT:/run/rdw/default/rdw.sock
```

# [56] This makes rdw drivable from environments without a full HTTP stack —
# embedded systems, minimal containers, or latency-sensitive tools where
# HTTP overhead matters.

---

## Making REST more Unix-like

# [57] REST is inherently verb-oriented (GET/POST/PATCH/DELETE) rather than
# data-flow-oriented (stdin → stdout). But it can be made more Unix-like
# by following these principles.

### Principle 1: every resource is a file

# [58] The KV store is a filesystem. `PUT /api/v1/kv/foo` is `write(foo)`.
# `GET /api/v1/kv/foo` is `read(foo)`. This maps directly to a FUSE
# filesystem: mount rdw-server as a directory and use standard file operations.

```sh
# With rdw-fuse (hypothetical optional component):
cat /mnt/rdw/kv/build.status       # GET /api/v1/kv/build.status
echo "passing" > /mnt/rdw/kv/build.status  # PUT /api/v1/kv/build.status
ls /mnt/rdw/panes/                  # GET /api/v1/session
echo "hello" >> /mnt/rdw/stream/log # POST /api/v1/stream/log
```

# [59] A FUSE mount is an optional component, not core. But offering it makes
# rdw accessible to any Unix tool that can read and write files — which is
# every Unix tool. Standard `cp`, `cat`, `echo`, `find` all become rdw clients.

# [60] The FUSE mount is read-write for KV, write-only for streams, and
# read-only for status. It maps the REST API onto file semantics exactly.

### Principle 2: idempotency

# [61] Every REST endpoint in the rewrite is idempotent. PUT replaces, PATCH
# modifies, POST creates-or-gets. There are no "create-only" operations that
# fail if the resource already exists. This makes scripting safe: a setup
# script can be run twice without error.

### Principle 3: composability via shell

# [62] Every REST operation should be expressible as a one-liner with curl.
# The rdw-cli is a convenience; it should never be required. Documentation
# always shows the curl equivalent alongside the rdw-cli command.

```sh
# rdw-cli and curl equivalents side by side:
rdw kv set foo bar
curl -X PUT http://localhost:7681/api/v1/kv/foo -d '{"value":"bar"}'

rdw window create build
curl -X POST http://localhost:7681/api/v1/windows -d '{"name":"build"}'
```

### Principle 4: the server emits events, not commands

# [63] The WebSocket carries events (things that happened) not commands
# (things to do). The browser decides what to do with an event. This is
# the Unix pub-sub model: the server publishes, consumers subscribe.
# No consumer is forced to handle any event.

# [64] Events are named nouns: `window.created`, `kv.changed`, `line.received`.
# Not verbs like `update_display` or `refresh_layout`. The distinction matters
# for composability: a consumer that only cares about KV changes subscribes
# to `kv.changed` and ignores everything else.

---

## The scrollback question in the rewrite

# [65] The rewrite replaces the scrollback buffer with a ring buffer of 200
# display lines per pane. This is the display cache; it is not a data store.

# [66] A user who needs scrollback history uses `tee`:
#   prog | tee ~/.rdw-logs/build.log | rdw-relay --id build
# Standard Unix log rotation (logrotate) then manages the file.

# [67] Export disappears from rdw entirely. The "export" is the tee'd file.
# `rdw save` becomes `cat ~/.rdw-logs/build.log | markdown_formatter`.
# No rdw involvement needed.

# [68] The ring buffer is used only for reconnect replay. When a browser
# reconnects, it receives the last 200 display lines. This gives a useful
# "catch up" without committing to full log storage.

# [69] 200 lines is configurable per server start: `rdw-server --ring-size 500`.
# It is not configurable per pane — all panes in a session share the same ring
# size. This simplifies the model.

---

## Breaking rdw into composable utilities

### rdw-server (Go, ~1000 lines)

# [70] The core: session geometry, KV store, WebSocket hub, formatter
# runner, ring buffer, REST API, auth. ~20 endpoints.

### rdw-relay (Go, ~100 lines OR bash ~20 lines)

# [71] stdin → server. Three flags: --id, --port, --token.
# The simplest possible transport. Could be a curl one-liner.

### rdw-compose (bash, ~100 lines)

# [72] Argument-to-pipeline compiler. Translates rdw-pipe-style arguments
# into shell pipelines. Always shows the generated pipeline. Dry-run mode.

### rdw-fuse (Go, optional, ~500 lines)

# [73] Mounts the rdw API as a filesystem. Optional; not needed for basic use.
# Enables any Unix tool to interact with rdw without knowing its API.

### rdw-cycle (bash, ~30 lines)

# [74] Focus cycle daemon. Reads the event pipe, rotates window focus.
# Currently an rdw-server feature; in the rewrite it is an independent process:

```sh
#!/bin/sh
# rdw-cycle -- rotate focus through windows at N second intervals
windows="$1"  # comma-separated
interval="${2:-5}"
IFS=, read -ra WINS <<< "$windows"
i=0
while true; do
  rdw window focus "${WINS[$i]}"
  i=$(( (i + 1) % ${#WINS[@]} ))
  sleep "$interval"
done
```

# [75] rdw-cycle is 10 lines of shell. It does not need to be in rdw-server.
# It talks to rdw via rdw-cli (one REST call per focus change). It runs as
# a background process the user starts and stops manually.

### rdw-fmt-* (bash/python, ~20-50 lines each)

# [76] A collection of example formatters shipped with rdw. These are scripts,
# not compiled code. Users copy and modify them.

```
rdw-fmt-json     -- jq pretty-print + colour (uses jq + sed)
rdw-fmt-csv      -- CSV to HTML table (uses awk or python csv module)
rdw-fmt-ansi     -- ANSI colour passthrough (default; trivial)
rdw-fmt-errors   -- highlight errors and warnings (uses grep + sed)
rdw-fmt-count    -- running line count in header (uses awk)
rdw-fmt-ts       -- prepend timestamp (uses ts from moreutils or date)
```

# [77] These formatters are pedagogical as much as functional. A user who
# reads rdw-fmt-json understands how to write their own. The code is 20 lines.
# Nothing is hidden in compiled binary code.

### rdw-kv-watch (bash, ~20 lines)

# [78] Polls a KV key and triggers a command when the value changes.
# Replaces the proposed KV change notification WebSocket feature.

```sh
#!/bin/sh
# rdw-kv-watch KEY CMD -- run CMD when KEY changes
key=$1; cmd=$2; prev=""
while true; do
  cur=$(rdw kv get "$key" 2>/dev/null)
  [ "$cur" != "$prev" ] && { prev=$cur; eval "$cmd"; }
  sleep 1
done
```

# [79] rdw-kv-watch is a polling loop. It is not elegant but it is Unix:
# any user can understand it, modify it, and extend it. A WebSocket subscription
# would be more efficient but would require a WebSocket client library.

### rdw-auth (bash, optional)

# [80] Token management as a shell script. Reads/writes ~/.config/rdw/tokens.
# In the rewrite, token auth is optional for local use (Unix socket covers it).

---

## The composition interface — how it works for users

# [81] The user experience question: if filtering is `prog | filter | rdw-relay`,
# do users need to learn three tools instead of one? The answer is no,
# because rdw-compose handles the composition. Users write rdw-style flags;
# rdw-compose generates the pipeline. Advanced users skip rdw-compose entirely.

### Three levels of use

**Level 1 — casual user (identical to current rdw):**

```sh
prog | rdw-compose --id log --filter 'grep ERROR' --title "App Log"
```

Same interface as today. rdw-compose generates the pipeline invisibly.

**Level 2 — intermediate user (reads the generated pipeline):**

```sh
# --dry-run shows the pipeline
prog | rdw-compose --id log --filter 'grep ERROR' --dry-run
# # rdw-compose generated pipeline:
# #   stdin | grep ERROR | rdw-relay --id 'log' --port 7681

# User copies and modifies:
prog | grep ERROR | sed 's/ERROR/[ERR]/' | rdw-relay --id log
```

# [82] The dry-run output is the escape hatch. The user who wants full control
# runs with --dry-run, copies the pipeline, modifies it, and never uses
# rdw-compose again. No lock-in.

**Level 3 — power user (no rdw-compose at all):**

```sh
prog \
  | grep ERROR \
  | python3 highlight.py \
  | tee /tmp/errors.log \
  | rdw-relay --id log
```

# [83] Level 3 users discover that rdw-relay is trivially replaceable.
# They might replace it with a curl one-liner, a socat command, or their
# own relay. The server does not care how lines arrive.

---

## Handling rdw's many arguments

### The problem stated precisely

# [84] rdw pipe currently has ~12 flags. Shell composition requires the user
# to know `tee`, process substitution, shell pipelines, and multiple rdw-cli
# commands. For a developer unfamiliar with advanced bash, this is hostile.

# [85] The solution is not to embed this knowledge in rdw. It is to provide
# rdw-compose as a learning aid that shows users what the shell pipeline looks
# like. Over time, users learn the underlying tools and stop needing rdw-compose.
# This is the Unix education model: tools that teach the user to not need them.

### rdw-compose as a teaching tool

# [86] rdw-compose ALWAYS prints the generated pipeline to stderr before
# running it. This is intentional and not suppressible. The user always sees
# what is happening. This is transparency as a design principle, not a debug mode.

# [87] The pipeline output is syntax-highlighted if the terminal supports it.
# The command boundary (`|`) is highlighted differently from arguments.
# This makes it easier to read and understand for new users.

### Argument profiles (named compositions)

# [88] A user can save a composition as a named profile:

```sh
# Save a composition profile
rdw-compose --id log \
  --filter 'grep -v DEBUG' \
  --title "Application Log" \
  --forward-to-file /tmp/app.log \
  --save-as myapp-log

# Use it later
prog | rdw-compose @myapp-log
```

# [89] Profiles are stored as shell scripts in ~/.config/rdw/profiles/.
# A profile is just the generated pipeline as a shell function:

```sh
# ~/.config/rdw/profiles/myapp-log.sh
myapp_log() {
  rdw pane rename log "Application Log" --port 7681
  cat | grep -v DEBUG | tee /tmp/app.log | rdw-relay --id log --port 7681
}
```

# [90] Because profiles are shell scripts, they can be shared, version-
# controlled, and modified without any rdw knowledge. They are not a
# proprietary format. `cat ~/.config/rdw/profiles/myapp-log.sh` shows
# everything.

---

## What stays in Go, what moves to bash

# [91] Go handles: anything that needs performance (relay buffering,
# WebSocket broadcasting, KV store access, HTTP serving), anything that
# needs structured error handling, anything that needs to be a stable binary.

# [92] Bash handles: anything that is composition of existing tools
# (rdw-compose, rdw-cycle, rdw-kv-watch, example formatters), anything
# that benefits from being readable and modifiable, anything where
# startup time does not matter.

| Component | Language | Reason |
| --- | --- | --- |
| rdw-server | Go | Performance, WebSocket, HTTP, SQLite |
| rdw-relay | Go | Per-line throughput, reconnect buffer |
| rdw-cli | Go | Structured errors, tab completion, token management |
| rdw-compose | bash | Composition, transparency, modifiability |
| rdw-cycle | bash | Trivial; 10 lines of shell |
| rdw-kv-watch | bash | Trivial; polling loop |
| rdw-fuse | Go | FUSE requires Go/C |
| rdw-fmt-* | bash/python | Pedagogy; users copy and modify |

# [93] The binary count goes from 1 to 3 (server, relay, cli). Everything
# else is a shell script. The total compiled code drops from ~8000 lines to
# ~2000 lines. The rest is shell scripts that any Unix user can read and modify.

---

## Migration path

# [94] The rewrite is not a forced migration. rdw 1.x continues to work.
# rdw 2.x ships both the new components and a compatibility shim:
# `rdw pipe` in 2.x calls rdw-compose internally.

# [95] Deprecated features emit warnings that show the replacement:
#   rdw pipe --filter 'grep ERROR' --id log
#   Warning: --filter is deprecated. Use: prog | grep ERROR | rdw-relay --id log
#   Or install rdw-compose: prog | rdw-compose --id log --filter 'grep ERROR'

# [96] The deprecation warning is informational, not blocking. The flag still
# works in 2.x. Removal happens in 3.0 with a minimum 12-month warning period.

# [97] The scrollback buffer deprecation is harder. Users who depend on
# `rdw save` or bookmarks need a migration path. The migration is:
#   Old: prog | rdw pipe --id log  (rdw stores lines)
#   New: prog | tee ~/.rdw/logs/log.txt | rdw-relay --id log  (you store lines)
# This is a one-line change to the pipeline.

# [98] The migration guide ships as a man page (rdw-migrate.7) with before/after
# examples for every deprecated feature. Nothing is removed without documentation.

---

## Summary: what the rewrite achieves

# [99] The rewrite makes rdw a set of tools, not a tool. Each component does
# one thing. They compose via Unix pipes, REST calls, and shell scripts. A
# user who dislikes any component can replace it. A user who only needs part
# of rdw can use just that part.

# [100] The most important change: formatters move out of rdw-server and into
# the user's shell. rdw-server goes back to what bash-rd always was at its
# core — a conduit between a process and a browser, with a hook for the user's
# display code. Everything else is composition.

---

## Component dependency graph

```
rdw-compose (bash)
  └── generates shell pipeline using:
        ├── any filter command (grep, awk, sed, python3, ...)
        └── rdw-relay (Go)
              └── rdw-server (Go)
                    ├── rdw-kv (embedded, REST)
                    ├── rdw-layout (embedded, REST)
                    └── rdw-ws-hub (embedded, WebSocket)
                          └── browser SPA or custom frontend

rdw-cli (Go) ──────────── rdw-server (REST)
rdw-cycle (bash) ────────── rdw-cli
rdw-kv-watch (bash) ──────── rdw-cli
rdw-fuse (Go, optional) ───── rdw-server (REST)
user formatters (any) ──── called by rdw-server per line
```

Every arrow is either a REST call, a shell pipe, or a Unix socket command.
Nothing is a function call across component boundaries at runtime.
