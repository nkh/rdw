# rdw Unix redesign — detailed design

This document describes the precise architectural changes needed to implement
the five recommendations from `docs/unix-analysis.md`. Each section covers the
problem in depth, the target design, how the user interacts with it, how the
code changes, and why it improves Unix alignment.

---

## Recommendation 1 — Collapse filter and formatter into one pipeline stage

### The problem

rdw currently has two distinct, non-interchangeable processing stages:

```
stdin
  │
  ▼
Filter (per-line, runs during ingestion, returns plain text)
  │
  ▼
Scrollback buffer (stores plain text lines)
  │
  ▼ (on demand)
Formatter (reads whole scrollback, returns HTML)
  │
  ▼
Browser
```

A filter cannot produce HTML. A formatter cannot see individual lines as they
arrive. A user who wants to syntax-highlight lines as they are ingested — not
on demand — cannot do so. A user who wants a formatter that reacts to a
threshold (e.g. "if line count > 100, summarise") cannot do so because the
formatter does not have access to per-line context.

In bash-rd there is no distinction. The "formatter" is sourced into the server
and runs on each line. It has `$rd_line`, the full KV store, and can emit
arbitrary output. It is a filter that happens to emit HTML.

### Target design

Replace the two-stage model with a single **stage** concept:

```
stdin
  │
  ▼
Stage 1: plain text filter (optional, per-line → per-line)
Stage 2: plain text filter (optional, per-line → per-line)
...
Stage N: rendering stage (per-line → HTML fragment OR deferred batch → HTML)
  │
  ▼
Scrollback buffer (stores the output of the last stage)
  │
  ▼
Browser
```

A stage is any external command. The contract:

- Receives one line on stdin per invocation (streaming mode) OR the full
  scrollback on stdin (batch mode, for backward compat with current formatters)
- Writes to stdout
- Has the current KV snapshot in its environment
- Marked as `--html` if its output should be sent to the browser as HTML
  rather than stored as text

This means:

```sh
# Today (two separate systems):
my_app | rdw pipe --id log --filter 'grep ERROR'
# Then separately: echo "f:json" | rdw pipe --id log

# Tomorrow (one system):
my_app | rdw pipe --id log \
  --stage 'grep ERROR' \
  --stage --html 'python3 colorize.py'
```

The `--stage --html` flag tells the pipeline that subsequent lines produced
by this stage are HTML fragments, not plain text. The browser renders them
inline alongside plain text lines (in a `<span class="rdw-html">` wrapper).

### User-facing interface

**Register a rendering stage at runtime:**

```sh
rdw pane stage add log --cmd 'python3 highlight.py' --html
rdw pane stage list log
rdw pane stage remove log 0
```

**In config (applies to all panes):**

```yaml
stages:
  - name: colorize
    cmd: /usr/local/bin/colorize.sh
    html: true
```

**Use case — live syntax highlighting as lines arrive:**

```sh
# highlight.py reads one line on stdin, writes one HTML span on stdout
cat > /tmp/highlight.py << 'EOF'
import sys, html
line = sys.stdin.readline().rstrip()
cls = "err" if "ERROR" in line else "warn" if "WARN" in line else "info"
print(f'<span class="hl-{cls}">{html.escape(line)}</span>')
EOF

my_app | rdw pipe --id log --stage --html 'python3 /tmp/highlight.py'
```

Every incoming line immediately appears in the browser with colour applied,
with no separate "apply formatter" step.

**Use case — filter then highlight:**

```sh
my_app | rdw pipe --id log \
  --stage 'grep -E "ERROR|WARN"' \
  --stage --html 'python3 /tmp/highlight.py'
```

Stage 1 drops non-error lines. Stage 2 colours the survivors. Both are
external commands. The user composes them.

**Use case — batch rendering on demand (backward compat):**

```sh
# Old way still works — "format on demand" is batch mode
rdw pane format log json
```

The `Formatter.Format([]string) (string, error)` interface is preserved for
batch use. It is no longer the only way to affect display.

### Architecture changes

#### `internal/pipeline`

Add a `StageKind` enum:

```go
type StageKind int
const (
    StageText StageKind = iota // per-line, plain text output
    StageHTML                  // per-line, HTML fragment output
)

type Stage struct {
    Kind StageKind
    Fn   func(line string) (string, bool)
}
```

`Pipeline.stages []Stage` replaces `Pipeline.filters []Filter`.

`processLine` applies stages in order. When a `StageHTML` stage produces
output, it is wrapped in `<span class="rdw-html">` before going to the
scrollback and sinks:

```go
func (p *Pipeline) processLine(raw string) error {
    line := raw
    for _, stage := range p.stages {
        out, keep := stage.Fn(line)
        if !keep {
            return nil
        }
        if stage.Kind == StageHTML {
            line = `<span class="rdw-html">` + out + `</span>`
        } else {
            line = out
        }
    }
    p.scrollback.Append(line)
    for _, sink := range p.sinks {
        sink(p.targetID, line)
    }
    return nil
}
```

#### `internal/pipeline/cmdstage.go`

Replace `cmdfilter.go` with `cmdstage.go`:

```go
type CmdStage struct {
    kind   StageKind
    cmdStr string
    kv     *kvstore.Store
}

func NewCmdStage(cmdStr string, kind StageKind, kv *kvstore.Store) *CmdStage

func (s *CmdStage) Stage() Stage {
    return Stage{Kind: s.kind, Fn: s.process}
}
```

The subprocess is restarted on each line with fresh KV (same as current
CmdFilter behaviour).

#### Server API

```
POST /api/v1/panes/{id}/stages   body:{cmd, html:bool}  → 204
GET  /api/v1/panes/{id}/stages                          → {stages:[{cmd,html}]}
DELETE /api/v1/panes/{id}/stages/{index}                → 204
```

#### Migration

- `--filter` on `rdw pipe` becomes `--stage` (with `--html` opt-in)
- `--filter` is kept as a deprecated alias for `--stage`
- `POST /api/v1/panes/{id}/filters` maps to `POST /api/v1/panes/{id}/stages`
  with `html:false`

### Why this is more Unix

In Unix a filter does not need to know whether its output will be displayed as
text or HTML. The display decision belongs at the output end of the pipeline,
not at a separate "format" invocation. Every stage in the pipeline is a
composable shell command. The distinction between "filter" (text) and
"formatter" (HTML) is an implementation detail that leaked into the user
interface.

---

## Recommendation 2 — Give built-in formatters access to KV

### The problem

When `json` or `csv` or any built-in formatter runs, it has no access to the
session KV store. A user cannot write:

```sh
rdw kv set json.indent 4
rdw kv set csv.separator "|"
```

and have the formatters respect those values. In bash-rd, the formatter is a
bash function that has `$rd_kv_json_indent` in its environment by default.
KV is the formatter's configuration interface.

### Target design

`Formatter.Format` receives a second argument: the current KV snapshot.

```go
type Formatter interface {
    Name() string
    Format(lines []string, kv map[string]string) (string, error)
}
```

Each built-in formatter reads well-known keys from `kv` with sensible defaults:

| Formatter | KV key | Effect | Default |
| --- | --- | --- | --- |
| `json` | `json.indent` | spaces per indent level | `2` |
| `json` | `json.compact` | disable pretty-print | `false` |
| `csv` | `csv.separator` | column separator char | `,` |
| `csv` | `csv.header` | treat first row as header | `true` |
| `markdown` | `markdown.theme` | `light` or `dark` | `dark` |
| `text` | `text.wrap` | column width for wrapping | `0` (no wrap) |
| `image` | `image.scale` | `fit`, `fill`, `native` | `fit` |
| `image` | `image.max_height` | CSS max-height | `` (none) |

### User-facing interface

```sh
# Make JSON output compact
rdw kv set json.compact true
rdw pane format log json

# Pipe-separated CSV
rdw kv set csv.separator "|"
rdw pane format data csv

# Apply while piping
echo "=:json.indent=4" | rdw pipe --id api
curl -s https://api.example.com/ | rdw pipe --id api
```

**Use case — dashboard with multiple formatters all reading shared config:**

```sh
# Set a global theme once
rdw kv set markdown.theme light
rdw kv set json.indent 2

# All formatters in all panes now use these settings
rdw pane format docs markdown
rdw pane format api  json
rdw pane format logs text
```

This is the bash-rd model: one `=:key=value` line affects all formatters in
the session simultaneously.

### Architecture changes

#### `internal/format/format.go`

```go
type Formatter interface {
    Name() string
    Format(lines []string, kv map[string]string) (string, error)
}
```

Every built-in formatter reads from `kv`:

```go
// internal/format/json.go
func (f *JSONFormatter) Format(lines []string, kv map[string]string) (string, error) {
    indent := 2
    if v, ok := kv["json.indent"]; ok {
        if n, err := strconv.Atoi(v); err == nil {
            indent = n
        }
    }
    compact := kv["json.compact"] == "true"
    // ... rest of formatter
}
```

#### `internal/server/routes.go` — `handlePaneFormat`

```go
func (s *Server) handlePaneFormat(w http.ResponseWriter, r *http.Request) {
    // ...
    kvSnap := make(map[string]string)
    for k, v := range s.kv.Snapshot() {
        kvSnap[k.String()] = v
    }
    html, err := f.Format(lines, kvSnap)
    // ...
}
```

#### `internal/format/cmd.go` — `CmdFormatter`

`CmdFormatter.Format` already injects KV into the subprocess environment.
The `kv map[string]string` parameter replaces the stored snapshot, making
it dynamic:

```go
func (f *CmdFormatter) Format(lines []string, kv map[string]string) (string, error) {
    cmd := exec.Command("sh", "-c", f.cmdStr)
    for k, v := range kv {
        cmd.Env = append(cmd.Env, k+"="+v)
    }
    // ...
}
```

This makes built-in and external formatters behave identically with respect
to KV — both receive the full current snapshot at invocation time.

### Why this is more Unix

A Unix tool reads its configuration from its environment. In bash-rd, the
formatter's environment *is* the KV store. Making rdw's built-in formatters
KV-aware closes the gap. A user who learns to configure rdw via `rdw kv set`
can configure every formatter — built-in or external — the same way.

---

## Recommendation 3 — Make `rdw pipe` dumb

### The problem

`rdw pipe` currently does:
- Resolve the server via discovery
- Apply a layout (`--layout`)
- Set the pane title (`--title`)
- Register filter stages (`--filter`, `--stage`)
- Set up stream mirroring (`--forward-to-file`, `--forward-to-cmd`, `--forward`)
- Relay stdin to the server

Items 2–5 are setup operations that have nothing to do with relaying a stream.
They belong in the shell, composed before the pipe, not inside the pipe command.

This violates Unix rule: a pipe is a transport. It should not have side effects
on the system it is transporting to. A `curl | grep | sort` pipeline does not
modify the remote server's configuration.

### Target design

`rdw pipe` does exactly one thing: relay stdin to a named pane.

```sh
rdw pipe --id log
```

The options that remain:
- `--id` (required)
- `--port` (server selection)

Everything else moves to its natural command:

| Removed from `rdw pipe` | Correct Unix composition |
| --- | --- |
| `--layout NAME` | `rdw layout apply NAME && prog \| rdw pipe --id log` |
| `--title TEXT` | `rdw pane rename log "My Log" && prog \| rdw pipe --id log` |
| `--filter CMD` | `rdw pane stage add log --cmd CMD && prog \| rdw pipe --id log` |
| `--forward-to-file PATH` | `prog \| tee PATH \| rdw pipe --id log` |
| `--forward-to-cmd CMD` | `prog \| tee >(CMD) \| rdw pipe --id log` |
| `--forward rd` | `prog \| tee >(rd -c compat) \| rdw pipe --id log` |

### User-facing interface

**Before (current rdw):**

```sh
make build 2>&1 | rdw pipe --id build \
  --title "CI Build" \
  --layout ci.yaml \
  --filter 'grep -v DEBUG' \
  --forward-to-file /tmp/build.log
```

**After (Unix composition):**

```sh
rdw layout apply ci.yaml
rdw pane rename build "CI Build"
rdw pane stage add build --cmd 'grep -v DEBUG'

make build 2>&1 | tee /tmp/build.log | rdw pipe --id build
```

The second form is longer but every step is visible and independently
reusable. The setup commands run once; the pipe runs every time.

**Use case — reusable setup script:**

```sh
#!/bin/sh
# setup-ci-panes.sh — run once before any build
rdw layout apply ci.yaml
rdw pane rename build "CI Build"
rdw pane rename test  "Test Suite"
rdw pane stage add build --cmd 'grep -v DEBUG'
rdw pane stage add test  --cmd 'grep -E "PASS|FAIL|ERROR"'
rdw kv set json.indent 2
```

```sh
./setup-ci-panes.sh

make build 2>&1       | rdw pipe --id build
make test  2>&1       | rdw pipe --id test
make bench 2>&1 | tee bench.log | rdw pipe --id bench
```

**Use case — ad-hoc with tee:**

```sh
# Unix standard: tee to a file, pipe to rdw
long_computation 2>&1 | tee /tmp/run.log | rdw pipe --id run

# Unix standard: split stream to multiple destinations
complex_app 2>&1 | tee >(grep ERROR >> errors.log) \
                       >(grep WARN  >> warnings.log) \
                       | rdw pipe --id app
```

### Architecture changes

#### `cmd/rdw/commands.go`

Remove from `pipeCmd.Flags()`:
- `--title`
- `--layout`
- `--filter` / `--stage`
- `--forward-to-file`
- `--forward-to-cmd`
- `--forward`

Remove from `runPipe`:
- Title PATCH call
- Layout apply call
- Filter registration loop
- Mirror sink setup
- Hybrid reader (see Recommendation 5)

`runPipe` becomes:

```go
func runPipe(cmd *cobra.Command, _ []string) error {
    idStr, _ := cmd.Flags().GetString("id")
    id, err := session.ParseTargetID(idStr)
    if err != nil {
        return fmt.Errorf("invalid target ID %q: %w", idStr, err)
    }

    port := portFlag(cmd)
    resolved, err := discovery.Resolve(port)
    if err != nil {
        return err
    }

    socketPath, _ := unixSocketPath(fmt.Sprintf("%d", resolved))

    return pipepkg.Relay(context.Background(), os.Stdin, pipepkg.Options{
        TargetID:          id,
        Port:              resolved,
        SocketPath:        socketPath,
        ReconnectQueueLen: 1000,
    })
}
```

#### Migration and backward compatibility

The removed flags are kept as **deprecated aliases** that print a warning and
delegate to the equivalent command before piping:

```sh
# Deprecated: rdw pipe --title "My Pane" --id log
# Warning: --title is deprecated; use: rdw pane rename log "My Pane"
# The flag still works in rdw 2.x but will be removed in 3.0
```

This gives users time to migrate their scripts.

### Why this is more Unix

A pipe is a transport. Configuration of the endpoint (layout, title, stages)
is orthogonal to data transport and should not be coupled to it. Mirroring
is what `tee(1)` is for — rdw does not need to re-implement it. Splitting
streams is what process substitution (`>(cmd)`) is for.

When `rdw pipe` is dumb, a user can reason about it in isolation. When it is
smart, the user must understand the order of operations inside `rdw pipe`
before understanding the pipeline.

---

## Recommendation 4 — Serve the SPA from a configurable directory

### The problem

The browser SPA is `internal/server/frontend.go` — a Go file containing
a multi-thousand-line HTML/CSS/JS string literal. To change the display
(different colour scheme, additional widgets, custom keyboard shortcuts beyond
the 32 supported actions) a user must fork rdw, edit the Go source, and
recompile.

In bash-rd, the formatter *is* the display. You write a shell function that
emits arbitrary HTML. You own the page completely. rdw replaced this with a
fixed SPA that the user cannot modify.

### Target design

`rdw server start --frontend DIR` serves files from `DIR` at `/`.
The embedded SPA is the default when no `--frontend` is given.

```
GET /          → serve DIR/index.html (or embedded default)
GET /rdw.js   → serve DIR/rdw.js (or embedded default)
GET /style.css → serve DIR/style.css (or embedded default)
```

The server exposes a **stable JavaScript API** that any custom frontend can
use. The API contract:

```js
// rdw-api.js — stable contract the server guarantees
const rdw = {
    // Connect to the WebSocket stream
    connect(token) → EventEmitter,

    // REST wrappers
    getSession()   → Promise<Session>,
    getBindings()  → Promise<Bindings>,
    getStatus()    → Promise<Status>,
    postStream(id, line) → Promise<void>,
    // ... etc
};
```

This is served at `/rdw-api.js` and never changes between versions.

### User-facing interface

**Default (no change):**

```sh
rdw server start --open-browser
```

Serves the embedded SPA.

**Custom frontend:**

```sh
rdw server start --frontend ~/my-rdw-ui/ --open-browser
```

Serves `~/my-rdw-ui/index.html`. The custom page connects to the same
WebSocket and REST API.

**Use case — minimal text-only display:**

```html
<!-- ~/my-rdw-ui/index.html -->
<!DOCTYPE html>
<html>
<head><style>
  body { background: #000; color: #0f0; font: 13px monospace; }
  .pane { border: 1px solid #0f0; padding: 8px; margin: 8px; height: 300px; overflow-y: scroll; }
</style></head>
<body>
<div id="log" class="pane"></div>
<script src="/rdw-api.js"></script>
<script>
  const ws = rdw.connect('');
  ws.on('line', ({target_id, line}) => {
    if (target_id === 'log') {
      const el = document.getElementById('log');
      el.innerHTML += line + '\n';
      el.scrollTop = el.scrollHeight;
    }
  });
</script>
</body>
</html>
```

**Use case — custom dashboard with D3 graphs:**

```html
<script src="https://d3js.org/d3.v7.min.js"></script>
<script src="/rdw-api.js"></script>
<script>
  // KV changes drive a D3 chart
  const ws = rdw.connect(TOKEN);
  const chart = d3.select('#chart');

  ws.on('kv_update', async () => {
    const kv = await rdw.getKV('metrics.');
    updateChart(chart, kv);
  });
</script>
```

**Use case — embed rdw in an existing web application:**

```html
<!-- In your existing app -->
<iframe src="http://localhost:7681?token=xxx" style="width:100%;height:400px">
</iframe>

<!-- Or use the API directly from your app's JS -->
<script src="http://localhost:7681/rdw-api.js"></script>
```

### Architecture changes

#### `internal/config/config.go`

```go
type ServerConfig struct {
    // ... existing fields
    FrontendDir string `yaml:"frontend_dir"` // empty = use embedded SPA
}
```

#### `cmd/rdw/server.go`

```sh
serverStartCmd.Flags().String("frontend", "",
    "serve custom SPA from this directory instead of the embedded default")
```

#### `internal/server/routes.go`

```go
func handleFrontend(w http.ResponseWriter, r *http.Request) {
    if frontendDir != "" {
        http.FileServer(http.Dir(frontendDir)).ServeHTTP(w, r)
        return
    }
    // embedded default
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _, _ = w.Write(frontendHTML)
}
```

#### New: `internal/server/rdwapi.js`

A stable, versioned JS file served at `/rdw-api.js`:

```js
// rdw-api.js v1
// Stable API contract — safe to depend on across rdw versions.
(function(global) {
  'use strict';

  function connect(token) {
    const url = 'ws://' + location.host + '/api/v1/ws' +
                (token ? '?token=' + encodeURIComponent(token) : '');
    const ws = new WebSocket(url, ['rdw-v1']);
    const handlers = {};

    ws.onmessage = function(e) {
      const msg = JSON.parse(e.data);
      const type = msg.type || 'line';
      if (handlers[type]) handlers[type].forEach(fn => fn(msg));
    };

    return {
      on(type, fn) { (handlers[type] = handlers[type] || []).push(fn); return this; },
      send(id, line) { ws.send(JSON.stringify({type:'line',target_id:id,line})); }
    };
  }

  async function get(path) {
    const r = await fetch('/api/v1/' + path);
    return r.json();
  }

  async function post(path, body) {
    return fetch('/api/v1/' + path, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body)
    });
  }

  global.rdw = { connect, getSession: () => get('session'),
                  getStatus: () => get('status'), getBindings: () => get('bindings') };
})(window);
```

### Why this is more Unix

"Write programs that do not assume they know the best way to display their
output" — this is the fundamental tension. rdw's embedded SPA assumes it
knows the best display. The configurable frontend directory restores the
user's ownership of the display, which is exactly what bash-rd's formatter
model provided — at the cost of needing to know HTML instead of just bash.

The stable JS API is the contract the server guarantees, analogous to how
bash-rd's `$rd_line` and `$rd_kv_*` variables are the contract the server
offers to formatters.

---

## Recommendation 5 — Replace sentinel framing with standard encoding

### The problem

The `image:` / `image:end` sentinel-framed binary protocol in the pipe:

1. **Is fragile** — if binary data contains a line that is exactly `image:end`,
   the frame closes early and the remaining bytes corrupt the next line
2. **Duplicates `base64(1)`** — Unix already has a tool for encoding binary
   as text; inventing a new framing scheme is not necessary
3. **Leaks into `rdw pipe`** — the hybrid reader that handles sentinel frames
   is non-trivial code in the relay path, adding complexity to what should be
   a transparent conduit
4. **Is invisible** — you cannot see what is being sent because rdw intercepts
   and re-encodes inside `rdw pipe`

The correct Unix answer: the producer is responsible for encoding. rdw is
responsible for routing.

### Target design

Remove `image:` / `image:end` and `svg:` / `svg:end` sentinel sequences.

Binary data arrives pre-encoded as base64, prefixed with the appropriate
control sequence:

```sh
# Image (standard Unix tools)
base64 chart.png | sed '1s/^/b64:/' | rdw pipe --id chart

# Or more clearly:
{ echo "f:image" ; base64 chart.png ; } | rdw pipe --id chart
```

SVG is text, so it does not need encoding — it arrives as plain lines:

```sh
{ echo "f:svg" ; cat diagram.svg ; } | rdw pipe --id diagram
```

The `f:svg` formatter handles multi-line SVG by accumulating lines until the
scrollback buffer contains a complete SVG document (detected by `</svg>` end
tag), then renders it.

### `rdw send` as the convenience layer

`rdw send` is the right place for convenience. It is explicitly a helper
command, not a pipe. It reads a file, detects type, encodes if necessary,
and sends the appropriate control lines. Its implementation is transparent:

```sh
# What rdw send --id chart chart.png actually does:
echo "f:image"
base64 chart.png
rdw pipe --id chart  # inside rdw send
```

Users who do not want the magic of `rdw send` can always compose it manually.
Users who want the magic use `rdw send`. The pipeline is visible either way.

### Architecture changes

#### Remove from `internal/control/control.go`

```go
// Remove:
KindImage   Kind = "image"
KindSVG     Kind = "svg"
SentinelImageEnd = "image:end"
SentinelSVGEnd   = "svg:end"
```

#### Remove from `internal/pipe/hybrid.go`

The entire `hybridReader` type and its `accumBinary` / `accumSVG` methods
are deleted. `relay()` in `pipe.go` returns to using a plain `bufio.Scanner`.

#### Add `f:svg` formatter

A new `SVGFormatter` that accumulates lines until it sees `</svg>`, then
renders the complete SVG inline:

```go
// internal/format/svg.go
type SVGFormatter struct{}

func (f *SVGFormatter) Name() string { return "svg" }

func (f *SVGFormatter) Format(lines []string, kv map[string]string) (string, error) {
    raw := strings.Join(lines, "\n")
    // Find last complete SVG document
    start := strings.LastIndex(raw, "<svg")
    end   := strings.LastIndex(raw, "</svg>")
    if start < 0 || end < 0 || end < start {
        return "<div class='rdw-svg-waiting'>awaiting complete SVG...</div>", nil
    }
    svg := raw[start : end+6]  // include </svg>
    scale := kv["image.scale"]
    if scale == "" { scale = "fit" }
    return fmt.Sprintf(`<div class="rdw-svg-block rdw-scale-%s">%s</div>`, scale, svg), nil
}
```

#### Update browser SPA

The `image_render` and `svg_render` WebSocket messages sent by the server's
control handler (for `image:` and `svg:` blocks) are removed. The browser
renders base64 images via the `image` formatter when `POST /api/v1/panes/{id}/format`
is called, or per-line via an HTML stage (Recommendation 1).

#### `rdw send` stays unchanged

`rdw send` internally does:

```go
case isImage:
    return "f:image\nb64:" + base64.StdEncoding.EncodeToString(data)
case isSVG:
    return "f:svg\n" + string(data)
```

This is exactly what the user would do manually. The composition is visible.

### User-facing migration

| Old | New |
| --- | --- |
| `{ echo "image:"; cat img.png; echo "image:end"; } \| rdw pipe --id p` | `{ echo "f:image"; base64 img.png; } \| rdw pipe --id p` |
| `{ echo "svg:"; cat d.svg; echo "svg:end"; } \| rdw pipe --id p` | `{ echo "f:svg"; cat d.svg; } \| rdw pipe --id p` |
| `rdw send --id p img.png` | unchanged |

The new form is actually shorter and uses `base64(1)` — a standard Unix tool
every developer already knows.

### Why this is more Unix

"Use the output of every program as the input of another" requires that
programs agree on encoding. The encoding for binary-in-text streams is
already standardised: base64. rdw's sentinel framing is a second,
incompatible encoding. Removing it reduces the number of protocols the
user must understand from two to one.

The fragility problem also disappears: base64 output never contains the
sequence `b64:` or `f:image`, so there are no framing edge cases.

---

## Implementation order

The five recommendations interact. The correct implementation order minimises
disruption:

1. **Recommendation 2 (KV in built-in formatters)** — purely additive,
   no breaking changes, can ship independently

2. **Recommendation 5 (remove sentinel framing)** — removes code and a
   control sequence; `rdw send` covers the convenience case; ship with a
   deprecation warning in rdw 2.x before removal in 3.0

3. **Recommendation 1 (collapse filter/formatter into stage)** — additive
   first (add `--stage`, keep `--filter`), then deprecate `--filter` in 2.x

4. **Recommendation 4 (configurable frontend)** — additive; embedded SPA
   remains the default; `--frontend` is a new option

5. **Recommendation 3 (dumb pipe)** — most disruptive; do last, after 1–4
   are in place and users have migrated; deprecate removed flags in 2.x,
   remove in 3.0

Each step makes rdw more composable and more Unix-like without breaking
existing users immediately.
