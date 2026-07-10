# Installation

## Prerequisites

- Go 1.22 or later
- gcc (for SQLite CGO build via `mattn/go-sqlite3`)
- A modern browser (Chromium, Firefox — evergreen)

Optional for terminal panes:
- `ttyd` or `socat`

## Build from source

```sh
git clone https://github.com/nkh/rdw
cd rdw
go mod download
go build -o rdw ./main
./rdw selftest     # 11/11 should pass
```

## Verify

```sh
./rdw --version
./rdw selftest
```

## Release binaries

Pre-built binaries for Linux and macOS (amd64 and arm64) are available via the [GitHub releases page](https://github.com/nkh/rdw/releases) and the Goreleaser config at `.goreleaser.yaml`.

## Config file location

```
~/.config/rdw/config.yaml
```

Created automatically on first run with defaults. See [Configuration](../reference/config.md) for all options.
