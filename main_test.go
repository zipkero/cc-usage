package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestShouldSuppressOutput(t *testing.T) {
	t.Run("empty identity suppresses output", func(t *testing.T) {
		if !shouldSuppressOutput(StdinInput{}) {
			t.Fatal("empty stdin should suppress output")
		}
	})

	t.Run("model identity renders", func(t *testing.T) {
		stdin := StdinInput{}
		stdin.Model.ID = "claude-sonnet-4"
		if shouldSuppressOutput(stdin) {
			t.Fatal("model identity should render")
		}
	})

	t.Run("workspace identity renders", func(t *testing.T) {
		stdin := StdinInput{}
		stdin.Workspace.CurrentDir = "/repo"
		if shouldSuppressOutput(stdin) {
			t.Fatal("workspace identity should render")
		}
	})

	t.Run("context identity renders", func(t *testing.T) {
		stdin := StdinInput{}
		stdin.ContextWindow.ContextWindowSize = 200000
		if shouldSuppressOutput(stdin) {
			t.Fatal("context identity should render")
		}
	})

	t.Run("rate limits alone do not bypass suppression", func(t *testing.T) {
		stdin := StdinInput{}
		if err := json.Unmarshal([]byte(`{
			"rate_limits": {
				"five_hour": { "used_percentage": 10, "resets_at": 0 }
			}
		}`), &stdin); err != nil {
			t.Fatalf("unmarshal stdin fixture: %v", err)
		}
		if !shouldSuppressOutput(stdin) {
			t.Fatal("rate limits without identity should suppress output")
		}
	})
}

func TestConfigHomeDir(t *testing.T) {
	t.Run("CLAUDE_CONFIG_DIR wins", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		if got := configHomeDir("/home/user"); got != dir {
			t.Fatalf("configHomeDir() = %q, want %q", got, dir)
		}
	})

	t.Run("blank CLAUDE_CONFIG_DIR falls back to home", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "   ")
		want := filepath.Join("/home/user", ".claude")
		if got := configHomeDir("/home/user"); got != want {
			t.Fatalf("configHomeDir() = %q, want %q", got, want)
		}
	})

	t.Run("defaultConfigPath uses CLAUDE_CONFIG_DIR", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		t.Setenv("HOME", t.TempDir())
		want := filepath.Join(dir, "cc-usage.json")
		if got := defaultConfigPath(); got != want {
			t.Fatalf("defaultConfigPath() = %q, want %q", got, want)
		}
	})

	t.Run("defaultConfigPath falls back to home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		t.Setenv("HOME", home)
		want := filepath.Join(home, ".claude", "cc-usage.json")
		if got := defaultConfigPath(); got != want {
			t.Fatalf("defaultConfigPath() = %q, want %q", got, want)
		}
	})
}
