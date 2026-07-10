# Authentication

## Tokens

All REST and WebSocket endpoints require a Bearer token unless `--no-auth` is set on the server.

```sh
# Create a token (plain text shown once only)
TOKEN=$(rdw token create --expires 24h)

# Use it
rdw kv set foo bar   # uses token from config automatically
curl -H "Authorization: Bearer $TOKEN" http://localhost:7681/api/v1/ping

# List tokens
rdw token list

# Revoke (closes active WebSocket connections using this token)
rdw token revoke TOKEN_ID
```

Tokens are stored SHA-256 hashed. The plain-text value is never stored and cannot be recovered.

## Unix socket (local, no token)

`rdw pipe` prefers the Unix socket at `$XDG_RUNTIME_DIR/rdw/<session_id>.sock` (mode 0600). The owning user connects without a token. This is the default for local use.

## Admin token

A separate token for the `/admin` introspection page. Set in config or via flag:

```yaml
auth:
  admin_token: mysecrettoken
```

```sh
rdw server start --admin-token mysecrettoken
```

## Development mode

```sh
rdw server start --no-auth
```

Never use in production.
