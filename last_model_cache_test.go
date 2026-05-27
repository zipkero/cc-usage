package main

import (
	"os"
	"path/filepath"
	"testing"
)

// isolateOneMCache redirects os.UserHomeDir to a fresh temp dir for the
// duration of the test, ensuring each test case starts with an absent
// one-m-by-cwd.json and does not pollute the real ~/.cache directory.
func isolateOneMCache(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	// os.UserHomeDir reads USERPROFILE on Windows, HOME on Unix.
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	return home
}

// TestLoadLastKnownOneMFileAbsent verifies that loadLastKnownOneM returns false
// when the cache file does not exist (first-run / cold cache scenario).
func TestLoadLastKnownOneMFileAbsent(t *testing.T) {
	isolateOneMCache(t)

	if loadLastKnownOneM("/home/user/project") {
		t.Fatal("want false for absent cache file, got true")
	}
}

// TestLoadLastKnownOneMEmptyCwd verifies that an empty cwd always returns false
// without touching the filesystem.
func TestLoadLastKnownOneMEmptyCwd(t *testing.T) {
	isolateOneMCache(t)

	if loadLastKnownOneM("") {
		t.Fatal("want false for empty cwd, got true")
	}
}

// TestSaveAndLoadLastKnownOneM is the basic round-trip: save true for a cwd,
// then load from the same cwd and expect true.
func TestSaveAndLoadLastKnownOneM(t *testing.T) {
	isolateOneMCache(t)

	cwd := "/home/user/myproject"
	saveLastKnownOneM(cwd, true)

	if !loadLastKnownOneM(cwd) {
		t.Fatal("want true after save, got false")
	}
}

// TestSaveLastKnownOneMFalse verifies that saving false for a cwd and then
// loading it returns false.
func TestSaveLastKnownOneMFalse(t *testing.T) {
	isolateOneMCache(t)

	cwd := "/home/user/myproject"
	saveLastKnownOneM(cwd, true)
	saveLastKnownOneM(cwd, false)

	if loadLastKnownOneM(cwd) {
		t.Fatal("want false after saving false, got true")
	}
}

// TestLastKnownOneMCrossCwdIsolation is the cross-cwd isolation check:
// saving true for cwd A must not be visible from cwd B (different hash).
func TestLastKnownOneMCrossCwdIsolation(t *testing.T) {
	isolateOneMCache(t)

	cwdA := "/home/user/project-a"
	cwdB := "/home/user/project-b"

	saveLastKnownOneM(cwdA, true)

	if loadLastKnownOneM(cwdB) {
		t.Fatal("cross-cwd leak: cwd B sees cwd A's true value")
	}
}

// TestLastKnownOneMPreservesOtherEntries verifies the read-merge-write
// invariant: writing for cwd B must not clobber cwd A's existing entry.
func TestLastKnownOneMPreservesOtherEntries(t *testing.T) {
	isolateOneMCache(t)

	cwdA := "/home/user/project-a"
	cwdB := "/home/user/project-b"

	saveLastKnownOneM(cwdA, true)
	saveLastKnownOneM(cwdB, true)

	// Both entries should still be present.
	if !loadLastKnownOneM(cwdA) {
		t.Error("cwd A entry lost after writing cwd B")
	}
	if !loadLastKnownOneM(cwdB) {
		t.Error("cwd B entry not found after save")
	}

	// Now update cwdB to false — cwdA must remain true.
	saveLastKnownOneM(cwdB, false)

	if !loadLastKnownOneM(cwdA) {
		t.Error("cwd A entry lost after updating cwd B")
	}
	if loadLastKnownOneM(cwdB) {
		t.Error("cwd B should be false after update, got true")
	}
}

// TestLastKnownOneMFileCreatedInCacheDir verifies that saveLastKnownOneM
// creates the file at the expected path inside ~/.cache/cc-usage/.
func TestLastKnownOneMFileCreatedInCacheDir(t *testing.T) {
	home := isolateOneMCache(t)

	saveLastKnownOneM("/some/project", true)

	expectedPath := filepath.Join(home, ".cache", "cc-usage", "one-m-by-cwd.json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected cache file at %s, got stat error: %v", expectedPath, err)
	}
}

// TestLoadLastKnownOneMCorruptFile verifies graceful false return when the
// cache file exists but contains invalid JSON.
func TestLoadLastKnownOneMCorruptFile(t *testing.T) {
	home := isolateOneMCache(t)

	cacheDir := filepath.Join(home, ".cache", "cc-usage")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cachePath := filepath.Join(cacheDir, "one-m-by-cwd.json")
	if err := os.WriteFile(cachePath, []byte("not valid json{{{"), 0644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	// Must return false without panicking.
	if loadLastKnownOneM("/any/cwd") {
		t.Fatal("want false for corrupt cache file, got true")
	}
}
