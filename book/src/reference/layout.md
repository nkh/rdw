# Layout Schema

Layout files are YAML with `schema_version: 1`.

```yaml
schema_version: 1
windows:
  - name: STRING           # window name, unique within session
    panes:
      - target_id: STRING  # routing identifier; pattern [a-zA-Z0-9_][a-zA-Z0-9_ -]* max 64
        split: h|v         # split direction relative to previous pane
        size: STRING       # N (cols/rows), Npx, or N%
        group: STRING      # optional group name
        private: BOOL      # hide from shared views (default false)
        scrollback_cap: N  # lines; overrides global default (default 10000)
```

## Notes

- `split` and `size` on the first pane in a window are ignored
- `schema_version` must equal `1`; any other value causes rejection
- The `target_id` field is validated at apply time; invalid IDs cause a 400 error
- Saved layouts are stored as JSON snapshots of the live session state; on-disk YAML files are uploaded and converted server-side
