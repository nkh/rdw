# Layouts

A layout describes how windows and panes are arranged. Layouts can be created interactively in the browser, defined in YAML files, or built programmatically via the API.

## Interactive layout editing

In the browser:

| Action | Key |
| --- | --- |
| Split horizontal | `s` |
| Split vertical | `v` |
| Close pane | `x` |
| Zoom pane | `z` |
| Swap panes | `S` then `h/j/k/l` |
| Resize | drag the gutter |
| Rename pane | double-click header |

## YAML layout files

```yaml
schema_version: 1
windows:
  - name: ci
    panes:
      - target_id: build
        split: h
        size: 60%
      - target_id: test
        split: v
        size: 40%
  - name: logs
    panes:
      - target_id: syslog
```

Apply: `rdw layout apply ci.yaml`

## Commands

```sh
rdw layout apply NAME|PATH   # apply saved or on-disk layout
rdw layout save NAME         # snapshot current session
rdw layout list              # list saved layouts
```

## Applying a layout on pipe connect

```sh
make build 2>&1 | rdw pipe --id build --layout ci.yaml
```

If the layout is already active, the stream is routed into it. If not, the layout is applied first.
