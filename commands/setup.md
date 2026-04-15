---
name: setup
description: Register cc-usage as your Claude Code status line
---

Find the cc-usage binary path and add the following to your Claude Code settings.json:

For default profile (~/.claude/):
```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/cc-usage"
  }
}
```

For custom profile (e.g., ~/.claude-triptopaz/):
```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/cc-usage --config ~/.claude-triptopaz/cc-usage.json"
  }
}
```

Replace `/path/to/cc-usage` with the actual binary path (e.g., the output of `which cc-usage` or the `dist/` directory).
