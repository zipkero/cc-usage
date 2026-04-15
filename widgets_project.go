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

	// git branch
	if _, err := exec.LookPath("git"); err != nil {
		return d, nil
	}

	gitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	branchCmd := exec.CommandContext(gitCtx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = currentDir
	if out, err := branchCmd.Output(); err == nil {
		d.Branch = strings.TrimSpace(string(out))
	}

	if d.Branch == "" {
		return d, nil
	}

	// ahead/behind
	abCtx, abCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer abCancel()

	abCmd := exec.CommandContext(abCtx, "git", "rev-list", "--count", "--left-right", "@{u}...HEAD")
	abCmd.Dir = currentDir
	if out, err := abCmd.Output(); err == nil {
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) == 2 {
			fmt.Sscanf(parts[0], "%d", &d.Behind)
			fmt.Sscanf(parts[1], "%d", &d.Ahead)
		}
	}

	return d, nil
}

func (w projectInfoWidget) Render(data any, ctx *Context) string {
	d := data.(*projectInfoData)
	theme := getTheme(ctx.Config.Theme)

	var b strings.Builder

	// dirname
	b.WriteString(fmt.Sprintf("%s%s%s", theme.Folder, d.DirName, RESET))

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
