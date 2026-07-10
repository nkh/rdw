# Keyboard Bindings

All 32 actions are configurable. Defaults are vim-like.

## Default bindings

| Key | Action |
| --- | --- |
| `h` | pane.focus.left |
| `l` | pane.focus.right |
| `j` | pane.focus.down |
| `k` | pane.focus.up |
| `H` | pane.resize.left |
| `L` | pane.resize.right |
| `J` | pane.resize.down |
| `K` | pane.resize.up |
| `g t` | window.next |
| `g T` | window.prev |
| `g g` | window.first |
| `G` | window.last |
| `z` | pane.zoom |
| `s` | pane.split.h |
| `v` | pane.split.v |
| `x` | pane.close |
| `r` | pane.rename |
| `S` | swap.enter |
| `/` | search.open |
| `n` | search.next |
| `N` | search.prev |
| `Escape` | mode.normal |
| `?` | help |

## Overriding in config

```yaml
bindings:
  window.next: ["gt"]
  pane.zoom:   ["z", "Z"]
```

Values are lists of key strings. A key string may be a single character or a two-character sequence (e.g. `gt`).

## Fetching from the server

The SPA loads bindings at startup from `GET /api/v1/bindings`. External clients can fetch the same endpoint.
