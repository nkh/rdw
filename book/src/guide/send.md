# rdw send

Send a file to a pane without manual base64 encoding or control sequences. Type is detected automatically from magic bytes, then extension.

## Usage

```sh
rdw send --id TARGET_ID FILE
```

## Type detection

| Extension / Magic bytes | Action |
| --- | --- |
| PNG (`\x89PNG`), JPEG (`\xFF\xD8`), GIF, WebP | `image_render` in browser |
| `.svg` or `<svg` prefix | `svg_render` inline in browser |
| `.csv`, `.tsv` | formatter set to csv |
| `.md`, `.markdown` | formatter set to markdown |
| everything else | appended as plain text lines |

## Examples

```sh
rdw send --id chart   chart.png       # image
rdw send --id diagram flow.svg        # inline SVG
rdw send --id data    report.csv      # sortable table
rdw send --id docs    README.md       # rendered markdown
rdw send --id log     output.txt      # plain text
```

## Transport

`rdw send` uses the same Unix socket / HTTP transport as `rdw pipe`. It calls `discovery.Resolve` to find the running server, derives the socket path, and sends a single synthetic line.
