package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// --- projectInfo widget ---

type projectInfoWidget struct{}

type projectInfoData struct {
	DirName  string
	Branch   string
	Ahead    int
	Behind   int
	Subpath  string
	Worktree string
}

func (w projectInfoWidget) ID() string { return "projectInfo" }

func (w projectInfoWidget) GetData(ctx *Context) (any, error) {
	currentDir := ctx.Stdin.Workspace.CurrentDir
	if currentDir == "" {
		return nil, nil
	}

	d := &projectInfoData{
		DirName: filepath.Base(currentDir),
	}

	// subpath: project_dir != current_dir
	projectDir := ctx.Stdin.Workspace.ProjectDir
	if projectDir != "" && projectDir != currentDir {
		rel, err := filepath.Rel(projectDir, currentDir)
		if err == nil && rel != "." {
			d.Subpath = rel
		}
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

	// dirname (truncate long names)
	dirName := truncate(d.DirName, 25)
	b.WriteString(fmt.Sprintf("%s%s%s", theme.Folder, dirName, RESET))

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

	// subpath
	if d.Subpath != "" {
		b.WriteString(fmt.Sprintf(" %s%s%s", theme.Dim, d.Subpath, RESET))
	}

	return b.String()
}

// --- registration ---

func init() {
	registerWidget(projectInfoWidget{})
}
