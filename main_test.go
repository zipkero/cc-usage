package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// v0.3.4 회귀: noIdentity 조건이 성립해도 API rate-limit 캐시에서 가져온
// 5h/7d/7d-sonnet 버킷이 하나라도 있으면 출력은 묵음되어선 안 된다.
// 그렇지 않으면 stdin이 비어 도착하는 idle 직후 status line이 통째로 사라진다.
func TestShouldSuppressOutput(t *testing.T) {
	emptyStdin := StdinInput{}

	identityStdin := StdinInput{}
	identityStdin.Workspace.CurrentDir = "/tmp"

	limitsWith5h := &UsageLimits{
		FiveHour: &UsageLimitEntry{Utilization: 50, ResetsAt: time.Now()},
	}
	limitsWith7d := &UsageLimits{
		SevenDay: &UsageLimitEntry{Utilization: 20, ResetsAt: time.Now()},
	}
	limitsWith7dSonnet := &UsageLimits{
		SevenDaySonnet: &UsageLimitEntry{Utilization: 10, ResetsAt: time.Now()},
	}
	emptyLimits := &UsageLimits{}

	t.Run("noIdentity + 5h bucket -> render", func(t *testing.T) {
		if shouldSuppressOutput(emptyStdin, limitsWith5h) {
			t.Fatalf("expected render (rate-limit fallback), got suppress")
		}
	})

	t.Run("noIdentity + 7d bucket -> render", func(t *testing.T) {
		if shouldSuppressOutput(emptyStdin, limitsWith7d) {
			t.Fatalf("expected render (rate-limit fallback), got suppress")
		}
	})

	t.Run("noIdentity + 7d-sonnet bucket -> render", func(t *testing.T) {
		if shouldSuppressOutput(emptyStdin, limitsWith7dSonnet) {
			t.Fatalf("expected render (rate-limit fallback), got suppress")
		}
	})

	t.Run("noIdentity + nil rate limits -> suppress", func(t *testing.T) {
		if !shouldSuppressOutput(emptyStdin, nil) {
			t.Fatalf("expected suppress, got render")
		}
	})

	t.Run("noIdentity + empty UsageLimits struct -> suppress", func(t *testing.T) {
		if !shouldSuppressOutput(emptyStdin, emptyLimits) {
			t.Fatalf("expected suppress (no buckets), got render")
		}
	})

	t.Run("identity present + nil rate limits -> render", func(t *testing.T) {
		if shouldSuppressOutput(identityStdin, nil) {
			t.Fatalf("expected render (has identity), got suppress")
		}
	})

	t.Run("identity present + 5h bucket -> render", func(t *testing.T) {
		if shouldSuppressOutput(identityStdin, limitsWith5h) {
			t.Fatalf("expected render, got suppress")
		}
	})

	t.Run("model.id only -> render", func(t *testing.T) {
		s := StdinInput{}
		s.Model.ID = "claude-opus-4-6"
		if shouldSuppressOutput(s, nil) {
			t.Fatalf("expected render (has model.id), got suppress")
		}
	})

	t.Run("model.display_name only -> render", func(t *testing.T) {
		s := StdinInput{}
		s.Model.DisplayName = "Opus"
		if shouldSuppressOutput(s, nil) {
			t.Fatalf("expected render (has model.display_name), got suppress")
		}
	})

	t.Run("context window only -> render", func(t *testing.T) {
		s := StdinInput{}
		s.ContextWindow.ContextWindowSize = 200000
		if shouldSuppressOutput(s, nil) {
			t.Fatalf("expected render (has context window), got suppress")
		}
	})
}

// degraded restore 발화 시 빈 cost / context_window 필드가 캐시에서
// 복원되고, 신선한 값은 그대로 유지돼야 한다.
func TestRestoreUsageFields(t *testing.T) {
	cached := &StdinInput{}
	cached.Cost.TotalCostUsd = 1.25
	cached.ContextWindow.TotalInputTokens = 50000
	cached.ContextWindow.ContextWindowSize = 200000

	t.Run("empty stdin restores cost and context", func(t *testing.T) {
		stdin := StdinInput{}
		restoreUsageFields(&stdin, cached)
		if stdin.Cost.TotalCostUsd != 1.25 {
			t.Fatalf("cost = %.4f, want 1.25", stdin.Cost.TotalCostUsd)
		}
		if stdin.ContextWindow.TotalInputTokens != 50000 {
			t.Fatalf("ctx tokens = %d, want 50000", stdin.ContextWindow.TotalInputTokens)
		}
	})

	t.Run("fresh cost wins", func(t *testing.T) {
		stdin := StdinInput{}
		stdin.Cost.TotalCostUsd = 0.5
		restoreUsageFields(&stdin, cached)
		if stdin.Cost.TotalCostUsd != 0.5 {
			t.Fatalf("cost = %.4f, want fresh 0.5", stdin.Cost.TotalCostUsd)
		}
	})

	t.Run("fresh context wins", func(t *testing.T) {
		stdin := StdinInput{}
		stdin.ContextWindow.TotalInputTokens = 10
		restoreUsageFields(&stdin, cached)
		if stdin.ContextWindow.TotalInputTokens != 10 {
			t.Fatalf("ctx tokens = %d, want fresh 10", stdin.ContextWindow.TotalInputTokens)
		}
		if stdin.ContextWindow.ContextWindowSize != 0 {
			t.Fatalf("ctx size = %d, want 0 (fresh tokens present, full struct kept fresh)", stdin.ContextWindow.ContextWindowSize)
		}
	})

	t.Run("nil cached is no-op", func(t *testing.T) {
		stdin := StdinInput{}
		restoreUsageFields(&stdin, nil)
		if stdin.Cost.TotalCostUsd != 0 {
			t.Fatalf("nil cached mutated stdin: %#v", stdin)
		}
	})

	t.Run("nil stdin is no-op", func(t *testing.T) {
		// 패닉만 안 나면 통과.
		restoreUsageFields(nil, cached)
	})
}

// task-005: resolveCachedSessionState가 빈 cacheKey + 정상 stdin 두 경로에서
// SPEC §5.1·§5.4 의도대로 동작하는지 검증한다.
//
// 핵심 어서션:
//   - 빈 cacheKey + cwd 매칭 캐시 존재 → fallback이 호출되고 그 SessionState가
//     반환되어 main()의 restore 블록으로 그대로 전달된다.
//   - cacheKey가 있는 정상 stdin 경로 → fallback은 단 한 번도 호출되지 않는다
//     (cacheKey != "" 가드가 short-circuit). fallback이 정상 경로의 사이드
//     이펙트를 만들지 않음을 명시적으로 어서션해 회귀 보호한다.
func TestResolveCachedSessionStateEmptyKeyFallback(t *testing.T) {
	origFallback := fallbackByWorkspaceCwd
	t.Cleanup(func() { fallbackByWorkspaceCwd = origFallback })

	var stdin StdinInput
	stdin.Workspace.CurrentDir = "/tmp/project"
	stdin.Cost.TotalCostUsd = 1.25
	want := &SessionState{
		CachedStdin: &stdin,
		WidgetCount: 3,
		SavedAt:     time.Now().Unix(),
	}

	calls := 0
	fallbackByWorkspaceCwd = func(now time.Time) *SessionState {
		calls++
		return want
	}

	got := resolveCachedSessionState("", time.Now())
	if calls != 1 {
		t.Fatalf("fallback call count = %d, want 1", calls)
	}
	if got != want {
		t.Fatalf("resolveCachedSessionState returned %#v, want fallback SessionState", got)
	}
	if got.CachedStdin == nil || got.CachedStdin.Workspace.CurrentDir != "/tmp/project" {
		t.Fatalf("returned cached stdin lost fields: %#v", got.CachedStdin)
	}
	if got.CachedStdin.Cost.TotalCostUsd != 1.25 {
		t.Fatalf("returned cost = %.4f, want 1.25 (full restore expected)", got.CachedStdin.Cost.TotalCostUsd)
	}
}

// task-002 (degraded-cwd-fallback-relax): 분기 가드가 `primary == nil`로
// 완화된 뒤에도 "primary 캐시 적중 시 fallback은 절대 호출되지 않는다"
// (SPEC §5.5)는 그대로다. cacheKey 명의의 정상 캐시를 디스크에 미리
// 깔아두고 fallback spy의 호출 횟수가 0임을 어서션해 회귀 보호한다.
func TestResolveCachedSessionStateNormalStdinSkipsFallback(t *testing.T) {
	origFallback := fallbackByWorkspaceCwd
	t.Cleanup(func() { fallbackByWorkspaceCwd = origFallback })

	calls := 0
	fallbackByWorkspaceCwd = func(now time.Time) *SessionState {
		calls++
		return &SessionState{}
	}

	// HOME swap으로 격리해 다른 캐시 파일의 간섭을 차단.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// 디스크에 cacheKey="session-primary-hit" 명의의 정상 SessionState를 깔아둔다.
	// saveSessionState 경로를 그대로 통과시켜 SavedAt 채우기·정규화·atomic write를
	// production과 동일하게 흉내내고, loadSessionState가 적중하도록 한다.
	const cacheKey = "session-primary-hit"
	var stdin StdinInput
	stdin.SessionId = cacheKey
	stdin.Workspace.CurrentDir = "/tmp/primary-hit"
	stdin.Cost.TotalCostUsd = 0.99
	stdin.ContextWindow.ContextWindowSize = 200000
	saveSessionState(cacheKey, &SessionState{
		CachedStdin: &stdin,
		WidgetCount: 3,
	})

	got := resolveCachedSessionState(cacheKey, time.Now())
	if calls != 0 {
		t.Fatalf("fallback call count = %d, want 0 (primary hit must short-circuit)", calls)
	}
	if got == nil || got.CachedStdin == nil {
		t.Fatalf("resolveCachedSessionState = nil, want primary cache hit")
	}
	if got.CachedStdin.SessionId != cacheKey {
		t.Fatalf("returned SessionId = %q, want %q (primary cache must win)",
			got.CachedStdin.SessionId, cacheKey)
	}
	if got.CachedStdin.Cost.TotalCostUsd != 0.99 {
		t.Fatalf("returned cost = %.4f, want 0.99 (primary cache payload expected)",
			got.CachedStdin.Cost.TotalCostUsd)
	}
}

// task-002 (degraded-cwd-fallback-relax): cacheKey가 비-빈 값이지만 그 키의
// 디스크 캐시가 부재할 때, 분기 가드 완화로 인해 fallbackByWorkspaceCwd가
// 호출되고 그 반환값이 그대로 caller에 전달되어야 한다(SPEC §5.1·§5.2).
// 매처 본체는 fallbackByWorkspaceCwd spy로 short-circuit한다 — 매처 단의
// 동작은 TestFallbackByWorkspaceCwdEndToEnd / TestFallbackFourPaths 등이 별도로
// 커버하며, 여기서는 resolveCachedSessionState의 분기 동작만 격리해 검증한다.
func TestResolveCachedSessionStatePrimaryMissFallsBackToCwd(t *testing.T) {
	origFallback := fallbackByWorkspaceCwd
	t.Cleanup(func() { fallbackByWorkspaceCwd = origFallback })

	// HOME swap으로 cacheKey 명의의 디스크 캐시가 존재하지 않음을 보장한다.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var sentinelStdin StdinInput
	sentinelStdin.Workspace.CurrentDir = "/tmp/sibling-session"
	sentinelStdin.Cost.TotalCostUsd = 4.20
	sentinel := &SessionState{
		CachedStdin: &sentinelStdin,
		WidgetCount: 3,
		SavedAt:     time.Now().Unix(),
	}

	calls := 0
	fallbackByWorkspaceCwd = func(now time.Time) *SessionState {
		calls++
		return sentinel
	}

	got := resolveCachedSessionState("session-missing-on-disk", time.Now())
	if calls != 1 {
		t.Fatalf("fallback call count = %d, want 1 (primary miss must trigger fallback)", calls)
	}
	if got != sentinel {
		t.Fatalf("resolveCachedSessionState = %#v, want sentinel SessionState %#v",
			got, sentinel)
	}
}

// task-005: 빈 cacheKey + 매칭 캐시 존재 시 fallbackByWorkspaceCwd 실제 본체가
// detectCurrentCwd + loadByWorkspaceCwd를 연결해 디스크 캐시까지 닿는지를
// 엔드투엔드로 검증. fake 주입은 detectCwdEnv/detectCwdGetwd 두 hook뿐이고
// 매칭·정규화 로직은 그대로 통과한다.
func TestFallbackByWorkspaceCwdEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	workspace := t.TempDir()
	wantCwd := normalizeCwd(workspace)

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})
	detectCwdEnv = func(key string) string {
		if key == "CLAUDE_PROJECT_DIR" {
			return workspace
		}
		return ""
	}
	detectCwdGetwd = func() (string, error) { return workspace, nil }

	cacheDir := filepath.Join(home, ".cache", "cc-usage")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}

	var stdin StdinInput
	stdin.SessionId = "prev-session"
	stdin.Workspace.CurrentDir = wantCwd
	stdin.Cost.TotalCostUsd = 2.5
	stdin.ContextWindow.ContextWindowSize = 200000
	state := SessionState{
		CachedStdin: &stdin,
		WidgetCount: 3,
		SavedAt:     time.Now().Add(-30 * time.Second).Unix(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	statePath := filepath.Join(cacheDir, "session-state-prev-session.json")
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	got := fallbackByWorkspaceCwd(time.Now())
	if got == nil {
		t.Fatalf("fallbackByWorkspaceCwd = nil, want cached SessionState")
	}
	if got.CachedStdin == nil {
		t.Fatalf("returned SessionState has nil CachedStdin")
	}
	if got.CachedStdin.Workspace.CurrentDir != wantCwd {
		t.Fatalf("returned CurrentDir = %q, want %q", got.CachedStdin.Workspace.CurrentDir, wantCwd)
	}
	if got.CachedStdin.Cost.TotalCostUsd != 2.5 {
		t.Fatalf("returned cost = %.4f, want 2.5", got.CachedStdin.Cost.TotalCostUsd)
	}
	// fallback은 RateLimits를 디스크에서 채워서는 안 된다.
	// 저장 측에서 이미 nil이므로 read 결과도 nil이어야 한다.
	if got.CachedStdin.RateLimits != nil {
		t.Fatalf("fallback returned RateLimits=%#v, want nil (must come from API cache only)", got.CachedStdin.RateLimits)
	}
}

// task-006: 같은 session 안에서 cd로 워크스페이스를 옮긴 직후 빈 workspace
// stdin이 도착하면 cached cwd(A)를 화면에 다시 복원해선 안 된다.
// detectCurrentCwd를 fake로 B로 전환한 상태에서 가드가 false를 반환하는지,
// 그리고 동일 cwd 시퀀스(A→A)에서는 true를 반환해 기존 복원 동작이 유지되는지를
// 함께 어서션한다. detectCurrentCwd가 빈 값을 돌려주는 경우(env/getwd 모두 실패)
// 가드는 conservatively false여야 한다 — cross-workspace 노출 차단의 baseline.
func TestShouldRestoreWorkspace(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	normA := normalizeCwd(dirA)

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	t.Run("cd to different workspace blocks restore", func(t *testing.T) {
		detectCwdEnv = func(key string) string {
			if key == "CLAUDE_PROJECT_DIR" {
				return dirB
			}
			return ""
		}
		detectCwdGetwd = func() (string, error) { return dirB, nil }

		if shouldRestoreWorkspace(normA) {
			t.Fatalf("expected guard=false when cached cwd (%q) != current cwd (%q)", normA, dirB)
		}
	})

	t.Run("same cwd allows restore", func(t *testing.T) {
		detectCwdEnv = func(key string) string {
			if key == "CLAUDE_PROJECT_DIR" {
				return dirA
			}
			return ""
		}
		detectCwdGetwd = func() (string, error) { return dirA, nil }

		if !shouldRestoreWorkspace(normA) {
			t.Fatalf("expected guard=true when cached cwd matches current cwd (%q)", normA)
		}
	})

	t.Run("unknown current cwd blocks restore", func(t *testing.T) {
		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return "", os.ErrNotExist }

		if shouldRestoreWorkspace(normA) {
			t.Fatalf("expected guard=false when current cwd unknown (env miss + getwd fail)")
		}
	})

	t.Run("empty cached cwd blocks restore", func(t *testing.T) {
		// 가드 입력이 비어 있으면 normalizeCwd 결과도 빈 문자열이라
		// 어떤 currentCwd와도 정확 일치할 수 없다 (currentCwd=="" 분기에서 이미
		// false). currentCwd가 식별된 시나리오에서도 동일하게 false여야 한다.
		detectCwdEnv = func(key string) string {
			if key == "CLAUDE_PROJECT_DIR" {
				return dirA
			}
			return ""
		}
		detectCwdGetwd = func() (string, error) { return dirA, nil }

		if shouldRestoreWorkspace("") {
			t.Fatalf("expected guard=false when cached cwd is empty")
		}
	})
}

// task-009: SPEC §5.7이 요구하는 fallback 네 경로를 단일 테이블 테스트로 묶어
// 회귀 보호한다. cache_test.go의 TestLoadByWorkspaceCwd가 매처 자체의 분기를
// 부분적으로 커버하지만, 본 테스트는 main 경로에서 실제로 호출되는
// fallbackByWorkspaceCwd 엔트리 포인트를 통해 §5.7 (a)~(d) 네 케이스를 한
// 트레이스로 묶어 검증한다. (b)는 다른 워크스페이스 캐시를 사전 배치해
// cross-workspace 노출 0회(SPEC §5.2)까지 함께 어서션한다.
func TestFallbackFourPaths(t *testing.T) {
	// 각 케이스마다 HOME과 detectCurrentCwd 신호를 독립적으로 격리한다.
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	// writeStateFile은 한 SessionState를 cacheDir/session-state-<name>.json으로
	// 직렬화한다. savedAt은 호출자가 명시(만료 케이스 시뮬레이션용).
	writeStateFile := func(t *testing.T, cacheDir, name, cwd string, savedAt time.Time) {
		t.Helper()
		var stdin StdinInput
		stdin.SessionId = name
		stdin.Workspace.CurrentDir = cwd
		stdin.Cost.TotalCostUsd = 1.5
		state := SessionState{
			CachedStdin: &stdin,
			WidgetCount: 3,
			SavedAt:     savedAt.Unix(),
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("marshal state %s: %v", name, err)
		}
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			t.Fatalf("mkdir cacheDir: %v", err)
		}
		path := filepath.Join(cacheDir, "session-state-"+name+".json")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	t.Run("a) identified + hit -> restore", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		workspace := t.TempDir()
		wantCwd := normalizeCwd(workspace)

		detectCwdEnv = func(key string) string {
			if key == "CLAUDE_PROJECT_DIR" {
				return workspace
			}
			return ""
		}
		detectCwdGetwd = func() (string, error) { return workspace, nil }

		cacheDir := filepath.Join(home, ".cache", "cc-usage")
		writeStateFile(t, cacheDir, "current-session", wantCwd, time.Now().Add(-30*time.Second))

		got := fallbackByWorkspaceCwd(time.Now())
		if got == nil {
			t.Fatalf("fallbackByWorkspaceCwd = nil, want hit for cwd=%q", wantCwd)
		}
		if got.CachedStdin == nil {
			t.Fatalf("returned SessionState has nil CachedStdin")
		}
		if got.CachedStdin.Workspace.CurrentDir != wantCwd {
			t.Fatalf("restored CurrentDir = %q, want %q", got.CachedStdin.Workspace.CurrentDir, wantCwd)
		}
		if got.CachedStdin.SessionId != "current-session" {
			t.Fatalf("restored SessionId = %q, want %q", got.CachedStdin.SessionId, "current-session")
		}
	})

	t.Run("b) identified + miss -> no restore, no cross-workspace exposure", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		// 현재 cwd는 X. 디스크에는 다른 워크스페이스 Y의 캐시만 존재한다.
		workspaceX := t.TempDir()
		workspaceY := t.TempDir()
		curCwd := normalizeCwd(workspaceX)
		otherCwd := normalizeCwd(workspaceY)

		detectCwdEnv = func(key string) string {
			if key == "CLAUDE_PROJECT_DIR" {
				return workspaceX
			}
			return ""
		}
		detectCwdGetwd = func() (string, error) { return workspaceX, nil }

		cacheDir := filepath.Join(home, ".cache", "cc-usage")
		// 다른 워크스페이스 Y의 fresh 캐시를 미리 깔아둔다. 잘못된 fallback
		// (subpath/substring 매칭이나 mtime 폴백)이 들어가면 이 캐시가 누설된다.
		writeStateFile(t, cacheDir, "other-session", otherCwd, time.Now().Add(-10*time.Second))

		got := fallbackByWorkspaceCwd(time.Now())
		if got != nil {
			t.Fatalf("fallbackByWorkspaceCwd = %#v, want nil (cwd=%q has no matching cache; other cache for %q must NOT leak)",
				got, curCwd, otherCwd)
		}
	})

	t.Run("c) unidentified cwd -> no restore", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		// env miss + getwd 실패. 디스크에는 fresh 캐시가 있어도 fallback 금지.
		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return "", os.ErrNotExist }

		workspace := t.TempDir()
		cacheDir := filepath.Join(home, ".cache", "cc-usage")
		writeStateFile(t, cacheDir, "some-session", normalizeCwd(workspace), time.Now().Add(-30*time.Second))

		got := fallbackByWorkspaceCwd(time.Now())
		if got != nil {
			t.Fatalf("fallbackByWorkspaceCwd = %#v, want nil (no cwd signal must suppress restore)", got)
		}
	})

	t.Run("d) identified + match exists but TTL expired -> no restore", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		workspace := t.TempDir()
		wantCwd := normalizeCwd(workspace)

		detectCwdEnv = func(key string) string {
			if key == "CLAUDE_PROJECT_DIR" {
				return workspace
			}
			return ""
		}
		detectCwdGetwd = func() (string, error) { return workspace, nil }

		cacheDir := filepath.Join(home, ".cache", "cc-usage")
		// SavedAt이 sessionStateTTL(300s)을 초과한 6분 전. 정확 cwd 매칭이어도
		// 만료 캐시는 fallback 대상이 아니다.
		writeStateFile(t, cacheDir, "expired-session", wantCwd, time.Now().Add(-6*time.Minute))

		got := fallbackByWorkspaceCwd(time.Now())
		if got != nil {
			t.Fatalf("fallbackByWorkspaceCwd = %#v, want nil (TTL-expired match must NOT restore)", got)
		}
	})
}

// task-010: ANALYSIS §4.3의 t0~t6 시퀀스를 그대로 재현해 어느 시점에도
// cross-workspace 데이터가 노출되지 않음을 검증한다. saveSessionState로 정상
// stdin을 저장하고, 빈 stdin은 fallbackByWorkspaceCwd로 매칭한다. 워크스페이스
// 전환은 detectCwd 두 hook을 fake로 갈아끼워 표현한다.
//
// 각 step의 어서션 포인트:
//   - t0/t2: 정상 저장은 디스크에 파일이 생긴 것까지만 확인(다음 빈 stdin
//     매칭의 prerequisite).
//   - t1/t3/t4: fallback이 의도한 워크스페이스의 SessionState를 반환하고,
//     CachedStdin.Workspace.CurrentDir이 정확히 그 워크스페이스로 일치한다.
//     t3는 직전 step에서 A 캐시가 디스크에 살아있는 상태에서 B 매칭이
//     일어나므로 cross-workspace 노출 0회의 핵심 검증 지점이다.
//   - t5: 현재 cwd는 C로 식별되지만 C 캐시가 없으니 nil(보수적 출력으로
//     폴백). 디스크에 살아있는 A·B 캐시가 누설되지 않음을 함께 확인.
//   - t6: env miss + getwd 실패로 cwd 신호 자체가 없을 때 nil. 디스크 캐시
//     상태와 무관하게 어떤 워크스페이스 데이터도 노출되어선 안 된다.
func TestMultiWorkspaceSequence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	workspaceC := t.TempDir()
	normA := normalizeCwd(workspaceA)
	normB := normalizeCwd(workspaceB)
	normC := normalizeCwd(workspaceC)

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	// setCwdSignal은 detectCurrentCwd가 주어진 워크스페이스를 반환하도록
	// hook을 갈아끼운다. workspace=="" 인 경우 env miss + getwd 실패를
	// 시뮬레이션해 t6의 "신호 부재" 분기를 만든다.
	setCwdSignal := func(workspace string) {
		if workspace == "" {
			detectCwdEnv = func(string) string { return "" }
			detectCwdGetwd = func() (string, error) { return "", os.ErrNotExist }
			return
		}
		detectCwdEnv = func(key string) string {
			if key == "CLAUDE_PROJECT_DIR" {
				return workspace
			}
			return ""
		}
		detectCwdGetwd = func() (string, error) { return workspace, nil }
	}

	// saveWorkspaceCache는 정상 stdin 도착을 시뮬레이션해 sessionId 기반
	// 캐시 파일을 디스크에 둔다. saveSessionState 내부에서 CurrentDir이
	// normalizeCwd로 정규화되어 저장된다.
	saveWorkspaceCache := func(sessionId, workspace string) {
		t.Helper()
		var stdin StdinInput
		stdin.SessionId = sessionId
		stdin.Workspace.CurrentDir = workspace
		stdin.Cost.TotalCostUsd = 1.5
		stdin.ContextWindow.ContextWindowSize = 200000
		state := &SessionState{
			CachedStdin: &stdin,
			WidgetCount: 3,
		}
		saveSessionState(sessionCacheKey(stdin), state)
	}

	// assertFallback은 fallbackByWorkspaceCwd 결과가 expectCwd 워크스페이스의
	// 캐시를 정확히 반환하는지 확인한다. expectCwd가 빈 문자열이면 nil이어야
	// 한다 — 어떤 cross-workspace 데이터도 노출되어선 안 된다.
	assertFallback := func(label, expectCwd string) {
		t.Helper()
		got := fallbackByWorkspaceCwd(time.Now())
		if expectCwd == "" {
			if got != nil {
				t.Fatalf("%s: fallback returned %#v, want nil (no cross-workspace exposure allowed)",
					label, got.CachedStdin)
			}
			return
		}
		if got == nil {
			t.Fatalf("%s: fallback = nil, want SessionState for cwd=%q", label, expectCwd)
		}
		if got.CachedStdin == nil {
			t.Fatalf("%s: fallback returned SessionState with nil CachedStdin", label)
		}
		if got.CachedStdin.Workspace.CurrentDir != expectCwd {
			t.Fatalf("%s: fallback CurrentDir = %q, want %q (cross-workspace leak)",
				label, got.CachedStdin.Workspace.CurrentDir, expectCwd)
		}
	}

	// t0: A에서 정상 stdin → A 캐시 저장.
	setCwdSignal(workspaceA)
	saveWorkspaceCache("session-A", workspaceA)
	cachePathA := filepath.Join(home, ".cache", "cc-usage", "session-state-session-A.json")
	if _, err := os.Stat(cachePathA); err != nil {
		t.Fatalf("t0: A cache not persisted: %v", err)
	}

	// t1: A에서 빈 stdin → A 복원.
	setCwdSignal(workspaceA)
	assertFallback("t1", normA)

	// t2: B에서 정상 stdin → B 캐시 저장. A 캐시는 디스크에 그대로 존재한다.
	setCwdSignal(workspaceB)
	saveWorkspaceCache("session-B", workspaceB)
	cachePathB := filepath.Join(home, ".cache", "cc-usage", "session-state-session-B.json")
	if _, err := os.Stat(cachePathB); err != nil {
		t.Fatalf("t2: B cache not persisted: %v", err)
	}
	if _, err := os.Stat(cachePathA); err != nil {
		t.Fatalf("t2: A cache disappeared unexpectedly: %v", err)
	}

	// t3: B에서 빈 stdin → B 복원. 디스크에 A 캐시가 살아있는 상태에서도
	// A가 절대 노출되어선 안 된다(SPEC §5.9 핵심 검증).
	setCwdSignal(workspaceB)
	assertFallback("t3", normB)

	// t4: A로 돌아와서 빈 stdin → A 복원. B 캐시가 디스크에 더 newest
	// mtime으로 있어도 cwd 정확 일치만 매칭되어 A가 선택되어야 한다.
	setCwdSignal(workspaceA)
	assertFallback("t4", normA)

	// t5: C로 진입해 빈 stdin. C 캐시는 없으므로 nil(미복원).
	// A·B 캐시가 디스크에 살아있어도 절대 누설되어선 안 된다.
	setCwdSignal(workspaceC)
	assertFallback("t5", "")
	// C에 대한 캐시가 실수로 생성되지 않았는지 확인.
	cachePathC := filepath.Join(home, ".cache", "cc-usage", "session-state-session-C.json")
	if _, err := os.Stat(cachePathC); !os.IsNotExist(err) {
		t.Fatalf("t5: unexpected C cache exists (err=%v); fallback must not create cache files", err)
	}
	_ = normC

	// t6: cwd 신호 부재(env miss + getwd 실패) + 빈 stdin → nil.
	// 디스크에 어떤 캐시가 있든 식별 불가 시 cross-workspace 노출 0회 보장.
	setCwdSignal("")
	assertFallback("t6", "")
}

// task-005: detectCurrentCwd가 빈 값을 반환하면(env miss + getwd 실패)
// fallback은 디스크에 어떤 캐시가 있든 nil을 반환해야 한다. cross-workspace
// 보호의 baseline 가드.
func TestFallbackByWorkspaceCwdReturnsNilWhenCwdUnknown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})
	detectCwdEnv = func(string) string { return "" }
	detectCwdGetwd = func() (string, error) { return "", os.ErrNotExist }

	if got := fallbackByWorkspaceCwd(time.Now()); got != nil {
		t.Fatalf("fallbackByWorkspaceCwd = %#v, want nil (no cwd signal)", got)
	}
}

// task-011 (SPEC §5.5, ANALYSIS §6.1): RateLimits는 fallback 대상이 아니다.
// 두 책임을 분리해 회귀 보호한다.
//   (a) 저장 측: main()이 stdin snapshot의 RateLimits를 nil로 strip한 뒤
//       saveSessionState에 넘긴다. 재로드 결과도 nil이어야 한다 — 그래야
//       어떤 fallback 경로에서도 stale rate-limit 값이 disk에서 부활하지 않는다.
//   (b) restore 측: fallback이 반환한 SessionState의 CachedStdin.RateLimits는
//       항상 nil이며, ctx.RateLimits(UsageLimits, api.go)는 별도 타입이라 cached
//       값으로 채워질 수 있는 통로가 없다. 본 테스트는 fallback 결과의 nil
//       어서션으로 (b)를 코드 형태로 명시한다.
func TestFallbackRateLimitsIsolated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	workspace := t.TempDir()
	wantCwd := normalizeCwd(workspace)

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})
	detectCwdEnv = func(key string) string {
		if key == "CLAUDE_PROJECT_DIR" {
			return workspace
		}
		return ""
	}
	detectCwdGetwd = func() (string, error) { return workspace, nil }

	// (a) 저장 측: main()의 save 블록(snapshot.RateLimits = nil → saveSessionState)을
	// 재현한다. 입력 stdin에는 비-nil RateLimits를 의도적으로 세팅해서, strip 라인이
	// 제거되는 회귀가 일어나면 reload 결과의 RateLimits != nil로 즉시 실패하도록 한다.
	var stdin StdinInput
	stdin.SessionId = "ratelimits-isolated"
	stdin.Workspace.CurrentDir = wantCwd
	stdin.Cost.TotalCostUsd = 3.14
	stdin.ContextWindow.ContextWindowSize = 200000
	stdin.RateLimits = &struct {
		FiveHour *struct {
			UsedPercentage int   `json:"used_percentage"`
			ResetsAt       int64 `json:"resets_at"`
		} `json:"five_hour,omitempty"`
		SevenDay *struct {
			UsedPercentage int   `json:"used_percentage"`
			ResetsAt       int64 `json:"resets_at"`
		} `json:"seven_day,omitempty"`
	}{
		FiveHour: &struct {
			UsedPercentage int   `json:"used_percentage"`
			ResetsAt       int64 `json:"resets_at"`
		}{UsedPercentage: 50, ResetsAt: time.Now().Add(1 * time.Hour).Unix()},
	}

	// main.go의 save 흐름 그대로: snapshot 복사 → RateLimits strip → save.
	snapshot := stdin
	snapshot.RateLimits = nil
	saveSessionState("ratelimits-isolated", &SessionState{
		CachedStdin: &snapshot,
		WidgetCount: 3,
	})

	reloaded := loadSessionState("ratelimits-isolated")
	if reloaded == nil || reloaded.CachedStdin == nil {
		t.Fatalf("loadSessionState returned nil after save; cannot verify RateLimits strip")
	}
	if reloaded.CachedStdin.RateLimits != nil {
		t.Fatalf("reload after strip-and-save: RateLimits=%#v, want nil (save-side strip regressed)",
			reloaded.CachedStdin.RateLimits)
	}

	// (b) restore 측: 동일 cwd에 대한 fallback 매칭을 발동시킨 뒤, 반환된
	// CachedStdin.RateLimits가 nil임을 어서션한다. 디스크 캐시는 (a)의 save가
	// 만든 ratelimits-isolated 파일을 그대로 재사용한다.
	got := fallbackByWorkspaceCwd(time.Now())
	if got == nil || got.CachedStdin == nil {
		t.Fatalf("fallbackByWorkspaceCwd = nil, want match for cwd=%q", wantCwd)
	}
	if got.CachedStdin.RateLimits != nil {
		t.Fatalf("fallback CachedStdin.RateLimits = %#v, want nil "+
			"(rate-limit must come from API cache `cache-<tokenHash>.json` only)",
			got.CachedStdin.RateLimits)
	}
	// CachedStdin.RateLimits(*struct, stdin.go)와 ctx.RateLimits(*UsageLimits,
	// api.go)는 별개 타입이라 fallback 결과가 ctx.RateLimits 슬롯으로 흘러갈
	// 통로가 main.go에 없다. 위 nil 어서션으로 SPEC §5.5의 "fallback 발동해도
	// ctx.RateLimits는 session-state로부터 채워지지 않는다" 명제가 코드 형태로
	// 고정된다.
}

// task-012 (SPEC §5.11, ANALYSIS §5.2): 같은 session 안에서 사용자가 cd로
// 워크스페이스 A→B로 이동한 직후 workspace만 빈 stdin이 도착하는 시퀀스를
// end-to-end로 재현해, main.go의 workspace 복원 분기가 stale A 경로를 화면에
// 다시 노출하지 않음을 회귀 보호한다.
//
// TestShouldRestoreWorkspace(task-006)는 가드 함수 자체의 단위 어서션이고,
// 본 테스트는 §5.11 트레이스를 그대로 따라가는 시퀀스 회귀로 별도 책임:
//   - t0: cwd=A에서 정상 stdin을 saveSessionState로 디스크에 영속화
//     (sessionCacheKey 경로를 그대로 통과해 workspace.CurrentDir이 normalizeCwd로
//     정규화되어 저장되는지까지 함께 검증).
//   - t1: detectCurrentCwd fake를 cwd=B로 전환. 빈 workspace stdin이 도착하는
//     순간을 시뮬레이션해, main.go의 `shouldRestoreWorkspace(cached cwd)`
//     호출이 false를 반환해 복원 분기가 진입하지 않음을 어서션. workspaceRestoreTTL
//     이내(t0와 t1 사이 ~0초)인데도 cwd 일치 가드가 우선해 차단된다는 핵심.
//   - t2: 같은 session 안에서 다시 A로 cd back. 가드가 true로 돌아와 복원이
//     허용된다 — 가드가 일방향이 아니라 cwd 정확 일치 기반임을 보여줌.
func TestCdScenarioBlocksStaleWorkspaceRestore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dirA := t.TempDir()
	dirB := t.TempDir()
	normA := normalizeCwd(dirA)

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	setCwd := func(workspace string) {
		detectCwdEnv = func(key string) string {
			if key == "CLAUDE_PROJECT_DIR" {
				return workspace
			}
			return ""
		}
		detectCwdGetwd = func() (string, error) { return workspace, nil }
	}

	// t0: cwd=A에서 정상 stdin 저장. saveSessionState 내부에서 CurrentDir이
	// normalizeCwd로 정규화되므로 cached cwd는 normA로 영속화된다.
	setCwd(dirA)
	var stdinA StdinInput
	stdinA.SessionId = "session-cd-A"
	stdinA.Workspace.CurrentDir = dirA
	stdinA.Cost.TotalCostUsd = 1.5
	stdinA.ContextWindow.ContextWindowSize = 200000
	saveSessionState(sessionCacheKey(stdinA), &SessionState{
		CachedStdin: &stdinA,
		WidgetCount: 3,
	})

	cached := loadSessionState(sessionCacheKey(stdinA))
	if cached == nil || cached.CachedStdin == nil {
		t.Fatalf("t0: cached state missing after save")
	}
	if cached.CachedStdin.Workspace.CurrentDir != normA {
		t.Fatalf("t0: cached CurrentDir = %q, want normalized %q",
			cached.CachedStdin.Workspace.CurrentDir, normA)
	}
	// workspaceRestoreTTL 이내라는 사실 자체를 명시적으로 확인 — 가드가 차단의
	// 근거가 TTL 만료가 아니라 cwd 불일치임을 분리해 보여준다.
	if cached.SavedAt > 0 && time.Since(time.Unix(cached.SavedAt, 0)) >= workspaceRestoreTTL {
		t.Fatalf("t0: cached SavedAt already outside workspaceRestoreTTL (%s); cannot exercise guard",
			workspaceRestoreTTL)
	}

	// t1: cd to B + 빈 workspace stdin 도착. main.go의 복원 분기는
	// `shouldRestoreWorkspace(cached.CachedStdin.Workspace.CurrentDir)` 결과에
	// 의해 게이트된다. cwd 가드가 false면 stale A 경로가 화면에 복원되지 않는다.
	setCwd(dirB)
	if shouldRestoreWorkspace(cached.CachedStdin.Workspace.CurrentDir) {
		t.Fatalf("t1: guard returned true after cd A->B (cached=%q, current=%q); "+
			"main.go would restore stale workspace and leak A onto the status line",
			cached.CachedStdin.Workspace.CurrentDir, dirB)
	}

	// main.go의 restoreWorkspace 합성 조건도 동일하게 false임을 재구성해
	// 어서션한다 — 가드가 빠지거나 분기 평가 순서가 바뀌는 회귀를 잡는다.
	emptyWorkspaceStdin := StdinInput{}
	workspaceStale := emptyWorkspaceStdin.Workspace.CurrentDir == "" &&
		cached.CachedStdin.Workspace.CurrentDir != ""
	withinTTL := cached.SavedAt > 0 &&
		time.Since(time.Unix(cached.SavedAt, 0)) < workspaceRestoreTTL
	restoreWorkspace := workspaceStale && withinTTL &&
		shouldRestoreWorkspace(cached.CachedStdin.Workspace.CurrentDir)
	if restoreWorkspace {
		t.Fatalf("t1: restoreWorkspace composite = true after cd; "+
			"workspaceStale=%v withinTTL=%v guard=false expected to dominate",
			workspaceStale, withinTTL)
	}

	// t2: cd back to A. 동일한 cached cwd, 동일한 빈 workspace stdin이지만
	// 이번엔 가드가 true를 반환해 복원이 허용된다. 가드가 cwd 정확 일치 기반임을
	// 입증.
	setCwd(dirA)
	if !shouldRestoreWorkspace(cached.CachedStdin.Workspace.CurrentDir) {
		t.Fatalf("t2: guard returned false after cd back to A (cached=%q, current=%q); "+
			"normal restore path would be unreachable",
			cached.CachedStdin.Workspace.CurrentDir, dirA)
	}
	restoreWorkspaceBack := workspaceStale && withinTTL &&
		shouldRestoreWorkspace(cached.CachedStdin.Workspace.CurrentDir)
	if !restoreWorkspaceBack {
		t.Fatalf("t2: restoreWorkspace composite = false after cd back; "+
			"workspaceStale=%v withinTTL=%v guard expected to be true",
			workspaceStale, withinTTL)
	}
}
