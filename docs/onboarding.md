# rdw — engineer onboarding

## Prerequisites

- Go 1.22+
- gcc (for `mattn/go-sqlite3` CGO build)
- A browser (Chromium, Firefox — evergreen)
- Optional: `ttyd` or `socat` for terminal pane support

## Clone and build

```sh
git clone https://github.com/nkh/rdw
cd rdw
go mod download
go build -o rdw ./main
./rdw selftest     # 11/11 should pass
```

## Repository layout

```
cmd/rdw/          CLI entry point and all cobra commands
internal/
  auth/           token store (SHA-256 hashed)
  bindings/       32 named keyboard actions, vim-like defaults
  browser/        cross-platform xdg-open/open/rundll32
  config/         YAML config loader
  control/        inline control sequence parser
  cycle/          focus cycle automation
  discovery/      multi-server registry (~/.cache/rdw/servers.json)
  export/         Markdown+assets bundle writer
  format/         six content formatters
  highlight/      regex highlight profile store
  kvstore/        in-memory KV store + SQLite persistence
  layout/         YAML layout schema (WindowSpec, PaneSpec)
  mirror/         stream tee to file or command
  pipe/           client-side output relay with reconnect queue
  pipeline/       per-pane line processing (filter chain, sinks)
  router/         TargetID → Pipeline routing
  selftest/       11 in-process smoke checks
  server/         HTTP/WebSocket daemon, REST API, embedded browser SPA
  session/        TargetID, ScrollbackBuffer, Manager, BookmarkStore
  terminal/       gotty/socat terminal pane launcher
docs/
  architecture.md  data flow, concurrency model, package descriptions
  manual.md        complete user manual (23 sections)
  requirements.md  original specification
  worklog.md       development history
  onboarding.md    this file
  examples.md      annotated usage examples
  reference.md     complete API and config reference
  improvements.md  60 possible improvements
man/rdw.1         groff man page
.goreleaser.yaml  release config (Linux + macOS, amd64 + arm64)
```

## Run tests

```sh
go test ./...                          # all packages
go test ./internal/server/... -v       # verbose server tests
go test ./... -coverprofile=c.out && go tool cover -html=c.out
```

Current: 353 tests, `go vet` clean.

## Run the server locally

```sh
./rdw server start --no-auth --open-browser
echo "hello world" | ./rdw pipe --id test
```

`--no-auth` disables token checks. Never use it in production.

## Key design decisions

**Why Go?** Goroutine-per-pane stream model fits naturally. Single static binary
with embedded SPA via `embed`. Standard library HTTP/WebSocket. Fast iteration
cycle. SQLite via CGO (`mattn/go-sqlite3`).

**Why a Unix socket?** The pipe client uses `$XDG_RUNTIME_DIR/rdw/<id>.sock`
(mode 0600) for owner-only auth without a token. HTTP Bearer token is the
fallback for remote use.

**Why a circular ScrollbackBuffer?** Bounded memory per pane regardless of how
long a process runs. Cap is configurable per pane or globally in config.

**Why embedded SPA?** No external assets means `rdw` works offline, in CI,
and in air-gapped environments with no setup beyond the binary.

## Adding a formatter

1. Create `internal/format/myfmt.go` implementing `Formatter` interface
2. Register in `format.go`'s `registry` map
3. Add tests in `format_test.go`
4. The browser picks it up automatically via `GET /api/v1/formatters`

## Adding a control sequence prefix

1. Add a constant to `internal/control/control.go`
2. Add to `knownKinds` (single-char) or `multiKinds` (multi-char)
3. Dispatch in `internal/server/server.go`'s `SetControlHandler` callback
4. Add browser handler in `internal/server/frontend.go`'s `ws.onmessage` switch
5. Document in man page and README control sequences table

## Adding a REST endpoint

1. Register in `internal/server/routes.go` via `authed()`/`unauth()`/`admin()`
2. Implement handler method on `*Server`
3. Add test in `internal/server/api_test.go`
4. Wire CLI command in `cmd/rdw/commands.go`
5. Update `docs/reference.md`

## Dependency management

The project uses a local module proxy (built during development due to network
restrictions). In a normal environment `go mod download` fetches from the
internet directly. The proxy setup scripts are not committed.

## Release

`goreleaser release --clean` builds Linux + macOS binaries for amd64 and arm64.
Requires a `GITHUB_TOKEN` with `contents: write`. See `.goreleaser.yaml`.
