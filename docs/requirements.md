# Remote Display Web (rdw) — Functional Requirements

## Vocabulary

- Server — the background singleton daemon that manages the session, routes data streams, maintains the KV store, authenticates requests, and serves the web front-end via WebSockets and REST.

- Session Manager — the component within the Server responsible for persisting session state: pane geometry, KV store contents, layout snapshots, and active tokens.

- Client — the single binary operating in command, configuration, or relay mode, used to pass data, layout updates, or configuration changes to the Server.

- Session — a single running instance of the Server and its associated collection of Pane Groups, Windows, Panes, authentication tokens, KV store, and connected browser interfaces.

- Pane Group — a named collection of Panes that can be collectively hidden, isolated, focused, or terminated with a single command. Multiple Pane Groups can exist simultaneously across different Windows or within the same Window.

- Window — a top-level visual container inside the web browser (equivalent to a tmux window or browser tab) containing one or more Panes.

- Pane — a distinct, bounded visual area inside a Window dedicated to rendering data for a specific Target ID. If a Window contains only a single Pane, layout, visibility, and closing actions applied to that Pane automatically apply to the encompassing Window. A Window MUST contain at least one Pane and MUST NOT contain more than 64 Panes.

- Target ID — a string identifier mapping an incoming data stream to a designated Pane. A Target ID MUST match the pattern `[a-zA-Z0-9_][a-zA-Z0-9_ -]*` and MUST NOT exceed 64 characters.

- Scrollback Buffer — the in-memory record of lines received by a Pane. The default cap is 10,000 lines; this is configurable per Pane up to 100,000 lines. Lines exceeding the cap are discarded from the oldest end.

- Control Sequence — a specially prefixed string embedded in a data stream that instructs the Server to perform a side-effect rather than render the payload as content. The recognized prefix set is defined in the Control Sequence Reference section.

- Filter — an external script or executable that the Server pipes incoming data through before rendering. A Filter transforms or discards content; it does not produce HTML.

- Formatter — a presentation engine within the Server that structures incoming raw text or KV store state into rendered HTML, grid, or tree layouts. A Formatter produces output; it does not transform the stream itself.

## Core Functional Requirements

### Data Routing, Interoperability, and Inter-Process Proxying

**Forwarding Targets**

- By default, the Client forwards `stdin` data exclusively to the Server.
- A configuration flag allows the Client to single-route input to `rdw` only, to `rd` (bash-rd) only, or to dual-route to both systems concurrently.
- Backward compatibility with bash-rd is optional and requires an explicit opt-in flag.

**Unassigned Target Rules**

- Incoming data addressed to a Target ID that does not map to an existing Pane MUST produce an error warning on the Client.
- An explicit configuration flag MUST be passed to allow the Server to dynamically create a new Pane or Window for unassigned Target IDs. This behaviour is off by default.

**Mid-Stream Data Transformation (Filters)**

- The Server supports the integration of external Filter binaries or scripts.
- Incoming data is piped through these defined external processes to modify, parse, or colorize the content prior to rendering.
- A Filter chain MUST NOT exceed 8 stages. Attempts to configure more MUST be rejected at startup with a descriptive error.

**Binary Encoding Contract**

- Binary data sent through the pipeline MUST be base64-encoded by the sender before transmission.
- The Server decodes base64 payloads transparently before passing them to Formatters or saving to the Scrollback Buffer.
- This matches the encoding convention established in bash-rd.

**Control Sequence Escaping**

- A `v:` verbatim prefix (matching bash-rd convention) MUST be supported, allowing clients to send data that would otherwise be interpreted as a Control Sequence.
- No other escaping mechanism is required.

**Stream Reconnection Buffering**

- The Client MUST maintain a local in-memory queue of up to 1,000 lines when the Server connection drops.
- On reconnect, the Client MUST flush the queued lines to the Server in order before resuming live forwarding.
- If the queue overflows before reconnection, the oldest lines are discarded and a warning is emitted to stderr.
- The Server MUST send a `RECONNECT` marker frame to the Client on re-established WebSocket connections so the Client knows when to begin the catch-up flush.

**Stream Optimization**

- Compressed transmissions: the Client pipeline supports Brotli compression as the default when forwarding data over remote network topologies. Zstd is available as an explicit opt-in flag. Compression is off by default on loopback connections.

### Dynamic Pipeline Modifications

**Dynamic Timestamps**

- The Server provides a configuration switch, toggleable at runtime via the CLI, REST API, or Control Sequence, to prepend a timestamp to all incoming data stream lines.

### Advanced Output Forwarding and Redirection

**Stream Redirection**

- The Server can duplicate and forward any incoming data stream to external targets alongside the web rendering engine.
- Users can specify a local file path or a Unix named pipe (FIFO) as an asynchronous mirror target for any specific Target ID.
- Incoming data can be piped as `stdin` into an arbitrary external shell command or executable specified at runtime.

### Key-Value Store and Control Commands

**Embedded Key-Value Store**

- The Server maintains an internal, stateful KV store scoped to the Session. Keys from all Panes and Windows share the same Session-wide namespace by default.
- Optional namespacing by Window or Pane is supported via a key-prefix convention: `window:<window_name>:<key>` and `pane:<target_id>:<key>`. The Server enforces this convention when a namespace prefix is present.
- KV keys MUST match the same character-set rule as Target IDs: `[a-zA-Z0-9_][a-zA-Z0-9_ -]*`, maximum 64 characters.
- KV values MUST NOT exceed 64 KB each. The total KV store size per Session MUST NOT exceed 64 MB.
- Data can be written to, read from, or updated inside the KV store using Control Commands via the CLI, REST API, or inline Control Sequences.

**KV Store Persistence**

- By default, the KV store is memory-only and is lost when the Server process exits.
- An optional `--kv-persist <path>` flag at server startup enables disk persistence via SQLite at the specified path.
- When persistence is enabled, the KV store is restored automatically on Server restart with `--restore`.

**Inline Control Sequences**

- The data stream parsing engine MUST detect Control Sequences embedded in text payloads.
- Control Sequences can manipulate Pane state (clear buffer, change color), update right-click menu definitions, or modify KV store values mid-stream.
- The full set of recognized prefixes is:

| Prefix | Action         | Description                                                        |
| ------ | -------------- | ------------------------------------------------------------------ |
| `v:`   | verbatim       | send data that looks like a Control Sequence without interpretation |
| `q:`   | quit           | stop the Server                                                    |
| `s:`   | semaphore      | increment the Server semaphore; Server quits when it reaches zero  |
| `c:`   | clear          | clear the target Pane's Scrollback Buffer                          |
| `t:`   | timestamp      | toggle timestamp prepending on the target Pane                     |
| `f:`   | set formatter  | dynamically set the Formatter for the target Pane                  |
| `r:`   | relay output   | format `r:location:[pid]`                                          |
| `=:`   | key=value      | write one or more KV pairs; multiple pairs separated by `;`        |

**Templating and Formatting Engine**

- Panes can use predefined templates to render incoming text or KV store state.
- Templates can read KV store values, substitute variables, and produce complete HTML layouts within the Pane.

### Content Handling and Presentation

**Image Ingestion Pipeline**

- CLI method: a dedicated command tells the Server to open a local image file path and dispatch it to the specified Target ID.
- REST API method: supports receiving either raw binary image payloads (PNG, JPG, SVG) in the request body, or a JSON payload containing a local file path for the Server to read.
- Binary image payloads sent through the standard stream pipeline MUST be base64-encoded per the Binary Encoding Contract above.

**Specialized Format Parsers**

- JSON/YAML: explicit subcommands and REST API paths receive raw JSON or YAML content and render it as an interactive, collapsible, syntax-highlighted tree. No automatic parsing is attempted on general stream data.
- Markdown: an explicit subcommand and REST API path compile Markdown input into formatted HTML inside designated Panes.
- CSV/TSV: dedicated CLI commands and REST API paths parse tabular text into interactive, sortable grid views.

**Advanced Formatting Engine**

- ANSI true color pass-through: the formatting engine decodes 24-bit ANSI escape sequences into precise CSS within the browser.
- Regex highlighting profiles: users can configure custom regex matchers to apply text color, background highlight, or flash to matched patterns inside live Pane output.

**Advanced Search**

- The web interface includes a search utility supporting exact string and fuzzy matching.
- Queries can be scoped to a single Pane, all Panes in the active Window, or all Windows in the Session.

### Window and Pane Management

**Dual-Mode Window Control**

- Layout definitions, window manipulations, pane sizing, and tab movements can be performed with full parity through two tracks:
  - Programmatic track: CLI subcommands or authenticated REST API.
  - Interactive track: drag-and-resize handles, UI toggle buttons, keyboard shortcuts, and click events in the browser.

**Pane Resize Units**

- `rdw pane resize` accepts size arguments in three forms: absolute character columns/rows (default), pixels (suffix `px`), or percentage of the Window (suffix `%`).
- The default unit is character columns/rows. All three forms are accepted by both the CLI and REST API.

**Pre-defined Layout Profiles**

- Comprehensive window and pane configurations can be saved in configuration files.
- A specific configuration file path can be passed at startup or via runtime layout commands to instantiate a complex layout.
- Layout files MUST carry a `schema_version` field. The current schema version is `1`. The Server MUST reject layout files with an unrecognized schema version and emit a descriptive error.
- The active runtime layout can be snapshotted and exported via a save command.

**Native vs Virtual Tabs**

- The Server supports rendering all Windows as virtual tabs within a single unified web page, or as independent native browser tabs.

**Dynamic Extraction**

- The Web UI provides a contextual action on every Pane, Window, and Pane Group to extract it into a separate native browser tab or window via a generated URL.

**Topology Re-Arrangement**

- Swapping: any two Panes within the same Window can have their positions swapped.
- Detachment and re-attachment: a Pane can be detached from its parent Window and re-attached into a split grid in a different Window. If the target Window has no available split slot, the Server MUST return an error rather than silently failing.

**Sticky Elements and Focus Modes**

- Layout sticky headers: Panes can freeze the first N lines of an incoming stream at the top of the viewport while the body continues to scroll.
- Pane zooming: Panes can be zoomed to occupy the full Window, reversing on toggle.
- Focus cycle automation: the active Window can be rotated automatically every X seconds for use on wall screens or dashboards.

**Pane Group Operations**

- Commands can purge, hide, focus, or terminate all Panes in a named group simultaneously.

**Web Interface Provisioning**

- Authorized users can create new Windows or Panes from the Web UI and assign Target IDs on the fly.
- Newly created layouts can be set as private (session owner only) or shared (accessible to users with write-access tokens).

**Terminal Sharing Integration**

- Panes can embed web-based terminal emulators.
- A `gotty` HTTP stream can be embedded inside a designated Pane to provide interactive terminal access within the rdw layout.
- Terminal-sharing Panes MUST run under a dedicated restricted Unix user account with no write access outside a designated working directory. This is a hard requirement; starting a terminal-sharing Pane without the restricted user configured MUST fail with a descriptive error.

### Advanced Browser Interface Interaction

**Persistent Buffer Navigation Marks**

- The Scrollback Buffer supports dropping explicit bookmarks at any line, navigable via keybindings or CLI commands.

**Configurable Interface Shortcuts**

- The Web UI supports user-defined hotkey maps. The default map follows Vim-like directional and edit patterns.

**Interactive Workspace Editor Panel**

- A visual layout scratchpad panel allows operators to manipulate text properties, edit configurations, or construct scripts without breaking the workspace layout.

**System Clipboard Integration**

- Right-click selections or UI macros can export highlighted plain text to the clipboard of the host running the browser.

**Graceful JavaScript-Off Degradation**

- The web interface MUST render Pane Scrollback Buffer content as static HTML when JavaScript is disabled or unavailable.
- Layout manipulation, live updates, and interactive features require JavaScript and are silently absent in this mode. No error page is shown; content is visible.

### Local Snapshot and Storage Exporting

**Universal Markdown Export**

- Any authenticated user can download the contents of a specific Pane, an entire Window, or all active Session Windows.
- The export is placed in a specified local output directory containing a Markdown file mapping scrollback history alongside an `assets/` subfolder housing all streamed binary image files.
- The Markdown file structure is: top-level heading per Window, second-level heading per Pane, body text as the scrollback content, image references pointing into `assets/`.

### User Interface Design

**Minimalistic Web UI**

- The interface follows a strictly minimalist aesthetic. Sidebars, borders, tabs, and controls use compact padding and collapse or hide when not explicitly focused.

**Color Scheme Profiling**

- The global configuration maintains Dark, Light, and custom operator color palette definitions.
- Color schemes can be targeted by name when initializing a Pane or Window.

### Interactive Elements

**Contextual Right-Click Menus**

- Panes support customizable right-click context menus defined via the Client.
- Read-only mode: displays dynamic text updated remotely via the API or KV store.
- Interactive mode execution: triggering server-side shell commands from a right-click menu is disabled by default and MUST be enabled via an explicit security flag at Server startup.

### Management Interfaces

**Administrative Console**

- A secure web page accessible to the Server owner or authorized users.
- Accessible on loopback only by default. The `--network-expose` flag at startup extends access to the configured network interface. This flag MUST be explicitly set; no implicit network exposure occurs.
- Provides visual tools to manipulate the layout tree, terminate active Target IDs, and view or revoke access tokens.

## Security and Authorization Architecture

### Transport and Protocol

**WebSocket Sub-Protocol**

- The Server MUST advertise and negotiate the WebSocket sub-protocol identifier `rdw-v1`.
- Clients that do not present `rdw-v1` in the `Sec-WebSocket-Protocol` header MUST be rejected.
- Future protocol versions increment the version suffix. A Server MAY support multiple versions simultaneously during a transition period.

**REST API Versioning**

- All REST API endpoints are rooted at `/api/v1/`.
- The Server MUST return HTTP 404 for requests to unversioned paths rather than silently falling through to the latest version.

**Default Port**

- The Server listens on port `7681` by default. This is overridden with `--port <port>` at startup.
- The port is documented in the annotated configuration file.

### API and Command Parity

**Full Command Mirroring**

- Every CLI command is fully exposed via the REST API at a corresponding `/api/v1/` path.

**Native Owner Privileges**

- CLI requests are authenticated by verifying that the calling process is the same Unix user that started the Server, enforced via a Unix domain socket at `$XDG_RUNTIME_DIR/rdw/<session_id>.sock` with file permissions `0600`.
- Remote CLI requests (via REST) require a token.

**Explicit Access Delegation**

- The Server owner can grant command execution permissions to other users via tokens. Authorized users can issue layout adjustments, pipeline modifications, and stream lifecycle actions via the authenticated REST API.

### Token-Based Access Control

**User Access Tokens**

- The Server configuration file maintains a registry of access tokens mapped to user profiles.
- Tokens MUST be stored hashed (SHA-256) in the configuration file and in any persistent store. Plain-text tokens MUST NOT be written to disk by the Server. The plain-text token is shown once at creation time only.

**Granular Pane Sharing**

- When creating a Pane or Window, the creator can restrict or permit access per registered user profile.
- Users without explicit authorization for a given Pane will not see its existence in list output, metadata queries, or the web dashboard.

**Dynamic Token Lifecycles**

- Tokens can be time-limited and expire automatically. Default expiry for newly created tokens is 24 hours unless overridden with `--expiry <duration>`.
- Token revocation MUST immediately terminate all active WebSocket connections bound to the revoked token.

**Rate Limiting**

- Unauthenticated REST endpoints MUST enforce a rate limit of 10 requests per minute per source IP to mitigate brute-force token discovery.

## Documentation, Testing, and System Integration

### System Deliverables

**Pre-Documented Reference Configuration**

- The installation package MUST ship with a fully annotated template configuration file documenting every security boundary, default port, token structure, browser preference, and layout profile definition.

**Shell Autocompletion**

- The binary MUST include `rdw completion bash` outputting standard autocomplete rules to stdout.

**Native System Documentation**

- A comprehensive set of UNIX man pages covering the binary syntax, subcommands, layout paradigms, and configuration options.

**Visual Implementation Guide**

- Project documentation MUST include an example-driven reference guide coupling real-world pipeline scenarios with UI screenshots.

**Headless Test Integration**

- The Server MUST operate in headless mode: initializing, managing multi-pane arrangements, and buffering incoming data without an active browser attached. This mode is used for CI pipeline testing.

**Self-Test Mode**

- The binary MUST support `rdw selftest`, which starts an in-process Server, sends a known payload to a test Target ID, and verifies the WebSocket output matches the expected result. Exit code 0 on success, non-zero on failure. This enables CI smoke testing without a browser.

**Standalone Deployment**

- All frontend assets, CSS, scripts, and interface controllers MUST be compiled into the binary. No external internet connection is required at runtime.

## Future Specifications Reserved For Implementation

- `[Reserved: REST API Webhook Target Adapters]` — normalization profiles for third-party data formats (GitHub, GitLab, corporate messaging webhooks) mapped to standard target log lines.

## Layout Management Commands

| Command | Target | UI Alternative | Description |
| ------- | ------ | -------------- | ----------- |
| `rdw window create [--config <path>] [--private\|--shared]` | Window | Click "New Tab" in browser | Creates a new window, optionally from a layout config. |
| `rdw window kill <name>` | Window | Click "X" on tab header | Destroys the window and tears down all nested pane streams. |
| `rdw window focus <name>` | Window | Click the tab title | Switches the active visible window in the browser viewport. |
| `rdw pane split <target_id> <h\|v> <new_id> [--group <name>] [--private\|--shared] [--allow-user <user>]` | Pane | Drag split handle | Splits the target pane horizontally or vertically, assigning a new Target ID. |
| `rdw pane swap <pane_id_a> <pane_id_b>` | Pane | Drag one pane over another | Swaps the display positions of two panes in the same window. |
| `rdw pane detach <pane_id> --to-window <window_name>` | Pane | Drag pane title bar out of tab | Moves a pane into a target window's split grid. Fails if no split slot is available. |
| `rdw pane zoom <pane_id>` | Pane | Double-click pane border | Toggles the pane between normal layout and full-window focus. |
| `rdw pane resize <pane_id> <direction> <value>[px\|%]` | Pane | Drag the gutter between panes | Resizes by columns/rows (default), pixels (px), or percentage (%). |
| `rdw pane clear <pane_id>` | Pane | Click "Clear" in pane menu | Empties the Scrollback Buffer and clears the visual field. |
| `rdw pane color <pane_id> --scheme <scheme_name>` | Pane | Style submenu in pane UI | Applies a named color scheme to the pane. |
| `rdw pane font <pane_id> --size <point_size>` | Pane | Style submenu or zoom hotkey | Sets the font size for the pane. |

## Core Reference Commands

- `rdw server start [--port <port>] [--config <path>] [--network-expose] [--no-auth] [--open-browser] [--restore] [--kv-persist <path>]`
  - Starts the background daemon. `--open-browser` opens the default browser on start. `--restore` restores the last saved session state. `--kv-persist` enables SQLite-backed KV persistence.

- `rdw open [--browser <name>] [--new-instance]`
  - Launches a local browser directed at the Server interface.

- `rdw group <create|hide|show|kill|focus> <group_name>`
  - Manages lifecycle, visibility, and multi-pane state of a named Pane Group.

- `rdw list [--json|--text]`
  - Lists all active windows, panes, pane groups, and streams. Subject to token authorization when called via REST.

- `rdw pipe --id <target_id> [--forward <rdw|rd|both>] [--allow-unassigned] [--forward-to-file <path>] [--forward-to-cmd <cmd>]`
  - Relays `stdin` to the target stream while optionally mirroring to files or secondary executables.

- `rdw timestamp --id <target_id> <on|off>`
  - Toggles timestamp prepending on lines within an active pipeline.

- `rdw json --id <target_id>`
  - Pipes data into the JSON tree formatter.

- `rdw yaml --id <target_id>`
  - Pipes data into the YAML tree formatter.

- `rdw markdown --id <target_id>`
  - Dispatches a text pipeline into the Markdown rendering engine.

- `rdw csv --id <target_id> [--delimiter <char>]`
  - Passes tabular data into the grid view engine.

- `rdw image --id <target_id> [--path <file_path>|--raw]`
  - Dispatches an image via file reference or raw base64-encoded piped data.

- `rdw save <pane|window|all> --target-id <id> --out-dir <directory_path>`
  - Exports scrollback history and image assets to a local Markdown bundle.

- `rdw layout save --name <preset_name> [--config <path>]`
  - Snapshots current pane geometry and appends it to the active configuration profile.

- `rdw kv <set|get|delete> <key> [<value>]`
  - Interacts with the Session KV store from the CLI.

- `rdw token <create|revoke> [--expiry <duration>] [--panes <list>] [--windows <list>]`
  - Creates or revokes session authentication tokens. Default expiry is 24 hours.

- `rdw completion bash`
  - Outputs the bash autocompletion script to stdout.

- `rdw selftest`
  - Runs the built-in smoke test suite. Exits 0 on success.

## Security Architecture Analysis and Vulnerability Review

### Identified Security Risks

**Target Boundary Escalation via Shared URL**

- Sharing a Pane via URL does not automatically restrict WebSocket scope. A malicious user could craft WebSocket messages targeting other Target IDs or administrative commands if the Server does not enforce per-token scope on every message.

**Local Network Exposure**

- Exposing the Server to the local network opens XSS vectors if Panes render raw HTML. An attacker on the network could inject malicious HTML payloads targeting the local browser session.

**Process Execution via Right-Click Menus**

- When interactive right-click execution is enabled, untrusted log data that rewrites menu actions could trick an administrator into executing arbitrary server-side commands (RCE).

**Token Storage**

- Long-lived tokens stored in plain text in configuration files expose permanent stream access to any process that can read the file. Mitigated by the hashed-storage requirement above.

**Collaborative Write Risks in Shared Panes**

- A compromised colleague with write access to a shared Pane could push malicious Control Sequences or scripts into the layout.

**Terminal Sharing Privilege Escalation**

- gotty-backed terminal Panes must not allow viewers to escalate privileges on the host. Mitigated by the mandatory restricted-user requirement above.

### Required Security Mitigations

**Strict Scope Isolation**

- The Server MUST validate every WebSocket message against the token used to connect. A token scoped to Pane X MUST be blind to messages, metadata, and layout changes for Pane Y.

**HTML Input Sanitization**

- The front-end MUST sanitize all incoming stream content before rendering. Raw script tag execution MUST require explicit configuration opt-in.

**Token Revocation Propagation**

- Revoking a token via the Admin Console MUST immediately terminate all WebSocket connections bound to that token.

## Enumerated Use Cases

- Distributed application log tailing — piping multiple microservice logs into distinct panes in a single browser window.
- Compiler and build system observation — routing stdout and stderr into separate highlighted panes.
- Continuous integration pipeline tracking — streaming job execution results into a shared pane for team visibility.
- Database query performance auditing — piping slow query logs through filters that colorize execution durations.
- Live infrastructure health dashboard — pushing system performance summaries into a multi-pane layout via cron.
- Network traffic monitoring — routing tcpdump summaries into a pane during connectivity testing.
- Remote collaboration debugging — sharing a pane layout so a colleague can inject event logs alongside local debug streams.
- Webhook payload verification — directing incoming webhook requests into a pane to view formatted JSON.
- Automated testing visualization — ingesting HTML test reports or failure screenshots into a browser window post-run.
- Graphic asset pipeline validation — sending rendered SVG or PNG output from a build script directly to a pane.
- Multi-server provisioning tracking — mapping individual Ansible host results into distinct pane sections.
- Secure read-only production log sharing — generating a scoped token to share a single log pane with a support tier without granting SSH access.
- Interactive terminal support sharing — embedding a restricted gotty instance for pair-programming with a remote colleague.
- Security audit log tailing — routing auth logs through a filter into a highlighted pane.
- Embedded system kernel tracking — streaming serial output or dmesg logs from a hardware fixture into a dashboard pane.
- Long-running task completion alerting — pushing a status image or HTML summary into a zoomed pane on script completion.
- Performance profile analysis — sending SVG flame graphs to a pane for immediate visualization.
- Kubernetes pod event streams — mirroring cluster lifecycle warnings into a defined pane group.
- IoT sensor aggregation — piping formatted metrics from multiple MQTT devices into individual monitored panes.
- Live database schema evolution tracking — directing migration runner output into a shared pane to keep teams synchronized.
