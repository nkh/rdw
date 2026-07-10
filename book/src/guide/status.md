# Introspection

## rdw status

Full server snapshot in tabular format:

```sh
rdw status
rdw status --json
```

Output includes: port, active browser connections, KV entry count, all panes (title/formatter/scrollback/bookmarks), saved layouts, registered formatters, highlight profiles, active tokens, and focus cycle state.

## rdw status pane ID

Per-pane detail:

```sh
rdw status pane build-log
rdw status pane build-log --json
```

Shows: target ID, title, formatter, saved formatter, scrollback length and cap, last line received, all bookmarks with line indices and timestamps.

## Via API

```sh
curl http://localhost:7681/api/v1/status \
  -H "Authorization: Bearer $TOKEN"

curl http://localhost:7681/api/v1/status/panes/build-log \
  -H "Authorization: Bearer $TOKEN"
```
