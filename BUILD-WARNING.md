# BUILD WARNING — BROKEN ON Go 1.22

This codebase currently fails to build with Go 1.22 on Ubuntu 24.04 due to an
internal compiler panic in the Go 1.22 toolchain when processing
`github.com/mattn/go-sqlite3`'s 9MB C amalgamation file (sqlite3-binding.c).

## Symptom

```
# github.com/mattn/go-sqlite3
<unknown line number>: internal compiler error: panic: runtime error: invalid memory address or nil pointer dereference
```

## Workaround

Use **Go 1.23** with an explicit `-gcflags` override:

```sh
# Build
go build -gcflags="github.com/mattn/go-sqlite3=-lang=go1.22" ./...

# Test (vet must be disabled for the same reason)
go test -vet=off -gcflags="github.com/mattn/go-sqlite3=-lang=go1.22" ./...
```

## Root cause

- Go 1.22's internal C compiler frontend panics on `sqlite3-binding.c`
- Go 1.23 fixes this but imposes stricter `-lang` enforcement, which breaks
  `go-sqlite3 v1.14.22` (declares `go 1.19` but uses `any` from go1.18+)
- The `-gcflags` workaround forces go1.22 lang compat for that package only

## Permanent fix options

1. Upgrade `go-sqlite3` to v1.14.23+ (fixes the `any` usage)
2. Replace `mattn/go-sqlite3` with `modernc.org/sqlite` (pure Go, no CGO)
3. Add `-gcflags` to `Makefile` and CI config as the interim workaround
