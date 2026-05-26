---
name: cc-usage-install
description: Install cc-usage status line into Claude Code settings
---

Install the cc-usage status line into Claude Code settings, pinning the path to a
**stable, version-less location** so the entry survives future plugin updates.

Behavior:
1. Detect OS (`darwin` / `linux` / `windows`).
2. Locate the target settings file:
   - Search these candidates in order and pick the first one that already
     contains a `statusLine` key:
     1. `.claude/settings.local.json` (project, per-machine, git-ignored)
     2. `.claude/settings.json` (project, shared)
     3. `~/.claude/settings.local.json` (user, per-machine)
     4. `~/.claude/settings.json` (user, shared)
   - If `statusLine` is found in any of the above, **update in-place there**
     (do not move it to another file — preserve the user's chosen scope).
   - If none contain `statusLine`, default to `~/.claude/settings.local.json`
     (per-machine, no risk of leaking a host-specific path into shared config).
     Create the file with `{}` if it does not exist; create the parent
     directory if needed.
   - If the user explicitly asks for project scope, use
     `.claude/settings.local.json` instead and apply the same create-if-missing
     rule.
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
- Target settings file path, including which candidate it matched (e.g.
  "found existing statusLine in ~/.claude/settings.local.json — updating
  in-place") or that the file was newly created.
- The before/after `statusLine` block.
- A one-line note if a stale `cache/<version>/` path was replaced (so the user
  sees why their previous setup was fragile).
