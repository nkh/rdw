# Filters

A filter is an external shell command attached to a pipeline stage. It receives each incoming line on stdin and writes a transformed (or suppressed) line to stdout. Filters run server-side before lines reach the scrollback or the browser.

## Attaching filters

```sh
my_app | rdw pipe --id log --filter 'grep -v DEBUG'
my_app | rdw pipe --id log \
  --filter 'grep -E "ERROR|WARN"' \
  --filter 'sed s/ERROR/[ERR]/'
```

Maximum 8 filter stages per pipeline.

## Suppressing lines

If the filter writes nothing to stdout for a given input line, that line is dropped.

```sh
# Only pass lines containing ERROR or WARN
my_app | rdw pipe --id log --filter 'grep -E "ERROR|WARN" || true'
```

## KV injection

The current session KV snapshot is injected into the filter subprocess as environment variables with original key names:

```sh
rdw kv set prefix "[PROD]"
rdw kv set threshold 100

my_app | rdw pipe --id log \
  --filter 'while read l; do echo "$prefix $l"; done'
```

KV injection is **dynamic** — the snapshot is refreshed on every line, so filters always see current values. Filters are **read-only** with respect to KV.

## Via the API

```sh
curl -X POST http://localhost:7681/api/v1/panes/log/filters \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"cmd":"grep ERROR"}'
```

## Differences from formatters

| | Filter | Formatter |
| --- | --- | --- |
| Runs | On every incoming line | On demand |
| Mutates stored data | Yes — lines are transformed before storage | No — read-only |
| Output | Plain text line | HTML |
| Timing | Before scrollback | After scrollback |
