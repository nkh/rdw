# User-Defined Formatters

A user formatter is an external shell command that receives pane scrollback on stdin and writes HTML to stdout. The output is embedded in a sandboxed `<iframe>` so scripts in the formatter output cannot reach the rdw SPA.

The current KV snapshot is injected into the subprocess environment as environment variables (original key names, read-only).

## Registration

**In config** (available on every server start):

```yaml
# ~/.config/rdw/config.yaml
formatters:
  - name: colorlog
    cmd: /usr/local/bin/colorlog.sh
  - name: myformat
    cmd: "python3 /usr/local/lib/rdw/myformat.py"
```

**At runtime:**

```sh
rdw formatter register colorlog '/usr/local/bin/colorlog.sh'
rdw formatter list
rdw formatter unregister colorlog
```

**Via the API:**

```sh
curl -X POST http://localhost:7681/api/v1/formatters \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"colorlog","cmd":"/usr/local/bin/colorlog.sh"}'

curl -X DELETE http://localhost:7681/api/v1/formatters/colorlog \
  -H "Authorization: Bearer $TOKEN"
```

## Using a user formatter

Once registered, use it exactly like a built-in:

```sh
echo "f:colorlog" | rdw pipe --id log
```

Or via API:

```sh
curl -X POST http://localhost:7681/api/v1/panes/log/format \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"formatter":"colorlog"}'
```

## Writing a formatter

The formatter receives lines on stdin, one per line. It must write complete valid HTML to stdout.

**Shell example:**

```sh
#!/bin/sh
echo '<div class="log">'
while IFS= read -r line; do
  case "$line" in
    *ERROR*) echo "<span style='color:red'>$line</span><br>" ;;
    *WARN*)  echo "<span style='color:orange'>$line</span><br>" ;;
    *)       echo "<span>$line</span><br>" ;;
  esac
done
echo '</div>'
```

**Python example:**

```python
#!/usr/bin/env python3
import sys, html
print('<table class="log-table">')
for line in sys.stdin:
    line = line.rstrip()
    cls = "err" if "ERROR" in line else "warn" if "WARN" in line else "info"
    print(f'<tr class="{cls}"><td>{html.escape(line)}</td></tr>')
print('</table>')
```

**Using KV in a formatter:**

```python
import os, sys, html
env_name = os.environ.get('env', 'unknown')
print(f'<h3>Environment: {html.escape(env_name)}</h3>')
print('<pre>')
for line in sys.stdin:
    print(html.escape(line), end='')
print('</pre>')
```

## Constraints

- Built-in names (text, json, yaml, markdown, csv, image) cannot be overridden
- Output is sandboxed — scripts in the output cannot access the parent SPA
- Formatters are stateless per invocation — they receive the full scrollback each time
