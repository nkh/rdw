# Control Sequences

Lines piped through `rdw pipe` that begin with a recognised prefix are intercepted server-side and acted on rather than displayed in the pane.

## Full sequence table

| Prefix | Payload | Action |
| --- | --- | --- |
| `v:` | any text | Verbatim passthrough — strip prefix, display as-is |
| `=:` | `k=v;k2=v2` | Write key-value pairs to the KV store |
| `f:` | formatter name | Switch pane formatter |
| `b64:` | base64 data | Binary passthrough for image formatter |
| `bm:` | name | Create scrollback bookmark at current line |
| `hl:` | profile name | Apply highlight profile to pane |
| `sc:` | `clear\|top\|bottom` | Scrollback control |
| `title:` | text | Set pane display title |
| `image:` | — | Start binary image block (terminated by `image:end`) |
| `svg:` | — | Start SVG block (terminated by `svg:end`) |
| `scale:` | `fit\|fill\|native` | Set image/SVG scaling mode |
| `t:` | — | Toggle timestamp prefix on subsequent lines |
| `c:` | — | Clear pane scrollback |
| `q:` | — | Stop the server |

## Examples

```sh
# KV write
echo "=:build.status=passing;duration=12s" | rdw pipe --id ci

# Verbatim (won't be interpreted as KV)
echo "v:=:this is literal text" | rdw pipe --id log

# Formatter switch
echo "f:json" | rdw pipe --id api

# Title
echo "title:Deploy v2.3" | rdw pipe --id deploy

# Bookmark
echo "bm:deploy-start" | rdw pipe --id deploy

# Highlight profile
echo "hl:errors" | rdw pipe --id log

# Scrollback
echo "sc:clear"  | rdw pipe --id log
echo "sc:top"    | rdw pipe --id log
echo "sc:bottom" | rdw pipe --id log

# Image block
{ echo "image:" ; cat chart.png ; echo "image:end" ; } | rdw pipe --id chart

# SVG block
{ echo "svg:" ; cat diagram.svg ; echo "svg:end" ; } | rdw pipe --id diagram

# Scale
echo "scale:fill" | rdw pipe --id chart
```
