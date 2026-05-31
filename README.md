# rdw — Remote Display Web

`rdw` pipes your program's output into a browser. Any process that can write
to a file descriptor can stream text, images, JSON, Markdown, or CSV into a
named pane in a multi-window web layout — locally or over the network.

It is the web-native successor to [bash-rd](https://github.com/nkh/bash-rd),
rewritten in Go as a self-contained binary with a persistent daemon,
token-based access control, and a live browser UI.

```sh
your_script | rdw pipe --id build-log
```

---

## What it does

- Routes `stdin` from any process to a named pane in the browser
- Supports multiple concurrent streams in split panes across multiple windows
- Renders plain text, ANSI color, JSON trees, YAML trees, Markdown, CSV grids,
  and images
- Maintains a session-scoped key-value store accessible from any stream or
  formatter
- Exposes every CLI command identically via a REST API at `/api/v1/`
- Runs entirely from a single static binary — no runtime dependencies, no
  internet required at runtime

---

## Concepts

```
Session
  └── Window  (browser tab)
        └── Pane  (bounded view area, receives one Target ID)
              └── Target ID  (the name you give a stream)
```

A **Target ID** is a string matching `[a-zA-Z0-9_][a-zA-Z0-9_ -]*`, max 64
characters, that maps a data stream to a pane.

A **Filter** is an external executable that transforms stream data before
rendering (max 8 stages per chain).

A **Formatter** renders a pane as HTML from the incoming stream and the current
KV store state.

A **Control Sequence** is a line in the stream with a recognised two-character
prefix (`x:`) that triggers a side-effect instead of rendering as content.

---

## Project status

Early implementation. The core pipeline, KV store, auth, config, layout
schema, and CLI scaffold are complete and tested. The HTTP/WebSocket server,
browser UI, and REST API are the next layer.

| Package | Description | Coverage |
| --- | --- | --- |
| `internal/session` | TargetID validation, ScrollbackBuffer | 100% |
| `internal/kvstore` | Session-scoped KV store | 98% |
| `internal/control` | Inline control sequence parser | 100% |
| `internal/pipeline` | Line relay, filter chain, KV dispatch | 93% |
| `internal/auth` | SHA-256 hashed token access control | 91% |
| `internal/config` | YAML config loader with validation | 76% |
| `internal/layout` | Window/pane schema, resize arg parser | 100% |
| `internal/selftest` | In-process smoke test suite | 80% |

96 tests, 83.2% overall statement coverage.

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

Send data to a pane:

```sh
# plain text
your_script | rdw pipe --id my-pane

# formatted JSON
curl -s api.example.com/status | rdw json --id api-status

# image from a file
rdw image --id chart --path ./output.png

# Markdown document
cat NOTES.md | rdw markdown --id notes
```

Stop the server:

```sh
rdw server stop
```

---

## Pane layout

Create and split panes from the CLI or by dragging in the browser.

```sh
# create a window from a layout config file
rdw window create --config layouts/debug.yaml

# split an existing pane vertically and assign a new target
rdw pane split build-log v error-log

# zoom a pane to full window
rdw pane zoom error-log

# resize — columns (default), pixels, or percentage
rdw pane resize build-log right 40%
rdw pane resize build-log right 200px
rdw pane resize build-log right 40

# swap two panes
rdw pane swap build-log error-log

# detach a pane into another window
rdw pane detach build-log --to-window debug
```

Layouts can be saved and restored:

```sh
rdw layout save --name debug-session
rdw server start --restore
```

Layout files carry a `schema_version` field. The server rejects files with an
unrecognised version.

---

## Key-value store

The server maintains a session-scoped KV store. Any stream or formatter can
read and write it.

```sh
# write from the CLI
rdw kv set build.status passing

# write inline from a stream using a control sequence
echo "=:build.status=passing;build.duration=42s" | rdw pipe --id build-log

# read from the CLI
rdw kv get build.status

# delete a key
rdw kv delete build.status

# optional SQLite persistence across server restarts
rdw server start --kv-persist ~/.rdw/kv.db
```

Keys match the pattern `[a-zA-Z0-9_][a-zA-Z0-9_ :-]*`, max 64 characters.
Values are capped at 64 KB each; the total store is capped at 64 MB.

Window- and pane-scoped keys use a prefix convention:

```
window:<window_name>:<key>
pane:<target_id>:<key>
```

---

## Control sequences

A line whose first two bytes are a recognised letter and a colon is treated as
a control sequence and is not forwarded to the renderer.

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

Example — clear a pane and set a KV value from within a script:

```sh
echo "c:" | rdw pipe --id build-log
echo "=:stage=linking;status=running" | rdw pipe --id build-log

# send a line that looks like a control sequence without it being interpreted
echo "v:=:this is literal content" | rdw pipe --id build-log
```

---

## Binary data

Binary payloads must be base64-encoded with a `b64:` prefix:

```sh
base64_data=$(base64 < image.png)
echo "b64:${base64_data}" | rdw pipe --id chart
```

The server decodes transparently before passing the data to formatters or the
scrollback buffer. This convention matches bash-rd.

---

## Filters and formatters

A **filter** is any external executable that reads stdin and writes stdout.
Filters are chained before rendering (maximum 8 stages per pane).

```sh
# configured in the layout file or via the REST API
# filter chain: strip ANSI codes | grep for ERRORs | colorize
```

A **formatter** renders a pane as HTML from the stream and the KV store.
Formatters are set per pane via the `f:` control sequence or the layout file:

```sh
echo "f:my_formatter" | rdw pipe --id build-log
```

---

## Sharing and access control

Tokens scope access to specific panes or windows. The plain-text token is shown
once at creation time; the server stores only the SHA-256 hash.

```sh
# create a token scoped to one pane, expiring in 8 hours
rdw token create --panes build-log --expiry 8h

# revoke immediately (terminates all active WebSocket connections for that token)
rdw token revoke <token-id>
```

CLI commands from the same Unix user that started the server are authenticated
via a Unix domain socket at `$XDG_RUNTIME_DIR/rdw/<session_id>.sock`. Remote
CLI use (via REST) requires a token.

The admin console and server are loopback-only by default. `--network-expose`
at startup extends access to the configured network interface.

---

## Exporting

Download a pane, window, or full session as a Markdown bundle:

```sh
rdw save pane --target-id build-log --out-dir ./export
rdw save all --out-dir ./export
```

The output directory contains a Markdown file structured as one top-level
heading per window, one second-level heading per pane, scrollback content as
body text, and an `assets/` subfolder holding all streamed binary images.

---

## bash-rd compatibility

`rdw` is the successor to [bash-rd](https://github.com/nkh/bash-rd). Opt-in
forwarding to a running `rd` instance is available:

```sh
rdw pipe --id my-pane --forward rd      # send to rd only
rdw pipe --id my-pane --forward both    # send to both rdw and rd
```

The KV store wire format, `=:key=value` control sequence syntax, and `b64:`
binary encoding convention are identical to bash-rd.

---

## Headless and CI use

The server runs without a browser attached. Use `rdw selftest` as a smoke test
in CI:

```sh
rdw selftest   # exits 0 on success, non-zero on failure
```

The server can be started, fed data, and queried entirely via the REST API at
`http://localhost:7681/api/v1/`.

---

## Configuration

The server reads `~/.config/rdw/config.yaml` by default. Pass `--config` to
override.

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
  persist_path: ""   # set to a file path to enable SQLite persistence

log:
  level: info        # debug | info | warn | error
  format: console    # console | json
```

---

## Security

- The admin console and server are loopback-only unless `--network-expose` is
  passed explicitly.
- Right-click menu execution of shell commands is disabled by default and
  requires a startup flag to enable.
- Terminal-sharing panes (via gotty) require a dedicated restricted Unix user
  account; the server refuses to start a terminal pane without one configured.
- All stream content is HTML-sanitised before rendering. Raw script execution
  in panes requires an explicit opt-in flag.
- Tokens are stored as SHA-256 hashes only. The plain-text token is shown once
  on creation and never stored.
- Unauthenticated REST endpoints are rate-limited to 10 requests per minute per
  source IP.

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
