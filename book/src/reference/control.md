# Control Sequences Reference

See [Control Sequences guide](../guide/control.md) for examples.

| Prefix | Kind | Payload | Action |
| --- | --- | --- | --- |
| `v:` | single | any | Verbatim passthrough |
| `=:` | single | `k=v;k2=v2` | KV write |
| `f:` | single | formatter name | Switch formatter |
| `b64:` | multi | base64 | Binary passthrough |
| `bm:` | multi | name | Create bookmark |
| `hl:` | multi | profile name | Apply highlight |
| `sc:` | multi | `clear\|top\|bottom` | Scrollback control |
| `title:` | multi | text | Set pane title |
| `image:` | sentinel | — | Start binary image block |
| `image:end` | sentinel | — | End binary image block |
| `svg:` | sentinel | — | Start SVG block |
| `svg:end` | sentinel | — | End SVG block |
| `scale:` | multi | `fit\|fill\|native` | Image/SVG scaling |
| `t:` | single | — | Toggle timestamp |
| `c:` | single | — | Clear scrollback |
| `q:` | single | — | Stop server |

Single-char prefixes use format `X:payload`. Multi-char prefixes use `prefix:payload`. Sentinel sequences span multiple lines.
