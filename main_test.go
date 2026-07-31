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

// setHomeEnv overrides the test process's home directory for both POSIX and
// Windows. os.UserHomeDir() reads exactly one env var per GOOS (HOME on
// unix, USERPROFILE on windows) and never both, so setting only one leaves
// the other GOOS resolving to the real home. Setting both to the same value
// is safe on every GOOS since the two keys are never consulted together.
func setHomeEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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
		setHomeEnv(t, t.TempDir())
		want := filepath.Join(dir, "cc-usage.json")
		if got := defaultConfigPath(); got != want {
			t.Fatalf("defaultConfigPath() = %q, want %q", got, want)
		}
	})

	t.Run("defaultConfigPath falls back to home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		setHomeEnv(t, home)
		want := filepath.Join(home, ".claude", "cc-usage.json")
		if got := defaultConfigPath(); got != want {
			t.Fatalf("defaultConfigPath() = %q, want %q", got, want)
		}
	})
}

// TestParseColumns locks task-009's rule that unset, unparsable, and
// non-positive COLUMNS values all collapse to 0 ("no constraint") — SPEC
// §5.3 requires none of these to trigger width fitting.
func TestParseColumns(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"unset (empty string)", "", 0},
		{"non-numeric", "wide", 0},
		{"zero", "0", 0},
		{"negative", "-80", 0},
		{"positive", "80", 80},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseColumns(c.raw); got != c.want {
				t.Fatalf("parseColumns(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}
