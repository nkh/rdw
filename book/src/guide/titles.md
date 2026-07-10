# Pane Titles

Every pane has a display **title** shown in its header bar. The title is separate from the Target ID — it is a human-readable label for display only. The Target ID is used for stream routing and never changes.

## Setting a title

**On connect:**
```sh
make build 2>&1 | rdw pipe --id build --title "CI Build"
```

**Inline from the stream:**
```sh
echo "title:Deploy Pipeline" | rdw pipe --id deploy
```

**From the CLI:**
```sh
rdw pane rename build "Build — release branch"
```

**In the browser:** double-click the pane header to edit inline. Enter to confirm, Escape to cancel.

**Via the API:**
```sh
curl -X PATCH http://localhost:7681/api/v1/panes/build \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"title":"Build Log"}'
```

The `label` field is also accepted for backward compatibility.

## Title vs Target ID

The Target ID is shown as a tooltip on the header when a title is set. Use `rdw status pane ID` to see both.
