package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectInfoFollowsCustomLinePosition(t *testing.T) {
	currentDir := t.TempDir()
	dirName := filepath.Base(currentDir)

	ctx := &Context{
		Config: Config{
			DisplayMode: "custom",
			Lines: [][]string{
				{"model", "projectInfo", "cost"},
			},
			Separator: "space",
		},
		Translations: loadTranslations("en"),
	}
	ctx.Stdin.Model.ID = "claude-opus-4-7"
	ctx.Stdin.Workspace.CurrentDir = currentDir
	ctx.Stdin.Workspace.ProjectDir = currentDir
	ctx.Stdin.Cost.TotalCostUsd = 1.25

	result := orchestrate(ctx)
	if result.WidgetCount != 3 {
		t.Fatalf("widget count = %d, want 3", result.WidgetCount)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(result.Lines))
	}

	line := result.Lines[0]
	modelAt := strings.Index(line, "claude-opus-4-7")
	projectAt := strings.Index(line, dirName)
	costAt := strings.Index(line, "$1.25")
	if modelAt < 0 || projectAt < 0 || costAt < 0 {
		t.Fatalf("line does not contain all widgets: %q", line)
	}
	if !(modelAt < projectAt && projectAt < costAt) {
		t.Fatalf("projectInfo did not follow custom order: %q", line)
	}
}

func TestProjectInfoUsesFullPathWhenLayoutHasMultipleConfiguredLines(t *testing.T) {
	currentDir := t.TempDir()
	dirName := filepath.Base(currentDir)

	ctx := &Context{
		Config: Config{
			DisplayMode: "custom",
			Lines: [][]string{
				{"model"},
				{"projectInfo"},
			},
			Separator: "space",
		},
		Translations: loadTranslations("en"),
	}
	ctx.Stdin.Model.ID = "claude-opus-4-7"
	ctx.Stdin.Workspace.CurrentDir = currentDir
	ctx.Stdin.Workspace.ProjectDir = currentDir

	result := orchestrate(ctx)
	if len(result.Lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(result.Lines))
	}
	if !strings.Contains(result.Lines[1], currentDir) {
		t.Fatalf("projectInfo line = %q, want full path %q", result.Lines[1], currentDir)
	}
	if result.Lines[1] == dirName {
		t.Fatalf("projectInfo line used only basename: %q", result.Lines[1])
	}
}
