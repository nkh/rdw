# rdw — 60 possible improvements

## Core pipeline

1. Per-pane filter chain configurable at runtime via `POST /api/v1/panes/{id}/filters` rather than only at server start
2. Named filter presets storable server-side (like highlight profiles) and applicable per-pane
3. Filter chain introspection endpoint to see what transformers are active on a pane
4. Backpressure signal from Hub to pipelines — slow browsers should slow the pipeline rather than drop lines silently
5. Per-pane configurable overflow policy: `drop-oldest`, `drop-newest`, or `block`
6. Stream rate limiting per TargetID to prevent one noisy source from starving others
7. Line batching — flush multiple lines in a single WebSocket frame when the source is faster than 60 fps
8. Gzip WebSocket frame compression (permessage-deflate extension)

## Browser SPA

9. Dark/light theme toggle with system `prefers-color-scheme` as default
10. Font size and font family configurable in the browser without a server restart
11. Pane-level scrollback search scoped to current pane only (current search is global)
12. Search history (up-arrow in search input recalls previous patterns)
13. Regex highlight rules applied live in the DOM as lines arrive, not just on `hl:` sequence
14. Pane timestamp overlay toggle from the browser (currently only via `t:` control sequence)
15. Copy-to-clipboard button per pane
16. Pane pinning — pin a pane to always show even when its window is not active
17. Connection status indicator (green/amber/red dot showing WebSocket state)
18. Notification badge on window header tab when new lines arrive in a non-active window
19. Mobile-responsive layout — single-column on narrow viewports
20. Keyboard binding help overlay (`?`) shows a rendered table, currently not implemented in SPA

## Layout system

21. Layout import from file drag-and-drop in the browser
22. Layout diffing — show what changed when `layout apply` is called
23. Layout versioning beyond `schema_version` — named snapshots with timestamps
24. Pane groups collapsible in the browser (hide/show without closing)
25. Grid snap-to-percent during gutter drag (currently continuous float)
26. Maximum pane count currently hard-coded at 64 — make it a per-window config field
27. Layout templates — predefined skeletons (2-col, 3-col, dashboard) selectable from the browser

## KV store

28. KV change notifications over WebSocket — subscribe to a key prefix and receive updates as messages
29. KV TTL — optional expiry per key, expired keys auto-deleted
30. KV namespaces as first-class API objects with list/delete-all operations
31. KV import/export endpoint (JSON dump of all keys)
32. KV size reporting in `GET /api/v1/session`

## Authentication and security

33. Multiple token scopes beyond per-pane: read-only, write-only, admin
34. Token rotation endpoint — create a replacement and revoke the old in one call
35. HTTPS support with auto-generated self-signed cert or configurable cert path
36. CORS origin allowlist in config for cross-origin browser access
37. Rate limit configuration in `config.yaml` (currently hardcoded at 10 req/min)
38. Audit log — append-only log of all auth events and API mutations

## Export and formatters

39. PDF export via headless browser (wkhtmltopdf or chromium `--print-to-pdf`)
40. JSON Lines (JSONL) formatter — one collapsible object per line
41. HTML export that includes the embedded SPA for a self-contained offline view
42. Diff formatter — show unified diff for consecutive JSON or text snapshots
43. Table formatter with column alignment for whitespace-delimited output
44. SQL formatter with syntax highlighting
45. Image slideshow mode — if multiple `b64:` lines arrive, cycle them

## Server operations

46. Graceful reload of config without restart (`SIGHUP`)
47. Prometheus metrics endpoint (`GET /metrics`) with pane line counts, connection counts, KV size
48. Structured JSON server log with configurable level (debug/info/warn/error)
49. Session persistence to disk — full window/pane layout saved and restored across restarts
50. Multi-user mode — separate KV namespaces and token scopes per user identity

## Developer experience

51. `rdw pipe --dry-run` — show what would be sent without connecting to server
52. `rdw pipe --replay FILE` — replay a saved log file at wall-clock speed or N× speed
53. `rdw server start --demo` — start with synthetic data streams in all panes for UI testing
54. OpenAPI 3 spec auto-generated from routes at build time
55. Shell completion for `--id`, `--window`, and `--layout` flags (query live server for valid values)

## Protocol

56. Binary WebSocket frames for `b64:` image data instead of base64-encoded JSON strings
57. Server-sent events (SSE) fallback for environments where WebSocket is blocked
58. gRPC streaming endpoint as alternative to WebSocket for server-to-server use
59. `rdw pipe` reconnect with exponential backoff rather than fixed 2s interval
60. Multi-target pipe — `rdw pipe --id a,b,c` fans one stream to multiple panes simultaneously
