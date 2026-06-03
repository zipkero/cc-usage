package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// pathDisplayMaxRunes is the soft length budget for the path token before
// segment-aware shrink kicks in (ANALYSIS §5.A). Kept widget-private — no
// config knob (SPEC §3).
const pathDisplayMaxRunes = 50

// detectCwdEnv and detectCwdGetwd are package-level indirection points so
// tests can swap the underlying lookups.
var (
	detectCwdEnv   = os.Getenv
	detectCwdGetwd = os.Getwd
)

// detectCurrentCwd returns a normalized cwd guess for projectInfo when
// workspace.current_dir is absent. CLAUDE_PROJECT_DIR wins over os.Getwd().
func detectCurrentCwd() string {
	cwd, _ := detectCurrentCwdWithSource()
	return cwd
}

func detectCurrentCwdWithSource() (cwd, source string) {
	if raw := detectCwdEnv("CLAUDE_PROJECT_DIR"); raw != "" {
		return normalizeCwd(raw), "env"
	}
	if raw, err := detectCwdGetwd(); err == nil && raw != "" {
		return normalizeCwd(raw), "getwd"
	}
	return "", ""
}

func normalizeCwd(raw string) string {
	if raw == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(raw); err == nil {
		return resolved
	}
	return filepath.Clean(raw)
}

// --- projectInfo widget ---

type projectInfoWidget struct{}

type projectInfoData struct {
	DisplayPath string
	Branch      string
	Ahead       int
	Behind      int
	Worktree    string
}

func (w projectInfoWidget) ID() string { return "projectInfo" }

func (w projectInfoWidget) GetData(ctx *Context) (any, error) {
	currentDir := ctx.Stdin.Workspace.CurrentDir
	if currentDir == "" {
		currentDir = detectCurrentCwd()
		if currentDir == "" {
			return nil, nil
		}
	}

	// home-tilde compression + segment-aware shrink. UserHomeDir failure
	// degrades to the absolute path (SPEC §5.2 fallback) — never to bare
	// base name. project_dir is intentionally ignored: the full path already
	// disambiguates same-base-name worktrees (SPEC §5.4).
	home, _ := os.UserHomeDir()
	d := &projectInfoData{
		DisplayPath: shrinkPath(compressHome(currentDir, home), pathDisplayMaxRunes),
	}

	// worktree
	if ctx.Stdin.Worktree != nil && ctx.Stdin.Worktree.Name != "" {
		d.Worktree = ctx.Stdin.Worktree.Name
	}

	// git branch + ahead/behind in a single call
	if _, err := exec.LookPath("git"); err != nil {
		return d, nil
	}

	gitCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// porcelain=v2 --branch emits `# branch.head <name>` and (when upstream exists)
	// `# branch.ab +<ahead> -<behind>` as the first lines. One fork instead of two.
	statusCmd := exec.CommandContext(gitCtx, "git", "status", "--porcelain=v2", "--branch")
	statusCmd.Dir = currentDir
	out, err := statusCmd.Output()
	if err != nil {
		return d, nil
	}

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "# branch.") {
			if len(line) > 0 && line[0] != '#' {
				break // header lines end; entry lines start
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			name := strings.TrimPrefix(line, "# branch.head ")
			if name != "(detached)" {
				d.Branch = name
			}
		case strings.HasPrefix(line, "# branch.ab "):
			ab := strings.TrimPrefix(line, "# branch.ab ")
			parts := strings.Fields(ab) // ["+<ahead>", "-<behind>"]
			if len(parts) == 2 {
				fmt.Sscanf(parts[0], "+%d", &d.Ahead)
				fmt.Sscanf(parts[1], "-%d", &d.Behind)
			}
		}
	}

	return d, nil
}

func (w projectInfoWidget) Render(data any, ctx *Context) string {
	d := data.(*projectInfoData)
	theme := getTheme(ctx.Config.Theme)

	var b strings.Builder

	// full display path (home-tilde compressed + length-normalized in GetData)
	b.WriteString(fmt.Sprintf("%s%s%s", theme.Folder, d.DisplayPath, RESET))

	// branch info
	if d.Branch != "" {
		b.WriteString(fmt.Sprintf(" %s(%s", theme.Branch, d.Branch))
		if d.Ahead > 0 {
			b.WriteString(fmt.Sprintf(" ↑%d", d.Ahead))
		}
		if d.Behind > 0 {
			b.WriteString(fmt.Sprintf(" ↓%d", d.Behind))
		}
		b.WriteString(fmt.Sprintf(")%s", RESET))
	}

	// worktree
	if d.Worktree != "" {
		b.WriteString(fmt.Sprintf(" %s[%s]%s", theme.Info, d.Worktree, RESET))
	}

	return b.String()
}

// compressHome substitutes the user's home directory prefix with "~". When
// home is empty (UserHomeDir failed) or current is outside home, the absolute
// path is returned unchanged (SPEC §5.1, §5.2; ANALYSIS §5.C — exact-match
// short-circuit + HasPrefix on home+sep, no filepath.Rel).
func compressHome(current, home string) string {
	if home == "" {
		return current
	}
	if current == home {
		return "~"
	}
	sep := string(filepath.Separator)
	if strings.HasPrefix(current, home+sep) {
		return "~" + sep + strings.TrimPrefix(current, home+sep)
	}
	return current
}

// shrinkPath enforces pathDisplayMaxRunes by collapsing leading segments into
// a single "…" while preserving the head marker ("~" or leading separator)
// and the base name (SPEC §5.3; ANALYSIS §5.A). If the minimal form
// "<head>/…/<base>" still exceeds max, or the path has no separable head,
// the base name is returned alone — never truncated mid-rune.
func shrinkPath(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	sep := string(filepath.Separator)
	var head string
	rest := s
	switch {
	case strings.HasPrefix(s, "~"+sep):
		head = "~"
		rest = strings.TrimPrefix(s, "~"+sep)
	case strings.HasPrefix(s, sep):
		head = ""
		rest = strings.TrimPrefix(s, sep)
	}
	// segments after the head; need at least head + 1 middle + base to shrink
	segs := strings.Split(rest, sep)
	if len(segs) < 2 {
		// only base (or head + base) — nothing to collapse, base goes as-is.
		return filepath.Base(s)
	}
	base := segs[len(segs)-1]
	var prefix string
	if head != "" {
		prefix = head + sep
	} else if strings.HasPrefix(s, sep) {
		prefix = sep
	}
	candidate := prefix + "…" + sep + base
	if utf8.RuneCountInString(candidate) <= max {
		return candidate
	}
	// even the minimal collapsed form busts the budget — base alone.
	return base
}

// --- registration ---

func init() {
	registerWidget(projectInfoWidget{})
}
