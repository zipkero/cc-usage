---
name: cc-usage-install
description: Install cc-usage status line into Claude Code settings
---

Install the cc-usage status line into Claude Code settings, pinning the path to a
**stable, version-less location** so the entry survives future plugin updates.

Behavior:
1. Detect OS (`darwin` / `linux` / `windows`).
2. Prefer project settings at `.claude/settings.json` if the user asks for project scope.
   Otherwise use user settings at `~/.claude/settings.json`.
3. Preserve every existing JSON key. Only add or update the `statusLine` field.
4. Resolve the binary path **from the marketplaces install location**, not the
   versioned cache:
   - `<SKILL_ROOT>` = the directory containing this SKILL.md.
   - `<PROJECT_ROOT>` = `<SKILL_ROOT>/../..`
   - **Sanity check**: if `<PROJECT_ROOT>` contains `/cache/` or a `/<x.y.z>/`
     version segment, abort and tell the user to invoke the skill from the
     `marketplaces/` install (the version-less path). Writing a cache/version
     path defeats the whole point — it breaks on the next `/plugin update`.
5. Pick the command path by OS:
   - **darwin, linux**: `<PROJECT_ROOT>/bin/run.sh` (OS-detect wrapper).
   - **windows**: `<PROJECT_ROOT>/bin/cc-usage-windows-amd64.exe` directly.
     Do NOT use `run.sh` on Windows by default — `run.sh` requires Git Bash /
     WSL and will not execute from cmd or PowerShell.
6. Normalize the path:
   - Convert to an absolute path.
   - **Use forward slashes only**, even on Windows
     (`C:/Users/.../bin/cc-usage-windows-amd64.exe`). Backslashes break the
     Claude Code settings parser.
7. If the existing `statusLine.command` already points to a `cache/<version>/`
   path (e.g. `~/.claude/plugins/cache/<marketplace>/<plugin>/0.2.0/bin/run.sh`),
   treat it as the prime case this skill exists to fix and rewrite it.
8. Set `statusLine.type` to `"command"` and `statusLine.command` to the resolved
   path. Do not modify any other field.
9. Show the exact diff before writing.

Expected output:
- Target settings file path (user vs project scope).
- The before/after `statusLine` block.
- A one-line note if a stale `cache/<version>/` path was replaced (so the user
  sees why their previous setup was fragile).
