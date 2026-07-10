# Terminal Panes

Launch an interactive terminal inside a pane running as the restricted `nobody` user. Requires `ttyd` or `socat`.

```sh
curl -X POST http://localhost:7681/api/v1/panes/shell/terminal \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"cmd":"/bin/bash"}'
# Returns: {"id":"shell","port":8682,"url":"http://127.0.0.1:8682"}
```

Open the returned URL in a browser tab for the interactive terminal.

## Stop

```sh
curl -X DELETE http://localhost:7681/api/v1/panes/shell/terminal \
  -H "Authorization: Bearer $TOKEN"
```

## Security

The subprocess runs as `nobody` via `su -s /bin/sh nobody`. If the rdw process lacks privilege to `su`, the launch fails immediately with a clear error — there is no silent fallback.
