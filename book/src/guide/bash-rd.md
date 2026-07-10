# bash-rd Compatibility

rdw is the successor to [bash-rd](https://github.com/nkh/bash-rd). The `--forward` flag enables co-existence.

```sh
my_app | rdw pipe --id log --forward rd    # sends to both rdw and bash-rd (rd)
my_app | rdw pipe --id log --forward both  # same as above
my_app | rdw pipe --id log --forward rdw   # rdw only (default)
```

`--forward rd` pipes the stream through `mirror.CmdSync("rd 2>/dev/null || true")` in parallel with the rdw relay.

## Differences from bash-rd

| Feature | bash-rd | rdw |
| --- | --- | --- |
| Language | Bash + Perl | Go |
| Transport | Unix socket | Unix socket + HTTP |
| Browser UI | gotty / ttyd | Embedded SPA |
| KV store | In-process Perl hash | Session-scoped, optional SQLite |
| Filters | Shell functions with KV access | External commands with KV env injection |
| Formatters | Shell scripts | Built-in (6) + user-defined external commands |
| Multi-server | No | Yes (`--port`) |
| Auth | None | Bearer tokens (SHA-256 hashed) |
