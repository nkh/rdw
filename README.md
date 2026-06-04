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
- Renders plain text with ANSI 24-bit color passthrough
- Maintains a session-scoped key-value store accessible from any stream
- Exposes every CLI command identically via a REST API at `/api/v1/`
- Multiple server instances can run simultaneously on different ports
- Runs entirely from a single static binary — no runtime dependencies

---

## Concepts

```
Session
  └── Window  (server-managed view, shown one at a time in the browser)
        └── Pane  (bounded view area, receives one Target ID)
              └── Target ID  (the name you give a stream)
```

**Windows** are server-managed. The browser displays one window at a time with
a persistent header bar listing all window names. Switch with `gt`/`gT` or by
clicking the header.

A **Target ID** matches `[a-zA-Z0-9_][a-zA-Z0-9_ -]*`, max 64 characters.

---

## Project status

The server, REST API, CLI, and core data pipeline are complete. The browser UI
is a functional placeholder (connects, receives lines, renders ANSI color). The
full interactive layout editor and formatters are next.

| Package | Description | Coverage |
| --- | --- | --- |
| `internal/session` | TargetID, ScrollbackBuffer, Manager | 98.6% |
| `internal/kvstore` | Session-scoped KV store | 98% |
| `internal/control` | Control sequence parser | 100% |
| `internal/pipeline` | Line relay, filter chain, KV dispatch | 93% |
| `internal/auth` | SHA-256 hashed token access control | 91% |
| `internal/router` | Target ID to pipeline mapping | 85.5% |
| `internal/layout` | Window/pane schema, YAML parser | 100% |
| `internal/bindings` | Keyboard binding model, vim defaults | 100% |
| `internal/pipe` | Client-side stdin relay | 67.3% |
| `internal/server` | HTTP/WebSocket server, full REST API | 67.9% |
| `internal/config` | YAML config loader | 76% |
| `internal/discovery` | Multi-server registry | 58% |
| `internal/selftest` | In-process smoke suite (11 checks) | 75.2% |

244 tests · 67.3% overall statement coverage

---

## Install

```sh
go install github.com/nkh/rdw@latest
```

Or build from source:

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

Listens on port `7681` by default.

Send data to a pane:

```sh
your_script | rdw pipe --id build-log

# route into a named window
your_script | rdw pipe --id build-log --window build

# with a layout — create on first use, reuse on subsequent calls
your_script | rdw pipe --id build-log --layout ./layouts/debug.yaml
```

Stop the server:

```sh
rdw server stop
```

---

## Multiple servers

```sh
rdw server start --port 7681
rdw server start --port 7682

your_script | rdw pipe --id log --port 7682
rdw server list
rdw server stop --port 7682
```

When `--port` is omitted, port 7681 is tried. If nothing is there, all
registered instances are listed in the error message.

---

## Layout description language

Layouts are YAML files with a versioned schema:

```yaml
schema_version: 1
name: debug

windows:
  - name: build
    panes:
      - target_id: stdout
      - target_id: stderr
        split: h        # h = below, v = right
        size: 30%       # N (columns), Npx, N%

  - name: metrics
    panes:
      - target_id: cpu
      - target_id: mem
        split: v
        size: 50%
```

Apply from CLI or pass directly to `pipe`:

```sh
rdw layout apply debug
rdw layout apply ./layouts/debug.yaml
your_script | rdw pipe --id build-log --layout debug
```

---

## Window management

```sh
rdw window create build
rdw window list
rdw window focus metrics
rdw window rename build build-v2
rdw window close build-v2
```

---

## Pane management

```sh
rdw pane split build-log v error-log
rdw pane resize build-log right 40%
rdw pane zoom error-log
rdw pane close error-log
```

---

## Keyboard bindings

All 32 actions have vim-like defaults. Configurable in `config.yaml`.

| Key | Action |
| --- | --- |
| `gt` / `gT` | next / previous window |
| `g0` / `g$` | first / last window |
| `gn` / `gx` / `gr` | new / close / rename window |
| `h` / `j` / `k` / `l` | focus pane left/down/up/right |
| `s` / `v` | split pane horizontally / vertically |
| `H` / `J` / `K` / `L` | resize pane |
| `q` / `z` / `R` / `x` | close / zoom / rename / swap pane |
| `Ctrl+u` / `Ctrl+d` | scroll up/down |
| `gg` / `G` | scroll to top / bottom |
| `/` / `n` / `N` | search open / next / prev |
| `Ctrl+w s` / `Ctrl+w r` | layout save / reload |

Override any binding in `config.yaml`:

```yaml
bindings:
  pane.focus.left:  [Left]
  window.next:      [Tab]
  pane.swap:        []   # disable
```

---

## Key-value store

```sh
rdw kv set build.status passing
rdw kv get build.status
rdw kv delete build.status

# inline from a stream
echo "=:build.status=passing;duration=42s" | rdw pipe --id build-log

# SQLite persistence
rdw server start --kv-persist ~/.rdw/kv.db
```

---

## Control sequences

| Prefix | Effect |
| --- | --- |
| `v:` | verbatim — pass through without interpretation |
| `q:` | quit the server |
| `s:` | semaphore — decrement counter; quit at zero |
| `c:` | clear scrollback |
| `t:` | toggle timestamp |
| `f:` | set formatter |
| `r:` | relay output |
| `=:` | write KV pairs (`key=val;key2=val2`) |

---

## Binary data

```sh
echo "b64:$(base64 < image.png)" | rdw pipe --id chart
```

Base64 lines are decoded transparently before rendering.

---

## Tokens and access control

```sh
rdw token create --panes build-log --expiry 8h
rdw token list
rdw token revoke <token-id>
```

CLI commands from the same Unix user that started the server authenticate
automatically via a Unix domain socket. Remote use requires a token.

The server binds to loopback only by default. `--network-expose` opts in to
network access.

---

## Export

```sh
rdw save pane --target-id build-log --out-dir ./export
rdw save all --out-dir ./export
```

---

## bash-rd compatibility

```sh
rdw pipe --id my-pane --forward rd     # rd only
rdw pipe --id my-pane --forward both   # rdw and rd
```

The `=:key=value` control sequence, `b64:` encoding, and all 8 control
sequence prefixes are identical to bash-rd.

---

## Headless and CI use

```sh
rdw selftest    # exits 0 on success
```

11 in-process checks covering all core packages including a live server ping.

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
  # override individual keys here
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
- `rdw completion bash` — bash autocompletion
- `man rdw` — UNIX man page (forthcoming)
- [docs/manual.md](docs/manual.md) — full user manual
- [docs/requirements.md](docs/requirements.md) — functional requirements
- [docs/status.md](docs/status.md) — implementation status and plan

---

## License

Artistic License 2.0 — see [LICENSE](LICENSE).

## Author

Nadim Khemir — [https://github.com/nkh](https://github.com/nkh)
