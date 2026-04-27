package main

import (
	"encoding/json"
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

func TestLoadSessionStateKeepsStrongIdentityAcrossIdle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)

	input := StdinInput{SessionId: "abc-123"}
	input.Model.ID = "claude-opus-4-6"
	saveSessionState("abc-123", &SessionState{
		CachedStdin: &input,
		WidgetCount: 2,
	})

	path := sessionStatePath("abc-123")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session state failed: %v", err)
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("json parse failed: %v", err)
	}
	state.SavedAt = time.Now().Add(-2 * time.Hour).Unix()
	data, err = json.Marshal(state)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write session state failed: %v", err)
	}

	if got := loadSessionState("abc-123"); got == nil {
		t.Fatal("strong identity cache expired during idle window")
	}
}

func TestLoadSessionStateExpiresCwdOnlyCacheQuickly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)

	input := StdinInput{}
	input.Workspace.CurrentDir = "C:/tmp/project"
	key := sessionCacheKey(input)
	saveSessionState(key, &SessionState{
		CachedStdin: &input,
		WidgetCount: 2,
	})

	path := sessionStatePath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session state failed: %v", err)
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("json parse failed: %v", err)
	}
	state.SavedAt = time.Now().Add(-10 * time.Minute).Unix()
	data, err = json.Marshal(state)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write session state failed: %v", err)
	}

	if got := loadSessionState(key); got != nil {
		t.Fatal("cwd-only cache survived past weak idle window")
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
