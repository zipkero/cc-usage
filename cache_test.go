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

// ── task-002: shouldRestoreFromSession + fillFromSessionCache ──────────────

// makeCachedForRestore builds a valid SessionState with the given cwd and
// SavedAt for use in shouldRestoreFromSession / fillFromSessionCache tests.
// detectCwdEnv / detectCwdGetwd must be patched by the caller so that
// shouldRestoreWorkspace returns true for cwd.
func makeCachedForRestore(cwd string, savedAt time.Time) *SessionState {
	var cs StdinInput
	cs.Workspace.CurrentDir = cwd
	cs.Model.ID = "claude-opus-4-6"
	cs.Model.DisplayName = "Opus"
	cs.Cost.TotalCostUsd = 1.25
	cs.ContextWindow.TotalInputTokens = 50000
	cs.ContextWindow.TotalOutputTokens = 10000
	return &SessionState{
		CachedStdin: &cs,
		WidgetCount: 4,
		SavedAt:     savedAt.Unix(),
	}
}

// patchCwdTo replaces detectCwdEnv/detectCwdGetwd so that detectCurrentCwd
// returns cwd, and restores originals via t.Cleanup.
func patchCwdTo(t *testing.T, cwd string) {
	t.Helper()
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})
	detectCwdEnv = func(string) string { return "" }
	detectCwdGetwd = func() (string, error) { return cwd, nil }
}

// TestShouldRestoreFromSession tests all four eligibility=false branches and
// the eligibility=true case.
func TestShouldRestoreFromSession(t *testing.T) {
	cwd := normalizeCwd(t.TempDir())
	now := time.Unix(1_700_000_000, 0)
	freshSavedAt := now.Add(-10 * time.Second)
	staleSavedAt := now.Add(-(workspaceRestoreTTL + time.Second))

	// Blank stdin that has all five backfill-eligible fields empty.
	emptyStdin := StdinInput{}

	t.Run("eligibility=false: cached==nil", func(t *testing.T) {
		patchCwdTo(t, cwd)
		if shouldRestoreFromSession(emptyStdin, nil, now) {
			t.Fatal("expected false with nil cached, got true")
		}
	})

	t.Run("eligibility=false: SavedAt==0", func(t *testing.T) {
		patchCwdTo(t, cwd)
		cached := makeCachedForRestore(cwd, freshSavedAt)
		cached.SavedAt = 0
		if shouldRestoreFromSession(emptyStdin, cached, now) {
			t.Fatal("expected false with SavedAt==0, got true")
		}
	})

	t.Run("eligibility=false: SavedAt expired", func(t *testing.T) {
		patchCwdTo(t, cwd)
		cached := makeCachedForRestore(cwd, staleSavedAt)
		if shouldRestoreFromSession(emptyStdin, cached, now) {
			t.Fatalf("expected false with stale SavedAt (%v >= workspaceRestoreTTL), got true",
				now.Sub(time.Unix(cached.SavedAt, 0)))
		}
	})

	t.Run("eligibility=false: cwd mismatch", func(t *testing.T) {
		otherCwd := normalizeCwd(t.TempDir())
		patchCwdTo(t, otherCwd) // current cwd != cached cwd
		cached := makeCachedForRestore(cwd, freshSavedAt)
		if shouldRestoreFromSession(emptyStdin, cached, now) {
			t.Fatal("expected false with cwd mismatch, got true")
		}
	})

	t.Run("eligibility=false: no empty fields", func(t *testing.T) {
		patchCwdTo(t, cwd)
		cached := makeCachedForRestore(cwd, freshSavedAt)
		// Build a fully-populated stdin so no field is empty.
		full := StdinInput{}
		full.Workspace.CurrentDir = cwd
		full.Model.ID = "claude-opus-4-6"
		full.Model.DisplayName = "Opus"
		full.Cost.TotalCostUsd = 2.00
		full.ContextWindow.TotalInputTokens = 1000
		// Worktree != nil
		full.Worktree = &struct {
			Name           string `json:"name"`
			Path           string `json:"path"`
			Branch         string `json:"branch"`
			OriginalCwd    string `json:"original_cwd"`
			OriginalBranch string `json:"original_branch"`
		}{Name: "wt"}
		if shouldRestoreFromSession(full, cached, now) {
			t.Fatal("expected false when stdin has no empty fields, got true")
		}
	})

	t.Run("eligibility=true: all empty fields and cwd match", func(t *testing.T) {
		patchCwdTo(t, cwd)
		cached := makeCachedForRestore(cwd, freshSavedAt)
		if !shouldRestoreFromSession(emptyStdin, cached, now) {
			t.Fatal("expected true with empty stdin and valid cache, got false")
		}
	})
}

// TestFillFromSessionCacheAllEmptyFields tests that when eligibility=true and
// stdin is entirely empty, all five fields are filled from the cache and
// RateLimits remains nil.
func TestFillFromSessionCacheAllEmptyFields(t *testing.T) {
	cwd := normalizeCwd(t.TempDir())
	patchCwdTo(t, cwd)

	now := time.Unix(1_700_000_000, 0)
	cached := makeCachedForRestore(cwd, now.Add(-10*time.Second))
	// Give cached a non-nil Worktree so the fill path exercises it.
	wt := struct {
		Name           string `json:"name"`
		Path           string `json:"path"`
		Branch         string `json:"branch"`
		OriginalCwd    string `json:"original_cwd"`
		OriginalBranch string `json:"original_branch"`
	}{Name: "my-worktree"}
	cached.CachedStdin.Worktree = &wt
	// Add RateLimits to cached to verify they are never copied.
	cached.CachedStdin.RateLimits = &struct {
		FiveHour *struct {
			UsedPercentage int   `json:"used_percentage"`
			ResetsAt       int64 `json:"resets_at"`
		} `json:"five_hour,omitempty"`
		SevenDay *struct {
			UsedPercentage int   `json:"used_percentage"`
			ResetsAt       int64 `json:"resets_at"`
		} `json:"seven_day,omitempty"`
	}{}

	stdin := StdinInput{} // all fields empty
	mask := fillFromSessionCache(&stdin, cached)

	if !mask.Workspace {
		t.Error("mask.Workspace should be true")
	}
	if !mask.Worktree {
		t.Error("mask.Worktree should be true")
	}
	if !mask.Model {
		t.Error("mask.Model should be true")
	}
	if !mask.Cost {
		t.Error("mask.Cost should be true")
	}
	if !mask.ContextWindow {
		t.Error("mask.ContextWindow should be true")
	}

	if stdin.Workspace.CurrentDir != cached.CachedStdin.Workspace.CurrentDir {
		t.Errorf("Workspace.CurrentDir = %q, want %q", stdin.Workspace.CurrentDir, cached.CachedStdin.Workspace.CurrentDir)
	}
	if stdin.Worktree == nil || stdin.Worktree.Name != "my-worktree" {
		t.Errorf("Worktree = %v, want non-nil with Name=my-worktree", stdin.Worktree)
	}
	if stdin.Model.ID != cached.CachedStdin.Model.ID {
		t.Errorf("Model.ID = %q, want %q", stdin.Model.ID, cached.CachedStdin.Model.ID)
	}
	if stdin.Cost.TotalCostUsd != cached.CachedStdin.Cost.TotalCostUsd {
		t.Errorf("Cost.TotalCostUsd = %v, want %v", stdin.Cost.TotalCostUsd, cached.CachedStdin.Cost.TotalCostUsd)
	}
	if stdin.ContextWindow.TotalInputTokens != cached.CachedStdin.ContextWindow.TotalInputTokens {
		t.Errorf("ContextWindow.TotalInputTokens = %d, want %d", stdin.ContextWindow.TotalInputTokens, cached.CachedStdin.ContextWindow.TotalInputTokens)
	}
	// RateLimits must remain nil regardless of what cached carries.
	if stdin.RateLimits != nil {
		t.Error("RateLimits must remain nil after fillFromSessionCache")
	}
}

// TestFillFromSessionCacheFreshFieldsNotOverwritten verifies that fields
// already carrying fresh values in stdin are not clobbered by cached values.
func TestFillFromSessionCacheFreshFieldsNotOverwritten(t *testing.T) {
	cwd := normalizeCwd(t.TempDir())
	patchCwdTo(t, cwd)

	now := time.Unix(1_700_000_000, 0)
	cached := makeCachedForRestore(cwd, now.Add(-10*time.Second))

	// stdin arrives with fresh model, workspace, and cost — only context is empty.
	stdin := StdinInput{}
	stdin.Workspace.CurrentDir = "/fresh/dir"
	stdin.Model.ID = "claude-fresh-model"
	stdin.Model.DisplayName = "Fresh"
	stdin.Cost.TotalCostUsd = 9.99
	stdin.ContextWindow.TotalInputTokens = 0
	stdin.ContextWindow.TotalOutputTokens = 0

	mask := fillFromSessionCache(&stdin, cached)

	// Fresh fields must be untouched.
	if stdin.Workspace.CurrentDir != "/fresh/dir" {
		t.Errorf("Workspace.CurrentDir overwritten: got %q, want /fresh/dir", stdin.Workspace.CurrentDir)
	}
	if stdin.Model.ID != "claude-fresh-model" {
		t.Errorf("Model.ID overwritten: got %q, want claude-fresh-model", stdin.Model.ID)
	}
	if stdin.Cost.TotalCostUsd != 9.99 {
		t.Errorf("Cost.TotalCostUsd overwritten: got %v, want 9.99", stdin.Cost.TotalCostUsd)
	}
	// Only ContextWindow (empty) should have been filled.
	if !mask.ContextWindow {
		t.Error("mask.ContextWindow should be true (was empty)")
	}
	if mask.Workspace || mask.Model || mask.Cost {
		t.Errorf("mask should only have ContextWindow set; got mask=%+v", mask)
	}
	if stdin.ContextWindow.TotalInputTokens != cached.CachedStdin.ContextWindow.TotalInputTokens {
		t.Errorf("ContextWindow not filled: got %d, want %d",
			stdin.ContextWindow.TotalInputTokens, cached.CachedStdin.ContextWindow.TotalInputTokens)
	}
}

// TestFillFromSessionCacheEligibilityFalseNoChange verifies that when
// eligibility is false (cached==nil), fillFromSessionCache leaves stdin
// unchanged and returns an all-false mask.
func TestFillFromSessionCacheEligibilityFalseNoChange(t *testing.T) {
	stdin := StdinInput{}
	mask := fillFromSessionCache(&stdin, nil)

	if mask.Workspace || mask.Worktree || mask.Model || mask.Cost || mask.ContextWindow {
		t.Errorf("expected all-false mask with nil cached, got %+v", mask)
	}
	if stdin.Workspace.CurrentDir != "" || stdin.Model.ID != "" || stdin.Cost.TotalCostUsd != 0 {
		t.Error("stdin was mutated despite nil cached")
	}
}

// ── task-004: stripRestoredFields ────────────────────────────────────────────

// TestStripRestoredFieldsAllTrue verifies that when every mask bit is true,
// all five fields are zeroed and unrelated fields (Version, SessionId, etc.)
// are left unchanged.
func TestStripRestoredFieldsAllTrue(t *testing.T) {
	snapshot := StdinInput{}
	snapshot.Workspace.CurrentDir = "/some/cwd"
	snapshot.Model.ID = "claude-opus-4-6"
	snapshot.Model.DisplayName = "Opus"
	snapshot.Cost.TotalCostUsd = 1.25
	snapshot.ContextWindow.TotalInputTokens = 50000
	snapshot.ContextWindow.TotalOutputTokens = 10000
	snapshot.ContextWindow.ContextWindowSize = 200000
	wt := &struct {
		Name           string `json:"name"`
		Path           string `json:"path"`
		Branch         string `json:"branch"`
		OriginalCwd    string `json:"original_cwd"`
		OriginalBranch string `json:"original_branch"`
	}{Name: "my-wt"}
	snapshot.Worktree = wt
	snapshot.Version = "1.2.3"   // unrelated — must survive
	snapshot.SessionId = "sid-1" // unrelated — must survive

	mask := restoredFieldMask{
		Workspace: true, Worktree: true, Model: true,
		Cost: true, ContextWindow: true,
	}
	stripRestoredFields(&snapshot, mask)

	if snapshot.Workspace.CurrentDir != "" {
		t.Errorf("Workspace.CurrentDir = %q, want empty after strip", snapshot.Workspace.CurrentDir)
	}
	if snapshot.Worktree != nil {
		t.Errorf("Worktree = %v, want nil after strip", snapshot.Worktree)
	}
	if snapshot.Model.ID != "" || snapshot.Model.DisplayName != "" {
		t.Errorf("Model = %+v, want zero after strip", snapshot.Model)
	}
	if snapshot.Cost.TotalCostUsd != 0 {
		t.Errorf("Cost.TotalCostUsd = %v, want 0 after strip", snapshot.Cost.TotalCostUsd)
	}
	if snapshot.ContextWindow.TotalInputTokens != 0 || snapshot.ContextWindow.ContextWindowSize != 0 {
		t.Errorf("ContextWindow = %+v, want zero after strip", snapshot.ContextWindow)
	}
	// Unrelated fields must be preserved.
	if snapshot.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3 (must not be stripped)", snapshot.Version)
	}
	if snapshot.SessionId != "sid-1" {
		t.Errorf("SessionId = %q, want sid-1 (must not be stripped)", snapshot.SessionId)
	}
}

// TestStripRestoredFieldsPartialMask verifies that only the fields whose mask
// bits are true are zeroed; fields with mask=false keep their values.
func TestStripRestoredFieldsPartialMask(t *testing.T) {
	snapshot := StdinInput{}
	snapshot.Workspace.CurrentDir = "/keep/me"
	snapshot.Model.ID = "keep-model"
	snapshot.Cost.TotalCostUsd = 9.99

	// Only Model is marked as cache-restored.
	mask := restoredFieldMask{Model: true}
	stripRestoredFields(&snapshot, mask)

	if snapshot.Workspace.CurrentDir != "/keep/me" {
		t.Errorf("Workspace.CurrentDir = %q, want /keep/me (fresh field must survive)", snapshot.Workspace.CurrentDir)
	}
	if snapshot.Cost.TotalCostUsd != 9.99 {
		t.Errorf("Cost.TotalCostUsd = %v, want 9.99 (fresh field must survive)", snapshot.Cost.TotalCostUsd)
	}
	if snapshot.Model.ID != "" || snapshot.Model.DisplayName != "" {
		t.Errorf("Model = %+v, want zero (cache-restored field must be stripped)", snapshot.Model)
	}
}

// TestStripRestoredFieldsAllFalse verifies that an all-false mask leaves the
// snapshot entirely unchanged (eligibility=false path).
func TestStripRestoredFieldsAllFalse(t *testing.T) {
	snapshot := StdinInput{}
	snapshot.Workspace.CurrentDir = "/unchanged"
	snapshot.Model.ID = "unchanged-model"
	snapshot.Cost.TotalCostUsd = 3.14

	mask := restoredFieldMask{} // all false
	stripRestoredFields(&snapshot, mask)

	if snapshot.Workspace.CurrentDir != "/unchanged" {
		t.Errorf("Workspace.CurrentDir changed: %q", snapshot.Workspace.CurrentDir)
	}
	if snapshot.Model.ID != "unchanged-model" {
		t.Errorf("Model.ID changed: %q", snapshot.Model.ID)
	}
	if snapshot.Cost.TotalCostUsd != 3.14 {
		t.Errorf("Cost.TotalCostUsd changed: %v", snapshot.Cost.TotalCostUsd)
	}
}

// TestFillFromSessionCacheMaskAccuracy verifies that the returned mask
// exactly reflects which fields were written — no more, no less.
func TestFillFromSessionCacheMaskAccuracy(t *testing.T) {
	cwd := normalizeCwd(t.TempDir())
	patchCwdTo(t, cwd)

	now := time.Unix(1_700_000_000, 0)
	cached := makeCachedForRestore(cwd, now.Add(-10*time.Second))

	// stdin is missing only Model and Cost; everything else is fresh.
	stdin := StdinInput{}
	stdin.Workspace.CurrentDir = cwd
	stdin.ContextWindow.TotalInputTokens = 1000
	// Worktree stays nil — but also cached has no Worktree set.
	// (cached.CachedStdin.Worktree is nil, so fill is a no-op.)

	mask := fillFromSessionCache(&stdin, cached)

	// Model and Cost were empty in stdin; cached has them → should be filled.
	if !mask.Model {
		t.Error("mask.Model should be true (stdin had empty model)")
	}
	if !mask.Cost {
		t.Error("mask.Cost should be true (stdin had zero cost)")
	}
	// Workspace was fresh → must not be filled.
	if mask.Workspace {
		t.Error("mask.Workspace should be false (stdin had fresh workspace)")
	}
	// ContextWindow was non-zero → must not be filled.
	if mask.ContextWindow {
		t.Error("mask.ContextWindow should be false (stdin had fresh context)")
	}
	// Worktree: stdin nil, cached nil → no fill.
	if mask.Worktree {
		t.Error("mask.Worktree should be false (both nil)")
	}
}
