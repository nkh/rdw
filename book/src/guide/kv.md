# Key-Value Store

The session KV store is shared across all panes. Any stream can write to it; any filter or formatter can read it.

## Writing from a stream

```sh
echo "=:build.status=passing;build.duration=12s" | rdw pipe --id ci
```

Multiple pairs in one line, separated by `;`.

## Writing from the CLI

```sh
rdw kv set build.status passing
rdw kv get build.status
rdw kv delete build.status
rdw kv list build.      # prefix filter
```

## Namespacing

Keys follow the same character rules as Target IDs. Use prefixes as a convention:

```sh
rdw kv set window:build:title "CI Build"
rdw kv set pane:log:color green
```

Prefixes are not enforced — they are a naming convention.

## Persistence

```sh
rdw server start --kv-persist ~/.rdw/kv.db
rdw server start --kv-persist ~/.rdw/kv.db --restore
```

Every `set` and `delete` writes through to SQLite. `--restore` loads the full store on startup.

## KV in filters and formatters

The current KV snapshot is injected into every filter and user-defined formatter subprocess as environment variables with original key names:

```sh
rdw kv set threshold 100
rdw kv set env production
```

The filter script sees `$threshold` and `$env` as environment variables. Injection is dynamic: the snapshot is refreshed on every line.

## Via the API

```sh
curl -H "Authorization: Bearer $TOKEN" http://localhost:7681/api/v1/kv
curl -H "Authorization: Bearer $TOKEN" http://localhost:7681/api/v1/kv/build.status
```
