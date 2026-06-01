# rdw User Manual

## Table of Contents

1. [Introduction](#1-introduction)
2. [Installation](#2-installation)
3. [Architecture](#3-architecture)
4. [Server Management](#4-server-management)
5. [The Window Model](#5-the-window-model)
6. [Pane Management](#6-pane-management)
7. [Routing Data Streams](#7-routing-data-streams)
8. [The Layout System](#8-the-layout-system)
9. [The Layout Description Language](#9-the-layout-description-language)
10. [Interactive Layout Editing](#10-interactive-layout-editing)
11. [Keyboard Bindings](#11-keyboard-bindings)
12. [Control Sequences](#12-control-sequences)
13. [The Key-Value Store](#13-the-key-value-store)
14. [Filters and Formatters](#14-filters-and-formatters)
15. [Binary Data](#15-binary-data)
16. [Authentication and Access Control](#16-authentication-and-access-control)
17. [Exporting](#17-exporting)
18. [Configuration Reference](#18-configuration-reference)
19. [Command Reference](#19-command-reference)
20. [bash-rd Compatibility](#20-bash-rd-compatibility)
21. [Security Model](#21-security-model)
22. [Headless and CI Use](#22-headless-and-ci-use)
23. [Troubleshooting](#23-troubleshooting)

---

## 1. Introduction

`rdw` (Remote Display Web) is a single-binary daemon that receives data from
any process and displays it in a live browser layout. Each data source is
identified by a **Target ID** — a short name you choose. The server routes
streams to named panes arranged inside server-managed windows, all rendered
in a single browser page.

`rdw` is the web-native successor to
[bash-rd](https://github.com/nkh/bash-rd). It preserves the core ideas —
Target IDs, inline control sequences, the KV store, and base64 binary
encoding — and adds a full web UI, token-based access control, multi-server
support, interactive keyboard-driven layout editing, and a REST API.

### Design principles

- **One binary.** No runtime dependencies, no CDN assets, no internet
  connection required at runtime. The frontend is compiled in.
- **Pipe-native.** Any process that writes to stdout can send data with
  `your_script | rdw pipe --id my-pane`. Nothing else is required.
- **Multiple servers.** Several rdw instances can run simultaneously on
  different ports. Every client command accepts `--port` to select the
  target instance.
- **Keyboard-driven.** The browser UI is fully operable without a mouse,
  using a vim-like default binding set that is entirely user-configurable.
- **Predictable.** The layout is a YAML file with a versioned schema. The
  server rejects files with an unrecognised schema version rather than
  guessing intent.

---

## 2. Installation

### From source

```sh
go install github.com/nkh/rdw@latest
```

### Build from repository

```sh
git clone https://github.com/nkh/rdw
cd rdw
make build          # produces ./rdw
make test           # run the test suite
make selftest       # build then run the built-in smoke tests
```

### Shell autocompletion

```sh
# Bash
rdw completion bash >> ~/.bashrc
source ~/.bashrc
```

### Verify installation

```sh
rdw selftest
```

All lines should print `[PASS]`. Exit code is 0 on success.

---

## 3. Architecture

### Components

```
┌─────────────────────────────────────────────────────┐
│  Browser                                            │
│  ┌───────────────────────────────────────────────┐  │
│  │ Window header bar:  [ build ] [ metrics ] ... │  │
│  │───────────────────────────────────────────────│  │
│  │ Active window                                 │  │
│  │  ┌──────────────────┬────────────────────┐   │  │
│  │  │ Pane: stdout     │ Pane: stderr       │   │  │
│  │  │                  │                    │   │  │
│  │  └──────────────────┴────────────────────┘   │  │
│  └───────────────────────────────────────────────┘  │
│             ▲ WebSocket (protocol: rdw-v1)           │
└─────────────┼───────────────────────────────────────┘
              │
┌─────────────────────────────────────────────────────┐
│  rdw server (port 7681)                             │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐ │
│  │ Session  │  │ KV Store │  │ Token registry    │ │
│  │ Manager  │  │          │  │                   │ │
│  └──────────┘  └──────────┘  └───────────────────┘ │
│                                                     │
│  ┌──────────────────────────────────────────────┐  │
│  │ Router: Target ID → Pipeline → Pane          │  │
│  └──────────────────────────────────────────────┘  │
│                                                     │
│  REST API /api/v1/     Unix socket (CLI auth)       │
└─────────────────────────────────────────────────────┘
              ▲
              │  rdw pipe --id build-log
              │  (stdin relay)
   your_script
```

### Key concepts

| Term | Meaning |
| --- | --- |
| Target ID | A string name that maps a data stream to a pane. Pattern: `[a-zA-Z0-9_][a-zA-Z0-9_ -]*`, max 64 chars. |
| Session | One running server instance: its windows, panes, KV store, and token registry. |
| Window | A server-managed view displayed in the browser. The browser shows one window at a time. |
| Pane | A bounded area inside a window that receives and displays data for one Target ID. |
| Pipeline | The per-pane processing chain: control sequence dispatch → filter chain → timestamp → scrollback → sinks. |
| Filter | An external executable piped into the stream before rendering. Max 8 per chain. |
| Formatter | A renderer that converts the stream and KV state to HTML inside a pane. |
| Scrollback | An in-memory circular line buffer per pane. Default cap: 10,000 lines, max: 100,000. |
| Control sequence | A line beginning with a two-character prefix (`x:`) that triggers a side-effect instead of rendering. |

### Data flow

```
stdin
  │
  ▼
rdw pipe (client binary)
  │  TCP connection to server port
  ▼
Server pipeline
  ├─ Control sequence check
  │     ├─ v: verbatim → strip prefix, treat as content
  │     ├─ =: KV write → write to store, drop line
  │     └─ other → dispatch to ControlHandler, drop line
  │
  ├─ b64: prefix → base64 decode
  │
  ├─ Filter chain (0–8 external stages)
  │
  ├─ Optional timestamp prefix
  │
  ├─ Scrollback buffer append
  │
  └─ Sinks (WebSocket broadcast, file mirrors, command pipes)
```

---

## 4. Server Management

### Starting a server

```sh
rdw server start
```

The server starts in the background, listens on the default port (7681),
and registers itself in the local server registry at
`$XDG_CACHE_HOME/rdw/servers.json`.

Common flags:

```sh
rdw server start --port 7682          # listen on a different port
rdw server start --open-browser       # open the browser automatically
rdw server start --restore            # restore the last saved session state
rdw server start --kv-persist PATH    # enable SQLite KV persistence
rdw server start --network-expose     # bind to all interfaces (not just loopback)
rdw server start --no-auth            # disable token checks (development only)
rdw server start --config PATH        # use a specific config file
```

### Multiple servers

Any number of rdw servers can run simultaneously. Each listens on a
distinct port and maintains a fully independent session.

```sh
rdw server start --port 7681   # first server (default)
rdw server start --port 7682   # second server
rdw server start --port 7683   # third server
```

Every client command has a global `--port` flag. When omitted, the client
probes port 7681. If no server answers there and other servers are
registered, the error message lists all running instances with their ports
and PIDs:

```
no rdw server on default port 7681; running servers:
  port 7682  pid 12345  started 2024-03-15T10:00:00Z
  port 7683  pid 12346  started 2024-03-15T10:05:00Z
use --port <port> to select one
```

### Stopping a server

```sh
rdw server stop              # stop server on default port
rdw server stop --port 7682  # stop a specific server
```

From within a data stream, the `q:` control sequence also stops the server:

```sh
echo "q:" | rdw pipe --id any-pane
```

### Listing running servers

```sh
rdw server list
```

This reads the server registry and displays all registered instances. Stale
entries (whose processes are no longer running) are pruned automatically.

### Server registry

The server registry is a JSON file at `$XDG_CACHE_HOME/rdw/servers.json`
(falls back to `$TMPDIR/rdw/servers.json` if the cache directory is
unavailable). It is written with permissions `0600`. Each entry records
the port, PID, start time, and Unix socket path.

The registry is used exclusively for client-side auto-detection. It is not
a security boundary; authentication is handled by the token system and
Unix socket ownership check described in section 16.

---

## 5. The Window Model

Windows in rdw are **server-managed views**, not browser tabs. The server
maintains an ordered list of named windows. The browser displays exactly one
window at a time and renders a persistent **header bar** at the top of the
page listing all window names.

```
┌──────────────────────────────────────────────────────────┐
│  [ build ] [ metrics ] [ logs ] [ alerts ]               │  ← header bar
├──────────────────────────────────────────────────────────┤
│                                                          │
│    Active window content (panes)                         │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### Why not browser tabs?

Browser tabs are managed by the browser, not the server. Using server-managed
windows means:

- The server controls window order and content
- Keyboard bindings can navigate between windows without browser shortcuts
  interfering
- The REST API and CLI can switch the active window programmatically
- Layout files define the complete multi-window arrangement in one document
- Token scoping can restrict access to specific windows

### Switching windows

**Keyboard:** use `gt` (next), `gT` (previous), `g0` (first), `g$` (last).
See section 11 for the full binding reference.

**Header bar:** click any window name in the header bar.

**CLI:**

```sh
rdw window focus metrics
```

**REST API:**

```sh
curl -X POST http://localhost:7681/api/v1/windows/metrics/focus \
  -H "Authorization: Bearer <token>"
```

### Creating and closing windows

```sh
rdw window create build            # empty window
rdw window create debug --layout ./layouts/debug.yaml   # from layout file
rdw window close old-window
```

In the browser, use `gn` to create a new empty window and `gx` to close
the active window (see section 10 for interactive editing).

### Renaming windows

```sh
rdw window rename build build-v2
```

In the browser, press `gr` to open the rename prompt for the active window.

### Listing windows

```sh
rdw window list
```

---

## 6. Pane Management

A pane is a bounded rectangular area inside a window that receives and
displays the data stream for one Target ID.

### Rules

- A window must contain at least one pane.
- A window may contain at most 64 panes.
- If a window has only one pane, closing it closes the window.
- Panes are not browser elements — they are rendered by the server's layout
  engine and streamed to the browser as WebSocket messages.

### Splitting

Panes are created by splitting existing ones. The split direction determines
where the new pane appears relative to the pane being split:

- `h` — horizontal split: new pane appears **below** the target pane
- `v` — vertical split: new pane appears **to the right** of the target pane

```sh
rdw pane split stdout v stderr       # split stdout vertically; stderr appears right
rdw pane split stdout h status       # split stdout horizontally; status appears below
rdw pane split build-log v error-log --group ci  # assign both to group "ci"
```

### Resizing

Sizes are specified as columns (default), pixels, or percentages:

```sh
rdw pane resize build-log right 40%   # percentage of the window
rdw pane resize build-log right 200px # pixels
rdw pane resize build-log right 40    # columns (default unit)
```

Valid directions: `left`, `right`, `up`, `down`.

In the browser, drag the gutter between panes. Resizing is also available
via keyboard: `H` / `J` / `K` / `L` nudge the active pane border.

### Zooming

```sh
rdw pane zoom build-log
```

Zooming makes the pane occupy the full window. Calling zoom again restores
the original layout. In the browser, press `z` or double-click a pane border.

### Swapping

```sh
rdw pane swap build-log error-log
```

The two panes exchange positions in the layout. In the browser, press `x`
to enter swap mode, then use `hjkl` to select the pane to swap with.

### Closing

```sh
rdw pane close error-log
```

In the browser, press `q` to close the focused pane.

### Pane groups

Panes can belong to a named group. Group operations apply to all panes in
the group simultaneously:

```sh
rdw group hide ci         # hide all panes in group "ci"
rdw group show ci
rdw group kill ci         # close all panes in group "ci"
rdw group focus ci        # bring all panes in group "ci" into focus
```

---

## 7. Routing Data Streams

### Basic piping

```sh
your_script | rdw pipe --id build-log
```

Any process writing to stdout can be a data source. The `pipe` command reads
stdin line by line and forwards each line to the server.

```sh
# tail a log file
tail -f /var/log/app.log | rdw pipe --id app-log

# build output
make 2>&1 | rdw pipe --id build-log

# separate stdout and stderr into different panes
your_script 1> >(rdw pipe --id stdout) 2> >(rdw pipe --id stderr)
```

### Target a specific server

```sh
your_script | rdw pipe --id build-log --port 7682
```

### Route into a specific window

```sh
your_script | rdw pipe --id build-log --window build
```

If the named window does not exist the server returns an error unless
`--allow-unassigned` is also set.

### Route with a layout

```sh
# load layout from file; create in browser if not yet active
your_script | rdw pipe --id build-log --layout ./layouts/debug.yaml

# load layout by saved preset name
your_script | rdw pipe --id build-log --layout debug
```

When `--layout` is given:

1. If a layout with the same name is already active on the server, the stream
   is routed into the pane matching `--id` within that layout.
2. If the layout is not yet active, it is created on the server and rendered
   in the browser before the first line of data arrives.
3. If the layout file does not contain a pane with the given `--id`, the
   server returns an error unless `--allow-unassigned` is set.

### Unassigned targets

By default, sending data to a Target ID that has no registered pane produces
an error. Pass `--allow-unassigned` to create a pane automatically:

```sh
your_script | rdw pipe --id new-pane --allow-unassigned
```

### Mirroring streams

A stream can be mirrored to a file, a FIFO, or another command alongside the
primary delivery to the browser:

```sh
# mirror to a file
your_script | rdw pipe --id build-log --forward-to-file /tmp/build.log

# mirror to a named pipe
your_script | rdw pipe --id build-log --forward-to-file /run/rdw-mirror.fifo

# mirror as stdin to another command
your_script | rdw pipe --id build-log --forward-to-cmd "grep ERROR | mail -s alerts ops@example.com"
```

### bash-rd forwarding

To forward data to a running `bash-rd` instance at the same time:

```sh
your_script | rdw pipe --id build-log --forward both   # rdw and rd
your_script | rdw pipe --id build-log --forward rd     # rd only
```

---

## 8. The Layout System

A layout describes the complete arrangement of windows and panes for a
session. Layouts can be:

- Loaded from a YAML file on disk
- Saved as named presets on the server
- Created interactively in the browser

### Applying a layout

```sh
# from a file
rdw layout apply ./layouts/debug.yaml

# from a saved preset
rdw layout apply debug
```

If the layout is already active (same name and structure), it is reused
rather than recreated.

### Saving a layout

```sh
rdw layout save --name debug
```

This snapshots the current window and pane arrangement and saves it as a
named preset. Presets are stored by the server and survive restarts when
`--restore` is used.

In the browser, press `Ctrl+w s` to save the current layout.

### Listing saved presets

```sh
rdw layout list
```

### Layout lifecycle with `rdw pipe`

Passing `--layout` to `rdw pipe` combines routing and layout management:

```sh
your_script | rdw pipe --id build-log --layout debug
```

Behaviour:

- If preset `debug` is active: route `build-log` into it.
- If preset `debug` is not active but a file `debug` or `debug.yaml` exists
  in the current directory or the config search path: load it, activate it,
  then route.
- If neither: error.

This makes it possible to write a single pipeline invocation that handles
first-run layout creation and subsequent stream routing identically.

---

## 9. The Layout Description Language

Layouts are expressed in YAML. The current schema version is `1`. The server
rejects files with any other value in `schema_version`.

### Full syntax

```yaml
schema_version: 1          # required; integer; must be 1
name: my-layout            # optional; string; used as the preset name

windows:                   # required; list; at least one entry
  - name: build            # required; string; shown in the window header bar
    panes:                 # required; list; 1–64 entries
      - target_id: stdout  # required; string; Target ID routed to this pane
        split: v           # optional; "h" or "v"; omit for the first pane
        size: 60%          # optional; N, Npx, or N%; default: equal share
        group: ci          # optional; string; pane group membership
        private: false     # optional; bool; hide from non-owner tokens
        scrollback_cap: 0  # optional; int; 0 means use server default
```

### Field reference

**`schema_version`** *(integer, required)*
Must be `1`. The server rejects any other value with a descriptive error.
This field exists to allow future breaking changes to be versioned cleanly.

**`name`** *(string, optional)*
A human-readable label for the layout preset. Used as the key when saving
and looking up presets by name. If omitted, a preset cannot be referenced
by name (file path reference still works).

**`windows`** *(list, required)*
An ordered list of window specifications. The order determines the initial
window order in the header bar. At least one window is required.

**`windows[].name`** *(string, required)*
The display name shown in the window header bar and used in all CLI window
commands. Must be non-empty. Window names must be unique within a layout.

**`windows[].panes`** *(list, required)*
An ordered list of pane specifications. At least one pane is required; at
most 64 panes are allowed per window.

**`panes[].target_id`** *(string, required)*
The Target ID of the data stream routed to this pane. Must match the
pattern `[a-zA-Z0-9_][a-zA-Z0-9_ -]*` and be at most 64 characters. Must
be non-empty.

**`panes[].split`** *(string, optional)*
How this pane is divided from the previous pane. Accepts `"h"` or `"v"`.
Omit for the first pane in a window (it occupies the full window).

- `"h"` — horizontal split: the new pane appears **below** the previous pane.
- `"v"` — vertical split: the new pane appears **to the right** of the previous pane.

**`panes[].size`** *(string, optional)*
The size of this pane along the split axis. Three formats are accepted:

| Format | Example | Meaning |
| --- | --- | --- |
| `N` | `40` | N character columns or rows (default unit) |
| `Npx` | `200px` | N pixels |
| `N%` | `30%` | N percent of the window dimension |

Fractional values (`12.5%`, `12.5px`) are rejected. Percentage values must
be in the range `[1, 100]`. When omitted, equal distribution is used.

**`panes[].group`** *(string, optional)*
The name of the pane group this pane belongs to. Group commands (`rdw group
hide`, `rdw group show`, etc.) operate on all panes sharing the same group
name simultaneously.

**`panes[].private`** *(bool, optional, default: false)*
When `true`, this pane is not visible to tokens that do not have explicit
access to it. It does not appear in list output or the browser UI for
non-owner users.

**`panes[].scrollback_cap`** *(int, optional, default: 0)*
Per-pane scrollback line cap. `0` means use the server default (10,000
lines). Must not be negative. Maximum accepted value is 100,000.

### Examples

#### Minimal layout

```yaml
schema_version: 1
windows:
  - name: main
    panes:
      - target_id: log
```

#### Two-pane horizontal split

```yaml
schema_version: 1
name: build
windows:
  - name: build
    panes:
      - target_id: stdout
      - target_id: stderr
        split: h
        size: 30%
```

#### Three-pane mixed split

```yaml
schema_version: 1
name: metrics
windows:
  - name: metrics
    panes:
      - target_id: cpu          # full width at top
      - target_id: mem
        split: v                # to the right of cpu
        size: 50%
      - target_id: disk
        split: h                # below the cpu/mem row
        size: 25%
```

#### Multi-window layout

```yaml
schema_version: 1
name: full-debug
windows:
  - name: build
    panes:
      - target_id: build-stdout
      - target_id: build-stderr
        split: h
        size: 30%

  - name: runtime
    panes:
      - target_id: app-log
      - target_id: app-errors
        split: v
        size: 40%

  - name: metrics
    panes:
      - target_id: cpu
      - target_id: mem
        split: v
        size: 50%
      - target_id: disk
        split: h
        size: 20%
        group: infra
```

#### Layout with groups and private panes

```yaml
schema_version: 1
name: shared-debug
windows:
  - name: ci
    panes:
      - target_id: build-log
        group: ci-build
      - target_id: test-log
        split: h
        size: 40%
        group: ci-test
      - target_id: internal-log
        split: v
        size: 30%
        private: true          # not visible to shared tokens
        scrollback_cap: 50000
```

---

## 10. Interactive Layout Editing

The browser UI supports full layout creation and modification without
touching the CLI. All operations are available via keyboard bindings and
the mouse.

### Modes

The browser UI is modal, following vim conventions.

**Normal mode** is the default. Keyboard bindings navigate and trigger
actions. Text input is not captured.

**Prompt mode** is entered when an action requires a name input (renaming a
window or pane, naming a new window). A small input field appears. Press
`Enter` to confirm, `Escape` to cancel.

**Swap mode** is entered via `x`. The currently focused pane is selected;
the next directional key (`hjkl`) picks the target pane. The two panes
exchange positions. Press `Escape` to cancel swap mode.

### Creating a layout from scratch

1. Start the server: `rdw server start --open-browser`
2. Press `gn` to create a new empty window. A prompt asks for the window name.
3. With the window active, press `s` or `v` to split the initial pane.
   A prompt asks for the Target ID of the new pane.
4. Use `H` / `J` / `K` / `L` to resize panes, or drag the gutter with
   the mouse.
5. Press `Ctrl+w s` to save the layout as a named preset.

### Splitting panes interactively

Press `s` to split the focused pane horizontally (new pane below).
Press `v` to split vertically (new pane to the right).
A prompt appears asking for the Target ID of the new pane.

Mouse: drag any pane border toward the centre of the pane to split it. A
split handle appears; release to confirm the split and enter the Target ID
in the resulting prompt.

### Resizing panes interactively

**Keyboard:** `H` shrinks the pane leftward, `L` grows it rightward,
`K` shrinks upward, `J` grows downward. Each keypress nudges the border
by the configured resize step (default: 5% of the window dimension).
Hold the key or repeat it to resize in larger increments.

**Mouse:** drag the gutter between two panes. The cursor changes to a
resize cursor when hovering over a gutter. Release to confirm.

### Renaming windows and panes

Press `gr` to rename the active window. A prompt appears with the current
name pre-filled. Edit and press `Enter`.

Press `R` to rename the Target ID of the focused pane. Note that renaming
a pane's Target ID changes which stream is routed into it. Any `rdw pipe`
command using the old ID must be updated.

Mouse: right-click a pane to open the context menu. Select "Rename pane"
to rename its Target ID.

### Zooming

Press `z` to zoom the focused pane to full window. Press `z` again to
restore the original layout. Double-click a pane border for the same
effect.

### Swapping panes

Press `x` to enter swap mode. The focused pane is highlighted. Press any
of `h`, `j`, `k`, `l` to select the adjacent pane in that direction.
The two panes exchange positions. Press `Escape` to cancel.

### Closing panes and windows

Press `q` to close the focused pane. If it is the only pane in the window,
the window is also closed.

Press `gx` to close the entire active window and all its panes.

### Saving and reloading

Press `Ctrl+w s` to save the current layout as a named preset. A prompt
asks for the preset name.

Press `Ctrl+w r` to reload the layout from the last saved preset or file,
discarding any unsaved interactive changes.

---

## 11. Keyboard Bindings

### Default binding table

All actions follow vim conventions. Key notation follows
`KeyboardEvent.key` from the Web APIs specification:
single characters are literal; modifiers use `+` (e.g., `Control+u`);
multi-key sequences are space-separated (e.g., `g t`).

#### Window navigation

| Key | Action name | Effect |
| --- | --- | --- |
| `g t` | `window.next` | Move to the next window |
| `g T` | `window.prev` | Move to the previous window |
| `g 0` | `window.first` | Jump to the first window |
| `g $` | `window.last` | Jump to the last window |
| `g n` | `window.new` | Create a new empty window (opens name prompt) |
| `g x` | `window.close` | Close the active window and all its panes |
| `g r` | `window.rename` | Rename the active window (opens prompt) |

#### Pane navigation

| Key | Action name | Effect |
| --- | --- | --- |
| `h` | `pane.focus.left` | Focus the pane to the left |
| `j` | `pane.focus.down` | Focus the pane below |
| `k` | `pane.focus.up` | Focus the pane above |
| `l` | `pane.focus.right` | Focus the pane to the right |

#### Pane editing

| Key | Action name | Effect |
| --- | --- | --- |
| `s` | `pane.split.h` | Split focused pane horizontally (new pane below) |
| `v` | `pane.split.v` | Split focused pane vertically (new pane right) |
| `H` | `pane.resize.left` | Shrink pane toward the left |
| `J` | `pane.resize.down` | Grow pane downward |
| `K` | `pane.resize.up` | Shrink pane upward |
| `L` | `pane.resize.right` | Grow pane toward the right |
| `q` | `pane.close` | Close the focused pane |
| `z` | `pane.zoom` | Toggle full-window zoom on the focused pane |
| `R` | `pane.rename` | Rename the focused pane's Target ID (opens prompt) |
| `x` | `pane.swap` | Enter swap mode; next `hjkl` picks the swap target |

#### Scrollback

| Key | Action name | Effect |
| --- | --- | --- |
| `Control+u` | `scroll.up` | Scroll up half a page |
| `Control+d` | `scroll.down` | Scroll down half a page |
| `g g` | `scroll.top` | Jump to the top of the scrollback buffer |
| `G` | `scroll.bottom` | Jump to the bottom (most recent line) |
| `Control+l` | `scroll.clear` | Clear the scrollback buffer for the focused pane |

#### Search

| Key | Action name | Effect |
| --- | --- | --- |
| `/` | `search.open` | Open the search bar |
| `n` | `search.next` | Jump to the next search match |
| `N` | `search.prev` | Jump to the previous search match |

#### Layout persistence

| Key | Action name | Effect |
| --- | --- | --- |
| `Control+w s` | `layout.save` | Save the current layout as a named preset |
| `Control+w r` | `layout.reload` | Reload layout from the last saved preset |

#### Mode

| Key | Action name | Effect |
| --- | --- | --- |
| `Escape` | `mode.escape` | Return to normal mode |
| `Control+c` | `mode.escape` | Return to normal mode (alternative) |

### Customising bindings

Bindings are configured in the `bindings:` section of the config file.
The value for each action is a YAML list of key strings. Any action not
listed keeps its default. An action set to an empty list is unbound.

```yaml
# ~/.config/rdw/config.yaml

bindings:
  # Replace vim directional keys with arrow keys
  pane.focus.left:  [Left]
  pane.focus.down:  [Down]
  pane.focus.up:    [Up]
  pane.focus.right: [Right]

  # Use Tab / Shift+Tab for window navigation
  window.next: [Tab]
  window.prev: [Shift+Tab]

  # Disable the swap action
  pane.swap: []
```

Key string format follows `KeyboardEvent.key`:

| Key | String |
| --- | --- |
| Letter keys | `a`, `b`, `A`, `B`, … |
| Enter | `Enter` |
| Escape | `Escape` |
| Tab | `Tab` |
| Backspace | `Backspace` |
| Arrow keys | `ArrowLeft`, `ArrowRight`, `ArrowUp`, `ArrowDown` |
| Function keys | `F1`, `F2`, … |
| With Control | `Control+a`, `Control+w` |
| With Shift | `Shift+Tab` |
| Space | ` ` (a single space) |

Multi-key sequences are written as space-separated key strings within a
single list element:

```yaml
bindings:
  window.next: ["g t"]   # press g then t
  scroll.top:  ["g g"]   # press g twice
```

### Conflict detection

The server validates the binding map on startup. If any key string is
assigned to more than one action, startup fails with a descriptive error
listing all conflicts. Fix the offending entries in the config and restart.

---

## 12. Control Sequences

A **control sequence** is a line in the data stream whose first two
characters are a recognised letter followed by a colon. The pipeline
intercepts it before it reaches the renderer; it is never stored in the
scrollback buffer.

### Prefix table

| Prefix | Action | Payload format | Notes |
| --- | --- | --- | --- |
| `v:` | verbatim | any text | Strips the `v:` prefix and passes the remainder to the renderer as literal content. Use this to send a line that would otherwise be interpreted as a control sequence. |
| `q:` | quit | (empty) | Stops the server gracefully. |
| `s:` | semaphore | (empty) | Decrements the server's semaphore counter. When the counter reaches zero the server stops. Useful for coordinating shutdown across multiple piped processes. |
| `c:` | clear | (empty) | Clears the scrollback buffer of the pane receiving this stream. |
| `t:` | timestamp | (empty) | Toggles RFC3339 UTC timestamp prepending for this pane. Each subsequent line has `2024-03-15T10:00:00Z ` prepended until the toggle is sent again. |
| `f:` | formatter | formatter name | Switches the formatter for this pane to the named formatter. Takes effect on the next incoming line. |
| `r:` | relay | `location:[pid]` | Redirects the output of this pane to an external location. |
| `=:` | KV write | `key=value;key2=value2` | Writes one or more key-value pairs to the session KV store. Multiple pairs are separated by `;`. |

### Verbatim passthrough

The `v:` prefix is the escape mechanism for the control sequence namespace.
Any data that begins with a recognised two-character prefix should be
wrapped in `v:` before being piped:

```sh
# This would be interpreted as a KV write:
echo "=:stage=build" | rdw pipe --id log

# This sends the literal text "=:stage=build":
echo "v:=:stage=build" | rdw pipe --id log
```

### KV sequences

The `=:` prefix writes directly to the session KV store. Multiple pairs
are separated by semicolons. Values may contain `=`; the split is at the
first `=` only.

```sh
echo "=:stage=build;status=running;url=https://ci.example.com/jobs/42" \
  | rdw pipe --id build-log
```

Invalid key names (not matching `[a-zA-Z0-9_][a-zA-Z0-9_ :-]*` or over
64 characters) cause the entire sequence to be rejected with an error.
Valid pairs that appear before the invalid one are not applied (the
operation is atomic per sequence).

### Semaphore pattern

A useful pattern for scripts that pipe multiple streams and want the server
to stop automatically when all streams have finished:

```sh
# Set semaphore to 3 (three streams)
# Each stream sends "s:" when it finishes
# Server stops when the counter reaches 0

stream_a | rdw pipe --id stream-a &
stream_b | rdw pipe --id stream-b &
stream_c | rdw pipe --id stream-c &

wait
echo "s:" | rdw pipe --id stream-a
echo "s:" | rdw pipe --id stream-b
echo "s:" | rdw pipe --id stream-c
```

---

## 13. The Key-Value Store

Every rdw session maintains a key-value store that is accessible from any
data stream, formatter, CLI command, or REST API call.

### Scope and namespacing

The KV store is scoped to the session (one running server instance). All
panes and windows share the same namespace by default.

Optional namespacing by window or pane is supported using key prefixes.
The server recognises these conventions but does not enforce them — any key
matching the prefix pattern is treated as namespaced:

```
window:<window-name>:<key>
pane:<target-id>:<key>
```

Examples:

```sh
rdw kv set build.status passing              # global
rdw kv set window:build:title "Build CI"    # window-scoped
rdw kv set pane:stdout:color green          # pane-scoped
```

### Key rules

Keys must match the pattern `[a-zA-Z0-9_][a-zA-Z0-9_ :-]*` and be at most
64 characters. The colon is explicitly allowed to support the namespace
prefix convention.

### Value limits

- Per-value maximum: 64 KB
- Total store maximum: 64 MB

Attempting to write a value that would exceed either limit returns an error;
the store is not modified.

### CLI operations

```sh
# Write
rdw kv set build.status passing

# Write multiple via control sequence
echo "=:build.status=passing;build.duration=42s" | rdw pipe --id build-log

# Read
rdw kv get build.status

# Delete
rdw kv delete build.status
```

### REST API

```sh
# Write
curl -X PUT http://localhost:7681/api/v1/kv/build.status \
  -H "Authorization: Bearer <token>" \
  -d '{"value": "passing"}'

# Read
curl http://localhost:7681/api/v1/kv/build.status \
  -H "Authorization: Bearer <token>"

# Delete
curl -X DELETE http://localhost:7681/api/v1/kv/build.status \
  -H "Authorization: Bearer <token>"

# List all keys
curl http://localhost:7681/api/v1/kv \
  -H "Authorization: Bearer <token>"
```

### Persistence

By default the KV store is memory-only and is lost when the server exits.

Enable SQLite persistence with `--kv-persist`:

```sh
rdw server start --kv-persist ~/.rdw/kv.db
```

With persistence enabled, the store is restored automatically when the
server starts with `--restore`. The SQLite file is created with permissions
`0600`.

---

## 14. Filters and Formatters

### Filters

A **filter** is an external executable or shell command that reads from
stdin and writes to stdout. It transforms or drops stream content before
the server passes it to the renderer.

Filters are applied per pane in a chain. A pane may have at most 8 filter
stages. Attempts to configure more than 8 stages are rejected at startup
with a descriptive error.

A filter that writes nothing for a given input line drops that line from
the stream. A filter that writes multiple lines for a single input line
expands the stream.

Filters are configured in the layout file or set dynamically via the REST
API. They do not produce HTML — they only transform text.

Common filter patterns:

```sh
# Strip ANSI escape sequences
ansi2txt

# Keep only lines containing "ERROR"
grep ERROR

# Prefix every line with a custom tag
awk '{ print "[BUILD] " $0 }'

# Format JSON one field per line
jq -r '.message'
```

### Formatters

A **formatter** is a renderer that converts the incoming text stream and
the current KV store state into HTML displayed inside the pane.

Built-in formatters:

| Formatter | Trigger | Output |
| --- | --- | --- |
| `text` | default | Plain text with ANSI color passthrough |
| `json` | `rdw json --id <id>` | Interactive collapsible JSON tree |
| `yaml` | `rdw yaml --id <id>` | Interactive collapsible YAML tree |
| `markdown` | `rdw markdown --id <id>` | Compiled Markdown |
| `csv` | `rdw csv --id <id>` | Sortable grid |
| `image` | `rdw image --id <id>` | Decoded image (PNG, JPG, SVG) |

Switch formatters at runtime via control sequence:

```sh
echo "f:json" | rdw pipe --id api-log
```

Or via CLI:

```sh
rdw pane formatter api-log json
```

---

## 15. Binary Data

Binary payloads (images, compressed data, non-UTF-8 content) must be
base64-encoded with a `b64:` prefix before being sent through the pipeline.
The server decodes transparently before passing to formatters or the
scrollback buffer.

This convention is identical to bash-rd.

```sh
# Send a PNG image
echo "b64:$(base64 < chart.png)" | rdw pipe --id chart

# Send compressed data
echo "b64:$(gzip -c data.bin | base64)" | rdw pipe --id raw

# Inline in a script
generate_chart | base64 | sed 's/^/b64:/' | rdw pipe --id chart
```

If the `b64:` prefix is present but the payload is not valid base64, the
line is passed through unchanged (the literal string `b64:...` appears in
the pane). No error is generated.

---

## 16. Authentication and Access Control

### CLI authentication

Commands run by the same Unix user that started the server are
authenticated via a Unix domain socket at:

```
$XDG_RUNTIME_DIR/rdw/<session_id>.sock
```

The socket is created with permissions `0600`. Only the owner process
can connect. No token is required for CLI commands from the same user.

### Token-based access

Remote CLI commands (via the REST API) and browser connections require an
access token. Tokens are stored as SHA-256 hashes; the plain-text token is
shown exactly once at creation time and is never stored.

```sh
# Create a token with 24-hour expiry (default)
rdw token create

# Create a scoped token for specific panes
rdw token create --panes build-log,error-log --expiry 8h

# Create a token for a specific window
rdw token create --windows build --expiry 24h

# Create a non-expiring token
rdw token create --expiry 0

# Revoke a token immediately
rdw token revoke <token-id>
```

Revoking a token immediately terminates all active WebSocket connections
bound to it.

### Token scoping

Tokens can be scoped to specific panes and windows. A scoped token holder:

- Cannot see panes or windows not in their scope
- Cannot issue layout commands outside their scope
- Cannot read KV keys associated with out-of-scope resources

A token with no explicit scope has access to all non-private resources.

### Private panes

A pane marked `private: true` in its layout spec is not visible to tokens
that do not have explicit access to it. It does not appear in list output,
metadata responses, or the browser dashboard for non-owner users.

### Token storage

Token hashes are stored in the server's config directory. Plain-text
tokens are shown once at creation and are not retained anywhere. If a
token is lost it must be revoked and a new one created.

### Admin console

The admin console is accessible at `http://127.0.0.1:<port>/admin` and is
restricted to loopback by default. It provides a visual interface for:

- Viewing and revoking active tokens
- Inspecting window and pane state
- Terminating active streams

To allow access from the local network:

```sh
rdw server start --network-expose
```

This flag must be passed explicitly; there is no implicit network exposure.

### Rate limiting

Unauthenticated REST endpoints enforce a rate limit of 10 requests per
minute per source IP address. This mitigates brute-force token discovery.
Authenticated endpoints are not rate-limited by default.

---

## 17. Exporting

The contents of any pane, window, or the entire session can be exported as
a Markdown bundle.

```sh
# Export a single pane
rdw save pane --target-id build-log --out-dir ./export

# Export all panes in a window
rdw save window --name build --out-dir ./export

# Export all windows in the session
rdw save all --out-dir ./export
```

### Output structure

```
export/
  session.md          # full export (rdw save all)
  assets/
    chart-001.png
    chart-002.png
    ...
```

The Markdown file is structured as:

```markdown
# Window: build

## Pane: build-log

<scrollback content>

## Pane: build-stderr

<scrollback content>

# Window: metrics
...
```

Images sent to panes via the image formatter are saved as files in the
`assets/` directory and referenced with relative paths in the Markdown.

---

## 18. Configuration Reference

The default config file is `$XDG_CONFIG_HOME/rdw/config.yaml`
(usually `~/.config/rdw/config.yaml`). Pass `--config` to override.

### Full annotated configuration

```yaml
# rdw configuration file
# All fields are optional; unspecified fields use the defaults shown here.

server:
  # Port the HTTP/WebSocket server listens on.
  # Default: 7681
  port: 7681

  # Bind to all network interfaces instead of loopback only.
  # Required for remote browser connections and network token use.
  # Default: false
  network_expose: false

  # Open the default browser automatically on server start.
  # Default: false
  open_browser: false

  # Maximum number of stages in a filter chain per pane.
  # Must be in [1, 64]. Default: 8
  filter_chain_max: 8

  # Default scrollback buffer line cap per pane.
  # Individual panes can override this in their layout spec.
  # Must be >= 1. Default: 10000
  scrollback_cap: 10000

  # Number of lines buffered client-side during a server reconnect.
  # Oldest lines are dropped if the queue fills before reconnection.
  # Must be >= 1. Default: 1000
  reconnect_queue_len: 1000

auth:
  # Disable token authentication entirely.
  # For development use only; never set in production.
  # Default: false
  no_auth: false

  # Restrict the admin console to loopback (127.0.0.1).
  # Setting this to false requires --network-expose to take effect.
  # Default: true
  admin_local_only: true

kv:
  # Path to the SQLite file for optional KV store persistence.
  # Empty string means memory-only (default).
  # The file is created with permissions 0600.
  persist_path: ""

log:
  # Minimum log level.
  # One of: debug, info, warn, error
  # Default: info
  level: info

  # Log output format.
  # One of: console (human-readable), json (structured)
  # Default: console
  format: console

bindings:
  # Override individual keyboard bindings.
  # Each entry is an action name: list of key strings.
  # Actions not listed keep their defaults.
  # Set to an empty list [] to remove a binding.
  #
  # Action names and their defaults:
  #   window.next:        ["g t"]
  #   window.prev:        ["g T"]
  #   window.first:       ["g 0"]
  #   window.last:        ["g $"]
  #   window.new:         ["g n"]
  #   window.close:       ["g x"]
  #   window.rename:      ["g r"]
  #   pane.focus.left:    ["h"]
  #   pane.focus.down:    ["j"]
  #   pane.focus.up:      ["k"]
  #   pane.focus.right:   ["l"]
  #   pane.split.h:       ["s"]
  #   pane.split.v:       ["v"]
  #   pane.resize.left:   ["H"]
  #   pane.resize.down:   ["J"]
  #   pane.resize.up:     ["K"]
  #   pane.resize.right:  ["L"]
  #   pane.close:         ["q"]
  #   pane.zoom:          ["z"]
  #   pane.rename:        ["R"]
  #   pane.swap:          ["x"]
  #   scroll.up:          ["Control+u"]
  #   scroll.down:        ["Control+d"]
  #   scroll.top:         ["g g"]
  #   scroll.bottom:      ["G"]
  #   scroll.clear:       ["Control+l"]
  #   search.open:        ["/"]
  #   search.next:        ["n"]
  #   search.prev:        ["N"]
  #   layout.save:        ["Control+w s"]
  #   layout.reload:      ["Control+w r"]
  #   mode.escape:        ["Escape", "Control+c"]
```

---

## 19. Command Reference

All commands share these global flags:

| Flag | Short | Default | Description |
| --- | --- | --- | --- |
| `--port` | `-p` | 0 (auto) | Server port. 0 probes default (7681) then registry. |
| `--config` | `-c` | (see §18) | Path to config file. |

---

### `rdw server start`

Start an rdw server daemon.

```
rdw server start [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--port` | 7681 | Port to listen on. |
| `--network-expose` | false | Bind to all interfaces. |
| `--no-auth` | false | Disable token authentication. |
| `--open-browser` | false | Open default browser on start. |
| `--restore` | false | Restore last saved session state. |
| `--kv-persist PATH` | "" | Enable SQLite KV persistence. |
| `--config PATH` | (default) | Config file path. |

---

### `rdw server stop`

Stop the server on the target port.

```
rdw server stop [--port PORT]
```

---

### `rdw server list`

List all registered rdw server instances.

```
rdw server list
```

Prints port, PID, and start time for each registered server. Stale entries
(processes no longer running) are removed before output.

---

### `rdw pipe`

Relay stdin to a named pane.

```
rdw pipe --id TARGET_ID [flags]
```

| Flag | Short | Description |
| --- | --- | --- |
| `--id` | `-i` | Target pane ID. Required. |
| `--layout` | `-l` | Layout preset name or file path. |
| `--window` | `-w` | Window name to route into. |
| `--forward` | | Also forward to `rd`, `rdw`, or `both`. |
| `--allow-unassigned` | | Create pane dynamically for unknown IDs. |
| `--forward-to-file PATH` | | Mirror stream to a file or FIFO. |
| `--forward-to-cmd CMD` | | Mirror stream as stdin to a shell command. |

---

### `rdw window create`

Create a new window.

```
rdw window create NAME [--layout LAYOUT]
```

| Flag | Short | Description |
| --- | --- | --- |
| `--layout` | `-l` | Layout file or preset name for initial panes. |

---

### `rdw window close`

Close a window and all its panes.

```
rdw window close NAME
```

---

### `rdw window rename`

Rename a window.

```
rdw window rename OLD_NAME NEW_NAME
```

---

### `rdw window focus`

Switch the browser to the named window.

```
rdw window focus NAME
```

---

### `rdw window list`

List all windows in the active session.

```
rdw window list
```

---

### `rdw pane split`

Split a pane and assign the new pane a Target ID.

```
rdw pane split TARGET_ID h|v NEW_ID [flags]
```

| Flag | Short | Description |
| --- | --- | --- |
| `--group` | `-g` | Assign pane to a group. |
| `--private` | | Hide from non-owner tokens. |

---

### `rdw pane resize`

Resize a pane.

```
rdw pane resize PANE_ID left|right|up|down SIZE
```

`SIZE` accepts `N` (columns), `Npx`, or `N%`.

---

### `rdw pane zoom`

Toggle a pane between normal and full-window zoom.

```
rdw pane zoom PANE_ID
```

---

### `rdw pane swap`

Swap the positions of two panes.

```
rdw pane swap PANE_ID_A PANE_ID_B
```

---

### `rdw pane close`

Close a pane.

```
rdw pane close PANE_ID
```

---

### `rdw layout apply`

Apply a layout preset or file to the active session.

```
rdw layout apply NAME_OR_PATH
```

---

### `rdw layout save`

Snapshot the current layout as a named preset.

```
rdw layout save --name NAME
```

---

### `rdw layout list`

List saved layout presets.

```
rdw layout list
```

---

### `rdw kv set`

Write a value into the KV store.

```
rdw kv set KEY VALUE
```

---

### `rdw kv get`

Read a value from the KV store.

```
rdw kv get KEY
```

---

### `rdw kv delete`

Delete a key from the KV store.

```
rdw kv delete KEY
```

---

### `rdw token create`

Create an access token.

```
rdw token create [flags]
```

| Flag | Description |
| --- | --- |
| `--expiry DURATION` | Token lifetime. `0` means no expiry. Default: 24h. |
| `--panes LIST` | Comma-separated list of pane IDs this token can access. |
| `--windows LIST` | Comma-separated list of window names. |

The plain-text token is printed to stdout exactly once. Store it securely.

---

### `rdw token revoke`

Revoke an access token immediately.

```
rdw token revoke TOKEN_ID
```

Terminates all active WebSocket connections bound to this token.

---

### `rdw group hide`

Hide all panes in a named group.

```
rdw group hide GROUP_NAME
```

---

### `rdw group show`

Show all panes in a named group.

```
rdw group show GROUP_NAME
```

---

### `rdw group focus`

Bring all panes in a named group into focus.

```
rdw group focus GROUP_NAME
```

---

### `rdw group kill`

Close all panes in a named group.

```
rdw group kill GROUP_NAME
```

---

### `rdw save`

Export scrollback history to a Markdown bundle.

```
rdw save pane   --target-id ID  --out-dir DIR
rdw save window --name NAME     --out-dir DIR
rdw save all                    --out-dir DIR
```

---

### `rdw selftest`

Run the built-in smoke test suite.

```
rdw selftest
```

Exits 0 on success, non-zero if any check fails.

---

### `rdw completion bash`

Output the bash completion script to stdout.

```
rdw completion bash >> ~/.bashrc
```

---

## 20. bash-rd Compatibility

`rdw` is wire-compatible with [bash-rd](https://github.com/nkh/bash-rd) for:

- The `=:key=value` KV control sequence syntax
- The `b64:` binary encoding prefix
- The full control sequence prefix set (`v:`, `q:`, `s:`, `c:`, `t:`, `f:`, `r:`)

To forward data to a running bash-rd instance at the same time as rdw:

```sh
your_script | rdw pipe --id my-pane --forward both   # rdw and rd simultaneously
your_script | rdw pipe --id my-pane --forward rd     # rd only, skip rdw
```

`--forward rdw` (default) sends only to rdw.

---

## 21. Security Model

### Threat surface

The rdw server is designed for developer and operator use in controlled
environments. The primary deployment scenario is a loopback-only server
accessed by a single user's browser. Network exposure is an explicit opt-in.

### Authentication layers

**CLI (same user):** The client binary connects to the server's Unix domain
socket at `$XDG_RUNTIME_DIR/rdw/<session_id>.sock` (permissions `0600`).
The kernel enforces that only the owning user can connect. No token is
required.

**REST API and browser:** Bearer token required in the `Authorization`
header. Token hashes are SHA-256. Plain-text tokens are shown once at
creation and not retained.

### What the server does not do by default

- Does not execute shell commands from right-click menus (requires an
  explicit startup flag)
- Does not render raw HTML from streams (sanitised before output)
- Does not accept connections from any interface except loopback
- Does not store plain-text tokens anywhere

### Terminal sharing (gotty panes)

gotty-backed terminal panes must run under a dedicated restricted Unix user
account with no write access outside a designated working directory. The
server refuses to start a terminal pane if this user is not configured.
This is a hard requirement, not a recommendation.

### Network exposure checklist

Before enabling `--network-expose`:

1. Create scoped tokens for every remote user. Do not share the owner token.
2. Mark panes as `private: true` if they should not be visible to shared tokens.
3. Verify that the admin console is accessible only to trusted IPs
   (`admin_local_only: true` in config, or firewall the `/admin` path).
4. Enable TLS termination in front of rdw (nginx, Caddy, etc.) for any
   connection that crosses a network boundary.
5. Verify that right-click menu execution is not enabled
   (`allow_menu_exec: false`, the default).

---

## 22. Headless and CI Use

### Smoke testing

```sh
rdw selftest
echo $?   # 0 on success
```

This starts an in-process test suite that exercises the pipeline, KV store,
control sequences, base64 decoding, and scrollback buffer. No server process
or browser is required.

### CI pipeline integration

```yaml
# Example: GitHub Actions step
- name: rdw selftest
  run: rdw selftest
```

### Headless server mode

The server operates without a browser attached. Start it, feed data, and
query it via the REST API:

```sh
rdw server start --port 7681

# Feed data
echo "build started" | rdw pipe --id build-log
make 2>&1 | rdw pipe --id build-log

# Query KV store
rdw kv get build.status

# Export results
rdw save all --out-dir ./ci-export

rdw server stop
```

### Scripted layout creation

```sh
rdw server start

# Create a layout programmatically
rdw window create build
rdw pane split build-log v error-log
rdw pane resize error-log right 40%

# Run tasks
make 2>&1 | rdw pipe --id build-log &
make test 2>&1 | rdw pipe --id error-log &
wait

rdw save all --out-dir ./build-results
rdw server stop
```

---

## 23. Troubleshooting

### No server found on default port

```
no rdw server found on default port 7681; start one with: rdw server start
```

Either no server is running, or it is on a non-default port. Run
`rdw server list` to see registered instances, then use `--port`.

### Server found but connection refused

The registered PID may be stale (the process crashed without deregistering).
Start a new server. The registry is pruned automatically on the next
`rdw server list` call.

### Permission denied on Unix socket

The CLI is running as a different user than the one that started the server.
Either switch to the correct user or use a token via the REST API.

### Layout rejected: unsupported schema version

```
unsupported layout schema version 2 (current: 1)
```

The layout file was created by a newer version of rdw. Downgrade the
`schema_version` field to `1` and remove any fields not present in section 9,
or upgrade rdw.

### Binding conflict on startup

```
key "h" assigned to both "pane.focus.left" and "pane.resize.left"
```

Two actions in the `bindings:` config share the same key. Resolve the
conflict and restart the server.

### Base64 line appears in pane verbatim

The `b64:` prefix was present but the payload failed base64 decoding
(e.g. line endings embedded in the encoded data). Ensure the encoder does
not wrap lines:

```sh
base64 -w 0 < file.bin    # GNU coreutils: no line wrapping
base64 < file.bin          # macOS: no wrapping by default
```

### KV write silently dropped

Control sequences are intercepted before reaching the scrollback buffer.
If a `=:` sequence does not appear to write to the store, check:

1. The key matches `[a-zA-Z0-9_][a-zA-Z0-9_ :-]*` (no slash, no dot).
2. The value does not exceed 64 KB.
3. The total store is not at its 64 MB limit (`rdw kv list | wc -l`).

### Pane shows "no registered target"

The Target ID in `--id` is not in the active layout and `--allow-unassigned`
was not passed. Either add the Target ID to the layout or pass
`--allow-unassigned`.
