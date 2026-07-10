# Export

Export pane scrollback to a Markdown bundle. Images are saved in an `assets/` subdirectory.

## Commands

```sh
rdw save /tmp/export                    # export entire session
rdw save /tmp/export --pane build-log  # single pane
rdw save /tmp/export --window ci       # single window
```

## Output format

```
/tmp/export/
  session.md        (or window-name.md / pane-id.md)
  assets/
    image-0.png
    image-1.svg
```

ANSI escape sequences are stripped. Base64-encoded images in the scrollback are decoded and saved as files; a Markdown image reference is inserted in their place.

## Via API

```sh
curl -X POST http://localhost:7681/api/v1/export/all \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"out_dir":"/tmp/export"}'

curl -X POST http://localhost:7681/api/v1/export/pane \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"id":"build-log","out_dir":"/tmp/export"}'

curl -X POST http://localhost:7681/api/v1/export/window \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"ci","out_dir":"/tmp/export"}'
```
