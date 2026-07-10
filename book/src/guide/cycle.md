# Focus Cycle

Automatically rotate browser focus through a list of windows at a fixed interval. Useful for wall-screen dashboards.

## Start

```sh
rdw cycle start build logs metrics --interval-ms 10000
```

## Status

```sh
rdw cycle status
```

Output:
```
running     true
windows     [build logs metrics]
interval_ms 10000
```

## Stop

```sh
rdw cycle stop
```

## Via API

```sh
# Start
curl -X POST http://localhost:7681/api/v1/cycle/start \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"windows":["build","logs","metrics"],"interval_ms":10000}'

# Status
curl http://localhost:7681/api/v1/cycle/status \
  -H "Authorization: Bearer $TOKEN"

# Stop
curl -X POST http://localhost:7681/api/v1/cycle/stop \
  -H "Authorization: Bearer $TOKEN"
```

## Notes

- Only one cycle runs at a time; starting a new one stops the previous
- If a window is deleted while a cycle is running, that window is skipped with a log message
- The cycle stops when the server shuts down
