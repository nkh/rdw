# rdw — Remote Display Web

`rdw` pipes process output into a browser. Any process that writes to its
stdout can stream text, images, JSON, Markdown, or CSV into a named pane in a
multi-window layout — locally or over the network.

It is the web-native successor to [bash-rd](https://github.com/nkh/bash-rd),
rewritten in Go as a self-contained binary with a persistent daemon,
token-based access control, and a live browser UI.

```sh
your_script | rdw pipe --id build-log
```

---

## What it does

- Routes process output to named panes in the browser
- Manages multiple windows within a single browser page (not browser tabs)
- Supports multiple concurrent streams in split panes across multiple windows
- Full ANSI 16/256/true-colour rendering
- Session-scoped key-value store accessible from any stream
- Full REST API at `/api/v1/` with parity to every CLI command
- Multiple server instances on different ports
- Single static binary — no runtime dependencies, no CDN, no internet required

---

## Project status

353 tests · 11/11 selftest checks pass · `go vet` clean

| Package | Description |
| --- | --- |
| `internal/session` | TargetID, ScrollbackBuffer, Manager, BookmarkStore |
| `internal/kvstore` | KV store, SQLite persistence |
| `internal/control` | Control sequence parser (all prefixes) |
| `internal/pipeline` | Line relay, filter chain, sinks |
| `internal/layout` | YAML layout schema |
| `internal/bindings` | 32 named keyboard actions |
| `internal/router` | TargetID → pipeline routing |
| `internal/auth` | Token access control |
| `internal/export` | Markdown bundle export |
| `internal/server` | HTTP/WebSocket server, REST API, browser SPA |
| `internal/pipe` | Client-side output relay |
| `internal/mirror` | Stream mirroring (file/command) |
| `internal/format` | Formatters: text/json/yaml/markdown/csv/image |
| `internal/highlight` | Regex highlight profiles |
| `internal/cycle` | Focus cycle automation |
| `internal/terminal` | gotty/socat terminal pane launcher |
| `internal/discovery` | Multi-server registry |
| `internal/browser` | Cross-platform browser open |

---

## Install

```sh
go install github.com/nkh/rdw@latest
```

Or build:

```sh
git clone https://github.com/nkh/rdw
cd rdw && make build
```

```sh
rdw completion bash >> ~/.bashrc
```

---

## Quick start

```sh
# Start server (opens browser)
rdw server start --open-browser

# Stream data to a pane
your_script | rdw pipe --id build-log

# Multiple servers on different ports
rdw server start --port 7682
your_script | rdw pipe --id log --port 7682

# With a layout file
your_script | rdw pipe --id build-log --layout ./layouts/debug.yaml
```

---

## Browser UI

The browser renders a live window/pane layout with full keyboard control:

| Key | Action |
| --- | --- |
| `gt` / `gT` | next / previous window |
| `gn` / `gx` / `gr` | new / close / rename window |
| `g0` / `g$` | first / last window |
| `h` `j` `k` `l` | focus pane left/down/up/right |
| `s` / `v` | split pane horizontally / vertically |
| `H` `J` `K` `L` | resize pane |
| `q` / `z` | close / zoom pane |
| `x` | swap mode — pick target with `hjkl` |
| `R` | rename pane target ID |
| `/` `n` `N` | search open / next / prev |
| `Ctrl+u` / `Ctrl+d` | scroll up/down |
| `gg` / `G` | scroll to top / bottom |
| `Ctrl+l` | clear scrollback |
| `Ctrl+w s` | save layout preset |
| `Escape` | return to normal mode |

All bindings are configurable in `config.yaml` under `bindings:`.

Mouse: drag gutters to resize, click header tabs to switch windows,
right-click pane for context menu, double-click border to zoom.

---

## Layout description language

```yaml
schema_version: 1
name: debug

windows:
  - name: build
    panes:
      - target_id: stdout
      - target_id: stderr
        split: h        # h = below, v = right
        size: 30%       # N cols | Npx | N%
  - name: metrics
    panes:
      - target_id: cpu
      - target_id: mem
        split: v
        size: 50%
```

```sh
rdw layout apply debug          # apply saved preset
rdw layout apply ./debug.yaml   # apply file directly
your_script | rdw pipe --id log --layout debug   # apply on first pipe
```

---

## Commands

```sh
# Server
rdw server start [--port PORT] [--open-browser] [--no-auth] [--network-expose]
rdw server stop  [--port PORT]
rdw server list

# Streams
your_script | rdw pipe --id ID [--layout LAYOUT] [--window WIN]

# Windows
rdw window create NAME
rdw window close NAME
rdw window rename OLD NEW
rdw window focus NAME
rdw window list

# Panes
rdw pane split PARENT h|v NEW_ID
rdw pane resize ID left|right|up|down SIZE
rdw pane zoom ID
rdw pane close ID

# Layouts
rdw layout apply NAME|PATH
rdw layout save --name NAME
rdw layout list

# KV store
rdw kv set KEY VALUE
rdw kv get KEY
rdw kv delete KEY

# Tokens
rdw token create [--expiry DURATION] [--panes LIST] [--windows LIST]
rdw token revoke TOKEN_ID
rdw token list

# Groups
rdw group hide|show|focus|kill GROUP_NAME

# Export
rdw save pane  --target-id ID  --out-dir DIR
rdw save window --name NAME   --out-dir DIR
rdw save all                  --out-dir DIR

# Utilities
rdw selftest
rdw completion bash
```

---

## Control sequences

| Prefix | Effect |
| --- | --- |
| `v:` | verbatim passthrough |
| `q:` | quit server |
| `s:` | semaphore (quit at zero) |
| `c:` | clear scrollback |
| `t:` | toggle timestamp |
| `f:` | set formatter |
| `r:` | relay output |
| `=:` | write KV pairs (`k=v;k2=v2`) |
| `b64:` | base64-encoded binary data |
| `bm:` | create scrollback bookmark at current line |
| `hl:` | apply named highlight profile to pane |
| `sc:` | scrollback control: `clear`, `top`, `bottom` |
| `image:` ... `image:end` | sentinel-framed binary block; rdw base64-encodes internally |
| `svg:` ... `svg:end` | sentinel-framed UTF-8 SVG; rendered inline in browser |
| `scale:` | image/SVG scaling: `fit` (default), `fill`, `native` |

```sh
echo "=:build.status=passing;duration=12s" | rdw pipe --id log
echo "v:=:literal content not a KV write" | rdw pipe --id log
echo "bm:section_start" | rdw pipe --id log
echo "hl:errors" | rdw pipe --id log
echo "sc:clear" | rdw pipe --id log

# Send an image inline (rdw handles the base64 encoding)
{ echo "image:" ; cat chart.png ; echo "image:end" ; } | rdw pipe --id chart

# Send an SVG
{ echo "svg:" ; cat diagram.svg ; echo "svg:end" ; } | rdw pipe --id diagram

# Set scaling
echo "scale:fill" | rdw pipe --id chart
```

---

## rdw send

Send any file directly to a pane — no manual base64 or control sequences needed:

```sh
rdw send --id chart  chart.png       # PNG/JPEG/GIF/WebP → image_render
rdw send --id diag   diagram.svg     # SVG → svg_render
rdw send --id data   report.csv      # CSV → formatter set to csv
rdw send --id docs   README.md       # Markdown → formatter set to markdown
rdw send --id log    output.txt      # Plain text → appended as lines
```

Type is detected from magic bytes first, then extension.

---

## Scrollback bookmarks

Bookmarks mark named positions in a pane's scrollback by line index. They can
be set inline via `bm:` control sequences or via the API.

```sh
# Via API
curl -X PUT http://localhost:7681/api/v1/panes/log/bookmarks/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"line_index": 142}'

# List bookmarks
curl http://localhost:7681/api/v1/panes/log/bookmarks \
  -H "Authorization: Bearer $TOKEN"
```

---

## Highlight profiles

Named regex profiles colour-match text in the browser. Profiles are stored on
the server and applied per-pane.

```sh
# Define a profile
curl -X PUT http://localhost:7681/api/v1/highlights/errors \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"rules":[{"pattern":"ERROR","class":"hl-error"},{"pattern":"WARN\\w+","class":"hl-warn"}]}'

# Apply to a pane via inline control sequence
echo "hl:errors" | rdw pipe --id log
```

CSS classes `hl-error`, `hl-warn` etc. are yours to style in a custom
stylesheet or via the config.

---

## Stream mirroring

Mirror a stream to a file or command while simultaneously sending it to rdw.

```sh
# Mirror to file
your_script | rdw pipe --id log --forward-to-file /tmp/debug.log

# Mirror to command
your_script | rdw pipe --id log --forward-to-cmd "grep ERROR >> errors.log"

# Also forward to bash-rd
your_script | rdw pipe --id log --forward rd
```

---

## Focus cycle

Automatically rotate browser focus through a list of windows at a set interval.
Useful for wall-screen / dashboard rotation.

```sh
# Start cycling every 10 seconds
curl -X POST http://localhost:7681/api/v1/cycle/start \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"windows":["build","logs","metrics"],"interval_ms":10000}'

# Stop
curl -X POST http://localhost:7681/api/v1/cycle/stop \
  -H "Authorization: Bearer $TOKEN"
```

---

## Terminal panes

Launch an interactive terminal (via ttyd or socat) inside a pane running as
the restricted `nobody` user.

```sh
curl -X POST http://localhost:7681/api/v1/panes/shell/terminal \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"cmd":"/bin/bash"}'
# Returns {"port":8682,"url":"http://127.0.0.1:8682"}
```

Requires `ttyd` or `socat` to be installed.

---

## Binary data

```sh
echo "b64:$(base64 < image.png)" | rdw pipe --id chart
```

---

## Key-value store

```sh
rdw kv set build.status passing
rdw kv get build.status
# Namespace by window or pane
rdw kv set window:build:title "Build CI"
rdw kv set pane:stdout:color green

# Persistence
rdw server start --kv-persist ~/.rdw/kv.db
```

---

## Tokens

```sh
rdw token create --panes build-log --expiry 8h
# prints token once — not stored in plain text
rdw token list
rdw token revoke TOKEN_ID
```

CLI from the same user as the server authenticates automatically via Unix socket.

---

## Export

```sh
rdw save all --out-dir ./export
# writes export/session.md and export/assets/
```

---

## Configuration

`~/.config/rdw/config.yaml`:

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
  level: info
  format: console
bindings:
  # override individual actions
  # pane.focus.left: [Left]
  # window.next: [Tab]
  # pane.swap: []   # disable
```

---

## bash-rd compatibility

```sh
rdw pipe --id my-pane --forward both   # rdw and rd simultaneously
```

The `=:key=value` syntax, `b64:` encoding, and all 8 control sequence
prefixes are wire-compatible with bash-rd.

---

## CI / headless use

```sh
rdw selftest   # 11 checks, exits 0 on success
```

---

## Development

```sh
make build      # build binary
make test       # -race tests
make vet        # go vet
make coverage   # coverage report
make selftest   # build + rdw selftest
make clean
```

---

## Reference

- `rdw --help`
- [docs/manual.md](docs/manual.md) — user manual
- [docs/requirements.md](docs/requirements.md) — functional requirements
- [docs/status.md](docs/status.md) — implementation status

---

## License

Artistic License 2.0

## Author

Nadim Khemir — [https://github.com/nkh](https://github.com/nkh)
