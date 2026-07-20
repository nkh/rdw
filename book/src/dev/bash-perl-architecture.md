# rdw rewrite — bash/perl implementation architecture

---

## What we are doing and why

This document describes a complete rewrite of rdw using bash and perl instead
of Go. Before explaining the components, it is worth being clear about what
rdw actually does, so the rewrite choices make sense.

**What rdw does at its core:**

A process writes output to its stdout. A shell pipe connects that stdout to
rdw. rdw sends each line of that output to a web server. The web server
pushes those lines to a browser over a WebSocket connection. The browser
displays them in a named pane inside a window layout. That is the entire
core function.

Everything else — filtering, formatting, authentication, KV store, layout
management, export, bookmarks, highlight profiles — is built on top of that
core. The Go rewrite packed all of these into one binary. The bash/perl
rewrite separates them into independent programs that communicate over Unix
pipes, sockets, and files.

**Why bash and perl instead of Go:**

Go is excellent for high-throughput network servers. But rdw is primarily a
debugging and introspection tool used by one developer at a time, not a
production system serving thousands of requests per second. The overhead of
a per-line subprocess (a formatter) or a polling loop (KV watch) is
acceptable at the scale rdw operates. In exchange, every component is
readable and modifiable by any Unix user without a compiler.

Perl is chosen (over Python) because it is universally available on Unix
systems, has no external dependencies for HTTP and JSON work beyond CPAN
modules that ship with most distributions, and is traditionally the language
of Unix text processing and CGI — exactly the two things rdw's server needs.

Bash is chosen for everything that is composition of existing Unix tools:
relaying lines, composing pipelines, cycling windows, watching KV keys.

**What we are keeping:**

All current rdw functionality is accounted for in this document. For each
feature we state whether it is kept, simplified, made optional, or removed
with a shell equivalent provided.

**The key conceptual shift:**

The Go rdw is one program that does many things. The bash/perl rdw is a
collection of small programs that each do one thing and are composed in
the shell. The user experience is identical for casual use (rdw-compose
handles the composition). For power users, each component is independently
useful and replaceable.

---

## Tool dependencies

The following existing Unix tools are used instead of writing equivalent Go code.

| Tool | Purpose | Why not write it |
| --- | --- | --- |
| `websocat` | WebSocket client/server from shell | Eliminates all WS client code |
| `socat` | Unix socket relay, TCP relay | Eliminates socket management code |
| `jq` | JSON parsing and generation | Eliminates all JSON code in bash |
| `perl + HTTP::Daemon` | HTTP server | Ships with perl; no CPAN install |
| `perl + JSON::PP` | JSON encoding/decoding | Ships with perl core since 5.14 |
| `perl + DBI + DBD::SQLite` | SQLite KV persistence | Available on most systems |
| `inotifywait` | File change detection (Linux) | Eliminates polling for KV watch |
| `tee` | Stream duplication | Already exists; no mirror module needed |
| `ts` (moreutils) | Timestamp prepend | Eliminates `t:` control sequence |
| `base64` | Binary encoding | Eliminates `image:end` sentinel framing |
| `nc` / `ncat` | TCP line relay fallback | When socat unavailable |
| `mkfifo` | Named pipes for event bus | Eliminates custom event system |
| `flock` | File locking for registry | Safe concurrent server registry |
| `logrotate` | Log management | Replaces export/scrollback entirely |
| `pandoc` / `cmark` | Markdown rendering | Replaces built-in markdown formatter |
| `jq` | JSON pretty-print | Replaces built-in json formatter |
| `column` | Table formatting | Replaces built-in csv formatter |

---

## Component overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  USER INTERFACE                                                     │
│                                                                     │
│  rdw          rdw-compose      browser                              │
│  (bash)       (bash)           SPA (HTML/JS) or custom             │
│  thin REST    pipeline         served by rdw-server                 │
│  client       composer                                              │
└──────┬────────────┬────────────────────────────────────────────────┘
       │            │
       ▼            ▼
┌─────────────────────────────────────────────────────────────────────┐
│  rdw-server (perl)                                                  │
│                                                                     │
│  HTTP::Daemon  WebSocket hub   Session mgr   KV store               │
│  REST API      (via websocat)  windows/panes (JSON file +           │
│  ~20 endpoints event fanout    layout mgmt   optional SQLite)       │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
              ┌────────────────┴───────────────┐
              ▼                                ▼
┌─────────────────────┐            ┌───────────────────────┐
│  rdw-relay (bash)   │            │  user formatters       │
│  stdin → socket     │            │  bash/perl scripts     │
│  pure transport     │            │  per-line, KV-injected │
│  ~30 lines bash     │            │  HTML or text output   │
└─────────────────────┘            └───────────────────────┘
```

---

## Module 1 — rdw-server (perl)

### What it is

The only persistent process. Written in perl using `HTTP::Daemon` (ships with
perl core) and `JSON::PP` (ships with perl core since 5.14). websocat handles
the WebSocket transport.

### What it is not

rdw-server does not filter lines. It does not store scrollback. It does not
run built-in formatters. It does not manage tokens beyond checking them. It
does not cycle windows. These are other programs' concerns.

### Architecture

rdw-server is a single perl script that forks into three cooperating processes:

```
rdw-server
  │
  ├── http_worker (perl, HTTP::Daemon)
  │     Handles all REST API calls
  │     Reads/writes session state from shared JSON file
  │     Reads/writes KV from shared JSON file (or SQLite)
  │
  ├── ws_hub (websocat + bash glue)
  │     websocat --server listens for browser WS connections
  │     Lines arrive on a named pipe: /run/rdw/ID/stream
  │     hub_fanout.sh reads the pipe and writes to all WS clients
  │
  └── event_bus (named pipes)
        /run/rdw/ID/events   -- all events as NDJSON
        /run/rdw/ID/stream   -- line stream for ws_hub
        /run/rdw/ID/kv       -- KV change events
```

### The session state file

Session state (windows, panes, layout) is stored as a JSON file:

```
/run/rdw/ID/session.json
```

All processes read and write it under `flock`. This replaces the Go
`session.Manager` struct. Perl's `JSON::PP` reads and writes it.

```perl
# read session state
sub read_session {
    my $path = "$RDW_RUN/session.json";
    open my $fh, '<', $path or return {windows=>[], active=>0};
    flock $fh, LOCK_SH;
    my $data = do { local $/; <$fh> };
    close $fh;
    return decode_json($data);
}

# write session state (exclusive lock)
sub write_session {
    my ($state) = @_;
    my $path = "$RDW_RUN/session.json";
    open my $fh, '>', $path or die "cannot write session: $!";
    flock $fh, LOCK_EX;
    print $fh encode_json($state);
    close $fh;
    # Notify watchers via event bus
    emit_event({type => 'layout_update', payload => $state});
}
```

### The KV store

KV is a JSON file `/run/rdw/ID/kv.json` with optional SQLite backing.

```perl
# Write a KV pair
sub kv_set {
    my ($key, $value) = @_;
    my $kv = read_kv();
    $kv->{$key} = $value;
    write_kv($kv);
    emit_event({type => 'kv_update', key => $key, value => $value});
    # Also write to SQLite if persistence enabled
    if ($ENV{RDW_KV_PERSIST}) {
        my $dbh = DBI->connect("dbi:SQLite:$ENV{RDW_KV_PERSIST}");
        $dbh->do("INSERT OR REPLACE INTO kv VALUES (?,?)", undef, $key, $value);
    }
}
```

### The WebSocket hub

websocat provides the WebSocket server. The hub is a bash script that reads
from the stream named pipe and writes to all connected clients.

```bash
#!/bin/bash
# rdw-ws-hub.sh -- fan out lines from stream pipe to WebSocket clients
# Called by rdw-server after starting websocat

RDW_RUN=${RDW_RUN:-/run/rdw/default}
CLIENTS_DIR="$RDW_RUN/clients"
STREAM="$RDW_RUN/stream"

mkdir -p "$CLIENTS_DIR"
mkfifo "$STREAM" 2>/dev/null || true

# websocat in server mode: each connection gets its own pipe in CLIENTS_DIR
websocat --server "ws://127.0.0.1:${RDW_WS_PORT:-7682}" \
  --exec-cmd "bash -c 'cat > $CLIENTS_DIR/\$\$; trap \"rm -f $CLIENTS_DIR/\$\$\" EXIT; cat'" &

# Fan-out loop: read from stream pipe, write to all client pipes
while IFS= read -r line; do
  for client in "$CLIENTS_DIR"/*; do
    [ -p "$client" ] && echo "$line" > "$client" 2>/dev/null || true
  done
done < "$STREAM"
```

### The REST API (perl HTTP::Daemon)

```perl
#!/usr/bin/env perl
use strict; use warnings;
use HTTP::Daemon; use HTTP::Response; use HTTP::Status;
use JSON::PP; use POSIX qw(WNOHANG);

my $d = HTTP::Daemon->new(
    LocalAddr => '127.0.0.1',
    LocalPort => $ENV{RDW_PORT} // 7681,
    ReuseAddr => 1,
) or die "cannot start server: $!";

while (my $c = $d->accept) {
    my $r = $c->get_request or next;
    my $path   = $r->uri->path;
    my $method = $r->method;
    my $body   = $r->content;

    my $response = route($method, $path, $body);
    $c->send_response($response);
    $c->close;
}

sub route {
    my ($method, $path, $body) = @_;
    my $params = $body ? eval { decode_json($body) } // {} : {};

    # Windows
    return create_window($params)     if $method eq 'POST'   && $path eq '/api/v1/windows';
    return list_windows()             if $method eq 'GET'    && $path eq '/api/v1/windows';
    return focus_window($path,$params) if $method eq 'POST'  && $path =~ m|/windows/([^/]+)/focus|;
    # KV
    return kv_get($path)              if $method eq 'GET'    && $path =~ m|/kv/(.+)|;
    return kv_set($path,$params)      if $method eq 'PUT'    && $path =~ m|/kv/(.+)|;
    # Stream ingest
    return ingest($path,$params)      if $method eq 'POST'   && $path =~ m|/stream/(.+)|;
    # Status
    return server_status()            if $method eq 'GET'    && $path eq '/api/v1/status';
    # Frontend
    return serve_spa()                if $method eq 'GET'    && $path eq '/';

    return HTTP::Response->new(RC_NOT_FOUND);
}
```

### Line ingest and formatter dispatch

When a line arrives at `POST /api/v1/stream/{id}`, the server:
1. Looks up any registered formatter for that pane (stored in KV as `fmt.ID.cmd`)
2. If found, calls it per-line with the current KV as environment
3. Writes the output to the stream named pipe (which the WS hub fans out)

```perl
sub ingest {
    my ($path, $params) = @_;
    my ($id) = $path =~ m|/stream/(.+)|;
    my $line  = $params->{line} // return HTTP::Response->new(RC_BAD_REQUEST);
    my $kv    = read_kv();
    my $fmt   = $kv->{"fmt.$id.cmd"};

    my $display = $fmt ? run_formatter($fmt, $line, $kv) : $line;

    # Write to stream pipe for WS hub
    open my $fh, '>>', "$RDW_RUN/stream" or return HTTP::Response->new(RC_INTERNAL_SERVER_ERROR);
    print $fh encode_json({type=>'line', target_id=>$id, line=>$display}) . "\n";
    close $fh;

    # Append to ring buffer (last 200 lines per pane)
    append_ring($id, $display);

    return HTTP::Response->new(RC_NO_CONTENT);
}

sub run_formatter {
    my ($cmd, $line, $kv) = @_;
    # Build environment: current KV keys injected as env vars
    my %env = (%ENV, map { $_ => $kv->{$_} } keys %$kv);
    # Run formatter with line on stdin
    local %ENV = %env;
    open my $in,  '<', \$line;
    open my $out, '>', \my $result;
    open my $fh,  '-|', "sh", "-c", $cmd or return $line;
    # Feed line to formatter stdin
    print $fh "$line\n";
    $result = <$fh>;
    close $fh;
    return $result // $line;
}
```

### Use cases for rdw-server outside this project

- Any project needing a minimal Perl HTTP + WebSocket server for real-time
  browser display of backend data
- A simple dashboard server for shell script output without a Node.js stack
- A named-pipe-to-WebSocket bridge for any Unix tool that writes to a pipe
- A lightweight REST API for session state stored as JSON files (no database needed)
- A building block for any browser-integrated debugging tool

---

## Module 2 — rdw-relay (bash, ~40 lines)

### What it is

Reads stdin line by line, sends each line to rdw-server via Unix socket or
HTTP. The simplest component. Could be a shell function.

```bash
#!/usr/bin/env bash
# rdw-relay -- relay stdin to a named rdw pane
# Usage: prog | rdw-relay --id PANE_ID [--port PORT]

set -euo pipefail

ID="" PORT=7681 TOKEN=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --id)    ID=$2;    shift 2 ;;
    --port)  PORT=$2;  shift 2 ;;
    --token) TOKEN=$2; shift 2 ;;
    *) echo "unknown: $1" >&2; exit 1 ;;
  esac
done
[[ -z $ID ]] && { echo "rdw-relay: --id required" >&2; exit 1; }

SOCK="${XDG_RUNTIME_DIR:-/run/user/$UID}/rdw/${PORT}/rdw.sock"
BASE="http://127.0.0.1:$PORT/api/v1"
AUTH=${TOKEN:+-H "Authorization: Bearer $TOKEN"}

# Reconnect queue: buffer lines when server unavailable
QUEUE=""
MAX_QUEUE=1000

send_line() {
    local line=$1
    local payload
    payload=$(jq -Rn --arg l "$line" --arg id "$ID" \
        '{line: $l, target_id: $id}')

    # Try Unix socket first (owner auth, no token needed)
    if [[ -S $SOCK ]]; then
        echo "$payload" | socat - "UNIX-CONNECT:$SOCK" 2>/dev/null && return 0
    fi

    # Fall back to HTTP
    curl -sf -X POST "$BASE/stream/$ID" \
        $AUTH \
        -H 'Content-Type: application/json' \
        -d "$payload" > /dev/null 2>&1 && return 0

    # Buffer on failure
    local count
    count=$(echo "$QUEUE" | wc -l)
    if (( count < MAX_QUEUE )); then
        QUEUE="${QUEUE}${line}"$'\n'
    fi
    return 1
}

flush_queue() {
    local saved="$QUEUE"
    QUEUE=""
    while IFS= read -r line; do
        [[ -z $line ]] && continue
        send_line "$line" || { QUEUE="${QUEUE}${line}"$'\n'; return 1; }
    done <<< "$saved"
}

while IFS= read -r line; do
    [[ -n $QUEUE ]] && flush_queue
    send_line "$line"
done
```

### Use cases outside this project

- Relay any Unix process output to any HTTP endpoint (change the URL)
- A buffered HTTP POST relay for unreliable network connections
- Drop-in replacement for `logger` that sends to a web endpoint instead of syslog
- Build custom monitoring pipelines: `tail -f app.log | rdw-relay --id monitor`

---

## Module 3 — rdw-compose (bash, ~120 lines)

### What it is

Translates rdw-style arguments into a shell pipeline. Prints the generated
pipeline before running it. Covers all the flags removed from rdw-relay.

```bash
#!/usr/bin/env bash
# rdw-compose -- compose a shell pipeline from rdw pipe arguments
# Usage: prog | rdw-compose [options]
# Always prints the generated pipeline to stderr before running

set -euo pipefail

ID="" PORT=7681 TITLE="" LAYOUT=""
FORWARD_FILE="" FORWARD_CMD="" FORWARD_RD=false
DRY_RUN=false
declare -a FILTERS=()

while [[ $# -gt 0 ]]; do
  case $1 in
    --id)               ID=$2;              shift 2 ;;
    --port)             PORT=$2;            shift 2 ;;
    --title)            TITLE=$2;           shift 2 ;;
    --layout)           LAYOUT=$2;          shift 2 ;;
    --filter)           FILTERS+=("$2");    shift 2 ;;
    --forward-to-file)  FORWARD_FILE=$2;    shift 2 ;;
    --forward-to-cmd)   FORWARD_CMD=$2;     shift 2 ;;
    --forward)          FORWARD_RD=true;    shift ;;
    --dry-run)          DRY_RUN=true;       shift ;;
    @*)
      # Named profile: source from ~/.config/rdw/profiles/
      profile="${1#@}"
      src="$HOME/.config/rdw/profiles/$profile.sh"
      [[ -f $src ]] && source "$src" || { echo "unknown profile: $profile" >&2; exit 1; }
      shift ;;
    *) echo "unknown: $1" >&2; exit 1 ;;
  esac
done

[[ -z $ID ]] && { echo "rdw-compose: --id required" >&2; exit 1; }

# Setup commands (run before pipeline)
declare -a SETUP=()
[[ -n $LAYOUT ]] && SETUP+=("rdw layout apply $(printf '%q' "$LAYOUT") --port $PORT")
[[ -n $TITLE  ]] && SETUP+=("rdw pane rename $(printf '%q' "$ID") $(printf '%q' "$TITLE") --port $PORT")

# Build relay with mirroring
RELAY="rdw-relay --id $(printf '%q' "$ID") --port $PORT"
[[ -n $FORWARD_FILE && -n $FORWARD_CMD ]] && \
    RELAY="tee $(printf '%q' "$FORWARD_FILE") >(${FORWARD_CMD}) | $RELAY"
[[ -n $FORWARD_FILE && -z $FORWARD_CMD  ]] && \
    RELAY="tee $(printf '%q' "$FORWARD_FILE") | $RELAY"
[[ -z $FORWARD_FILE && -n $FORWARD_CMD  ]] && \
    RELAY="tee >(${FORWARD_CMD}) | $RELAY"
$FORWARD_RD && \
    RELAY="tee >(rd -c $(printf '%q' "$ID") 2>/dev/null || true) | $RELAY"

# Build filter chain
STAGES=""
for f in "${FILTERS[@]}"; do
    STAGES="${STAGES:+$STAGES | }$f"
done

PIPELINE="${STAGES:+$STAGES | }$RELAY"

# Always show generated pipeline
{
    echo "# rdw-compose:"
    for s in "${SETUP[@]}"; do echo "#   $s"; done
    echo "#   stdin | $PIPELINE"
} >&2

$DRY_RUN && exit 0

# Run setup
for cmd in "${SETUP[@]}"; do eval "$cmd"; done

# Execute pipeline
exec bash -c "cat | $PIPELINE"
```

### Profile system

```bash
# Save a profile
rdw-compose --id log --filter 'grep ERROR' --title "App Log" \
    --forward-to-file /tmp/app.log --save-profile myapp

# Profile is stored as a plain shell script:
# ~/.config/rdw/profiles/myapp.sh
# #!/usr/bin/env bash
# # rdw-compose profile: myapp
# ID=log PORT=7681 TITLE="App Log"
# FORWARD_FILE=/tmp/app.log
# FILTERS=("grep ERROR")

# Use it:
prog | rdw-compose @myapp
```

### Use cases outside this project

- A general-purpose pipeline composer for any tool with filter/mirror/setup flags
- Template for "smart argument parsers" that decompose into shell primitives
- Teaching tool: shows students what shell composition looks like for complex pipelines
- CI pipeline builder: `rdw-compose --dry-run` generates the pipeline for documentation

---

## Module 4 — rdw (bash, thin REST client, ~200 lines)

### What it is

The user-facing CLI. Every subcommand maps to exactly one REST call. No local
logic. Uses curl + jq.

```bash
#!/usr/bin/env bash
# rdw -- thin REST client for rdw-server

set -euo pipefail

RDW_PORT=${RDW_PORT:-7681}
RDW_TOKEN=${RDW_TOKEN:-$(cat ~/.config/rdw/token 2>/dev/null || true)}
BASE="http://127.0.0.1:$RDW_PORT/api/v1"
AUTH=${RDW_TOKEN:+-H "Authorization: Bearer $RDW_TOKEN"}

api_get()    { curl -sf      "$BASE/$1" $AUTH; }
api_post()   { curl -sf -X POST   "$BASE/$1" $AUTH -H 'Content-Type: application/json' -d "$2"; }
api_put()    { curl -sf -X PUT    "$BASE/$1" $AUTH -H 'Content-Type: application/json' -d "$2"; }
api_patch()  { curl -sf -X PATCH  "$BASE/$1" $AUTH -H 'Content-Type: application/json' -d "$2"; }
api_delete() { curl -sf -X DELETE "$BASE/$1" $AUTH; }

case "${1:-help} ${2:-}" in
  # Server
  "server start") shift 2; exec rdw-server "$@" ;;
  "server stop")  api_post 'server/stop' '{}' ;;
  "server list")  cat "${XDG_CACHE_HOME:-$HOME/.cache}/rdw/servers.json" 2>/dev/null | jq . ;;

  # Windows
  "window create") api_post "windows" "$(jq -n --arg n "$3" '{name:$n}')" ;;
  "window close")  api_delete "windows/$3" ;;
  "window rename") api_patch  "windows/$3" "$(jq -n --arg n "$4" '{name:$n}')" ;;
  "window focus")  api_post   "windows/$3/focus" '{}' ;;
  "window list")   api_get    "windows" | jq -r '.windows[].name' ;;

  # Panes
  "pane split")    api_post   "panes/$3/split" "$(jq -n --arg d "${4:-h}" '{dir:$d}')" ;;
  "pane zoom")     api_post   "panes/$3/zoom" '{}' ;;
  "pane resize")   api_post   "panes/$3/resize" "$(jq -n --arg s "$4" '{size:$s}')" ;;
  "pane rename")   api_patch  "panes/$3" "$(jq -n --arg t "$4" '{title:$t}')" ;;
  "pane close")    api_delete "panes/$3" ;;

  # KV
  "kv set")    api_put    "kv/$3" "$(jq -n --arg v "$4" '{value:$v}')" ;;
  "kv get")    api_get    "kv/$3" | jq -r '.value' ;;
  "kv delete") api_delete "kv/$3" ;;
  "kv list")   api_get    "kv${3:+?prefix=$3}" | jq -r '.keys[]' ;;

  # Layout
  "layout apply") api_post "layouts/$3/apply" '{}' ;;
  "layout save")  api_post "layouts" "$(jq -n --arg n "$3" '{name:$n}')" ;;
  "layout list")  api_get  "layouts" | jq -r '.layouts[]' ;;

  # Formatter
  "formatter register")   rdw kv set "fmt.$3.cmd" "$4" ;;
  "formatter unregister") rdw kv delete "fmt.$3.cmd" ;;
  "formatter list")       rdw kv list fmt. | sed 's/\.cmd$//' | sort -u ;;

  # Status
  "status")      api_get "status" | jq . ;;
  "status pane") api_get "status/panes/$3" | jq . ;;

  # Cycle
  "cycle start") exec rdw-cycle "$3" "${4:-5}" ;;
  "cycle stop")  pkill -f rdw-cycle 2>/dev/null || true ;;

  # Send
  "send")  shift; exec rdw-send "$@" ;;

  # Pipe (backward compat → rdw-compose)
  "pipe")  shift; exec rdw-compose "$@" ;;

  *) echo "usage: rdw COMMAND [args]" >&2; exit 1 ;;
esac
```

### Use cases outside this project

- Template for a "thin CLI over REST API" in bash — copy and change BASE URL
- A curl wrapper pattern that handles auth tokens and JSON cleanly
- Any project needing a shell-accessible REST client without Python or Node

---

## Module 5 — rdw-ws-hub (bash + websocat)

### What it is

Manages WebSocket connections to browsers. Reads from a named pipe, fans
out to all connected clients. websocat does the actual WebSocket protocol.

This replaces the Go `internal/server/hub.go` (123 lines of Go).

```bash
#!/usr/bin/env bash
# rdw-ws-hub -- WebSocket fan-out hub using websocat
# Each browser connects to websocat; lines arrive on $STREAM pipe

set -euo pipefail

RDW_RUN="${RDW_RUN:-/run/rdw/default}"
WS_PORT="${RDW_WS_PORT:-7682}"
STREAM="$RDW_RUN/stream"
CLIENTS="$RDW_RUN/clients"

mkdir -p "$CLIENTS"
mkfifo "$STREAM" 2>/dev/null || true

# Start websocat server. Each incoming connection is handled by a subshell
# that reads from its own named pipe in CLIENTS/.
websocat --server "ws://127.0.0.1:$WS_PORT" \
    -E "bash -c '
        id=\$\$
        pipe=$CLIENTS/\$id
        mkfifo \"\$pipe\"
        trap \"rm -f \$pipe\" EXIT
        cat \"\$pipe\"
    '" &
WS_PID=$!

# Fan-out loop: read from stream, write to all client pipes
# Also append to ring buffer (last 200 lines per pane)
declare -A rings

while IFS= read -r line; do
    # Ring buffer: extract target_id from JSON, store last 200 display lines
    target_id=$(echo "$line" | jq -r '.target_id // empty' 2>/dev/null)
    if [[ -n $target_id ]]; then
        rings[$target_id]+="$line"$'\n'
        # Trim to last 200 lines
        rings[$target_id]=$(echo "${rings[$target_id]}" | tail -200)
    fi

    # Fan out to all connected clients
    for pipe in "$CLIENTS"/*; do
        [[ -p $pipe ]] || continue
        echo "$line" > "$pipe" 2>/dev/null || rm -f "$pipe"
    done
done < <(tail -f "$STREAM")

wait $WS_PID
```

### Reconnect replay

When a browser reconnects, rdw-server sends the ring buffer for each pane.
The ring buffer is stored in memory in the fan-out process and also persisted
to `/run/rdw/ID/rings/PANEID` (200-line text files, one per pane).

```bash
# On reconnect: replay ring buffer for requested pane
replay_pane() {
    local pane_id=$1
    local ring_file="$RDW_RUN/rings/$pane_id"
    [[ -f $ring_file ]] || return 0
    cat "$ring_file"
}
```

### Use cases outside this project

- Any project needing WebSocket fan-out from a shell pipeline
- A real-time log broadcaster: `tail -f app.log | rdw-ws-hub-simple`
- Multi-client notification system using only bash + websocat
- Building block for browser-integrated monitoring without Node.js

---

## Module 6 — rdw-kv (bash + perl, KV store)

### What it is

The session KV store. In the rewrite it is a directory of files
(`/run/rdw/ID/kv/`) where each key is a file. This is maximally Unix-like:
`cat`, `echo`, `find`, and `ls` all work on the KV store directly.

```bash
#!/usr/bin/env bash
# rdw-kv-set -- write a KV key
# Usage: rdw-kv-set KEY VALUE
RDW_RUN="${RDW_RUN:-/run/rdw/default}"
KV_DIR="$RDW_RUN/kv"
mkdir -p "$KV_DIR"
key=$1 value=$2
# Sanitise key: replace : and / with _ for filesystem safety
safe_key="${key//[:\/]/_}"
echo -n "$value" > "$KV_DIR/$safe_key"
# Emit change event
echo "{\"type\":\"kv_update\",\"key\":\"$key\",\"value\":\"$value\"}" \
    >> "$RDW_RUN/events"

#!/usr/bin/env bash
# rdw-kv-get -- read a KV key
RDW_RUN="${RDW_RUN:-/run/rdw/default}"
key=$1
safe_key="${key//[:\/]/_}"
cat "$RDW_RUN/kv/$safe_key" 2>/dev/null || echo ""
```

The `=:key=value` control sequence is handled by rdw-server's ingest path:

```perl
# In rdw-server's ingest: detect =: prefix and write KV
if ($line =~ /^=:(.+)/) {
    for my $pair (split /;/, $1) {
        my ($k, $v) = split /=/, $pair, 2;
        kv_set(trim($k), trim($v));
    }
    return;  # don't display KV lines
}
```

### SQLite persistence (perl)

```perl
# Optional: persist KV to SQLite
sub kv_persist {
    my ($key, $value) = @_;
    return unless $ENV{RDW_KV_PERSIST};
    require DBI;
    my $dbh = DBI->connect("dbi:SQLite:$ENV{RDW_KV_PERSIST}", '', '',
        {RaiseError => 1, AutoCommit => 1});
    $dbh->do("CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value TEXT)");
    $dbh->do("INSERT OR REPLACE INTO kv VALUES (?,?)", undef, $key, $value);
}
```

### Use cases outside this project

- Any shell project needing a lightweight shared key-value store across processes
- Process coordination: `rdw-kv-set build.status running` visible to all watchers
- Configuration distribution: set config values that multiple scripts read
- Simple pub-sub: watch a key, trigger actions when it changes

---

## Module 7 — rdw-kv-watch (bash, ~25 lines)

### What it is

Watches a KV key and runs a command when the value changes. Uses inotifywait
on Linux (from inotify-tools) or a polling fallback.

```bash
#!/usr/bin/env bash
# rdw-kv-watch -- watch a KV key and run CMD on change
# Usage: rdw-kv-watch KEY CMD [--poll N]

set -euo pipefail
KEY=$1 CMD=$2 POLL=${3:-1}
RDW_RUN="${RDW_RUN:-/run/rdw/default}"
safe_key="${KEY//[:\/]/_}"
KV_FILE="$RDW_RUN/kv/$safe_key"

prev=$(cat "$KV_FILE" 2>/dev/null || echo "")

if command -v inotifywait >/dev/null 2>&1; then
    # Efficient: inotify on Linux
    while inotifywait -q -e close_write "$KV_FILE" 2>/dev/null; do
        cur=$(cat "$KV_FILE")
        [[ "$cur" != "$prev" ]] && { prev=$cur; RDW_KV_VALUE="$cur" eval "$CMD"; }
    done
else
    # Fallback: polling
    while true; do
        cur=$(cat "$KV_FILE" 2>/dev/null || echo "")
        [[ "$cur" != "$prev" ]] && { prev=$cur; RDW_KV_VALUE="$cur" eval "$CMD"; }
        sleep "$POLL"
    done
fi
```

### Use cases outside this project

- Trigger deployment scripts when a CI status KV key changes
- Reload nginx config when a KV key changes: `rdw-kv-watch config.version "nginx -s reload"`
- Dashboard automation: change a window's layout based on time-of-day KV key

---

## Module 8 — rdw-cycle (bash, ~25 lines)

Rotates window focus through a list at a configurable interval.

```bash
#!/usr/bin/env bash
# rdw-cycle -- rotate browser focus through windows
# Usage: rdw-cycle WINDOW,WINDOW,... [INTERVAL_SECONDS]

set -euo pipefail
IFS=, read -ra WINS <<< "$1"
INTERVAL=${2:-5}
i=0

# Register in KV so rdw status shows cycle state
rdw kv set cycle.running true
rdw kv set cycle.windows "$1"
rdw kv set cycle.interval_ms "$(( INTERVAL * 1000 ))"
trap 'rdw kv set cycle.running false' EXIT

while true; do
    rdw window focus "${WINS[$i]}"
    i=$(( (i + 1) % ${#WINS[@]} ))
    sleep "$INTERVAL"
done
```

---

## Module 9 — The windowing system in bash/perl

### Why this is the most complex part

The windowing system (windows, panes, split/resize/zoom/swap, layout YAML)
is the hardest component to move from Go. In Go it is a typed struct tree
with mutexes. In bash/perl it must be a JSON file with file locking. This
section documents the design in detail.

### Data model

The session state is a JSON file:

```json
{
  "schema_version": 1,
  "active_window": 0,
  "windows": [
    {
      "name": "build",
      "panes": [
        {
          "target_id": "log",
          "title": "Build Log",
          "split": "h",
          "size": "60%",
          "group": "",
          "private": false,
          "formatter": ""
        }
      ]
    }
  ]
}
```

### Reading and writing safely in perl

```perl
#!/usr/bin/env perl
# rdw-session.pm -- session state management
package RDW::Session;
use strict; use warnings;
use JSON::PP; use Fcntl qw(:flock);

my $SESSION_FILE = $ENV{RDW_RUN} . '/session.json';

sub read {
    open my $fh, '<', $SESSION_FILE or return _empty();
    flock $fh, LOCK_SH;
    my $json = do { local $/; <$fh> };
    close $fh;
    return decode_json($json);
}

sub write {
    my ($state) = @_;
    open my $fh, '>', $SESSION_FILE or die "session write: $!";
    flock $fh, LOCK_EX;
    print $fh encode_json($state) . "\n";
    close $fh;
    _emit_layout_update($state);
}

sub _emit_layout_update {
    my ($state) = @_;
    my $event = encode_json({type => 'layout_update', payload => $state});
    open my $fh, '>>', $ENV{RDW_RUN} . '/events' or return;
    flock $fh, LOCK_EX;
    print $fh "$event\n";
    close $fh;
}

sub _empty { {schema_version=>1, active_window=>0, windows=>[]} }

# Window operations
sub create_window {
    my ($name) = @_;
    my $s = read();
    die "window exists: $name\n" if grep { $_->{name} eq $name } @{$s->{windows}};
    push @{$s->{windows}}, {name => $name, panes => []};
    write($s);
}

sub focus_window {
    my ($name) = @_;
    my $s = read();
    my ($idx) = grep { $s->{windows}[$_]{name} eq $name } 0..$#{$s->{windows}};
    defined $idx or die "window not found: $name\n";
    $s->{active_window} = $idx;
    write($s);
}

sub add_pane {
    my ($window_name, $pane) = @_;
    my $s = read();
    my ($win) = grep { $_->{name} eq $window_name } @{$s->{windows}};
    die "window not found: $window_name\n" unless $win;
    die "max panes reached\n" if @{$win->{panes}} >= 64;
    push @{$win->{panes}}, $pane;
    write($s);
}

sub find_pane {
    my ($target_id) = @_;
    my $s = read();
    for my $win (@{$s->{windows}}) {
        for my $pane (@{$win->{panes}}) {
            return ($win, $pane) if $pane->{target_id} eq $target_id;
        }
    }
    return ();
}

sub set_pane_title {
    my ($target_id, $title) = @_;
    my $s = read();
    for my $win (@{$s->{windows}}) {
        for my $pane (@{$win->{panes}}) {
            if ($pane->{target_id} eq $target_id) {
                $pane->{title} = $title;
                write($s);
                return 1;
            }
        }
    }
    die "pane not found: $target_id\n";
}

sub apply_layout {
    my ($layout) = @_;
    # Layout is a hash with {windows:[{name,panes:[...]}]}
    # Merge: add windows not present, add panes not present
    my $s = read();
    my %existing = map { $_->{name} => 1 } @{$s->{windows}};
    for my $lwin (@{$layout->{windows}}) {
        unless ($existing{$lwin->{name}}) {
            push @{$s->{windows}}, {name => $lwin->{name}, panes => $lwin->{panes}};
        }
    }
    write($s);
}

1;
```

### Layout YAML parsing

rdw-server uses perl's `YAML::Tiny` (ships with perl) or calls the system
`yq` tool to parse YAML layouts:

```perl
# Parse layout file using YAML::Tiny (ships with many perl distributions)
use YAML::Tiny;
sub load_layout_file {
    my ($path) = @_;
    my $yaml = YAML::Tiny->read($path) or die "cannot parse $path: $!";
    return $yaml->[0];
}
```

If YAML::Tiny is unavailable, fall back to `yq` (Go tool, widely installed):

```bash
load_layout() {
    local path=$1
    # Convert YAML to JSON using yq, then parse with jq
    yq eval -o=json "$path" | jq .
}
```

### Why the windowing system needs extra documentation

The Go implementation uses mutex-protected struct trees with atomic updates.
In bash/perl, the equivalent is:

1. **All state in one JSON file** — single source of truth, readable by any tool
2. **flock for mutual exclusion** — `flock(1)` wraps any shell command safely
3. **Atomic write** — write to a temp file, `mv` to final path (atomic on same filesystem)
4. **Event emission** — every state change appends to the events FIFO

```bash
# Atomic session write pattern used throughout
atomic_write() {
    local dest=$1 content=$2
    local tmp=$(mktemp "$dest.XXXXXX")
    echo "$content" > "$tmp"
    mv "$tmp" "$dest"
}

# Safe session mutation from bash
mutate_session() {
    local mutation_fn=$1
    (
        flock 9
        local state
        state=$(cat "$RDW_RUN/session.json")
        local new_state
        new_state=$(echo "$state" | jq "$mutation_fn")
        atomic_write "$RDW_RUN/session.json" "$new_state"
        echo "$new_state" | \
            jq -c '{type:"layout_update", payload:.}' >> "$RDW_RUN/events"
    ) 9>"$RDW_RUN/session.lock"
}

# Example: set active window index to 1
mutate_session '.active_window = 1'
```

### Concurrency safety analysis

The flock+atomic-write pattern is safe for the concurrency level rdw operates
at: one user, a handful of concurrent REST requests. It is not safe for high
concurrency (100+ simultaneous writers). This is acceptable because rdw is a
debugging tool, not a production server.

The pattern has one failure mode: if the perl/bash process dies mid-write, the
session file may be incomplete. mitigation: write to a temp file and mv (atomic).

---

## Module 10 — rdw-send (bash, ~50 lines)

Detects file type, sends to pane. Uses `file`, `base64`, standard Unix tools.

```bash
#!/usr/bin/env bash
# rdw-send -- send a file to a named rdw pane
# Usage: rdw-send --id PANE FILE

ID="" PORT=7681
while [[ $# -gt 0 ]]; do
  case $1 in
    --id) ID=$2; shift 2 ;;
    --port) PORT=$2; shift 2 ;;
    *) FILE=$1; shift ;;
  esac
done

mime=$(file --brief --mime-type "$FILE")
ext="${FILE##*.}"

case "$mime" in
    image/png|image/jpeg|image/gif|image/webp)
        { echo "f:image"; base64 "$FILE"; } | rdw-relay --id "$ID" --port "$PORT"
        ;;
    image/svg+xml|*xml*)
        [[ $ext == svg ]] && { echo "f:svg"; cat "$FILE"; } | rdw-relay --id "$ID" --port "$PORT"
        ;;
    text/csv|text/tab-separated-values)
        { echo "f:csv"; cat "$FILE"; } | rdw-relay --id "$ID" --port "$PORT"
        ;;
    text/markdown|text/x-markdown)
        { echo "f:markdown"; cat "$FILE"; } | rdw-relay --id "$ID" --port "$PORT"
        ;;
    *)
        cat "$FILE" | rdw-relay --id "$ID" --port "$PORT"
        ;;
esac
```

---

## Module 11 — Example formatters (bash/perl)

### rdw-fmt-json (bash, uses jq)

```bash
#!/usr/bin/env bash
# rdw-fmt-json -- pretty-print JSON lines using jq
while IFS= read -r line; do
    echo "$line" | jq -C . 2>/dev/null || echo "$line"
done
```

### rdw-fmt-errors (bash)

```bash
#!/usr/bin/env bash
# rdw-fmt-errors -- highlight errors and warnings
# KV: $HIGHLIGHT_COLOR from rdw kv set HIGHLIGHT_COLOR red
while IFS= read -r line; do
    case "$line" in
        *ERROR*) echo "<span class='err'>${line}</span>" ;;
        *WARN*)  echo "<span class='warn'>${line}</span>" ;;
        *)       echo "$line" ;;
    esac
done
```

### rdw-fmt-table (perl)

```perl
#!/usr/bin/env perl
# rdw-fmt-table -- accumulate lines, render as HTML table when complete
# Long-lived mode: keeps running, re-renders on each line
use strict; use warnings;
use HTML::Entities;

my @rows;
my $max_cols = 0;

while (<STDIN>) {
    chomp;
    my @cells = split /\t/, $_;
    push @rows, \@cells;
    $max_cols = @cells if @cells > $max_cols;
    # Re-render entire table on each line
    print render_table(\@rows, $max_cols);
    $| = 1;
}

sub render_table {
    my ($rows, $cols) = @_;
    my $html = "<table class='rdw-table'>\n";
    my $first = 1;
    for my $row (@$rows) {
        my $tag = $first ? 'th' : 'td';
        $html .= "<tr>" . join('', map { "<$tag>" . encode_entities($_//'') . "</$tag>" } @$row) . "</tr>\n";
        $first = 0;
    }
    return $html . "</table>\n";
}
```

---

## Features to remove or simplify

| Feature | Recommendation | Reason |
| --- | --- | --- |
| Built-in formatters (text/json/yaml/markdown/csv/image) | Remove | User provides bash/perl scripts |
| Scrollback buffer (10k lines) | Replace with 200-line ring | `tee` stores; ring is for reconnect |
| Export (`rdw save`) | Remove | `cat tee-file \| pandoc` |
| Bookmarks | Remove | Needs scrollback |
| Highlight profiles API | Simplify to KV keys | `rdw kv set hl.errors.pattern ERROR` |
| Terminal panes | Remove | Use `ttyd` directly |
| `image:`/`svg:` sentinel framing | Remove | `base64 \| rdw-relay` |
| `t:` timestamp sequence | Remove | `prog \| ts \| rdw-relay` |
| `c:` clear sequence | Keep as `sc:clear` | Useful shorthand |
| Filter chain in server | Remove | Shell pipeline before relay |
| `POST /api/v1/panes/{id}/filters` | Remove | Same |
| Focus cycle as server feature | Move to rdw-cycle | External 25-line script |
| Admin page auth token | Simplify | Unix socket access only for admin |
| ANSI CSV column sort (JS) | Remove | Formatter provides its own JS |
| `formatter_set` WS message | Remove | No built-in formatters |
| `image_render`/`svg_render` WS | Remove | Formatter handles display |
| Per-pane token scope | Simplify | Unix socket = no token needed locally |

---

## Comparison table: current Go vs bash/perl rewrite

Lines in the table are wrapped at 145 characters.

```
┌─────────────────────────┬──────────────────────────────────┬───────────────────────────────────┐
│ Feature                 │ Current (Go)                     │ Rewrite (bash/perl)               │
├─────────────────────────┼──────────────────────────────────┼───────────────────────────────────┤
│ HTTP server             │ net/http in Go (~200 lines)      │ perl HTTP::Daemon (~150 lines)    │
│ WebSocket               │ gorilla/websocket (~300 lines)   │ websocat + bash hub (~60 lines)   │
│ Session state           │ Go struct + mutex                │ JSON file + flock                 │
│ KV store                │ Go map + RWMutex                 │ directory of files + flock        │
│ KV persistence          │ mattn/go-sqlite3 (CGO)          │ perl DBI + DBD::SQLite (pure)     │
│ Line relay              │ Go binary + bufio.Scanner        │ bash + curl/socat (~40 lines)     │
│ Filter chain            │ server-side Go function chain    │ shell pipeline before relay       │
│ Built-in formatters     │ 6 Go implementations             │ removed; user provides scripts    │
│ User formatters         │ CmdFormatter Go struct           │ KV key fmt.ID.cmd, called by perl │
│ Pipeline composition    │ flags on rdw pipe                │ rdw-compose bash script           │
│ Stream mirroring        │ --forward-to-file/cmd flags      │ tee in shell (no rdw code)        │
│ Scrollback              │ 10k-line circular buffer (Go)    │ 200-line ring files per pane      │
│ Export                  │ rdw save → Markdown bundle       │ removed; use tee + pandoc         │
│ Bookmarks               │ BookmarkStore Go struct          │ removed with scrollback           │
│ Window management       │ session.Manager Go struct        │ perl RDW::Session + JSON file     │
│ Layout YAML             │ layout.LoadFile Go               │ YAML::Tiny or yq + jq             │
│ Auth tokens             │ SHA-256 in Go auth.Store         │ simplified; Unix socket = no      │
│                         │                                  │ token needed for local use        │
│ Discovery (registry)    │ JSON file + kill -0 (Go)         │ JSON file + kill -0 (bash)        │
│ Focus cycle             │ cycle.Cycle Go struct in server  │ rdw-cycle: 25-line bash script    │
│ KV watch                │ proposed WebSocket subscription  │ rdw-kv-watch: 25-line bash/       │
│                         │                                  │ inotifywait script                │
│ Highlight profiles      │ highlight.Store Go struct        │ KV keys hl.NAME.pattern/class     │
│ Terminal panes          │ terminal.Manager Go struct       │ removed; use ttyd directly        │
│ Admin page              │ /admin HTML served by Go         │ /admin served by perl, same HTML  │
│ Browser SPA             │ embedded in Go binary            │ static files on disk (default     │
│                         │                                  │ shipped; user can replace)        │
│ Event bus               │ WebSocket hub broadcasts         │ named pipe + tail -f; WS via      │
│                         │                                  │ websocat hub                      │
│ Bindings config         │ Go bindings.Store                │ JSON file read by SPA at startup  │
│ Control sequences       │ control.Parse Go function        │ perl regex in ingest path         │
│ Binary image transport  │ image:/image:end sentinel        │ removed; base64 FILE | relay      │
│ rdw send                │ Go binary with magic detection   │ bash + file(1) command            │
│ Test suite              │ 378 Go tests                     │ bats (bash test framework) +      │
│                         │                                  │ perl Test::More                   │
│ Binary size             │ ~15MB static Go binary           │ no compiled binary; ~2000 lines   │
│                         │                                  │ bash + ~500 lines perl            │
│ Build system            │ go build (CGO for SQLite)        │ none; scripts run directly        │
│ Startup time            │ <100ms                           │ <50ms (no JVM/Python startup)    │
│ Lines/sec throughput    │ ~50k/sec (Go goroutines)         │ ~2k/sec (curl per line); socat    │
│                         │                                  │ mode: ~20k/sec                    │
│ Memory per instance     │ ~20MB (Go runtime)               │ ~5MB (perl + bash processes)      │
│ Cross-platform          │ go build for any OS/arch         │ requires bash 4+, perl 5.14+,     │
│                         │                                  │ websocat; Linux/macOS             │
│ Modifiability           │ requires Go compiler             │ any text editor; no build step    │
│ Debuggability           │ go pprof, delve                  │ bash -x, perl -d, strace          │
└─────────────────────────┴──────────────────────────────────┴───────────────────────────────────┘
```

---

## Testing strategy

Each module is independently testable:

```bash
# Test rdw-relay with bats (bash automated testing system)
# test/rdw-relay.bats
@test "relay sends line to mock server" {
    # Start a mock HTTP server that records requests
    nc -l 7681 > /tmp/captured &
    NC_PID=$!
    echo "hello world" | rdw-relay --id test --port 7681
    kill $NC_PID
    grep -q '"line":"hello world"' /tmp/captured
}

# Test KV store directly
@test "kv set and get round-trip" {
    export RDW_RUN=$(mktemp -d)
    rdw-kv-set mykey myvalue
    result=$(rdw-kv-get mykey)
    [ "$result" = "myvalue" ]
}

# Test session management with perl Test::More
# test/session.t
use Test::More;
use RDW::Session;
$ENV{RDW_RUN} = tempdir(CLEANUP => 1);
RDW::Session::create_window("build");
my $s = RDW::Session::read();
is(scalar @{$s->{windows}}, 1);
is($s->{windows}[0]{name}, "build");
done_testing;
```
