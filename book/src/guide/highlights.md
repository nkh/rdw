# Highlight Profiles

Named regex profiles that colour-match text in the browser. Profiles are stored server-side and applied per-pane.

## Define a profile

```sh
curl -X PUT http://localhost:7681/api/v1/highlights/errors \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"rules":[
    {"pattern":"ERROR","class":"hl-error"},
    {"pattern":"WARN\\w+","class":"hl-warn"},
    {"pattern":"PASS","class":"hl-ok"}
  ]}'
```

## Apply to a pane

```sh
echo "hl:errors" | rdw pipe --id log
```

Or via API:

```sh
curl -X PUT http://localhost:7681/api/v1/highlights/errors \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"rules":[{"pattern":"ERROR","class":"hl-error"}]}'
```

## CSS classes

The `class` field in each rule is applied to matching text as a `<span>` element. Add your own CSS to the browser to style them:

```css
.hl-error { color: #ff4444; font-weight: bold; }
.hl-warn  { color: #ffaa00; }
.hl-ok    { color: #44ff44; }
```

## Manage profiles

```sh
# List
curl http://localhost:7681/api/v1/highlights -H "Authorization: Bearer $TOKEN"

# Delete
curl -X DELETE http://localhost:7681/api/v1/highlights/errors \
  -H "Authorization: Bearer $TOKEN"
```

## Pattern validation

All patterns are validated as Go regular expressions at registration time. Invalid patterns are rejected with a 400 response.
