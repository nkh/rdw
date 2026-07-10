# Admin Page

A browser-based introspection dashboard at `http://localhost:PORT/admin`. Auto-refreshes every 10 seconds.

## Setup

Set a separate admin token (distinct from session tokens):

```yaml
# ~/.config/rdw/config.yaml
auth:
  admin_token: mysecrettoken
```

Or via flag:

```sh
rdw server start --admin-token mysecrettoken
```

## Access

```
http://localhost:7681/admin?token=mysecrettoken
```

Or with a header:

```sh
curl -H "Authorization: Bearer mysecrettoken" http://localhost:7681/admin
```

## What it shows

- Server port and active WebSocket connection count
- KV store entry count
- All panes: title, formatter, scrollback length, bookmark count
- Registered formatters (built-in + user-defined)
- Highlight profiles
- Saved layouts
- Active tokens (ID, scope, expiry)
- Focus cycle state

## Security

The admin token is intentionally separate from session tokens so introspection access can be granted without giving stream write access. If no admin token is set, `/admin` is open to anyone who can reach the server (loopback by default).
