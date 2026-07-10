# Configuration

Default location: `~/.config/rdw/config.yaml`

```yaml
server:
  port: 7681                 # TCP port (default 7681)
  filter_chain_max: 8        # max filter stages per pipeline
  scrollback_cap: 10000      # lines per pane

auth:
  no_auth: false             # disable token auth (development only)
  admin_local_only: true     # restrict /api/v1/admin/* to loopback
  admin_token: ""            # token for /admin page (empty = open)

kv:
  persist_path: ""           # SQLite path; empty = memory only

formatters:                  # user-defined external formatters
  - name: myformat
    cmd: /usr/local/bin/myformat.sh

bindings:                    # keyboard binding overrides
  pane.focus.left: ["h"]
  layout.reload: ["R"]
```

## CLI flags override config

All `server start` flags take precedence over config file values:

```sh
rdw server start \
  --port 8080 \
  --no-auth \
  --kv-persist ~/.rdw/kv.db \
  --restore \
  --open-browser \
  --admin-token secret
```
