# rdw — Remote Display Web

`rdw` pipes your program's output into a browser. Any process that can write
to a file descriptor can stream text, images, JSON, Markdown, or CSV into a
named pane in a multi-window layout — locally or over the network.

It is the web-native successor to [bash-rd](https://github.com/nkh/bash-rd),
rewritten in Go as a self-contained binary with a persistent daemon,
token-based access control, and a live browser UI.

```sh
your_script | rdw pipe --id build-log
```

---

## What it does

- Routes `stdin` from any process to a named pane in the browser
- Manages multiple windows within a single browser page (not browser tabs)
- Supports multiple concurrent streams in split panes across multiple windows
- Renders plain text, ANSI color, JSON trees, YAML trees, Markdown, CSV grids,
  and images
- Maintains a session-scoped key-value store accessible from any stream or
  formatter
- Exposes every CLI command identically via a REST API at `/api/v1/`
- Multiple server instances can run simultaneously on different ports
- Runs entirely from a single static binary — no runtime dependencies, no
  internet required at runtime

---

## Concepts

```
Session
  └── Window  (server-managed view, shown one at a time in the browser)
        └── Pane  (bounded view area, receives one Target ID)
              └── Target ID  (the name you give a stream)
```

**Windows** are server-managed. The browser displays one window at a time and
shows a persistent header bar listing all window names. Switch between windows
using the keyboard bindings (`gt` / `gT`) or by clicking the header.

A **Target ID** is a string matching `[a-zA-Z0-9_][a-zA-Z0-9_ -]*`, max 64
characters, that maps a data stream to a pane.

A **Filter** is an external executable that transforms stream data before
rendering (max 8 stages per chain).

A **Formatter** renders a pane as HTML from the incoming stream and the current
KV store state.

A **Control Sequence** is a line in the stream with a recognised two-character
prefix (`x:`) that triggers a side-effect instead of rendering as content.

---

## Multiple servers

Multiple rdw servers can run simultaneously on different ports. Use `--port`
on every command to target a specific instance:

```sh
# start two servers
rdw server start --port 7681
rdw server start --port 7682

# send data to the second server
your_script | rdw pipe --id build-log --port 7682

# list all running servers
rdw server list
```

When `--port` is omitted, the default port (7681) is tried. If no server
answers there, all registered instances are listed in the error message.

---

## Project status

Early implementation. The core pipeline, KV store, auth, config, layout
schema, discovery, bindings, and CLI scaffold are complete and tested. The
HTTP/WebSocket server, browser UI, and REST API are the next layer.

| Package | Description | Coverage |
| --- | --- | --- |
| `internal/session` | TargetID validation, ScrollbackBuffer | 100% |
| `internal/kvstore` | Session-scoped KV store | 98% |
| `internal/control` | Inline control sequence parser | 100% |
| `internal/pipeline` | Line relay, filter chain, KV dispatch | 93% |
| `internal/auth` | SHA-256 hashed token access control | 91% |
| `internal/config` | YAML config loader with validation | 76% |
| `internal/layout` | Window/pane schema, YAML parser, resize | 100% |
| `internal/bindings` | Keyboard binding model, vim defaults | 100% |
| `internal/discovery` | Multi-server registry and auto-detect | 58% |
| `internal/selftest` | In-process smoke test suite | 80% |

134 tests, 76.1% overall statement coverage.

---

## Install

```sh
go install github.com/nkh/rdw@latest
```

Or clone and build:

```sh
git clone https://github.com/nkh/rdw
cd rdw
make build
```

Shell autocompletion:

```sh
rdw completion bash >> ~/.bashrc
```

---

## Quick start

Start the server:

```sh
rdw server start --open-browser
```

The server listens on port `7681` by default. The browser opens automatically.
To run a second server:

```sh
rdw server start --port 7682 --open-browser
```

Send data to a pane:

```sh
# plain text
your_script | rdw pipe --id my-pane

# with a layout — create layout on first use, reuse on subsequent calls
your_script | rdw pipe --id build-log --layout layouts/debug.yaml

# route into a specific window by name
your_script | rdw pipe --id build-log --window build

# formatted JSON
curl -s api.example.com/status | rdw json --id api-status

# image from a file
rdw image --id chart --path ./output.png
```

Stop the server:

```sh
rdw server stop
rdw server stop --port 7682
```

---

## Layout description language

Layouts are YAML files. The server uses them to construct the pane arrangement
in the browser when a layout is applied or first referenced.

```yaml
schema_version: 1        # required; must be 1
name: debug              # optional preset name

windows:
  - name: build          # display name shown in the window header bar
    panes:
      - target_id: stdout          # stream name routed to this pane
      - target_id: stderr
        split: h                   # h = new pane below; v = new pane to the right
        size: 30%                  # N (columns), Npx, N%
        group: ci                  # optional pane group
        private: false             # hide from non-owner tokens
        scrollback_cap: 5000       # per-pane cap (default: 10 000, max: 100 000)

  - name: metrics
    panes:
      - target_id: cpu
      - target_id: mem
        split: v
        size: 50%
      - target_id: disk
        split: h
        size: 25%
```

The first pane in each window needs no `split` field; it occupies the whole
window until subsequent panes subdivide it.

Apply a layout from the CLI:

```sh
# from a file
rdw layout apply ./layouts/debug.yaml

# from a saved preset
rdw layout apply debug

# create the layout interactively and save it
rdw layout save --name debug
```

Pass a layout directly to `pipe` — if it is not already active it is created:

```sh
your_script | rdw pipe --id build-log --layout debug
```

---

## Window management

Windows are server-managed views within the browser page. The browser shows
one window at a time and a header bar lists all windows.

```sh
rdw window create build
rdw window create metrics
rdw window list
rdw window focus metrics
rdw window rename build build-v2
rdw window close build-v2
```

Switch windows in the browser using the keyboard bindings or by clicking
the header bar.

---

## Pane management

```sh
# split vertically and assign a new target
rdw pane split build-log v error-log

# resize — columns (default), pixels, or percentage
rdw pane resize build-log right 40%
rdw pane resize build-log right 200px
rdw pane resize build-log right 40

# zoom a pane to full window (toggle)
rdw pane zoom error-log

# swap two panes
rdw pane swap build-log error-log

# close a pane
rdw pane close error-log
```

---

## Keyboard bindings

The browser UI is fully keyboard-driven with vim-like defaults. All bindings
can be overridden in the configuration file under the `bindings:` section.

### Window navigation

| Key | Action |
| --- | --- |
| `gt` | next window |
| `gT` | previous window |
| `g0` | first window |
| `g$` | last window |
| `gn` | create new window |
| `gx` | close active window |
| `gr` | rename active window (opens prompt) |

### Pane navigation

| Key | Action |
| --- | --- |
| `h` | focus pane left |
| `j` | focus pane below |
| `k` | focus pane above |
| `l` | focus pane right |

### Pane editing

| Key | Action |
| --- | --- |
| `s` | split horizontally (new pane below) |
| `v` | split vertically (new pane right) |
| `H` | resize pane left |
| `J` | resize pane down |
| `K` | resize pane up |
| `L` | resize pane right |
| `q` | close focused pane |
| `z` | toggle zoom (full window) |
| `R` | rename pane target ID (opens prompt) |
| `x` | enter swap mode: next `hjkl` picks the target to swap with |

### Scrollback

| Key | Action |
| --- | --- |
| `Ctrl+u` | scroll up |
| `Ctrl+d` | scroll down |
| `gg` | scroll to top |
| `G` | scroll to bottom |
| `Ctrl+l` | clear scrollback |

### Search

| Key | Action |
| --- | --- |
| `/` | open search |
| `n` | next match |
| `N` | previous match |

### Layout

| Key | Action |
| --- | --- |
| `Ctrl+w s` | save current layout |
| `Ctrl+w l` | reload layout from disk |
| `Escape` / `Ctrl+c` | return to normal mode |

### Mouse support

All pane operations are also available with the mouse:

- **Drag** the gutter between panes to resize
- **Click** a window name in the header to switch windows
- **Double-click** a pane border to zoom
- **Drag** a pane title bar to detach and re-attach it to another window
- **Right-click** a pane for the context menu (clear, close, rename, etc.)

### Customising bindings

Override individual bindings in `~/.config/rdw/config.yaml`:

```yaml
bindings:
  pane.focus.left:  [Left]
  pane.focus.right: [Right]
  pane.focus.up:    [Up]
  pane.focus.down:  [Down]
  window.next:      [Tab]
  window.prev:      [Shift+Tab]
```

Any action not listed in the overrides keeps its default binding. Set an
action to an empty list to remove its binding entirely:

```yaml
bindings:
  pane.swap: []   # disable swap mode
```

---

## Key-value store

The server maintains a session-scoped KV store. Any stream or formatter can
read and write it.

```sh
rdw kv set build.status passing
rdw kv get build.status
rdw kv delete build.status

# write inline from a stream using a control sequence
echo "=:build.status=passing;build.duration=42s" | rdw pipe --id build-log

# optional SQLite persistence across server restarts
rdw server start --kv-persist ~/.rdw/kv.db
```

Keys match `[a-zA-Z0-9_][a-zA-Z0-9_ :-]*`, max 64 characters. Values are
capped at 64 KB each; the total store is capped at 64 MB.

Window- and pane-scoped keys use a prefix convention:

```
window:<window_name>:<key>
pane:<target_id>:<key>
```

---

## Control sequences

A line whose first two bytes are a recognised letter and a colon is a control
sequence and is not forwarded to the renderer.

| Prefix | Effect |
| --- | --- |
| `v:` | verbatim — send a line that looks like a control sequence as literal content |
| `q:` | quit the server |
| `s:` | semaphore — server quits when count reaches zero |
| `c:` | clear the pane scrollback buffer |
| `t:` | toggle timestamp prepending |
| `f:` | set the formatter for the pane |
| `r:` | relay output to a location |
| `=:` | write KV pairs; multiple pairs separated by `;` |

---

## Binary data

Binary payloads must be base64-encoded with a `b64:` prefix:

```sh
echo "b64:$(base64 < image.png)" | rdw pipe --id chart
```

---

## Sharing and access control

```sh
# create a token scoped to one pane, expiring in 8 hours
rdw token create --panes build-log --expiry 8h

# revoke immediately
rdw token revoke <token-id>
```

The server is loopback-only by default. `--network-expose` at startup extends
access to the configured network interface. Tokens are stored as SHA-256 hashes.

---

## Exporting

```sh
rdw save pane --target-id build-log --out-dir ./export
rdw save all --out-dir ./export
```

Output is a Markdown file per window with an `assets/` subfolder for images.

---

## bash-rd compatibility

```sh
rdw pipe --id my-pane --forward rd      # send to rd only
rdw pipe --id my-pane --forward both    # send to both rdw and rd
```

The KV store wire format, `=:key=value` control sequence syntax, and `b64:`
binary encoding convention are identical to bash-rd.

---

## Headless and CI use

```sh
rdw selftest   # exits 0 on success
```

---

## Configuration

`~/.config/rdw/config.yaml` (pass `--config` to override):

```yaml
server:
  port: 7681
  network_expose: false
  open_browser: false
  filter_chain_max: 8
  scrollback_cap: 10000
  reconnect_queue_len: 1000

auth:
  no_auth: false
  admin_local_only: true

kv:
  persist_path: ""

log:
  level: info       # debug | info | warn | error
  format: console   # console | json

bindings:
  # override individual keys; omitted actions keep their defaults
  # pane.focus.left: [Left]
```

---

## Development

```sh
make build      # build the binary
make test       # run tests with -race
make vet        # run go vet
make coverage   # generate coverage report
make selftest   # build then run rdw selftest
make clean      # remove build artefacts
```

---

## Reference

- `rdw --help` — full command list
- `rdw server start --help` — all server flags
- `rdw completion bash` — bash autocompletion script
- `man rdw` — UNIX man page (forthcoming)
- Full functional requirements: [docs/requirements.md](docs/requirements.md)

---

## License

Artistic License 2.0 — see [LICENSE](LICENSE).

## Author

Nadim Khemir — [https://github.com/nkh](https://github.com/nkh)
