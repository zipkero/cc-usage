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

// task-003: saveSessionState가 디스크 직렬화 직전에 Workspace.CurrentDir을
// normalizeCwd로 정규화해 저장하는지 라운드트립으로 확인. 또한 호출자의
// *StdinInput에는 정규화가 누설되지 않아야 한다(fallback 매칭 외 경로의
// in-memory 시각 영향 0).
func TestSaveSessionStateNormalizesWorkspaceCurrentDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	workspaceDir := t.TempDir()
	// 비정규 표기: trailing separator + `.` 세그먼트.
	rawCwd := filepath.Join(workspaceDir, ".") + string(filepath.Separator)
	wantCwd := normalizeCwd(rawCwd)
	if wantCwd == rawCwd {
		t.Fatalf("test setup: rawCwd %q is already normalized; expected normalizeCwd to change it", rawCwd)
	}

	input := StdinInput{SessionId: "task-003"}
	input.Workspace.CurrentDir = rawCwd

	saveSessionState("task-003", &SessionState{
		CachedStdin: &input,
		WidgetCount: 2,
	})

	// 호출자의 in-memory 값은 변경되지 않아야 한다 (사이드이펙트 없음).
	if input.Workspace.CurrentDir != rawCwd {
		t.Fatalf("caller-visible Workspace.CurrentDir mutated: got %q, want %q", input.Workspace.CurrentDir, rawCwd)
	}

	// 디스크에 직접 읽어 직렬화된 값 자체가 정규화됐는지 확인.
	path := sessionStatePath("task-003")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session state: %v", err)
	}
	var disk SessionState
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("unmarshal session state: %v", err)
	}
	if disk.CachedStdin == nil {
		t.Fatalf("disk CachedStdin is nil")
	}
	if disk.CachedStdin.Workspace.CurrentDir != wantCwd {
		t.Fatalf("disk Workspace.CurrentDir = %q, want %q (normalized)", disk.CachedStdin.Workspace.CurrentDir, wantCwd)
	}

	// loadSessionState 라운드트립에서도 동일하게 정규화 값이 보여야 한다.
	loaded := loadSessionState("task-003")
	if loaded == nil || loaded.CachedStdin == nil {
		t.Fatalf("loadSessionState returned nil or empty CachedStdin")
	}
	if loaded.CachedStdin.Workspace.CurrentDir != wantCwd {
		t.Fatalf("loaded Workspace.CurrentDir = %q, want %q (normalized)", loaded.CachedStdin.Workspace.CurrentDir, wantCwd)
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

// SPEC §5.11 / ANALYSIS §12 D5: workspaceRestoreTTL은 cwd 일치 가드(SPEC §5.11)
// 다음에 위치한 2차 안전 한계로 60s를 넘지 않는다. 가드가 정확성을 책임지므로
// 이 값은 stale cwd 노출 시간 창의 상한을 좁히는 역할만 한다. 값을 다시
// sessionStateTTL(300s)에 맞추면 v0.3.4에서 관찰된 stale 노출 회귀로 되돌아간다.
func TestWorkspaceRestoreTTLBoundedAt60s(t *testing.T) {
	const want = 60 * time.Second
	if workspaceRestoreTTL != want {
		t.Fatalf("workspaceRestoreTTL = %v, want %v", workspaceRestoreTTL, want)
	}
	if workspaceRestoreTTL >= sessionStateTTL {
		t.Fatalf("workspaceRestoreTTL = %v must be shorter than sessionStateTTL = %v", workspaceRestoreTTL, sessionStateTTL)
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

// task-001: normalizeCwd가 EvalSymlinks 가능 경로는 평가된 결과를, 실패 경로는
// filepath.Clean 결과를 반환하고 빈 입력은 빈 문자열을 반환하는지 검증.
func TestNormalizeCwd(t *testing.T) {
	t.Run("empty input returns empty string", func(t *testing.T) {
		if got := normalizeCwd(""); got != "" {
			t.Fatalf("normalizeCwd(\"\") = %q, want \"\"", got)
		}
	})

	t.Run("trailing slash is cleaned", func(t *testing.T) {
		dir := t.TempDir()
		raw := dir + string(filepath.Separator)
		want, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q) failed: %v", dir, err)
		}
		if got := normalizeCwd(raw); got != want {
			t.Fatalf("normalizeCwd(%q) = %q, want %q", raw, got, want)
		}
	})

	t.Run("dot segment is collapsed", func(t *testing.T) {
		dir := t.TempDir()
		raw := filepath.Join(dir, ".")
		want, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q) failed: %v", dir, err)
		}
		if got := normalizeCwd(raw); got != want {
			t.Fatalf("normalizeCwd(%q) = %q, want %q", raw, got, want)
		}
	})

	t.Run("symlink is resolved to target", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "real")
		if err := os.MkdirAll(target, 0755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		link := filepath.Join(base, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlink not supported on this platform: %v", err)
		}
		want, err := filepath.EvalSymlinks(link)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q) failed: %v", link, err)
		}
		got := normalizeCwd(link)
		if got != want {
			t.Fatalf("normalizeCwd(%q) = %q, want %q (resolved target)", link, got, want)
		}
	})

	t.Run("nonexistent path falls back to Clean", func(t *testing.T) {
		raw := filepath.Join(t.TempDir(), "does", "not", "exist", ".", "x")
		want := filepath.Clean(raw)
		if got := normalizeCwd(raw); got != want {
			t.Fatalf("normalizeCwd(%q) = %q, want %q (Clean fallback)", raw, got, want)
		}
	})
}

// task-002: detectCurrentCwd가 세 분기 — env hit, env miss + getwd hit,
// 둘 다 실패 — 에서 spec(SPEC §5.2/§5.3, ANALYSIS §12 D1)대로 동작하는지 검증.
// 패키지 변수 detectCwdEnv·detectCwdGetwd를 일시 swap하는 방식으로 격리.
func TestDetectCurrentCwd(t *testing.T) {
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	t.Run("env hit returns normalized env value", func(t *testing.T) {
		dir := t.TempDir()
		raw := dir + string(filepath.Separator)
		want, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q) failed: %v", dir, err)
		}
		detectCwdEnv = func(key string) string {
			if key != "CLAUDE_PROJECT_DIR" {
				t.Fatalf("unexpected env key: %q", key)
			}
			return raw
		}
		detectCwdGetwd = func() (string, error) {
			t.Fatalf("getwd must not be called when env hit")
			return "", nil
		}
		if got := detectCurrentCwd(); got != want {
			t.Fatalf("detectCurrentCwd() = %q, want %q (normalized env)", got, want)
		}
	})

	t.Run("env miss + getwd hit returns normalized getwd value", func(t *testing.T) {
		dir := t.TempDir()
		want, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q) failed: %v", dir, err)
		}
		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return dir, nil }
		if got := detectCurrentCwd(); got != want {
			t.Fatalf("detectCurrentCwd() = %q, want %q (normalized getwd)", got, want)
		}
	})

	t.Run("env miss + getwd error returns empty string", func(t *testing.T) {
		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return "", errors.New("getwd failed") }
		if got := detectCurrentCwd(); got != "" {
			t.Fatalf("detectCurrentCwd() = %q, want \"\"", got)
		}
	})
}

// task-004: loadByWorkspaceCwd가 정규화 정확 일치 + TTL + mtime newest 규칙을
// 따라 적중·미적중·만료·빈 입력 네 케이스에서 spec(SPEC §5.1/§5.2/§5.7,
// ANALYSIS §3.2/§4.2/§12 D2,D3)대로 동작하는지 검증한다. cross-workspace
// 노출 0회 보장이 본 테스트의 핵심 목적이다.
func TestLoadByWorkspaceCwd(t *testing.T) {
	dir := t.TempDir()

	cwdA := normalizeCwd(t.TempDir())
	cwdB := normalizeCwd(t.TempDir())
	cwdZ := normalizeCwd(t.TempDir())

	writeState := func(t *testing.T, name, cwd string, savedAt time.Time) string {
		t.Helper()
		var stdin StdinInput
		stdin.Workspace.CurrentDir = cwd
		state := SessionState{
			CachedStdin: &stdin,
			WidgetCount: 2,
			SavedAt:     savedAt.Unix(),
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal state: %v", err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	now := time.Unix(1_700_000_000, 0)
	pathA := writeState(t, "session-state-A.json", cwdA, now.Add(-30*time.Second))
	_ = writeState(t, "session-state-B.json", cwdB, now.Add(-10*time.Second))

	t.Run("matches cwd A and returns A state", func(t *testing.T) {
		got := loadByWorkspaceCwd(dir, cwdA, now)
		if got == nil {
			t.Fatalf("loadByWorkspaceCwd(cwdA) = nil, want A state")
		}
		if got.CachedStdin == nil {
			t.Fatalf("returned SessionState has nil CachedStdin")
		}
		if got.CachedStdin.Workspace.CurrentDir != cwdA {
			t.Fatalf("returned CurrentDir = %q, want %q (cwdA)", got.CachedStdin.Workspace.CurrentDir, cwdA)
		}
	})

	t.Run("returns nil for unknown cwd Z", func(t *testing.T) {
		if got := loadByWorkspaceCwd(dir, cwdZ, now); got != nil {
			t.Fatalf("loadByWorkspaceCwd(cwdZ) = %#v, want nil", got)
		}
	})

	t.Run("returns nil for empty cwd", func(t *testing.T) {
		if got := loadByWorkspaceCwd(dir, "", now); got != nil {
			t.Fatalf("loadByWorkspaceCwd(empty) = %#v, want nil", got)
		}
	})

	t.Run("returns nil when match is expired beyond sessionStateTTL", func(t *testing.T) {
		// pathA SavedAt이 6분 전이 되도록 파일을 다시 써넣어 만료 시나리오를 만든다.
		var stdin StdinInput
		stdin.Workspace.CurrentDir = cwdA
		expired := SessionState{
			CachedStdin: &stdin,
			WidgetCount: 2,
			SavedAt:     now.Add(-6 * time.Minute).Unix(),
		}
		data, err := json.Marshal(expired)
		if err != nil {
			t.Fatalf("marshal expired: %v", err)
		}
		if err := os.WriteFile(pathA, data, 0644); err != nil {
			t.Fatalf("rewrite pathA: %v", err)
		}
		if got := loadByWorkspaceCwd(dir, cwdA, now); got != nil {
			t.Fatalf("loadByWorkspaceCwd(expired) = %#v, want nil", got)
		}
	})
}

// task-004: 같은 cwd로 저장된 캐시가 둘 이상 있을 때 mtime newest를 선택한다.
// session 식별자가 달라 사실상 동일 워크스페이스에 대한 캐시가 둘 이상 남는
// 경우(예: 동일 cwd로 새 session 시작)의 자연스러운 해소 규칙이다.
func TestLoadByWorkspaceCwdPicksNewestModTime(t *testing.T) {
	dir := t.TempDir()
	cwd := normalizeCwd(t.TempDir())
	now := time.Unix(1_700_000_000, 0)

	write := func(name string, mtime time.Time) {
		t.Helper()
		var stdin StdinInput
		stdin.SessionId = name
		stdin.Workspace.CurrentDir = cwd
		state := SessionState{
			CachedStdin: &stdin,
			WidgetCount: 2,
			SavedAt:     now.Add(-30 * time.Second).Unix(),
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		path := filepath.Join(dir, "session-state-"+name+".json")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	write("older", now.Add(-time.Hour))
	write("newer", now.Add(-time.Minute))

	got := loadByWorkspaceCwd(dir, cwd, now)
	if got == nil || got.CachedStdin == nil {
		t.Fatalf("loadByWorkspaceCwd = nil, want newer state")
	}
	if got.CachedStdin.SessionId != "newer" {
		t.Fatalf("selected SessionId = %q, want %q (newest mtime)", got.CachedStdin.SessionId, "newer")
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
