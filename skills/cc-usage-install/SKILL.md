---
name: cc-usage-install
description: Install cc-usage status line into Claude Code settings
---

Install the cc-usage status line into Claude Code settings.

Behavior:
1. Detect OS.
2. Prefer project settings at `.claude/settings.json` if the user asks for project scope.
3. Otherwise use user settings at `~/.claude/settings.json`.
4. Preserve existing JSON keys.
5. Add or update only the `statusLine` field.
6. Resolve the binary path from this skill's location:
   - `<SKILL_ROOT>` = the directory containing this SKILL.md.
   - `<PROJECT_ROOT>` = `<SKILL_ROOT>/../..`
   - Use `<PROJECT_ROOT>/bin/run.sh` as the command path.
   - On Windows, if the user prefers a direct binary path, use `<PROJECT_ROOT>/bin/cc-usage-windows-amd64.exe` instead.
   - Convert to absolute path with **forward slashes only** (even on Windows — backslashes break Claude Code settings).
7. Set `statusLine.type` to `"command"` and `statusLine.command` to the resolved path.
8. Show the exact diff before writing.
9. Do not modify any other fields.

Expected output:
- target settings file path
- final `statusLine` block
- whether this is user scope or project scope