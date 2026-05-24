package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionCacheKeyFallbackOrder(t *testing.T) {
	input := StdinInput{SessionId: " session-1 "}
	if got := sessionCacheKey(input); got != "session-1" {
		t.Fatalf("session id key = %q, want session-1", got)
	}

	input = StdinInput{}
	input.Remote = &struct {
		SessionId string `json:"session_id"`
	}{SessionId: "remote-1"}
	if got := sessionCacheKey(input); got != "remote-remote-1" {
		t.Fatalf("remote key = %q, want remote-remote-1", got)
	}

	input = StdinInput{AgentId: "agent-1"}
	if got := sessionCacheKey(input); got != "agent-agent-1" {
		t.Fatalf("agent key = %q, want agent-agent-1", got)
	}

	input = StdinInput{TranscriptPath: "C:/tmp/session/transcript.jsonl"}
	wantTranscript := "transcript-" + hashCacheKey(input.TranscriptPath)
	if got := sessionCacheKey(input); got != wantTranscript {
		t.Fatalf("transcript key = %q, want %q", got, wantTranscript)
	}

	input = StdinInput{}
	input.Workspace.CurrentDir = "C:/tmp/project"
	wantCwd := "cwd-" + hashCacheKey(input.Workspace.CurrentDir)
	if got := sessionCacheKey(input); got != wantCwd {
		t.Fatalf("cwd key = %q, want %q", got, wantCwd)
	}
}

func TestSessionStatePathDoesNotUseLegacyFallback(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())

	if got := sessionStatePath(""); got != "" {
		t.Fatalf("empty cache key path = %q, want empty", got)
	}

	path := sessionStatePath("abc-123")
	if filepath.Base(path) != "session-state-abc-123.json" {
		t.Fatalf("session state path = %q", path)
	}
}

func TestSaveAndLoadSessionState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)

	input := StdinInput{SessionId: "abc-123"}
	input.Model.ID = "claude-opus-4-6"
	input.ContextWindow.ContextWindowSize = 200000

	saveSessionState("abc-123", &SessionState{
		CachedStdin: &input,
		WidgetCount: 2,
	})

	legacyPath := filepath.Join(home, ".cache", "cc-usage", "session-state.json")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy session-state.json exists or stat failed unexpectedly: %v", err)
	}

	state := loadSessionState("abc-123")
	if state == nil {
		t.Fatal("loadSessionState returned nil")
	}
	if state.CachedStdin == nil || state.CachedStdin.Model.ID != input.Model.ID {
		t.Fatalf("loaded cached stdin = %#v", state.CachedStdin)
	}
	if state.WidgetCount != 2 {
		t.Fatalf("widget count = %d, want 2", state.WidgetCount)
	}
}

func TestAtomicWriteFileReplacesValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	if err := atomicWriteFile(path, []byte(`{"version":1}`), 0644); err != nil {
		t.Fatalf("initial atomicWriteFile failed: %v", err)
	}
	if err := atomicWriteFile(path, []byte(`{"version":2}`), 0644); err != nil {
		t.Fatalf("replacement atomicWriteFile failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var obj struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("json parse failed: %v; data=%q", err, data)
	}
	if obj.Version != 2 {
		t.Fatalf("version = %d, want 2", obj.Version)
	}
}

func TestShouldRestoreCost(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	freshSavedAt := now.Add(-30 * time.Second).Unix()
	staleSavedAt := now.Add(-(sessionStateTTL + time.Second)).Unix()

	makeStdin := func(cost float64) StdinInput {
		var s StdinInput
		s.Cost.TotalCostUsd = cost
		return s
	}
	makeCached := func(cost float64, savedAt int64) *SessionState {
		cs := makeStdin(cost)
		return &SessionState{
			CachedStdin: &cs,
			WidgetCount: 2,
			SavedAt:     savedAt,
		}
	}

	t.Run("restores when stdin cost=0 and cache cost>0 fresh", func(t *testing.T) {
		got := shouldRestoreCost(makeStdin(0), makeCached(1.25, freshSavedAt), now)
		if !got {
			t.Fatalf("shouldRestoreCost = false, want true")
		}
	})

	t.Run("no restore when cached is nil", func(t *testing.T) {
		if shouldRestoreCost(makeStdin(0), nil, now) {
			t.Fatalf("shouldRestoreCost = true with nil cache, want false")
		}
	})

	t.Run("no restore when cached.CachedStdin is nil", func(t *testing.T) {
		state := &SessionState{CachedStdin: nil, WidgetCount: 2, SavedAt: freshSavedAt}
		if shouldRestoreCost(makeStdin(0), state, now) {
			t.Fatalf("shouldRestoreCost = true with nil CachedStdin, want false")
		}
	})

	t.Run("no restore when cache cost=0", func(t *testing.T) {
		if shouldRestoreCost(makeStdin(0), makeCached(0, freshSavedAt), now) {
			t.Fatalf("shouldRestoreCost = true with cache cost=0, want false")
		}
	})

	t.Run("no restore when SavedAt=0", func(t *testing.T) {
		if shouldRestoreCost(makeStdin(0), makeCached(1.25, 0), now) {
			t.Fatalf("shouldRestoreCost = true with SavedAt=0, want false")
		}
	})

	t.Run("no restore when SavedAt older than sessionStateTTL", func(t *testing.T) {
		if shouldRestoreCost(makeStdin(0), makeCached(1.25, staleSavedAt), now) {
			t.Fatalf("shouldRestoreCost = true with stale SavedAt, want false")
		}
	})

	t.Run("no restore when stdin cost>0", func(t *testing.T) {
		if shouldRestoreCost(makeStdin(0.5), makeCached(1.25, freshSavedAt), now) {
			t.Fatalf("shouldRestoreCost = true with stdin cost>0, want false")
		}
	})
}

func TestCleanOldSessionStates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	prev := lastSessionStateCleanup
	lastSessionStateCleanup = time.Time{}
	t.Cleanup(func() {
		lastSessionStateCleanup = prev
	})

	cacheDir := filepath.Join(home, ".cache", "cc-usage")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}

	writeFixture := func(name string, mtime time.Time) string {
		t.Helper()
		path := filepath.Join(cacheDir, name)
		if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
		return path
	}

	now := time.Now()
	stalePath := writeFixture("session-state-stale.json", now.Add(-(sessionStateTTL + time.Minute)))
	freshPath := writeFixture("session-state-fresh.json", now)
	cachePath := writeFixture("cache-old.json", now.Add(-2*time.Hour))

	cleanOldSessionStates()

	if _, err := os.Stat(stalePath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale session-state still exists (err=%v), want removed", err)
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatalf("fresh session-state stat: %v, want present", err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache-old.json stat: %v, want present (different prefix)", err)
	}
}

func TestCacheFileLockSerializesAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- withCacheFileLock(path, func() error {
			close(locked)
			<-release
			return nil
		})
	}()

	<-locked
	secondAcquired := make(chan error, 1)
	go func() {
		secondAcquired <- withCacheFileLock(path, func() error {
			return nil
		})
	}()

	select {
	case err := <-secondAcquired:
		t.Fatalf("second lock acquired before first released: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	if err := <-secondAcquired; err != nil {
		t.Fatalf("second lock failed: %v", err)
	}
}
