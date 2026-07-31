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

func gitBranch(dir string) string {
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}

	gitCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	statusCmd := exec.CommandContext(gitCtx, "git", "status", "--porcelain=v2", "--branch")
	statusCmd.Dir = dir
	out, err := statusCmd.Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "# branch.") {
			if len(line) > 0 && line[0] != '#' {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "# branch.head ") {
			name := strings.TrimPrefix(line, "# branch.head ")
			if name == "(detached)" {
				return ""
			}
			return name
		}
	}

	return ""
}

// worktreeName extracts the last path segment from raw, treating both '/'
// and '\' as separators regardless of build OS. workspace.git_worktree's
// documented contents don't specify whether the value is a path or a bare
// name (ANALYSIS §5 D4); either way this operation yields the display name,
// so the ambiguity doesn't need to be resolved. Trailing separators are
// stripped first so e.g. "/repo/wt/" still yields "wt". Empty input (the
// "absent" sentinel) returns "".
func worktreeName(raw string) string {
	trimmed := strings.TrimRight(raw, `/\`)
	if idx := strings.LastIndexAny(trimmed, `/\`); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

// appendWorktreeToken appends the shared bracketed worktree tag to b when
// worktree is non-empty. Both project widgets call this so the token's form
// stays identical across them (SPEC §5.4; ANALYSIS §5 D4).
func appendWorktreeToken(b *strings.Builder, worktree string, theme ThemeColors) {
	if worktree == "" {
		return
	}
	fmt.Fprintf(b, " %s[%s]%s", theme.Info, worktree, RESET)
}

// --- projectInfo widget ---

type projectInfoWidget struct{}

type projectInfoData struct {
	DisplayPath string
	Branch      string
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
		Branch:      gitBranch(currentDir),
		Worktree:    worktreeName(ctx.Stdin.Workspace.GitWorktree),
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
		b.WriteString(fmt.Sprintf(" %s(%s)%s", theme.Branch, d.Branch, RESET))
	}

	appendWorktreeToken(&b, d.Worktree, theme)

	return b.String()
}

// --- projectName widget ---

type projectNameWidget struct{}

type projectNameData struct {
	Name     string
	Branch   string
	Worktree string
}

func (w projectNameWidget) ID() string { return "projectName" }

func (w projectNameWidget) GetData(ctx *Context) (any, error) {
	currentDir := ctx.Stdin.Workspace.CurrentDir
	if currentDir == "" {
		currentDir = detectCurrentCwd()
		if currentDir == "" {
			return nil, nil
		}
	}

	return &projectNameData{
		Name:     filepath.Base(currentDir),
		Branch:   gitBranch(currentDir),
		Worktree: worktreeName(ctx.Stdin.Workspace.GitWorktree),
	}, nil
}

func (w projectNameWidget) Render(data any, ctx *Context) string {
	d := data.(*projectNameData)
	theme := getTheme(ctx.Config.Theme)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s%s%s", theme.Folder, d.Name, RESET))
	if d.Branch != "" {
		b.WriteString(fmt.Sprintf(" %s(%s)%s", theme.Branch, d.Branch, RESET))
	}
	appendWorktreeToken(&b, d.Worktree, theme)
	return b.String()
}

// --- repoInfo widget ---

type repoInfoWidget struct{}

func (w repoInfoWidget) ID() string { return "repoInfo" }

// GetData renders only when workspace.repo is present with both Owner and
// Name non-empty. Host is deliberately not part of the gate or the render —
// owner/name alone identifies the repo and costs less width (ANALYSIS §5 D5).
func (w repoInfoWidget) GetData(ctx *Context) (any, error) {
	repo := ctx.Stdin.Workspace.Repo
	if repo == nil || repo.Owner == "" || repo.Name == "" {
		return nil, nil
	}
	return repo.Owner + "/" + repo.Name, nil
}

func (w repoInfoWidget) Render(data any, ctx *Context) string {
	ownerName := data.(string)
	theme := getTheme(ctx.Config.Theme)
	return fmt.Sprintf("%s%s%s", theme.Secondary, ownerName, RESET)
}

// --- pullRequest widget ---

type pullRequestWidget struct{}

type pullRequestData struct {
	Number      int
	URL         string
	StateSymbol string
}

func (w pullRequestWidget) ID() string { return "pullRequest" }

// reviewStateSymbols maps pr.review_state's documented values to a single
// display-safe symbol. All four come from the Dingbats/General Punctuation
// blocks — East Asian Ambiguous width, same category as the model widget's
// Geometric Shapes symbols (◆ ◇ ○ ●), which this codebase already treats as
// 1 column wide. task-009's display-width table will need to cover this same
// category (ANALYSIS §5 D5, D8).
var reviewStateSymbols = map[string]string{
	"approved":          "✓",
	"changes_requested": "✗",
	"pending":           "…",
	"draft":             "✎",
}

// GetData renders only when pr.number is positive. A merged/closed PR (or
// the "no open PR" case documented for this field) means Claude Code omits
// pr entirely; a non-positive number is treated the same way as absence
// (ANALYSIS §5 D5).
func (w pullRequestWidget) GetData(ctx *Context) (any, error) {
	pr := ctx.Stdin.PR
	if pr == nil || pr.Number <= 0 {
		return nil, nil
	}

	// review_state can be absent independently of number/url. An unrecognized
	// value is logged and dropped rather than passed through — new enum
	// values must not leak an unmapped string onto stdout (SPEC §3).
	symbol := ""
	if pr.ReviewState != "" {
		if s, ok := reviewStateSymbols[pr.ReviewState]; ok {
			symbol = s
		} else {
			debugLog("pullRequest", "unknown review_state %q, omitting symbol", pr.ReviewState)
		}
	}

	return &pullRequestData{
		Number:      pr.Number,
		URL:         pr.URL,
		StateSymbol: symbol,
	}, nil
}

// Render wraps "#<number>" in an OSC 8 link when URL is present, then
// appends the review-state symbol (if any) after a space.
func (w pullRequestWidget) Render(data any, ctx *Context) string {
	d := data.(*pullRequestData)
	theme := getTheme(ctx.Config.Theme)

	text := fmt.Sprintf("#%d", d.Number)
	if d.URL != "" {
		text = osc8Link(d.URL, text)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s", theme.Secondary, text, RESET)
	if d.StateSymbol != "" {
		fmt.Fprintf(&b, " %s", d.StateSymbol)
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
	registerWidget(projectNameWidget{})
	registerWidget(repoInfoWidget{})
	registerWidget(pullRequestWidget{})
}
