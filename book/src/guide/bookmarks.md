# Scrollback Bookmarks

Named positions in a pane's scrollback buffer, identified by line index.

## Create from stream

```sh
echo "bm:deploy-start" | rdw pipe --id deploy
```

The bookmark is set at the current scrollback length at the moment the line is processed.

## Create via API

```sh
curl -X PUT http://localhost:7681/api/v1/panes/log/bookmarks/milestone \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"line_index":250}'
```

## List bookmarks

```sh
curl http://localhost:7681/api/v1/panes/log/bookmarks \
  -H "Authorization: Bearer $TOKEN"
```

Returns bookmarks sorted by `line_index`.

## Delete a bookmark

```sh
curl -X DELETE http://localhost:7681/api/v1/panes/log/bookmarks/milestone \
  -H "Authorization: Bearer $TOKEN"
```

## View in status

```sh
rdw status pane log
```

Shows all bookmarks with their line indices and creation timestamps.
