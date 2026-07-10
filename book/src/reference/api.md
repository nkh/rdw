# REST API Reference

Base URL: `http://HOST:PORT/api/v1`

All authenticated endpoints require `Authorization: Bearer TOKEN`.
Rate limit: 10 req/min on unauthenticated endpoints.

## Health

| Method | Path | Auth | Response |
| --- | --- | --- | --- |
| GET | `/api/v1/ping` | none | 200 OK |
| GET | `/api/v1/session` | token | `{windows, active_window}` |
| GET | `/api/v1/status` | token | full snapshot |
| GET | `/api/v1/status/panes/{id}` | token | per-pane detail |
| GET | `/api/v1/admin/connections` | admin+token | `{connections:[...]}` |
| GET | `/api/v1/bindings` | none | `{actions:{key:action}}` |
| GET | `/api/v1/formatters` | none | `{formatters:[...]}` |

## WebSocket

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/api/v1/ws` | Upgrade; sub-protocol `rdw-v1`; `?token=` or header |

## Stream ingest

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| POST | `/api/v1/stream/{id}` | `{line}` | 204 |

## Windows

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| GET | `/api/v1/windows` | — | `{windows:[...]}` |
| POST | `/api/v1/windows` | `{name}` | 204 |
| DELETE | `/api/v1/windows/{name}` | — | 204 |
| PATCH | `/api/v1/windows/{name}` | `{name}` | 204 (rename) |
| POST | `/api/v1/windows/{name}/focus` | — | 204 |

## Panes

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| POST | `/api/v1/panes/{id}/split` | `{dir:"h"\|"v"}` | 204 |
| POST | `/api/v1/panes/{id}/zoom` | — | 204 |
| POST | `/api/v1/panes/{id}/resize` | `{size}` | 204 |
| POST | `/api/v1/panes/{id}/swap` | `{target}` | 204 |
| DELETE | `/api/v1/panes/{id}` | — | 204 |
| PATCH | `/api/v1/panes/{id}` | `{title}` | 204 (set title) |
| POST | `/api/v1/panes/{id}/format` | `{formatter}` | `{html}` |
| POST | `/api/v1/panes/{id}/filters` | `{cmd}` | 204 |
| POST | `/api/v1/panes/{id}/terminal` | `{cmd}` | `{port,url}` |
| DELETE | `/api/v1/panes/{id}/terminal` | — | 204 |

## Bookmarks

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| GET | `/api/v1/panes/{id}/bookmarks` | — | `{bookmarks:[...]}` |
| PUT | `/api/v1/panes/{id}/bookmarks/{name}` | `{line_index}` | 204 |
| DELETE | `/api/v1/panes/{id}/bookmarks/{name}` | — | 204 |

## KV store

| Method | Path | Query / Body | Response |
| --- | --- | --- | --- |
| GET | `/api/v1/kv` | `?prefix=PREFIX` | `{keys:[...]}` |
| GET | `/api/v1/kv/{key}` | — | `{key,value}` |
| PUT | `/api/v1/kv/{key}` | `{value}` | 204 |
| DELETE | `/api/v1/kv/{key}` | — | 204 |

## Formatters

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| GET | `/api/v1/formatters` | — | `{formatters:[...]}` |
| POST | `/api/v1/formatters` | `{name,cmd}` | 204 |
| DELETE | `/api/v1/formatters/{name}` | — | 204 |

## Highlights

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| GET | `/api/v1/highlights` | — | `{profiles:[...]}` |
| PUT | `/api/v1/highlights/{name}` | `{rules:[{pattern,class}]}` | 204 |
| DELETE | `/api/v1/highlights/{name}` | — | 204 |

## Layouts

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| GET | `/api/v1/layouts` | — | `{layouts:[...]}` |
| POST | `/api/v1/layouts` | `{name}` | 204 |
| POST | `/api/v1/layouts/{name}/apply` | — | 204 |

## Tokens

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| GET | `/api/v1/tokens` | — | `{tokens:[...]}` |
| POST | `/api/v1/tokens` | `{scope?,expires?}` | `{id,plain_text}` |
| DELETE | `/api/v1/tokens/{id}` | — | 204 |

## Groups

| Method | Path | Response |
| --- | --- | --- |
| POST | `/api/v1/groups/{name}/hide` | 204 |
| POST | `/api/v1/groups/{name}/show` | 204 |
| POST | `/api/v1/groups/{name}/focus` | 204 |
| POST | `/api/v1/groups/{name}/kill` | 204 |

## Export

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| POST | `/api/v1/export/pane` | `{id,out_dir}` | 204 |
| POST | `/api/v1/export/window` | `{name,out_dir}` | 204 |
| POST | `/api/v1/export/all` | `{out_dir}` | 204 |

## Cycle

| Method | Path | Body | Response |
| --- | --- | --- | --- |
| POST | `/api/v1/cycle/start` | `{windows:[...],interval_ms}` | `{windows,interval_ms}` |
| POST | `/api/v1/cycle/stop` | — | 204 |
| GET | `/api/v1/cycle/status` | — | `{running,windows,interval_ms}` |
