package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr replaces os.Stderr with a pipe for the duration of fn,
// then restores it and returns whatever fn wrote to stderr.
func captureStderr(fn func()) string {
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		// Restore and surface failure via empty string; tests will fail their assertions.
		return ""
	}
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	fn()

	_ = w.Close()
	os.Stderr = orig
	return <-done
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cc-usage.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadConfigMergesDefaults(t *testing.T) {
	path := writeTempConfig(t, `{"language":"en"}`)

	var cfg Config
	stderr := captureStderr(func() {
		cfg = loadConfig(path)
	})

	if stderr != "" {
		t.Fatalf("expected no stderr, got %q", stderr)
	}
	if cfg.Language != "en" {
		t.Errorf("Language = %q, want %q", cfg.Language, "en")
	}
	if cfg.DisplayMode != "compact" {
		t.Errorf("DisplayMode = %q, want %q (default merged)", cfg.DisplayMode, "compact")
	}
}

func TestLoadConfigAcceptsV020Fields(t *testing.T) {
	path := writeTempConfig(t, `{"dailyBudget":5.0,"plan":"max","cache":{"ttlSeconds":300},"language":"ko"}`)

	var cfg Config
	stderr := captureStderr(func() {
		cfg = loadConfig(path)
	})

	if cfg.Language != "ko" {
		t.Errorf("Language = %q, want %q", cfg.Language, "ko")
	}
	if stderr != "" {
		t.Errorf("expected no stderr for v0.2.0 fields, got %q", stderr)
	}
	for _, field := range []string{"dailyBudget", "plan", "cache", "ttlSeconds"} {
		if strings.Contains(stderr, field) {
			t.Errorf("stderr leaked ignored field %q: %q", field, stderr)
		}
	}
}

func TestLoadConfigWarnsOnUnknownEnum(t *testing.T) {
	path := writeTempConfig(t, `{"displayMode":"bogus"}`)

	var cfg Config
	stderr := captureStderr(func() {
		cfg = loadConfig(path)
	})

	for _, needle := range []string{"displayMode", "bogus", "compact"} {
		if !strings.Contains(stderr, needle) {
			t.Errorf("stderr missing %q; got %q", needle, stderr)
		}
	}
	if cfg.DisplayMode != "compact" {
		t.Errorf("DisplayMode = %q, want %q (fallback)", cfg.DisplayMode, "compact")
	}
}
