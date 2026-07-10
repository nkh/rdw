# WebSocket Protocol

Sub-protocol: `rdw-v1`

Connect: `ws://HOST:PORT/api/v1/ws?token=TOKEN`

All frames are newline-delimited JSON objects.

## Server → Browser message types

| `type` | Fields | Trigger |
| --- | --- | --- |
| `line` | `target_id`, `line` | New output line for a pane |
| `layout_update` | `payload.windows`, `payload.active_window` | Any layout change |
| `RECONNECT` | — | Client reconnected; flush queue |
| `highlight_set` | `target_id`, `profile` | `hl:` control sequence |
| `scrollback_ctl` | `target_id`, `action` | `sc:` control sequence |
| `image_render` | `target_id`, `data`, `scale` | `image:` sentinel block |
| `svg_render` | `target_id`, `data`, `scale` | `svg:` sentinel block |
| `pane_scale` | `target_id`, `scale` | `scale:` control sequence |
| `formatter_set` | `target_id`, `formatter` | Formatter restored after image/svg |

## Reconnect behaviour

On reconnect the server sends a `RECONNECT` frame. The client flushes its local reconnect queue (up to 1000 lines buffered during disconnect) in order. The server has already cleared its in-flight queue; the client queue covers the gap.
