# rdw examples

## Basic output display

```sh
# Start the server, open the browser
rdw server start --open-browser

# Send output of any command to a pane named "log"
tail -f /var/log/syslog | rdw pipe --id log

# Any program that writes to stdout works
ping 8.8.8.8 | rdw pipe --id ping

# Multiple sources simultaneously, each in its own pane
journalctl -f | rdw pipe --id journal &
top -b       | rdw pipe --id top &
```

## Named windows and layouts

```sh
# Route into a specific window
my_script | rdw pipe --id output --window build

# Apply a saved layout before streaming
my_script | rdw pipe --id output --layout my-layout

# Apply a layout file
my_script | rdw pipe --id output --layout ./layouts/dashboard.yaml
```

## Multi-pane dashboard

```yaml
# layouts/dashboard.yaml
schema_version: 1
windows:
  - name: dashboard
    panes:
      - target_id: cpu
        split: h
        size: 50%
      - target_id: mem
        split: v
        size: 40%
      - target_id: disk
```

```sh
rdw layout apply dashboard

while true ; do top -bn1 | head -20 ; sleep 2 ; done | rdw pipe --id cpu &
while true ; do free -h ; sleep 2 ; done                | rdw pipe --id mem &
while true ; do df -h ; sleep 5 ; done                  | rdw pipe --id disk &
```

## Control sequences

```sh
# Set KV values inline from the stream
echo "=:build.status=passing;build.duration=12s" | rdw pipe --id ci

# Create a scrollback bookmark at a significant point
echo "bm:deploy-start" | rdw pipe --id deploy

# Switch the pane's formatter
echo "f:json" | rdw pipe --id api-log
curl -s https://api.example.com/status | rdw pipe --id api-log

# Apply a highlight profile
rdw kv set hl:errors '{"rules":[{"pattern":"ERROR","class":"hl-error"}]}'
echo "hl:errors" | rdw pipe --id log

# Clear scrollback, jump to top, or bottom
echo "sc:clear"  | rdw pipe --id log
echo "sc:top"    | rdw pipe --id log
echo "sc:bottom" | rdw pipe --id log

# Verbatim passthrough — not interpreted as control sequence
echo "v:=:this is not a KV write" | rdw pipe --id log
```

## Formatters

```sh
# JSON — syntax highlighted, pretty-printed
echo "f:json" | rdw pipe --id api
curl -s https://httpbin.org/get | rdw pipe --id api

# Markdown
echo "f:markdown" | rdw pipe --id docs
cat README.md | rdw pipe --id docs

# CSV / TSV — sortable table
echo "f:csv" | rdw pipe --id data
echo "name,age,city" | rdw pipe --id data
echo "alice,30,london" | rdw pipe --id data

# Image (base64-encoded PNG/JPEG/SVG)
echo "f:image" | rdw pipe --id chart
echo "b64:$(base64 -w0 chart.png)" | rdw pipe --id chart
```

## Key-value store

```sh
# Set and get
rdw kv set build.status passing
rdw kv get build.status

# Namespaced keys
rdw kv set window:build:title "CI Build"
rdw kv set pane:log:color green

# List all keys with a prefix
rdw kv list build.

# Persistence across restarts
rdw server start --kv-persist ~/.rdw/kv.db
rdw server start --kv-persist ~/.rdw/kv.db --restore
```

## Authentication and tokens

```sh
# Start with auth enabled (default)
rdw server start

# Create a token
TOKEN=$(rdw token create --expires 24h)

# Use it
rdw kv set foo bar  # uses token automatically from config
curl -H "Authorization: Bearer $TOKEN" http://localhost:7681/api/v1/ping

# Revoke
rdw token list
rdw token revoke <id>
```

## Stream mirroring

```sh
# Mirror to a file while streaming to rdw
long_build.sh | rdw pipe --id build --forward-to-file /tmp/build.log

# Mirror to a command
my_app | rdw pipe --id app --forward-to-cmd "grep -i error >> /tmp/errors.log"

# Also forward to bash-rd for backward compatibility
my_app | rdw pipe --id app --forward rd
```

## Export

```sh
# Export a single pane's scrollback
rdw save /tmp/export --pane log

# Export an entire window
rdw save /tmp/export --window build

# Export everything
rdw save /tmp/export
```

## Focus cycle (wall-screen rotation)

```sh
# Cycle through windows every 15 seconds
curl -X POST http://localhost:7681/api/v1/cycle/start \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"windows":["build","logs","metrics"],"interval_ms":15000}'

# Stop
curl -X POST http://localhost:7681/api/v1/cycle/stop \
  -H "Authorization: Bearer $TOKEN"
```

## Bookmarks

```sh
# Add a bookmark via API
curl -X PUT http://localhost:7681/api/v1/panes/log/bookmarks/deploy \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"line_index":250}'

# List bookmarks
curl http://localhost:7681/api/v1/panes/log/bookmarks \
  -H "Authorization: Bearer $TOKEN"

# Delete
curl -X DELETE http://localhost:7681/api/v1/panes/log/bookmarks/deploy \
  -H "Authorization: Bearer $TOKEN"
```

## Multiple servers

```sh
# Start two instances on different ports
rdw server start --port 7681 &
rdw server start --port 7682 &

# Address each with --port
echo "hello" | rdw pipe --port 7681 --id a
echo "world" | rdw pipe --port 7682 --id b

# List all running instances
rdw server list
```

## CI / headless use

```sh
# No browser, auth disabled, check selftest
rdw server start --no-auth &
SERVER_PID=$!

# Stream a build
make build 2>&1 | rdw pipe --id build

# Check exit
wait $!
kill $SERVER_PID
```

## Terminal pane

```sh
# Launch an interactive shell in a pane (requires ttyd or socat)
curl -X POST http://localhost:7681/api/v1/panes/shell/terminal \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"cmd":"/bin/bash"}'
# Returns: {"port":8682,"url":"http://127.0.0.1:8682"}
```

## REST API direct use

```sh
BASE="http://localhost:7681/api/v1"
AUTH="Authorization: Bearer $TOKEN"

# Session state
curl -H "$AUTH" $BASE/session

# Create a window
curl -X POST -H "$AUTH" $BASE/windows -d '{"name":"build"}'

# Split a pane
curl -X POST -H "$AUTH" $BASE/panes/log/split -d '{"dir":"h"}'

# Resize a pane
curl -X POST -H "$AUTH" $BASE/panes/log/resize -d '{"size":"40%"}'

# Zoom a pane
curl -X POST -H "$AUTH" $BASE/panes/log/zoom

# Swap two panes
curl -X POST -H "$AUTH" $BASE/panes/log/swap -d '{"target":"metrics"}'

# Apply a highlight profile
curl -X PUT -H "$AUTH" $BASE/highlights/errors \
  -d '{"rules":[{"pattern":"ERROR","class":"hl-error"},{"pattern":"WARN\\w+","class":"hl-warn"}]}'

# Format pane content
curl -X POST -H "$AUTH" $BASE/panes/log/format -d '{"formatter":"json"}'
```

## Pane titles

```sh
# Set on connect
make build 2>&1 | rdw pipe --id build --title "CI Build"

# Set inline from stream
echo "title:Deploy v2.3" | rdw pipe --id deploy

# Rename from CLI
rdw pane rename build "Build — main branch"

# Double-click the pane header in the browser to edit inline
```

## Images and SVG

```sh
# Send a PNG (rdw handles base64 internally)
{ echo "image:" ; cat screenshot.png ; echo "image:end" ; } | rdw pipe --id screen

# Send an SVG (fully interactive in browser, click/hover work)
{ echo "svg:" ; gnuplot -e "set terminal svg; plot sin(x)" ; echo "svg:end" ; } | rdw pipe --id plot

# Simplest: use rdw send (auto-detects type)
rdw send --id chart   chart.png
rdw send --id diagram flow.svg
rdw send --id data    results.csv

# Control scaling
echo "scale:fill"   | rdw pipe --id chart   # fill the pane
echo "scale:native" | rdw pipe --id chart   # intrinsic size, scroll
echo "scale:fit"    | rdw pipe --id chart   # width 100% (default)
```

## Filters with KV

```sh
# Set a KV value that filters can read
rdw kv set env production
rdw kv set prefix "[PROD]"

# The filter sees $env and $prefix as environment variables
my_app | rdw pipe --id log --filter 'while read l; do echo "$prefix $l"; done'

# Chain: first filter errors only, second annotates with env
my_app | rdw pipe --id log \
  --filter 'grep -E "ERROR|WARN"' \
  --filter 'while read l; do echo "[$env] $l"; done'
```

## User-defined formatters

```sh
# Register a formatter from a script
rdw formatter register colorlog ./formatters/colorlog.sh

# List all (built-in + user-defined)
rdw formatter list

# Apply it to a pane
echo "f:colorlog" | rdw pipe --id log

# Or via API
curl -X POST http://localhost:7681/api/v1/panes/log/format \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"formatter":"colorlog"}'

# Remove when done
rdw formatter unregister colorlog
```

Example formatter script (`formatters/colorlog.sh`):

```sh
#!/bin/sh
echo '<div class="log">'
while IFS= read -r line; do
  case "$line" in
    *ERROR*) echo "<span class='err'>$line</span><br>" ;;
    *WARN*)  echo "<span class='warn'>$line</span><br>" ;;
    *)       echo "<span class='info'>$line</span><br>" ;;
  esac
done
echo '</div>'
```

## Server introspection

```sh
# Full snapshot
rdw status

# Machine-readable
rdw status --json | jq .panes

# Per-pane detail
rdw status pane build-log

# Open the admin web UI (requires --admin-token on server start)
rdw server start --admin-token secret
open http://localhost:7681/admin?token=secret
```

## Focus cycle (wall-screen rotation)

```sh
# Cycle through three windows every 15 seconds
rdw cycle start build logs metrics --interval-ms 15000

# Check what's running
rdw cycle status

# Stop
rdw cycle stop
```
