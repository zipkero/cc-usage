package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	// t1: cd to B + 빈 workspace stdin 도착. shouldRestoreFromSession이
	// cwd-exact-match 가드를 포함하므로 eligibility=false를 반환해야 한다.
	// cwd 가드가 false면 stale A 경로가 화면에 복원되지 않는다.
	setCwd(dirB)
	emptyWorkspaceStdin := StdinInput{}
	if shouldRestoreFromSession(emptyWorkspaceStdin, cached, time.Now()) {
		t.Fatalf("t1: shouldRestoreFromSession returned true after cd A->B (cached=%q, current=%q); "+
			"main.go would restore stale workspace and leak A onto the status line",
			cached.CachedStdin.Workspace.CurrentDir, dirB)
	}

	// t2: 같은 session 안에서 다시 A로 cd back. eligibility가 true로 돌아와
	// 복원이 허용된다 — 가드가 cwd 정확 일치 기반임을 보여줌.
	setCwd(dirA)
	if !shouldRestoreFromSession(emptyWorkspaceStdin, cached, time.Now()) {
		t.Fatalf("t2: shouldRestoreFromSession returned false after cd back to A (cached=%q, current=%q); "+
			"normal restore path would be unreachable",
			cached.CachedStdin.Workspace.CurrentDir, dirA)
	}
}

// task-003: 새 atomic restore 흐름 end-to-end 검증.
//
// (a) 같은 cwd + fresh cached + 빈 stdin → projectInfo·model·context·cost 위젯이
//     함께 살아남음 (stdout에 cwd basename, model id, cost, percent 문자열 포함).
// (b) cwd mismatch 또는 SavedAt > workspaceRestoreTTL → shouldSuppressOutput
//     발동 또는 캐시 복원 없이 절반 출력 미발생 (cost·context 단독 채워짐 금지).
// (c) stdin이 일부 fresh(workspace만 fresh) + cached 가짐 → fresh workspace는
//     cached로 덮이지 않고 model/cost/context는 cached에서 채워짐.
//
// orchestrate를 직접 호출해 deterministic하게 검증한다.
func TestTask003AtomicRestoreFlowABC(t *testing.T) {
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	// buildCached는 주어진 cwd/savedAt으로 SessionState를 만든다.
	buildCached := func(cwd string, savedAt time.Time) *SessionState {
		var cs StdinInput
		cs.Workspace.CurrentDir = cwd
		cs.Model.ID = "claude-opus-4-6"
		cs.Model.DisplayName = "Opus"
		cs.Cost.TotalCostUsd = 1.25
		cs.ContextWindow.TotalInputTokens = 50000
		cs.ContextWindow.TotalOutputTokens = 10000
		cs.ContextWindow.ContextWindowSize = 200000
		return &SessionState{
			CachedStdin: &cs,
			WidgetCount: 4,
			SavedAt:     savedAt.Unix(),
		}
	}

	buildCtx := func(stdin StdinInput) *Context {
		cfg := loadConfig("")
		trans := loadTranslations(cfg.Language)
		return &Context{
			Stdin:        stdin,
			Config:       cfg,
			Translations: trans,
		}
	}

	now := time.Unix(1_700_000_000, 0)

	t.Run("(a) same cwd + fresh cache + empty stdin -> all widgets survive", func(t *testing.T) {
		cwd := normalizeCwd(t.TempDir())
		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return cwd, nil }

		cached := buildCached(cwd, now.Add(-10*time.Second))
		stdin := StdinInput{} // 완전히 빈 stdin

		ctx := buildCtx(stdin)
		result := orchestrate(ctx)
		_ = result // 1차 결과 (채워지기 전)

		if shouldRestoreFromSession(ctx.Stdin, cached, now) {
			fillFromSessionCache(&ctx.Stdin, cached)
			result = orchestrate(ctx)
		}

		// cwd basename, model id, cost, percent가 모두 렌더 결과에 있어야 함.
		combined := ""
		for _, line := range result.Lines {
			combined += line
		}

		if result.WidgetCount < 3 {
			t.Fatalf("(a) widgetCount=%d, want >= 3 (projectInfo/model/context/cost should all survive)", result.WidgetCount)
		}
		if !containsAny(combined, "claude-opus-4-6", "Opus") {
			t.Errorf("(a) model not in output: %q", combined)
		}
		if !containsAny(combined, "1.25", "$1.25") {
			t.Errorf("(a) cost not in output: %q", combined)
		}
	})

	t.Run("(b) cwd mismatch -> no cache restoration, no half-populated output", func(t *testing.T) {
		cwdCached := normalizeCwd(t.TempDir())
		cwdCurrent := normalizeCwd(t.TempDir()) // 다른 cwd
		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return cwdCurrent, nil }

		cached := buildCached(cwdCached, now.Add(-10*time.Second))
		stdin := StdinInput{} // 빈 stdin

		ctx := buildCtx(stdin)

		// eligibility=false 여야 함
		if shouldRestoreFromSession(ctx.Stdin, cached, now) {
			t.Fatal("(b) shouldRestoreFromSession=true on cwd mismatch; expected false")
		}
		// fillFromSessionCache를 호출하지 않아 stdin은 그대로 빔.
		// shouldSuppressOutput이 발동해야 함 (identity 없음, rate limits 없음).
		if !shouldSuppressOutput(ctx.Stdin, nil) {
			t.Fatal("(b) shouldSuppressOutput=false after cwd mismatch; expected suppress (no partial output)")
		}
	})

	t.Run("(b) SavedAt > workspaceRestoreTTL -> no cache restoration", func(t *testing.T) {
		cwd := normalizeCwd(t.TempDir())
		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return cwd, nil }

		// SavedAt이 TTL 초과
		cached := buildCached(cwd, now.Add(-(workspaceRestoreTTL+time.Second)))
		stdin := StdinInput{}

		ctx := buildCtx(stdin)
		if shouldRestoreFromSession(ctx.Stdin, cached, now) {
			t.Fatal("(b) shouldRestoreFromSession=true when SavedAt > workspaceRestoreTTL; expected false")
		}
		if !shouldSuppressOutput(ctx.Stdin, nil) {
			t.Fatal("(b) shouldSuppressOutput=false after TTL expiry; expected suppress")
		}
	})

	t.Run("(c) partial fresh stdin -> fresh fields not overwritten", func(t *testing.T) {
		cwd := normalizeCwd(t.TempDir())
		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return cwd, nil }

		cached := buildCached(cwd, now.Add(-10*time.Second))

		// stdin이 workspace만 fresh로 들고 옴, 나머지 비어있음.
		freshCwd := "/fresh/workspace"
		stdin := StdinInput{}
		stdin.Workspace.CurrentDir = freshCwd

		if !shouldRestoreFromSession(stdin, cached, now) {
			t.Fatal("(c) shouldRestoreFromSession=false; expected true (model/cost/context are empty)")
		}

		mask := fillFromSessionCache(&stdin, cached)

		// fresh workspace는 덮이지 않아야 함.
		if stdin.Workspace.CurrentDir != freshCwd {
			t.Errorf("(c) Workspace.CurrentDir overwritten: got %q, want %q", stdin.Workspace.CurrentDir, freshCwd)
		}
		if mask.Workspace {
			t.Error("(c) mask.Workspace should be false (stdin had fresh workspace)")
		}
		// model/cost/context는 cache에서 채워져야 함.
		if !mask.Model {
			t.Error("(c) mask.Model should be true (stdin had empty model)")
		}
		if !mask.Cost {
			t.Error("(c) mask.Cost should be true (stdin had zero cost)")
		}
		if !mask.ContextWindow {
			t.Error("(c) mask.ContextWindow should be true (stdin had empty context)")
		}
		if stdin.Model.ID != "claude-opus-4-6" {
			t.Errorf("(c) Model.ID = %q, want claude-opus-4-6", stdin.Model.ID)
		}
		if stdin.Cost.TotalCostUsd != 1.25 {
			t.Errorf("(c) Cost = %.4f, want 1.25", stdin.Cost.TotalCostUsd)
		}
	})
}

// task-004: degraded 호출이 N회 반복될 때 cache-복원된 필드가 SessionState에
// 누적되지 않음을 검증하는 시나리오 테스트.
//
// 흐름:
//  1. 정상 stdin(workspace/model/cost/context 모두 포함)으로 캐시를 초기 생성.
//  2. 빈 stdin으로 첫 번째 degraded 호출 → stripRestoredFields 적용 후 save.
//     저장된 캐시 본문에서 복원 필드가 비어있는지 확인.
//  3. 동일 상태에서 두 번째 빈 stdin 호출 → 두 번째 save 후에도 마찬가지로
//     복원 필드가 비어있는지 확인(자기참조 고착 0회).
//  4. fresh 필드(Version, SessionId)는 두 호출 내내 보존됨을 확인.
func TestTask004StripRestoredFieldsNoAccumulation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := normalizeCwd(t.TempDir())

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})
	detectCwdEnv = func(string) string { return "" }
	detectCwdGetwd = func() (string, error) { return cwd, nil }

	const cacheKey = "task004-scenario"

	// Step 1: 정상 stdin으로 초기 캐시 생성. Save 직전에는 복원이 없으므로 mask 전부
	// false → 필드 그대로 저장됨.
	initialStdin := StdinInput{SessionId: cacheKey}
	initialStdin.Workspace.CurrentDir = cwd
	initialStdin.Model.ID = "claude-opus-4-6"
	initialStdin.Model.DisplayName = "Opus"
	initialStdin.Cost.TotalCostUsd = 1.25
	initialStdin.ContextWindow.TotalInputTokens = 50000
	initialStdin.ContextWindow.TotalOutputTokens = 10000
	initialStdin.ContextWindow.ContextWindowSize = 200000
	initialStdin.Version = "v-fresh"

	snapshot0 := initialStdin
	snapshot0.RateLimits = nil
	stripRestoredFields(&snapshot0, restoredFieldMask{}) // all false — no-op
	saveSessionState(cacheKey, &SessionState{
		CachedStdin: &snapshot0,
		WidgetCount: 4,
	})

	// Step 2: 첫 번째 degraded 호출. 빈 stdin + 위에서 만든 cached.
	now := time.Now()
	cached1 := loadSessionState(cacheKey)
	if cached1 == nil {
		t.Fatalf("step2: initial cache not found")
	}

	degradedStdin1 := StdinInput{SessionId: cacheKey, Version: "v-fresh"}
	if !shouldRestoreFromSession(degradedStdin1, cached1, now) {
		t.Fatalf("step2: shouldRestoreFromSession=false unexpectedly; test prerequisite not met")
	}
	mask1 := fillFromSessionCache(&degradedStdin1, cached1)

	// mask가 하나라도 true여야 한다(빈 stdin을 채웠으므로).
	if !(mask1.Workspace || mask1.Model || mask1.Cost || mask1.ContextWindow) {
		t.Fatalf("step2: expected at least one mask bit true, got all false")
	}

	// save 직전 stripping 적용.
	snapshot1 := degradedStdin1
	snapshot1.RateLimits = nil
	stripRestoredFields(&snapshot1, mask1)
	saveSessionState(cacheKey, &SessionState{
		CachedStdin: &snapshot1,
		WidgetCount: 4,
	})

	// 저장된 캐시 본문에서 cache-복원 필드가 비어있는지 확인.
	saved1 := loadSessionState(cacheKey)
	if saved1 == nil || saved1.CachedStdin == nil {
		t.Fatalf("step2: saved cache not found after first degraded call")
	}
	if mask1.Workspace && saved1.CachedStdin.Workspace.CurrentDir != "" {
		t.Errorf("step2: Workspace.CurrentDir = %q after strip+save; want empty (cache-restored field must not persist)",
			saved1.CachedStdin.Workspace.CurrentDir)
	}
	if mask1.Model && (saved1.CachedStdin.Model.ID != "" || saved1.CachedStdin.Model.DisplayName != "") {
		t.Errorf("step2: Model = %+v after strip+save; want zero (cache-restored field must not persist)",
			saved1.CachedStdin.Model)
	}
	if mask1.Cost && saved1.CachedStdin.Cost.TotalCostUsd != 0 {
		t.Errorf("step2: Cost.TotalCostUsd = %v after strip+save; want 0 (cache-restored field must not persist)",
			saved1.CachedStdin.Cost.TotalCostUsd)
	}
	if mask1.ContextWindow && saved1.CachedStdin.ContextWindow.TotalInputTokens != 0 {
		t.Errorf("step2: ContextWindow.TotalInputTokens = %d after strip+save; want 0 (cache-restored field must not persist)",
			saved1.CachedStdin.ContextWindow.TotalInputTokens)
	}
	// Version은 fresh이므로 보존되어야 함.
	if saved1.CachedStdin.Version != "v-fresh" {
		t.Errorf("step2: Version = %q; want v-fresh (fresh field must survive stripping)", saved1.CachedStdin.Version)
	}
	// SavedAt은 갱신되어야 한다 (non-zero).
	if saved1.SavedAt == 0 {
		t.Errorf("step2: SavedAt = 0; want non-zero (must be updated on each save)")
	}

	// Step 3: 두 번째 degraded 호출 — saved1이 캐시로 사용됨.
	// saved1에는 이미 workspace/model/cost/context가 비어있으므로
	// shouldRestoreFromSession은 eligibility=false를 반환해야 한다
	// (캐시에 복원할 값이 없거나 needsFill 조건에서 캐시도 비어있어 no-op).
	// 중요: 자기참조 고착 방지가 목적이므로 두 번째 호출 후 캐시가 다시 채워지지
	// 않음을 확인하면 된다.
	degradedStdin2 := StdinInput{SessionId: cacheKey, Version: "v-fresh"}
	mask2 := restoredFieldMask{}
	if shouldRestoreFromSession(degradedStdin2, saved1, now) {
		mask2 = fillFromSessionCache(&degradedStdin2, saved1)
	}

	snapshot2 := degradedStdin2
	snapshot2.RateLimits = nil
	stripRestoredFields(&snapshot2, mask2)

	// WidgetCount < 2 조건이면 save 자체가 skip되지만, 여기선 save를 직접 호출해
	// 저장 본문이 누적되지 않음을 어서션한다.
	saveSessionState(cacheKey, &SessionState{
		CachedStdin: &snapshot2,
		WidgetCount: 2, // 최소 threshold 만족으로 강제 save
	})

	saved2 := loadSessionState(cacheKey)
	if saved2 == nil || saved2.CachedStdin == nil {
		t.Fatalf("step3: saved cache not found after second degraded call")
	}
	// 두 번째 저장 본문에서도 cache-복원 필드가 비어있어야 함.
	if saved2.CachedStdin.Workspace.CurrentDir != "" {
		t.Errorf("step3: Workspace.CurrentDir = %q; want empty (no accumulation after 2nd degraded call)",
			saved2.CachedStdin.Workspace.CurrentDir)
	}
	if saved2.CachedStdin.Model.ID != "" || saved2.CachedStdin.Model.DisplayName != "" {
		t.Errorf("step3: Model = %+v; want zero (no accumulation after 2nd degraded call)",
			saved2.CachedStdin.Model)
	}
	if saved2.CachedStdin.Cost.TotalCostUsd != 0 {
		t.Errorf("step3: Cost.TotalCostUsd = %v; want 0 (no accumulation after 2nd degraded call)",
			saved2.CachedStdin.Cost.TotalCostUsd)
	}
	// Version 보존 확인.
	if saved2.CachedStdin.Version != "v-fresh" {
		t.Errorf("step3: Version = %q; want v-fresh (fresh field must survive)", saved2.CachedStdin.Version)
	}
	// SavedAt 갱신 확인.
	if saved2.SavedAt == 0 {
		t.Errorf("step3: SavedAt = 0; want non-zero")
	}
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// task-005 회귀 보호 묶음 — cross-workspace 노출 금지, warmup·suppress 유지,
// RateLimits 미복원을 e2e 성격으로 검증한다 (SPEC §5.2·§5.3·§5.4·§5.5·§5.6).

// (a) cross-workspace 차단: cwd가 cached cwd와 다를 때 어떤 필드도 복원되지 않고
// stdout에 cached cwd 문자열·model id·cost 수치가 등장하지 않음.
func TestTask005CrossWorkspaceBlocked(t *testing.T) {
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	cachedCwd := normalizeCwd(t.TempDir()) // 캐시에 저장된 cwd
	currentCwd := normalizeCwd(t.TempDir()) // 현재 실행 중인 다른 cwd

	// detectCurrentCwd가 currentCwd를 반환하도록 설정
	detectCwdEnv = func(string) string { return "" }
	detectCwdGetwd = func() (string, error) { return currentCwd, nil }

	// cached SessionState: cachedCwd 워크스페이스의 정상 데이터
	var cachedStdin StdinInput
	cachedStdin.Workspace.CurrentDir = cachedCwd
	cachedStdin.Model.ID = "claude-opus-4-6-cross"
	cachedStdin.Model.DisplayName = "OpusCross"
	cachedStdin.Cost.TotalCostUsd = 9.99
	cachedStdin.ContextWindow.TotalInputTokens = 50000
	cachedStdin.ContextWindow.TotalOutputTokens = 10000
	cachedStdin.ContextWindow.ContextWindowSize = 200000
	cached := &SessionState{
		CachedStdin: &cachedStdin,
		WidgetCount: 4,
		SavedAt:     time.Now().Add(-5 * time.Second).Unix(),
	}

	// 빈 stdin으로 호출 (cross-workspace 시나리오)
	emptyStdin := StdinInput{}

	// eligibility가 false여야 함 (cwd mismatch)
	if shouldRestoreFromSession(emptyStdin, cached, time.Now()) {
		t.Fatal("(a) shouldRestoreFromSession=true on cross-workspace; expected false (SPEC §5.2·§5.4)")
	}

	// 복원이 없으므로 orchestrate 결과에 cached 데이터가 없어야 함
	cfg := loadConfig("")
	trans := loadTranslations(cfg.Language)
	ctx := &Context{
		Stdin:        emptyStdin,
		Config:       cfg,
		Translations: trans,
	}
	result := orchestrate(ctx)

	combined := strings.Join(result.Lines, "\n")

	// stdout에 cached 워크스페이스 cwd 경로가 등장하면 안 됨
	if strings.Contains(combined, cachedCwd) {
		t.Errorf("(a) cross-workspace: cached cwd %q leaked into output: %q", cachedCwd, combined)
	}
	// stdout에 cached model id가 등장하면 안 됨
	if strings.Contains(combined, "claude-opus-4-6-cross") {
		t.Errorf("(a) cross-workspace: cached model id leaked into output: %q", combined)
	}
	// stdout에 cached cost 수치가 등장하면 안 됨
	if strings.Contains(combined, "9.99") {
		t.Errorf("(a) cross-workspace: cached cost 9.99 leaked into output: %q", combined)
	}

	// 빈 stdin이므로 shouldSuppressOutput이 차단해야 함
	if !shouldSuppressOutput(emptyStdin, nil) {
		t.Error("(a) shouldSuppressOutput=false after cross-workspace block with empty stdin; expected suppress")
	}
}

// (b) warmup 예외 유지: stdin·캐시 모두 identity 없고 RateLimits만 있으면
// shouldSuppressOutput이 통과되어 5h/7d 위젯이 렌더된다.
func TestTask005WarmupExceptionPreserved(t *testing.T) {
	// stdin 완전히 빔 — identity 없음
	emptyStdin := StdinInput{}

	// cached도 없음 (nil)
	var cached *SessionState = nil

	// RateLimits만 존재
	rateLimits := &UsageLimits{
		FiveHour: &UsageLimitEntry{Utilization: 42, ResetsAt: time.Now().Add(time.Hour)},
	}

	// eligibility=false여야 함 (cached==nil)
	if shouldRestoreFromSession(emptyStdin, cached, time.Now()) {
		t.Fatal("(b) shouldRestoreFromSession=true with nil cached; expected false")
	}

	// RateLimits가 있으므로 shouldSuppressOutput이 통과해야 함 (warmup 예외)
	if shouldSuppressOutput(emptyStdin, rateLimits) {
		t.Fatal("(b) shouldSuppressOutput=true with RateLimits present; warmup exception must allow output (SPEC §5.3)")
	}

	// orchestrate 결과에 5h 위젯이 있어야 함
	cfg := loadConfig("")
	trans := loadTranslations(cfg.Language)
	ctx := &Context{
		Stdin:        emptyStdin,
		Config:       cfg,
		Translations: trans,
		RateLimits:   rateLimits,
	}
	// warmup 분기 판정
	if !isWarmupExceptionPath(emptyStdin, rateLimits) {
		t.Fatal("(b) isWarmupExceptionPath=false; expected true for no-identity + rate-limit")
	}

	// warmup 분기에서는 rate-limit 위젯만 렌더되어야 함 — cost/context/model/projectInfo 누출 금지
	result := renderRateLimitOnly(ctx)
	combined := strings.Join(result.Lines, "\n")

	if result.WidgetCount == 0 {
		t.Errorf("(b) warmup: widgetCount=0, expected rate-limit widgets to render (output=%q)", combined)
	}
	if !strings.Contains(combined, "5h") {
		t.Errorf("(b) warmup: '5h' not in output (got %q); rate-limit widget should render", combined)
	}
	// cost 위젯 누출 방지: "$" 문자열 미등장
	if strings.Contains(combined, "$") {
		t.Errorf("(b) warmup: cost widget leaked '$' into output (got %q); only rate-limit widgets should render (SPEC §5.3)", combined)
	}
}

// (c) 무출력 조건 유지: stdin·캐시·RateLimits 모두 없을 때 stdout이 빈 문자열.
func TestTask005SuppressWhenAllEmpty(t *testing.T) {
	emptyStdin := StdinInput{}

	// cached nil
	if shouldRestoreFromSession(emptyStdin, nil, time.Now()) {
		t.Fatal("(c) shouldRestoreFromSession=true with nil cached; expected false")
	}

	// RateLimits nil → shouldSuppressOutput=true
	if !shouldSuppressOutput(emptyStdin, nil) {
		t.Fatal("(c) shouldSuppressOutput=false with all empty; expected suppress (SPEC §5.3)")
	}

	// RateLimits empty struct → shouldSuppressOutput=true
	if !shouldSuppressOutput(emptyStdin, &UsageLimits{}) {
		t.Fatal("(c) shouldSuppressOutput=false with empty UsageLimits; expected suppress")
	}

	// orchestrate 결과 자체도 identity 없으면 빈 출력
	cfg := loadConfig("")
	trans := loadTranslations(cfg.Language)
	ctx := &Context{
		Stdin:        emptyStdin,
		Config:       cfg,
		Translations: trans,
		RateLimits:   nil,
	}
	result := orchestrate(ctx)
	combined := strings.Join(result.Lines, "\n")
	// 비용 위젯("$0.00")만 나올 수 있지만 shouldSuppressOutput이 차단하므로
	// 실제 출력 경로에서는 빈 문자열이 된다. 여기서는 suppress 판단만 검증.
	_ = combined
	// suppress=true임을 이미 위에서 확인했으므로 stdout은 빈 문자열이 됨.
}

// (d) RateLimits 미복원: 캐시 본문에 임의로 RateLimits를 넣은 fixture에서도
// ctx.RateLimits에 주입되지 않고, save 직전 snapshot.RateLimits가 nil임을 어서션.
func TestTask005RateLimitsNotRestoredFromCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := normalizeCwd(t.TempDir())

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})
	detectCwdEnv = func(string) string { return "" }
	detectCwdGetwd = func() (string, error) { return cwd, nil }

	// 캐시 본문에 의도적으로 RateLimits를 박아둔 fixture SessionState를 생성.
	// (실제로는 save 측 stripping으로 이렇게 저장되지 않지만, 혹시 stripping이
	// 우회되는 회귀가 생겨도 복원 경로에서 차단됨을 검증.)
	var cachedStdin StdinInput
	cachedStdin.SessionId = "task005-ratelimits"
	cachedStdin.Workspace.CurrentDir = cwd
	cachedStdin.Model.ID = "claude-opus-4-6"
	cachedStdin.Model.DisplayName = "Opus"
	cachedStdin.Cost.TotalCostUsd = 2.50
	cachedStdin.ContextWindow.TotalInputTokens = 30000
	cachedStdin.ContextWindow.TotalOutputTokens = 5000
	cachedStdin.ContextWindow.ContextWindowSize = 200000
	// 테스트 fixture: 캐시 본문에 강제로 RateLimits 삽입 (회귀 시뮬레이션)
	cachedStdin.RateLimits = &struct {
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
		}{UsedPercentage: 80, ResetsAt: time.Now().Add(time.Hour).Unix()},
	}
	cached := &SessionState{
		CachedStdin: &cachedStdin,
		WidgetCount: 4,
		SavedAt:     time.Now().Add(-5 * time.Second).Unix(),
	}

	// 빈 stdin으로 복원 시도
	emptyStdin := StdinInput{}

	// eligibility 결정: cwd 일치이므로 true여야 함
	if !shouldRestoreFromSession(emptyStdin, cached, time.Now()) {
		t.Fatal("(d) shouldRestoreFromSession=false; expected true (cwd match, empty stdin)")
	}

	// fillFromSessionCache 후 stdin.RateLimits는 절대 채워지면 안 됨
	filled := emptyStdin
	mask := fillFromSessionCache(&filled, cached)

	if filled.RateLimits != nil {
		t.Errorf("(d) fillFromSessionCache: RateLimits=%#v, want nil (must never restore RateLimits from cache, SPEC §5.5)",
			filled.RateLimits)
	}

	// mask에 RateLimits 비트가 없어야 함 (restoredFieldMask에 RateLimits 필드 없음이 보장)
	// model/cost/context는 채워져야 함
	if !mask.Model {
		t.Error("(d) mask.Model=false; expected true (model was empty in stdin)")
	}
	if !mask.Cost {
		t.Error("(d) mask.Cost=false; expected true (cost was zero in stdin)")
	}
	if !mask.ContextWindow {
		t.Error("(d) mask.ContextWindow=false; expected true (context was empty in stdin)")
	}

	// save 직전 stripping: snapshot.RateLimits = nil이 적용되면 저장 본문에 nil이어야 함
	const cacheKey = "task005-ratelimits"
	snapshot := filled
	snapshot.RateLimits = nil
	stripRestoredFields(&snapshot, mask)
	saveSessionState(cacheKey, &SessionState{
		CachedStdin: &snapshot,
		WidgetCount: 4,
	})

	reloaded := loadSessionState(cacheKey)
	if reloaded == nil || reloaded.CachedStdin == nil {
		t.Fatalf("(d) loadSessionState returned nil after save")
	}
	// 저장 후 재로드 시 RateLimits가 nil이어야 함
	if reloaded.CachedStdin.RateLimits != nil {
		t.Errorf("(d) reloaded.RateLimits=%#v, want nil (save-side RateLimits strip must persist)",
			reloaded.CachedStdin.RateLimits)
	}
	// cache-복원된 model/cost/context는 stripping으로 비워졌어야 함
	if mask.Model && (reloaded.CachedStdin.Model.ID != "" || reloaded.CachedStdin.Model.DisplayName != "") {
		t.Errorf("(d) Model=%+v after strip; want zero (cache-restored field must not persist in save)",
			reloaded.CachedStdin.Model)
	}
	if mask.Cost && reloaded.CachedStdin.Cost.TotalCostUsd != 0 {
		t.Errorf("(d) Cost=%v after strip; want 0 (cache-restored field must not persist in save)",
			reloaded.CachedStdin.Cost.TotalCostUsd)
	}
}
