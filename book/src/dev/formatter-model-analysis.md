# rdw formatter model — design analysis

This document analyses the tension between bash-rd's formatter model and
rdw's current architecture, and presents concrete design options with
honest pros and cons.

## What bash-rd actually does

bash-rd has **one processing concept**: the formatter. There are no filters.
The formatter:

- Is a bash function sourced into the server process
- Is called on every line: `formatter "$rd_line"`
- Has direct access to `$rd_line` (the current line) and all KV variables
  (`$rd_kv_mykey` etc.) in its bash environment
- Has no access to previous lines — there is no scrollback
- Must maintain its own accumulated state if it needs it (shell variables,
  temp files, or by writing to a named pipe)
- Outputs whatever it wants: plain text, ANSI sequences, or HTML depending
  on the display target

The server calls the formatter and sends its output to the browser (or
terminal). The server does not store anything. There is no buffer between
the formatter and the display.

This is the Unix model: the formatter is the user's program. The server is
a conduit with a hook.

---

## The four tensions

### Tension 1: formatter lifetime

**bash-rd model:** one formatter invocation per line, always fresh. The
formatter starts, reads `$rd_line`, writes output, exits. KV is always
current because the bash environment is re-read from the server's variables
on each call (they are in the same process).

**Efficiency problem:** if the formatter needs to process a burst of 10,000
lines, it spawns 10,000 subprocesses. This is fine for interactive debugging
(bash-rd's intended use) but not for high-throughput log display.

**rdw's current approach:** `CmdFilter` keeps a subprocess alive across lines
(long-lived). This is efficient but means KV must be re-injected from outside
the subprocess — the subprocess cannot read new KV values after it starts.

The tradeoff is stark:

| Model | Subprocess cost | KV freshness | State accumulation |
| --- | --- | --- | --- |
| Per-line (bash-rd) | One process per line | Always current | Via shell vars in parent |
| Long-lived | One process total | Stale after start | Natural (process memory) |
| Per-line with env inject | One process per line | Always current | None (stateless) |
| Long-lived with signal | One process total | On signal/request | Natural |

### Tension 2: KV freshness vs formatter statefulness

These two desirable properties are in direct conflict in any long-lived
subprocess model:

- **KV freshness**: the formatter sees the current value of every KV key on
  every line. In bash-rd this is free because the formatter runs in the same
  bash process as the server.
- **Formatter statefulness**: the formatter can accumulate data across lines
  (count errors, build a table, track a running average). This requires the
  formatter to persist between line invocations.

If the formatter is long-lived, it cannot see KV changes after it starts
(unless we add a side channel). If the formatter is per-line, it cannot
accumulate state across lines (unless we use shared storage outside the
formatter).

bash-rd sidesteps this by keeping the formatter in the same process. rdw
cannot do this cleanly (Go is not bash; you cannot `source` an arbitrary
script into a Go process).

### Tension 3: the scrollback buffer

bash-rd has no scrollback. The browser receives lines and displays them.
To "update" a previously displayed line, bash-rd re-sends the entire screen
(or uses ANSI sequences, which only work in a terminal, not a browser).

rdw's scrollback buffer solves a real problem: a new browser connection can
replay what it missed. But it creates a second problem: the scrollback is
now a source of truth that the formatter does not control. If the formatter
wants to replace line 42, it cannot — the scrollback is append-only.

This matters for formatters that want to maintain a live summary:

```sh
# bash-rd: formatter re-emits the full table on every line
# Works because there is no scrollback to contradict it
formatter() {
  update_table "$rd_line"
  print_full_table   # browser replaces previous display
}
```

In rdw, every line the formatter emits is appended to the scrollback. If
the formatter re-emits the full table on every line, the scrollback grows
without bound and the browser shows 1000 copies of the table.

### Tension 4: transport purity vs display intelligence

The unix-analysis.md recommendation is that rdw should be a pure transport.
But bash-rd is also not a pure transport — it runs the formatter (user code)
in the server process. The server is dumb about the content but smart about
invocation.

The real question is not "is the server dumb?" but "what is the right
interface between the server and the user's code?"

- bash-rd's interface: `$rd_line` and `$rd_kv_*` variables in bash
- rdw's filter interface: stdin/stdout per line, KV as environment
- rdw's formatter interface: stdin (full scrollback), stdout (HTML)

None of these is a pure transport. All of them run user code.

---

## Solution options

### Option A — Per-line formatter, stateless, KV-fresh (bash-rd faithful)

Remove the scrollback buffer. Remove filters. The formatter is the only
processing stage: called once per line, receives the line on stdin and KV
as environment, writes output (text or HTML) to stdout.

```
stdin line
  │
  ▼
formatter subprocess (per line)
  - stdin: one line
  - env: full KV snapshot
  - stdout: display line (text or HTML)
  │
  ▼
browser (append-only, no stored state)
```

**Pros:**
- Exact bash-rd model — simple, predictable, well-understood
- KV always fresh
- No scrollback inconsistency
- Formatter is stateless by default; user can add state via files/pipes if needed
- `rdw pipe` becomes a pure transport

**Cons:**
- One subprocess per line — high cost for busy streams (10k lines/s = 10k processes/s)
- No replay for new browser connections (no scrollback)
- No export (nothing stored)
- Existing rdw features (bookmark, export, `rdw status pane`) lose their data source
- Users accustomed to rdw's scrollback lose it

**Best for:** interactive debugging (bash-rd's use case), low-volume streams,
maximum Unix purity.

---

### Option B — Long-lived formatter, stateful, KV via signal

The formatter is a long-lived subprocess. It reads lines from stdin continuously.
When KV changes, the server signals the formatter (SIGUSR1) and the formatter
re-reads a KV file:

```
stdin line
  │
  ▼
formatter subprocess (long-lived, reads stdin line by line)
  - stdin: continuous line stream
  - SIGUSR1: re-read /tmp/rdw-kv-{id}.json
  - stdout: display lines (text or HTML)
  │
  ▼
thin ring buffer (last N lines only, for reconnect replay)
  │
  ▼
browser
```

**Pros:**
- Efficient — one subprocess total, no spawn overhead
- Formatter can accumulate state naturally (process variables)
- KV updates are possible (via signal + file) but explicit
- Small ring buffer (last N lines) enables reconnect without full scrollback

**Cons:**
- Signal-based KV update is non-standard and fragile (SIGUSR1 races, file I/O)
- Formatter must handle SIGUSR1 — adds complexity to user's formatter script
- Long-lived subprocess makes error recovery harder (what if it crashes?)
- Stateful formatters are harder to test and reason about
- Not composable with standard Unix tools (a formatter that dies takes down the pane)

**Best for:** high-throughput streams, stateful display (tables, counters,
running averages).

---

### Option C — Two modes: per-line (default) and long-lived (opt-in)

The formatter has two modes selected at registration:

```sh
rdw pane formatter set log --cmd './fmt.sh'           # per-line (default)
rdw pane formatter set log --cmd './fmt.sh' --stateful # long-lived
```

Per-line mode: KV fresh, no state.
Long-lived mode: stateful, KV at start time only (or via explicit refresh).

**Pros:**
- Covers both use cases explicitly
- User chooses the tradeoff
- Per-line is the safe default (bash-rd compatible)

**Cons:**
- Two modes to understand, test, and document
- Long-lived mode still has the KV staleness problem
- Formatter scripts must be written differently for each mode
- Complexity doubles the surface area

**Best for:** a tool trying to serve both interactive debugging and
high-throughput log display simultaneously.

---

### Option D — Keep scrollback, add formatter-controlled replacement

Keep the scrollback buffer but add a protocol for the formatter to **replace**
previously sent lines rather than append. The formatter emits tagged lines:

```
@replace:42:new content for line 42
@clear
new content line 1
new content line 2
```

`@replace:N:content` replaces line N in the scrollback and updates the browser.
`@clear` clears the scrollback.

The formatter is per-line (bash-rd model) but gains the ability to modify the display.

**Pros:**
- Per-line model preserved (KV always fresh)
- Enables live-updating displays (tables, counters) without re-sending full screen
- Scrollback replay still works (stores final state)
- Composable with existing rdw features

**Cons:**
- `@replace` is a new protocol — more to learn, more to implement
- Line numbers in the scrollback are fragile (lines can be added/removed)
- Browser must handle replace events efficiently
- Formatter must track which line numbers to replace — adds state burden to the formatter

**Best for:** dashboards that need to update in-place (progress bars, metric tables).

---

### Option E — Formatter is a WebSocket client (maximum flexibility)

Remove the formatter from the pipeline entirely. The formatter is an independent
process that connects to rdw's WebSocket, receives lines, and sends display
commands back:

```
stdin → rdw pipe → server → WebSocket → formatter process
                                   ↑
                    display commands (HTML fragments, replacements, etc.)
```

The formatter is just another WebSocket client. It can read KV via REST,
send formatted output via `POST /api/v1/stream/{id}`, and update the display
in any way the API supports.

**Pros:**
- Maximum flexibility — formatter can do anything the API supports
- KV always fresh (formatter polls or subscribes)
- Formatter can be written in any language
- State accumulation is trivial (process memory)
- No coupling between formatter and pipeline

**Cons:**
- Not a Unix pipe — it is a daemon
- Much more complex to write a formatter
- Two network connections per pane (input stream + formatter connection)
- Formatter must handle reconnect, auth, etc.
- Violates the "formatter is a simple script" principle

**Best for:** complex, stateful displays that need full control. Not for
casual use.

---

## Recommended approach: Option A with a thin ring buffer

The purest answer is Option A (bash-rd faithful, per-line, stateless, KV-fresh)
with one concession to rdw's browser-display use case: a **thin ring buffer**
of the last N formatted lines (default: 200) for reconnect replay.

This gives:

- Exact bash-rd semantics for the formatter
- KV always fresh
- Efficient enough for interactive use (the intended use case — not 10k lines/s)
- Reconnect replay for browser sessions that briefly disconnect
- Export of the last N lines (limited but honest)

The scrollback buffer is replaced by a ring buffer scoped to display, not
to data. The distinction: the scrollback buffer is a data store; the ring
buffer is a display cache.

```
stdin line
  │
  ▼
formatter (per line, env=KV, stdin=line, stdout=display output)
  │
  ▼
ring buffer (last N display outputs, for reconnect only)
  │
  ▼
browser (append display outputs, no state model)
```

For the high-throughput case (streaming logs at scale), the answer is not
rdw — it is a dedicated log tool (Loki, Splunk, ELK). rdw is a debugging
and introspection tool. It should be excellent at that and honest about
what it is not.

---

## What this means for rdw's existing features

| Feature | Keep? | Notes |
| --- | --- | --- |
| Scrollback buffer | Replace with ring buffer | Reconnect only, not a data store |
| Export | Limited to ring buffer | Export last N lines |
| Bookmarks | Remove | No stable line indices without scrollback |
| `rdw status pane` scrollback_len | Replace with ring_len | |
| Filters (`--filter`) | Remove | Formatter covers this use case |
| `POST /api/v1/panes/{id}/format` | Remove | No scrollback to format |
| KV injection into formatter | Keep, always fresh | Core bash-rd behaviour |
| `f:` formatter switch | Keep | Formatter is per-pane, switchable |
| `sc:clear` | Keep | Clears ring buffer + browser |
| `image:` sentinel framing | Remove (as per Rec 5) | Use `f:image` + `base64` |

---

## The scrollback question directly answered

Should rdw have a scrollback buffer?

**If rdw is a debugging tool (bash-rd's intent):** No. The formatter decides
what the display shows. The server is a conduit. A scrollback contradicts the
formatter's authority over the display.

**If rdw is a log viewer (a different tool):** Yes. A log viewer needs to
store lines for search, export, and replay.

These are two different tools. rdw started as one and grew into the other.
The Unix redesign should pick one and be excellent at it.

The recommendation: rdw is a debugging and introspection tool. The scrollback
becomes a ring buffer (display cache). Users who need log storage should pipe
to a file with `tee` — that is what `tee` is for.
