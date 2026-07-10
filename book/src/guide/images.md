# Images and SVG

## Sentinel framing

`image:` and `svg:` use sentinel lines to delimit the payload. rdw handles base64 encoding internally.

```sh
# PNG/JPEG/GIF/WebP
{ echo "image:" ; cat chart.png ; echo "image:end" ; } | rdw pipe --id chart

# SVG — rendered inline, fully interactive (hover, click handlers work)
{ echo "svg:" ; cat diagram.svg ; echo "svg:end" ; } | rdw pipe --id diagram

# SVG from a command
{ echo "svg:" ; gnuplot -e "set terminal svg; plot sin(x)" ; echo "svg:end" ; } | rdw pipe --id plot
```

## Scaling

| Sequence | Effect |
| --- | --- |
| `scale:fit` | width 100%, height auto (default) |
| `scale:fill` | width 100%, height 100% (fills pane) |
| `scale:native` | intrinsic size, scrollable |

```sh
echo "scale:fill" | rdw pipe --id chart
```

`scale:` applies to all subsequent images/SVGs in the pane and updates existing ones immediately.

## Formatter save/restore

When an `image:` or `svg:` block is processed, the server automatically saves the pane's current formatter, switches to `image`/`svg` for the render, then restores the original formatter. The producer does not need to manage this.

## Base64 passthrough

For cases where the source already has base64 data:

```sh
echo "b64:$(base64 -w0 chart.png)" | rdw pipe --id chart
```

This requires the `image` formatter to be active on the pane (`echo "f:image" | rdw pipe --id chart` first).
