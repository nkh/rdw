# Quick Start

## 1. Start the server

```sh
rdw server start --open-browser
```

This starts the server on port 7681 and opens the browser. For development, add `--no-auth` to skip token authentication.

## 2. Pipe output to a pane

In a second terminal:

```sh
ping 8.8.8.8 | rdw pipe --id ping
```

The browser shows a pane named `ping` with live output.

## 3. Multiple panes

```sh
tail -f /var/log/syslog | rdw pipe --id syslog &
top -b               | rdw pipe --id top &
```

Split the panes in the browser by pressing `s` (horizontal) or `v` (vertical) while a pane is focused.

## 4. Send a file

```sh
rdw send --id chart chart.png    # PNG displayed as image
rdw send --id data  report.csv   # CSV displayed as sortable table
```

## 5. Check server state

```sh
rdw status
rdw status pane ping
```

## 6. Stop

```sh
rdw server stop
```

## Next steps

- [Piping Output](pipe.md) — filters, titles, binary mode, mirroring
- [Layouts](layouts.md) — multi-pane arrangements
- [Control Sequences](control.md) — inline commands from the stream
- [Key-Value Store](kv.md) — shared state across panes
