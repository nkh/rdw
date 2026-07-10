# Piping Output

## Basic usage

```sh
your_script | rdw pipe --id TARGET_ID
```

`rdw pipe` reads its stdin line by line and relays each line to the server pane identified by `TARGET_ID`. It connects via Unix socket (owner-auth, no token needed) and falls back to HTTP with Bearer token for remote servers.

## Flags

| Flag | Description |
| --- | --- |
| `--id ID` | Target pane (required) |
| `--title TEXT` | Set pane display title on connect |
| `--layout NAME\|PATH` | Apply layout before streaming |
| `--window NAME` | Route into a specific window |
| `--filter CMD` | Attach a filter stage (repeatable, max 8) |
| `--forward rd\|rdw\|both` | Also forward to bash-rd |
| `--forward-to-file PATH` | Mirror stream to file or FIFO |
| `--forward-to-cmd CMD` | Mirror stream as stdin to a shell command |

## Filters

```sh
my_app | rdw pipe --id log \
  --filter 'grep -v DEBUG' \
  --filter 'sed s/ERROR/[ERR]/'
```

The current KV snapshot is injected into each filter subprocess as environment variables. See [Filters](filters.md).

## Binary and image mode

Use `image:` / `image:end` sentinel framing to send binary data. rdw base64-encodes it internally:

```sh
{ echo "image:" ; cat chart.png ; echo "image:end" ; } | rdw pipe --id chart
```

Or use [rdw send](send.md) for files — type is detected automatically.

## Reconnect behaviour

If the server is unavailable, `rdw pipe` buffers up to 1000 lines. When the server comes back, buffered lines are flushed in order. Lines beyond the buffer limit are silently dropped (oldest first).
