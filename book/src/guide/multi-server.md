# Multiple Servers

Multiple rdw instances can run simultaneously on different ports. Every command accepts `--port / -p` to address a specific instance.

```sh
rdw server start --port 7681 &
rdw server start --port 7682 &

echo "hello" | rdw pipe --port 7681 --id a
echo "world" | rdw pipe --port 7682 --id b

rdw server list
```

## Discovery

When `--port` is omitted, `rdw` probes port 7681. If that fails, it reads `$XDG_CACHE_HOME/rdw/servers.json` for registered instances and lists them in the error message.

## Registry

The server registers itself in `servers.json` on start and removes its entry on stop. Stale entries (dead processes) are pruned automatically.
