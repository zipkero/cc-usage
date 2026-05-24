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

// task-003: 정상 release 흐름이 끝난 직후 <path>.lock 파일이 디스크에 남지 않음을 확인.
// Unix/Windows 양쪽에서 release closure가 동등하게 os.Remove(lockPath)를 수행해야 한다.
func TestWithCacheFileLockRemovesLockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	lockPath := path + ".lock"

	err := withCacheFileLock(path, func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("withCacheFileLock failed: %v", err)
	}

	if _, err := os.Stat(lockPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lock file still exists after release (err=%v), want fs.ErrNotExist", err)
	}
}

// task-003: cleanOldSessionStates가 stale session-state-*.json.lock도 정리하고,
// fresh lock과 다른 family(cache-*.json.lock)는 보존하는지 회귀 검증.
func TestCleanOldSessionStatesHandlesLocks(t *testing.T) {
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
	staleJSON := writeFixture("session-state-stale.json", now.Add(-(sessionStateTTL + time.Minute)))
	freshJSON := writeFixture("session-state-fresh.json", now)
	staleLock := writeFixture("session-state-stale.json.lock", now.Add(-(sessionStateTTL + time.Minute)))
	freshLock := writeFixture("session-state-fresh.json.lock", now)
	otherFamilyLock := writeFixture("cache-old.json.lock", now.Add(-2*time.Hour))

	cleanOldSessionStates()

	if _, err := os.Stat(staleJSON); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale session-state .json still exists (err=%v), want removed", err)
	}
	if _, err := os.Stat(staleLock); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale session-state .json.lock still exists (err=%v), want removed", err)
	}
	if _, err := os.Stat(freshJSON); err != nil {
		t.Fatalf("fresh session-state .json stat: %v, want present", err)
	}
	if _, err := os.Stat(freshLock); err != nil {
		t.Fatalf("fresh session-state .json.lock stat: %v, want present", err)
	}
	if _, err := os.Stat(otherFamilyLock); err != nil {
		t.Fatalf("cache-old.json.lock stat: %v, want present (different family)", err)
	}
}

// v0.3.4 회귀: workspaceRestoreTTL이 sessionStateTTL과 동등하게 유지되어야
// idle 후 빈 stdin이 와도 cwd를 복원할 수 있다. 짧게 되돌리면 status line이
// 30초 이상 idle 직후 사라지는 원래 증상이 재발한다.
func TestWorkspaceRestoreTTLAlignedWithSessionStateTTL(t *testing.T) {
	if workspaceRestoreTTL != sessionStateTTL {
		t.Fatalf("workspaceRestoreTTL = %v, want %v (sessionStateTTL)", workspaceRestoreTTL, sessionStateTTL)
	}
}

// v0.3.4 회귀: SIGKILL로 atomicWriteFile defer가 못 돈 경우 dot-prefix
// .session-state-*.json.tmp-* leftover가 누적된다. cleanOldSessionStates가
// 같은 TTL로 이걸 정리해야 한다.
func TestCleanOldSessionStatesHandlesTempLeftovers(t *testing.T) {
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
	staleTmp := writeFixture(".session-state-abc.json.tmp-12345", now.Add(-(sessionStateTTL + time.Minute)))
	freshTmp := writeFixture(".session-state-xyz.json.tmp-67890", now)

	cleanOldSessionStates()

	if _, err := os.Stat(staleTmp); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale .session-state .tmp leftover still exists (err=%v), want removed", err)
	}
	if _, err := os.Stat(freshTmp); err != nil {
		t.Fatalf("fresh .session-state .tmp leftover stat: %v, want present", err)
	}
}

// v0.3.4 회귀: 옛 포맷의 좀비 session-state.json(키 suffix 없음)이 남아 있으면
// TTL 초과 시 cleanOldSessionStates가 자동 정리해야 한다.
func TestCleanOldSessionStatesRemovesLegacyZombie(t *testing.T) {
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
	staleLegacy := writeFixture("session-state.json", now.Add(-(sessionStateTTL + time.Minute)))

	cleanOldSessionStates()

	if _, err := os.Stat(staleLegacy); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale legacy session-state.json still exists (err=%v), want removed", err)
	}
}

// 갓 만들어진 legacy 좀비는 TTL 안에선 보존되어야 한다(혹시 마이그레이션 중인
// 다른 프로세스가 쓸 가능성 차단). 다만 새 코드는 이 이름으로 쓰지 않으므로
// 실제로 fresh한 케이스는 거의 없음.
func TestCleanOldSessionStatesKeepsFreshLegacy(t *testing.T) {
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

	legacy := filepath.Join(cacheDir, "session-state.json")
	if err := os.WriteFile(legacy, []byte("{}"), 0644); err != nil {
		t.Fatalf("write legacy fixture: %v", err)
	}

	cleanOldSessionStates()

	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("fresh legacy session-state.json removed unexpectedly: %v", err)
	}
}

// v0.3.5: 빈 stdin 도착 시 sessionCacheKey()가 ""를 반환해 키-기반 조회가
// miss하더라도, 가장 최근의 non-expired session-state-*.json을 fallback으로
// 골라 복원할 수 있어야 한다.
func TestLoadMostRecentSessionStatePicksNewest(t *testing.T) {
	dir := t.TempDir()

	now := time.Now()
	writeState := func(name string, model string, mtime time.Time) {
		t.Helper()
		input := StdinInput{}
		input.Model.ID = model
		state := SessionState{
			CachedStdin: &input,
			WidgetCount: 2,
			SavedAt:     now.Unix(),
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}

	writeState("session-state-old.json", "old-model", now.Add(-3*time.Minute))
	writeState("session-state-middle.json", "middle-model", now.Add(-1*time.Minute))
	writeState("session-state-newest.json", "newest-model", now)

	got := loadMostRecentSessionStateFrom(dir)
	if got == nil {
		t.Fatal("loadMostRecentSessionStateFrom returned nil, want newest")
	}
	if got.CachedStdin == nil || got.CachedStdin.Model.ID != "newest-model" {
		t.Fatalf("loaded model.id = %q, want newest-model", got.CachedStdin.Model.ID)
	}
}

func TestLoadMostRecentSessionStateRespectsTTL(t *testing.T) {
	dir := t.TempDir()

	now := time.Now()
	staleSavedAt := now.Add(-(sessionStateTTL + time.Minute)).Unix()

	input := StdinInput{}
	input.Model.ID = "stale-model"
	state := SessionState{
		CachedStdin: &input,
		WidgetCount: 2,
		SavedAt:     staleSavedAt,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, "session-state-stale.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// ModTime은 최근으로 두되, SavedAt이 TTL 밖이면 loadSessionStateByPath의
	// 체크에 걸려야 한다 (TTL의 진실은 ModTime이 아닌 SavedAt).
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if got := loadMostRecentSessionStateFrom(dir); got != nil {
		t.Fatalf("loadMostRecentSessionStateFrom returned %#v, want nil (TTL expired)", got)
	}
}

func TestLoadMostRecentSessionStateIgnoresLockAndTmp(t *testing.T) {
	dir := t.TempDir()

	now := time.Now()

	// fresh valid session-state (older mtime)
	input := StdinInput{}
	input.Model.ID = "valid-model"
	state := SessionState{
		CachedStdin: &input,
		WidgetCount: 2,
		SavedAt:     now.Unix(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	statePath := filepath.Join(dir, "session-state-valid.json")
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if err := os.Chtimes(statePath, now.Add(-30*time.Second), now.Add(-30*time.Second)); err != nil {
		t.Fatalf("chtimes state: %v", err)
	}

	// .lock과 .tmp-*를 더 최신 mtime으로 둔다. 골라지면 안 됨.
	lockPath := filepath.Join(dir, "session-state-valid.json.lock")
	if err := os.WriteFile(lockPath, []byte(""), 0644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if err := os.Chtimes(lockPath, now, now); err != nil {
		t.Fatalf("chtimes lock: %v", err)
	}
	tmpPath := filepath.Join(dir, ".session-state-valid.json.tmp-99999")
	if err := os.WriteFile(tmpPath, []byte("garbage"), 0644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.Chtimes(tmpPath, now, now); err != nil {
		t.Fatalf("chtimes tmp: %v", err)
	}

	got := loadMostRecentSessionStateFrom(dir)
	if got == nil {
		t.Fatal("loadMostRecentSessionStateFrom returned nil, want valid state")
	}
	if got.CachedStdin == nil || got.CachedStdin.Model.ID != "valid-model" {
		t.Fatalf("loaded model.id = %q, want valid-model", got.CachedStdin.Model.ID)
	}
}

func TestLoadMostRecentSessionStateEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if got := loadMostRecentSessionStateFrom(dir); got != nil {
		t.Fatalf("empty dir returned %#v, want nil", got)
	}
}

func TestLoadMostRecentSessionStateMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if got := loadMostRecentSessionStateFrom(dir); got != nil {
		t.Fatalf("missing dir returned %#v, want nil", got)
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
