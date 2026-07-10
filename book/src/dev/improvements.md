# rdw — remaining work and improvements

## What is left to do

All previously listed items are now resolved:

- `rdw pipe --filter CMD` — implemented: `pipeline.CmdFilter`, `POST /api/v1/panes/{id}/filters`, `--filter` flag on `rdw pipe`
- `rdw pipe --layout FILE` schema_version validation — added: client validates `schema_version == 1` before upload
- Layout apply router reconciliation — implemented: `handleLayoutApply` diffs pane sets and calls `Deregister`/`Register`
- `sc:clear` end-to-end test — added: `TestAPI_ScrollbackClear_EmptiesBuffer`
- `rdw cycle status` — implemented: `GET /api/v1/cycle/status`, `rdw cycle status` CLI command

## 50 improvements

### Pipeline

1. `rdw pipe --filter CMD` — attach an external filter (any shell command reading stdin, writing stdout) to the pipeline at runtime
2. Named filter presets stored server-side, applicable per-pane via `POST /api/v1/panes/{id}/filters`
3. Filter introspection endpoint — `GET /api/v1/panes/{id}/filters` lists active stages
4. Backpressure from the WebSocket Hub to slow pipelines instead of silent line drops
5. Per-pane overflow policy configurable at pane creation: `drop-oldest`, `drop-newest`, `block`
6. Per-TargetID rate limit to prevent one noisy source from starving others
7. Line batching — flush multiple lines in a single WebSocket frame when source rate exceeds 60 fps
8. `rdw pipe --replay FILE [--speed N]` — replay a saved log at wall-clock or Nx speed

### Browser SPA

9. Dark/light theme toggle, defaulting to `prefers-color-scheme`
10. Per-pane font size and font family, adjustable in the browser
11. Pane-scoped search — current `/` searches all panes; add a flag to scope to the focused pane
12. Search history — up-arrow in the search input recalls previous patterns
13. Highlight rules applied live as new lines arrive, not only on `hl:` sequence arrival
14. Pane timestamp overlay toggle from the browser UI
15. Copy-to-clipboard button per pane
16. Pane pinning — keep a pane visible across window switches
17. WebSocket connection status indicator in the window header
18. Unread badge on window tab when new lines arrive in a non-active window
19. Mobile-responsive layout — single-column on narrow viewports
20. Keyboard binding help overlay (`?` key) rendered as a table — currently absent from the SPA

### Layout

21. Layout import by file drag-and-drop in the browser
22. Layout diff view — show what changed when `layout apply` is called
23. Named layout snapshots with timestamps, not just a flat name map
24. Pane group collapse/expand in the browser without closing panes
25. Grid snap-to-percent during gutter drag
26. Layout templates — predefined skeletons selectable from the browser (2-col, dashboard, etc.)
27. Per-window pane count limit as a layout field, not a global constant

### KV store

28. KV change notifications over WebSocket — subscribe to a key prefix and receive updates
29. Per-key TTL with automatic expiry
30. KV namespace operations — list all keys under a prefix, delete-all for a namespace
31. KV import/export endpoint — JSON dump and restore of all keys
32. KV size and entry count reported in `GET /api/v1/session`

### Auth and security

33. Read-only token scope — allows stream viewing but no mutations
34. Token rotation — atomically create a replacement and revoke the old in one call
35. HTTPS support — configurable cert path or auto-generated self-signed cert
36. CORS origin allowlist in config for cross-origin browser access
37. Rate limit configuration in `config.yaml` — currently hardcoded at 10 req/min
38. Audit log — append-only record of all auth events and API mutations

### Export and formatters

39. PDF export via headless Chromium or wkhtmltopdf
40. HTML export — self-contained file including the SPA for offline replay
41. JSON Lines formatter — one collapsible tree per line
42. Diff formatter — unified diff between consecutive JSON or text snapshots
43. SQL formatter with syntax highlighting
44. Image slideshow mode — cycle multiple `b64:` frames in the same pane

### Operations

45. `SIGHUP` config reload without restart
46. Prometheus metrics endpoint — line counts per pane, connection count, KV size
47. Structured JSON server log with configurable level
48. Session persistence — save and restore the full window/pane layout across restarts
49. `rdw server start --demo` — start with synthetic data for UI testing

### Protocol

50. `rdw pipe` reconnect with exponential backoff instead of a fixed 2-second interval
