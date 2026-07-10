# CI and Headless Use

rdw works without a browser. The server runs as a daemon; CI pipelines stream output to it; results are exported or queried via the API.

## Basic CI setup

```sh
rdw server start --no-auth &
SERVER_PID=$!
sleep 1

make build 2>&1 | rdw pipe --id build
make test  2>&1 | rdw pipe --id test

rdw status --json > ci-status.json
rdw save /tmp/ci-export

kill $SERVER_PID
```

## Selftest

```sh
rdw selftest   # exits 0 if all 11 checks pass
```

## Querying results

```sh
# Check scrollback of a pane
curl http://localhost:7681/api/v1/status/panes/build \
  -H "Authorization: Bearer $TOKEN" | jq .scrollback_len

# Read KV written by the build
rdw kv get build.status
rdw kv get build.duration
```

## Headless browser check

The server's selftest includes a live HTTP ping but does not require a browser. CI smoke testing works without a display.
