# rdw — Remote Display Web

**rdw** pipes process output into a browser. Any process that writes to its stdout can stream text, images, JSON, Markdown, or CSV into a named pane in a multi-window layout — locally or over the network.

```sh
your_script | rdw pipe --id build-log
```

rdw is the web-native successor to [bash-rd](https://github.com/nkh/bash-rd), rewritten in Go as a self-contained binary with a persistent daemon, token-based access control, and a live browser UI.

## What rdw does

- Routes process output to named panes in the browser in real time
- Manages multiple windows within a single browser page
- Supports multiple concurrent streams in split panes
- Full ANSI 16/256/true-colour rendering
- Session-scoped key-value store accessible from any stream
- Full REST API at `/api/v1/` with CLI command parity
- Multiple server instances on different ports
- Single static binary — no runtime dependencies, no CDN, no internet required

## Key concepts

| Term | Meaning |
| --- | --- |
| **Target ID** | Identifier for a pane stream, e.g. `build-log` |
| **Pane** | A display area in the browser receiving one stream |
| **Window** | A named group of panes, like a tmux window |
| **Session** | The running server instance with all its windows |
| **Pipeline** | The per-pane processing chain (filters → scrollback → browser) |
| **KV store** | Session-scoped key-value store shared across all panes |

## Project status

378 tests · 11/11 selftest · `go vet` clean · [GitHub](https://github.com/nkh/rdw)
