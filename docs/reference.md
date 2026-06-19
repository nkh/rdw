# rdw reference

## CLI commands

All commands accept `--port / -p PORT` (default 7681) to address a specific
server instance.

### rdw server

```
rdw server start [flags]
  --port           TCP port to listen on (default 7681)
  --open-browser   Open the browser UI on startup
  --no-auth        Disable token authentication
  --kv-persist PATH  Enable SQLite KV persistence at PATH
  --restore        Load persisted KV store on startup

rdw server stop
rdw server list
```

### rdw pipe

```
rdw pipe --id TARGET_ID [flags]
  --id             Target pane identifier (required)
  --layout NAME    Apply layout before streaming
  --window NAME    Route into a specific window
  --forward MODE   Also forward to: rd, rdw, or both
  --forward-to-file PATH   Mirror stream to file or FIFO
  --forward-to-cmd CMD     Mirror stream as stdin to shell command
```

`rdw pipe` reads from its stdin (the piped process's stdout) line by line and
relays each line to the named pane. It connects via Unix socket first; falls
back to HTTP with Bearer token.

### rdw window

```
rdw window create NAME
rdw window close NAME
rdw window rename OLD NEW
rdw window focus NAME
rdw window list
```

### rdw pane

```
rdw pane split ID [--dir h|v]
rdw pane resize ID SIZE       SIZE: N (cols/rows), Npx, N%
rdw pane zoom ID
rdw pane swap ID1 ID2
rdw pane close ID
```

### rdw kv

```
rdw kv set KEY VALUE
rdw kv get KEY
rdw kv delete KEY
rdw kv list [PREFIX]
```

Keys: `[a-zA-Z0-9_][a-zA-Z0-9_ -]*` (max 64 chars). Optional namespace
prefix: `window:NAME:key` or `pane:ID:key`.

### rdw token

```
rdw token create [--scope SCOPE] [--expires DURATION]
rdw token revoke ID
rdw token list
```

### rdw layout

```
rdw layout apply NAME|PATH
rdw layout save NAME
rdw layout list
```

### rdw group

```
rdw group hide NAME
rdw group show NAME
rdw group focus NAME
rdw group kill NAME
```

### rdw save

```
rdw save DIR [--pane ID] [--window NAME]
```

Exports scrollback as Markdown to DIR. Images are saved in DIR/assets/.

### rdw selftest

Runs 11 in-process smoke checks and exits 0 if all pass.

### rdw completion

```
rdw completion bash
```

---

## Control sequences

Lines piped through `rdw pipe` that begin with a recognised prefix are
intercepted and acted on rather than displayed.

| Prefix | Action |
| --- | --- |
| `v:PAYLOAD` | Verbatim — strip prefix, display PAYLOAD as-is |
| `=:k=v;k2=v2` | Write key-value pairs to the KV store |
| `f:NAME` | Set pane formatter (text/json/yaml/markdown/csv/image) |
| `b64:DATA` | Base64-encoded binary data; decoded by export and image formatter |
| `bm:NAME` | Create a scrollback bookmark at the current line |
| `hl:PROFILE` | Apply the named highlight profile to the pane |
| `sc:ACTION` | Scrollback control: clear / top / bottom |
| `t:` | Toggle timestamp prefix on subsequent lines |
| `c:` | Clear pane scrollback |
| `q:` | Stop the server |

---

## REST API

Base: `http://HOST:PORT/api/v1`

All authenticated endpoints require `Authorization: Bearer TOKEN`.
The admin endpoint additionally requires a loopback source address.
Rate limit: 10 req/min on unauthenticated endpoints.

### Health

```
GET /api/v1/ping             → 200 OK
GET /api/v1/session          → {windows, active_window}   [authed]
GET /api/v1/admin/connections → {connections:[...]}        [admin+authed]
GET /api/v1/bindings         → {actions:{key:action,...}}  [unauthed]
GET /api/v1/formatters       → {formatters:[...]}          [unauthed]
```

### WebSocket

```
GET /api/v1/ws               Upgrade to WebSocket, sub-protocol rdw-v1
```

Query: `?token=TOKEN` or `Authorization: Bearer TOKEN` header.

### Stream ingest

```
POST /api/v1/stream/{id}     body: {line:STRING}  [authed]
```

HTTP alternative to Unix socket / `rdw pipe`.

### Windows

```
GET    /api/v1/windows                → {windows:[...]}
POST   /api/v1/windows                body:{name}       → 204
DELETE /api/v1/windows/{name}                           → 204
PATCH  /api/v1/windows/{name}         body:{name}       → 204  (rename)
POST   /api/v1/windows/{name}/focus                     → 204
```

### Panes

```
POST   /api/v1/panes/{id}/split       body:{dir:"h"|"v"} → 204
POST   /api/v1/panes/{id}/zoom                           → 204
POST   /api/v1/panes/{id}/resize      body:{size}        → 204
POST   /api/v1/panes/{id}/swap        body:{target}      → 204
DELETE /api/v1/panes/{id}                                → 204
PATCH  /api/v1/panes/{id}             body:{label}       → 204  (rename)
POST   /api/v1/panes/{id}/format      body:{formatter}   → {html}
POST   /api/v1/panes/{id}/terminal    body:{cmd}         → {port,url}
DELETE /api/v1/panes/{id}/terminal                       → 204
```

### Bookmarks

```
GET    /api/v1/panes/{id}/bookmarks                     → {bookmarks:[...]}
PUT    /api/v1/panes/{id}/bookmarks/{name}  body:{line_index} → 204
DELETE /api/v1/panes/{id}/bookmarks/{name}              → 204
```

### KV store

```
GET    /api/v1/kv              ?prefix=PREFIX   → {keys:[...]}
GET    /api/v1/kv/{key}                         → {key,value}
PUT    /api/v1/kv/{key}        body:{value}     → 204
DELETE /api/v1/kv/{key}                         → 204
```

### Layouts

```
GET  /api/v1/layouts                    → {layouts:[...]}
POST /api/v1/layouts                    body:{name} → 204  (save current)
POST /api/v1/layouts/{name}/apply               → 204
```

### Tokens

```
GET    /api/v1/tokens                   → {tokens:[...]}
POST   /api/v1/tokens     body:{scope?,expires?} → {id,plain_text}
DELETE /api/v1/tokens/{id}              → 204
```

### Groups

```
POST /api/v1/groups/{name}/hide
POST /api/v1/groups/{name}/show
POST /api/v1/groups/{name}/focus
POST /api/v1/groups/{name}/kill
```

All return 204.

### Highlights

```
GET    /api/v1/highlights               → {profiles:[...]}
PUT    /api/v1/highlights/{name}        body:{rules:[{pattern,class},...]} → 204
DELETE /api/v1/highlights/{name}        → 204
```

### Export

```
POST /api/v1/export/pane    body:{id, out_dir}         → 204
POST /api/v1/export/window  body:{name, out_dir}        → 204
POST /api/v1/export/all     body:{out_dir}              → 204
```

### Cycle

```
POST /api/v1/cycle/start  body:{windows:[...], interval_ms:N} → {windows,interval_ms}
POST /api/v1/cycle/stop                                        → 204
```

---

## Layout YAML schema

```yaml
schema_version: 1
windows:
  - name: STRING          # window name, unique
    panes:
      - target_id: STRING  # TargetID pattern [a-zA-Z0-9_][a-zA-Z0-9_ -]* max 64
        split: h|v         # split direction relative to previous pane
        size: STRING       # N (cols/rows), Npx, or N%
        group: STRING      # optional group name
        private: BOOL      # hide from shared views
        scrollback_cap: N  # override global default
```

---

## Config file (`~/.config/rdw/config.yaml`)

```yaml
server:
  port: 7681
  filter_chain_max: 8
  scrollback_cap: 10000

auth:
  no_auth: false
  admin_local_only: true

kv:
  persist_path: ""     # empty = memory only
```

---

## WebSocket message types (server → browser)

| type | fields | meaning |
| --- | --- | --- |
| `line` | `target_id`, `line` | new output line for a pane |
| `layout_update` | `windows`, `active` | layout changed |
| `RECONNECT` | — | client should flush reconnect queue |
| `highlight_set` | `target_id`, `profile` | apply highlight profile |
| `scrollback_ctl` | `target_id`, `action` | clear/top/bottom |

---

## TargetID rules

- Pattern: `[a-zA-Z0-9_][a-zA-Z0-9_ -]*`
- Max length: 64 characters
- No leading space or hyphen
- Same rules apply to KV keys and layout pane identifiers

---

## Token format

Tokens are returned as plain text once. They are stored SHA-256 hashed on the
server. Expiry is a Go duration string: `1h`, `24h`, `7d`. Zero means no expiry.

---

## Keyboard bindings (browser defaults)

| Key | Action |
| --- | --- |
| `h` / `l` | focus left / right pane |
| `j` / `k` | focus down / up pane |
| `H` / `L` | resize pane left / right |
| `J` / `K` | resize pane down / up |
| `g t` | next window |
| `g T` | previous window |
| `g g` | first window |
| `G` | last window |
| `z` | toggle zoom |
| `s` | split horizontal |
| `v` | split vertical |
| `x` | close pane |
| `r` | rename pane |
| `S` | enter swap mode |
| `/` | open search |
| `n` / `N` | next / previous search match |
| `Escape` | exit mode |
| `?` | show binding help |

All bindings are configurable in `~/.config/rdw/config.yaml` under a
`bindings:` key mapping action names to key strings.
