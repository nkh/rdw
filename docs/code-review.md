# rdw code review

## Summary

The codebase is well-structured, all packages compile cleanly, `go vet` is
clean, and 353 tests pass. This report identifies concrete issues and
architectural weaknesses for prioritisation.

---

## Bugs

### `cycleCancel` accessed without a mutex

`Server.cycleCancel` is a `context.CancelFunc` written in `handleCycleStart`
and `handleCycleStop`, both called from concurrent HTTP handler goroutines.
There is no mutex protecting it. A concurrent start and stop can race.

Fix: add a `cycleMu sync.Mutex` field and lock it in both handlers.

```go
// routes.go
s.cycleMu.Lock()
defer s.cycleMu.Unlock()
```

### Swap handler silently ignores window reference

In `handlePaneSwap`, after the swap the local variables `winA` and `winB` are
silenced with `_ = winA ; _ = winB`. If the pane's parent window needs to
notify observers or update its own state, these references are already
available but unused. Currently harmless, but will cause confusion if window
state is ever extended.

Fix: remove the blank identifiers or, if they are not needed, remove the
assignments.

### `handleCycleStart` goroutine leaks on server shutdown

The goroutine started in `handleCycleStart` runs until the cycle's context is
cancelled. If the server shuts down without calling `handleCycleStop`, the
goroutine continues running until the context passed to `cycle.Run` is
cancelled — but that context is derived from `context.Background()`, not from
the server's shutdown context. The goroutine will not stop when the server
stops.

Fix: derive the cycle context from the server's run context rather than
`context.Background()`.

### Terminal pane ports start at `server.port + 1000`

`terminal.New(port + 1000)` allocates terminal ports starting 1000 above the
server port. This can collide with other services and with other rdw instances.
The `allocPort` loop tries up to 100 ports before giving up. On a busy machine
with multiple rdw instances near the same port range, allocation will fail.

Fix: use `net.Listen("tcp", ":0")` to let the OS assign a free port, or
document the port range clearly and make it configurable.

---

## Data races (potential)

### `Server.kvDB` written in `Run`, read in handlers

`kvDB` is assigned in `Run()` after construction and before accepting
connections. Provided the server's `httpSrv.ListenAndServe` is called after
the assignment, no race occurs. But there is no structural guarantee of this
ordering — if the initialization order is changed the race becomes real.

Fix: assign `kvDB` in `New()` rather than `Run()`, using the options already
available.

### `Server.highlights` and `Server.terminals` are thread-safe internally

`highlight.Store` and `terminal.Manager` both use their own `sync.RWMutex`.
No issue here.

---

## Architecture concerns

### `cycleCancel` belongs in a dedicated cycle state struct

The bare `context.CancelFunc` field on `Server` has no associated metadata
(which windows, what interval). If the API is extended to query cycle state,
the field is insufficient. A `type cycleState struct { cancel context.CancelFunc ; windows []string ; interval time.Duration }` pointer would be cleaner and make the nil check
self-documenting.

### `handleLayoutApply` restores `PaneState` but does not reconcile the Router

When a snapshot is restored via `RestoreSnapshot`, the session manager replaces
its window/pane list. The `Router` retains all previously registered pipelines
keyed by `TargetID`. Panes present in the old session but absent in the
snapshot are never deregistered from the router. Panes present in the snapshot
but absent from the router have no pipeline and will produce 404 on stream
ingest until a new `rdw pipe` connects.

This is not a crash, but it is a semantic gap. A proper apply should diff the
old and new pane sets and call `router.Deregister` / `router.Register`
accordingly.

### `Server.layoutMu` and `Server.layouts` are separate from the session Manager

Saved layouts are stored as `map[string][]byte` on the Server under
`layoutMu`, while live session state is in the Manager. This creates two
sources of truth. If a layout is saved, then the session changes, and the
layout is applied, the applied state may differ from what was saved.

Consider moving layout storage into the Manager so save/apply are atomic with
respect to session state.

### Pipeline filter chain is always empty

The `FilterChainMax` config field and `pipeline.Options.FilterChainMax` exist,
but no code path registers a filter on a newly created pipeline. The filter
chain is always empty. The `--filter` / `rdw pipe --filter` flag is not
present in the CLI.

### `internal/browser` package is one function

The entire package is a single `Open(url string) error` function. It could
be a free function in `cmd/rdw/server.go` with the same effect. The package
boundary adds indirection without providing a useful abstraction point.

### `internal/cycle` package is not integrated with the Manager

`cycle.Cycle` calls `s.manager.FocusWindow(ev.Window)` through a closure in
the server's `handleCycleStart`. The session manager does not know a cycle is
running. If a window is deleted while a cycle is running, `FocusWindow` will
return an error that is silently ignored (`_ =`). The browser will not update.

Fix: check the error and stop the cycle if the target window no longer exists.

---

## Code quality

### Silenced errors in export and session paths

`_ = s.manager.RemovePane(...)` in `handlePaneSplit` and
`handlePaneClose` silences errors from the Manager. If RemovePane fails, the
pane state is inconsistent between the manager and the broadcast sent to the
browser. These errors should be logged at minimum.

### `jsonResponse` is called `jsonResponse` in some files and inlined in others

Consistency is good — `jsonResponse` is defined and used in most routes, but
some handlers write directly to `w` with `w.WriteHeader`. No functional issue,
but a helper `apiNoContent(w)` would make the pattern uniform.

### Test helper `mustID` is defined locally in `api_test.go`

The same `ParseTargetID` wrapper is written in multiple test files. A shared
`testutil` package or a `session.MustTargetID` (panic variant for tests) would
eliminate duplication.

### `internal/terminal/terminal.go` `allocPort` uses a mutex that is also held by callers

`allocPort` acquires `m.mu`, but `Launch` calls `allocPort` without holding
`m.mu`. Then `Launch` acquires `m.mu` again to store the result. Because
`allocPort` acquires and releases the mutex independently, a concurrent `Launch`
could allocate the same port. Fix: pre-allocate the port outside the mutex, or
hold the mutex for the entire `Launch` operation.

---

## Missing functionality (already noted in status.md)

- The `--filter` flag is declared in the requirements but no CLI flag exists
- `rdw pipe --layout FILE` loads a YAML file but does not validate the
  schema_version field before sending to the server
- `sc:clear` via control sequence clears the server-side ScrollbackBuffer but
  does not broadcast a message to trigger `innerHTML = ''` in existing browser
  sessions (only new lines will be absent; old lines remain visible until reload)
