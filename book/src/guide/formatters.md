# Built-in Formatters

Formatters transform the pane scrollback into HTML for browser display. They run on demand and do not affect the stored data.

## Available formatters

| Name | Description |
| --- | --- |
| `text` | Default; ANSI colour passthrough, HTML-escaped |
| `json` | Syntax-highlighted, pretty-printed JSON (one object per line) |
| `yaml` | Multi-document YAML with key/value colouring |
| `markdown` | Rendered HTML: headings, bold, lists, code fences, links |
| `csv` | Sortable HTML table; header row auto-detected; TSV supported |
| `image` | base64-decoded PNG/JPEG/GIF/SVG/WebP as `<img>` element |

## Switching formatter

**Inline from the stream:**
```sh
echo "f:json" | rdw pipe --id api
curl -s https://api.example.com/status | rdw pipe --id api
```

**Via the CLI:**
```sh
rdw formatter list
```

**Via the API:**
```sh
curl -X POST http://localhost:7681/api/v1/panes/api/format \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"formatter":"json"}'
```

Returns `{"html":"..."}` — the rendered HTML string.

## CSV sort

Click any column header in the browser to sort ascending; click again for descending. Numeric columns are detected automatically.

## Formatter save/restore

When `image:` or `svg:` sentinel blocks are processed, the active formatter is saved, switched to image or svg for the display, then restored. This is automatic — the producer does not need to manage formatter state.

## User-defined formatters

See [User-Defined Formatters](user-formatters.md) for adding your own.
