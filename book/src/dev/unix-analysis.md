# rdw — Unix philosophy analysis and bash-rd intent

## What bash-rd wanted to achieve

bash-rd is a **debugging and introspection tool**. Its README opens with a real-world use case: a developer had a bug in `tdiff`, a complex interactive program, and needed to observe internal state without breaking the interactivity. The tool that emerged from that need has a specific, narrow purpose:

- Send data from a running process to another terminal, a web page, or a remote machine
- Let a **formatter** — a shell script — decide what that data means and how to display it
- Use a **KV store** as the communication channel between the producer and the formatter
- Stay out of the way otherwise

The core model: `program | rd -c ID` where `rd -c ID` reads from the program and a formatter running in a different process reads the KV store and decides what to render. The formatter is the user's code, not rd's. rd is a conduit.

This is deeply Unix: small tool, composable, the user brings the intelligence.

---

## Where rdw departs from Unix principles

### 1. The formatter is in the wrong place

In bash-rd, the formatter runs **alongside the server** — it is sourced into the server process and has full access to the KV store, to `rd_line` (the current line), to bash's environment. It is the user's code running in the server context. The server is dumb; the formatter is smart.

In rdw, built-in formatters (text, json, yaml, markdown, csv, image) run **on demand in the server process** against the stored scrollback. They produce HTML. The user cannot override them. User-defined formatters are external commands called once per format request — not once per line, not with access to the pipeline, not integrated into the processing chain.

This inverts the bash-rd model. In bash-rd, **you write the formatter**. In rdw, **the formatter is built in** and you are a consumer of six fixed choices.

The Unix fix: a formatter should be a filter in the pipeline — a command that reads lines and writes transformed lines or HTML, called per-line as data arrives, with the full KV store available. It should be no different from a filter. The distinction between filter (per-line, runs during ingestion) and formatter (on-demand, runs after storage) was introduced in rdw but does not exist in bash-rd.

### 2. KV is disconnected from the formatter pipeline

In bash-rd, KV is the **primary communication mechanism between producer and formatter**. The producer writes `=:x=1`, the server stores `x=1`, and the formatter reads `$x` directly from its bash environment. The formatter *reacts* to KV changes. This is how the `ptt` (pretty table) formatter works: you set variables, then ask the formatter to display them.

In rdw, KV is injected into filter subprocesses and user-defined formatter subprocesses as environment variables — but only at invocation time. The built-in formatters (which handle most users' needs) have no access to KV at all. A user cannot write `"f:json"` and have the formatter consult `$my_threshold` from the KV store. The formatter and the KV store are separate subsystems that do not communicate.

The Unix fix: every formatter invocation — built-in or external — should receive the current KV snapshot. For built-in formatters this means passing KV as a parameter set. For external formatters it already works. The inconsistency is the problem.

### 3. `rdw pipe` is an active relay, not a passive pipe

In bash-rd, `rd -c ID` is essentially `cat` with a destination. The data flows through it unchanged; the formatter at the other end decides what to do.

In rdw, `rdw pipe` is a complex relay with a reconnect queue, a hybrid binary reader, filter registration, title setting, layout application, and mirror forking. The client is doing work that should happen at the server or not happen at all.

The Unix issue: a pipe should be a transparent conduit. The intelligence belongs at the server (for server-side concerns) or in the user's pipeline (for user-side concerns). `rdw pipe` mixing client-side mirroring with server-side layout application violates separation.

A cleaner model: `rdw pipe` does one thing — send lines to the server. Mirroring is `tee`. Layout application is `rdw layout apply`. Title setting is `rdw pane rename`. The user composes these.

### 4. The control sequence namespace is growing into a protocol

bash-rd has 10 control sequences covering quit, verbatim, semaphore, clear, timestamp, when-to-run, clean-up, set-formatter, relay, and KV-write. Each is a single character. The protocol is minimal.

rdw currently has 16 control sequence kinds including multi-char prefixes (`image:`, `svg:`, `svg-data:`, `scale:`, `title:`, `bm:`, `hl:`, `sc:`). The sentinel-framed binary blocks (`image:` / `image:end`) are effectively a framing protocol embedded in a line-oriented stream.

This is the classic Unix anti-pattern: a tool that starts as a pipe and grows its own wire protocol. The binary/image problem is real, but the correct Unix answer is to not embed binary in a text stream at all — keep two separate channels, or use a well-known encoding (`base64 | rdw pipe`) rather than inventing a new sentinel framing scheme.

The rdw `image:end` sentinel is particularly fragile: if the binary data contains a line that reads `image:end`, the framing breaks. A length-prefix would be safer, but the real answer is that binary transport is not a line-oriented pipe concern.

### 5. The server has too many responsibilities

rdw's `internal/server` package contains: HTTP server, WebSocket hub, Unix socket listener, REST API (50+ endpoints), browser SPA (~1200 lines of JS/HTML), session management, router, KV store access, highlight profile store, terminal pane launcher, focus cycle, admin page, formatter registry. This is not "do one thing well."

bash-rd's server does: receive lines, run the formatter, echo output. Three responsibilities.

The Unix fix: each concern should be separable. The browser SPA could be a separate static file. The REST API and WebSocket server could be separate daemons. The terminal pane launcher is really `ttyd` — rdw wrapping it adds complexity without adding value beyond discoverability.

### 6. `rdw send` breaks the pipeline model

`rdw send --id chart chart.png` reads a file and sends it to the server. This is convenient but wrong from a Unix perspective. The correct composition is:

```sh
cat chart.png | rdw pipe --id chart
```

or with explicit encoding:

```sh
base64 chart.png | rdw pipe --id chart
```

`rdw send` is a convenience wrapper that hides the data flow. In Unix, you should be able to see what is happening in the pipeline. `rdw send` trades transparency for convenience.

### 7. Authentication and tokens are not composable

Unix tools authenticate via file permissions and process ownership. `rdw pipe` correctly uses a Unix socket with `0600` permissions for local use. But the REST API authentication (Bearer tokens) is a separate system that requires explicit token creation, management, and transmission. Tokens cannot be composed with standard Unix tools like `curl` without setup.

bash-rd's security model: "if you can reach the socket, you can send data." Simple, Unix-native. rdw's token system is appropriate for a networked service but adds friction for the primary use case (local development and debugging).

### 8. The browser SPA is embedded, not separate

Embedding the SPA in the binary guarantees it works offline, which is correct for a self-contained tool. But it makes the frontend unmodifiable without recompiling. A user who wants to customise the display — different colour scheme, different layout algorithm, additional pane widgets — cannot.

bash-rd's formatter model solves this: **you bring the HTML**. The formatter writes arbitrary HTML; you own the display. rdw's embedded SPA means Anthropic owns the display.

The Unix fix: serve the SPA from a configurable directory, fall back to the embedded default. Users who want customisation can provide their own HTML/JS.

---

## What bash-rd got right that rdw should inherit more fully

**The formatter is the user's code.** In bash-rd you write a shell function that has access to `$rd_line` and all KV variables. The formatter is sourced, not called. This means zero process overhead per line, full bash environment, and complete flexibility. rdw's external command formatter is correct in direction but wrong in granularity — it is called once per format request against the entire scrollback, not once per line during ingestion.

**KV is the formatter's interface to the world.** In bash-rd, KV is not a side feature — it is the primary way formatters receive configuration and context. `=:level=DEBUG` changes the formatter's behaviour on the next line. rdw implements this correctly in theory (KV injected into filter/formatter environments) but not in practice (built-in formatters ignore KV entirely).

**Relays are composable.** `rd -r ID` relays the server's output to another destination. This is just `tee` with an rd destination — the composition is explicit and visible. rdw's `--forward-to-cmd` and `--forward-to-file` are opaque options rather than explicit pipeline stages.

**The server is tiny.** bash-rd's server is a bash function. It receives lines, runs the formatter (if any), and echoes. There is no HTTP server, no WebSocket hub, no authentication, no REST API. For local debugging this is exactly right. rdw's complexity is justified for the network-remote and browser-display use cases, but it means rdw is no longer a debugging tool — it is an infrastructure component.

---

## Summary table

| Principle | bash-rd | rdw | Verdict |
| --- | --- | --- | --- |
| Do one thing | receive lines, run formatter | receive, route, store, authenticate, format, display, export, introspect | rdw violates |
| Composability | formatter is user code, relays are pipes | formatter is server code, mirroring is a flag | rdw partially violates |
| Transparency | data flow is visible | rdw pipe hides buffering, encoding, layout application | rdw partially violates |
| KV as formatter interface | KV is formatter's environment | KV reaches filters/user-formatters; built-ins ignore it | rdw partially violates |
| Text streams | line-oriented, no binary framing | sentinel-framed binary protocol | rdw violates |
| Small server | bash function | 50+ endpoint REST daemon | rdw violates (justified for browser display) |
| User owns the display | formatter writes arbitrary HTML | SPA is embedded and fixed | rdw violates |
| Unix auth | socket permissions | token system over REST | rdw partially violates |

---

## Concrete recommendations

1. **Make every formatter a pipeline filter.** A formatter should be callable per-line during ingestion, not just on-demand against the scrollback. The distinction between filter and formatter should collapse: both are external commands in the pipeline. Format-on-demand becomes a special case of running the pipeline over stored lines.

2. **Give built-in formatters access to KV.** When a built-in formatter runs, the current KV snapshot should be passed to it (as a parameter, not just to external subprocesses). A json formatter that can read `$kv_indent` to control pretty-printing behaves like bash-rd's formatters.

3. **Make `rdw pipe` dumb.** Remove mirroring, title setting, layout application, and filter registration from `rdw pipe`. Those belong in `tee`, `rdw pane rename`, `rdw layout apply`, and `rdw formatter register` respectively. The user composes them in the shell.

4. **Serve the SPA from a configurable directory.** The embedded SPA is the default; `--frontend /path/to/dir` serves a custom one. This restores the bash-rd principle that the user owns the display.

5. **Replace sentinel framing with standard encoding.** `image:` / `image:end` should not exist. Binary data should arrive as `base64 | rdw pipe --id chart` with the pane formatter set to `image`. The encoding is handled by the standard `base64` tool, not by a custom framing protocol inside rdw.
