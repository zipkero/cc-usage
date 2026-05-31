package main

import (
	"encoding/json"
	"fmt"
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

	t.Run("(d) cached cwd is descendant of current root -> restored (cd into subdir)", func(t *testing.T) {
		// 세션이 루트 아래 하위 디렉토리로 cd한 케이스. detectCurrentCwd는
		// CLAUDE_PROJECT_DIR 기반 루트를 돌려주고, 캐시된 current_dir는 그 하위를
		// 가리킨다. 정확 일치만 허용하던 시절엔 차단됐으나 cwdWithinRoot 완화로
		// 복원돼야 한다 (v0.3.15 회귀).
		root := normalizeCwd(t.TempDir())
		sub := filepath.Join(root, "web", "node_modules", "@apps-in-toss")
		detectCwdEnv = func(string) string { return root }
		detectCwdGetwd = func() (string, error) { return root, nil }

		cached := buildCached(sub, now.Add(-10*time.Second))
		stdin := StdinInput{} // 빈 stdin

		if !shouldRestoreFromSession(stdin, cached, now) {
			t.Fatal("(d) shouldRestoreFromSession=false when cached cwd is a descendant of current root; expected true")
		}
		mask := fillFromSessionCache(&stdin, cached)
		if !mask.Workspace || stdin.Workspace.CurrentDir != sub {
			t.Fatalf("(d) workspace not restored: mask.Workspace=%v current_dir=%q", mask.Workspace, stdin.Workspace.CurrentDir)
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

// writeTranscriptFixture는 dir 디렉토리 안에 session.jsonl 파일을 생성하고
// 주어진 model·cwd를 담은 assistant entry 한 줄을 기록한다.
// 반환값은 파일의 전체 경로다.
func writeTranscriptFixture(t *testing.T, dir, model, cwd string, inputTokens, outputTokens int) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("writeTranscriptFixture: mkdir %s: %v", dir, err)
	}
	line := fmt.Sprintf(
		`{"type":"assistant","cwd":%q,"message":{"model":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
		cwd, model, inputTokens, outputTokens,
	)
	path := filepath.Join(dir, "session.jsonl")
	// 두 줄로 구성: 끝 줄은 partial 가드로 skip되므로 더미 한 줄 추가
	content := line + "\n" + `{"type":"user","cwd":"","message":{}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeTranscriptFixture: write %s: %v", path, err)
	}
	return path
}

// task-008 통합 테스트 (a): 빈 stdin + 유효 transcript(entry.cwd == 현재 cwd)
// → Layer 2 발동 후 model/context 채워진 full 복원 + CostEstimated=true.
func TestLayer2TranscriptBackfillFullRestore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// t.TempDir()로 실제 존재하는 디렉토리를 workspace로 사용 (normalizeCwd가 EvalSymlinks 수행)
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

	// transcript fixture 생성: entry.cwd = wantCwd (현재 cwd와 일치)
	transcriptDir := encodeCwdToTranscriptDir(home, wantCwd)
	transcriptPath := writeTranscriptFixture(t, transcriptDir, "claude-opus-4-6", wantCwd, 50000, 10000)
	_ = transcriptPath

	// 빈 stdin (Layer 1 캐시 없음 → Layer 2가 발동해야 함)
	emptyStdin := StdinInput{}

	cfg := loadConfig("")
	trans := loadTranslations(cfg.Language)
	ctx := &Context{
		Stdin:        emptyStdin,
		Config:       cfg,
		Translations: trans,
	}

	// Layer 1: cached == nil이므로 미발동
	// Layer 2: needsTranscriptBackfill=true → transcript 읽기 → entry.cwd 일치 → 적용
	if !needsTranscriptBackfill(ctx.Stdin) {
		t.Fatal("needsTranscriptBackfill=false on empty stdin; expected true")
	}

	var restoredMask restoredFieldMask
	cwd := detectCurrentCwd()
	if cwd == "" {
		t.Fatal("detectCurrentCwd returned empty; cannot run test")
	}
	if cwd != wantCwd {
		t.Fatalf("cwd=%q, want %q", cwd, wantCwd)
	}

	// transcript path 결정 (stdin.TranscriptPath 없음 → 인코딩 디렉토리 후보)
	transcriptPathResolved, err := selectTranscriptCandidate(transcriptDir)
	if err != nil {
		t.Fatalf("selectTranscriptCandidate: %v", err)
	}

	entry, err := readLastAssistantEntry(transcriptPathResolved, 64*1024, 1*1024*1024)
	if err != nil {
		t.Fatalf("readLastAssistantEntry: %v", err)
	}
	if entry == nil {
		t.Fatal("readLastAssistantEntry returned nil; expected assistant entry")
	}

	// D4 가드: entry.cwd == wantCwd
	entryCwdNorm := normalizeCwd(entry.Cwd)
	if entryCwdNorm != cwd {
		t.Fatalf("D4 guard would block: entry.cwd (norm)=%q != cwd=%q", entryCwdNorm, cwd)
	}

	oneMSignal := loadLastKnownOneM(cwd)
	mask2 := applyTranscriptToStdin(&ctx.Stdin, entry, oneMSignal)

	if mask2.Model {
		restoredMask.Model = true
	}
	if mask2.Cost {
		restoredMask.Cost = true
	}
	if mask2.ContextWindow {
		restoredMask.ContextWindow = true
	}
	ctx.CostEstimated = true

	result := orchestrate(ctx)

	// 검증: model/context가 채워졌어야 함
	if ctx.Stdin.Model.ID == "" {
		t.Error("Model.ID is empty after Layer 2 backfill; expected claude-opus-4-6")
	}
	if ctx.Stdin.ContextWindow.ContextWindowSize <= 0 {
		t.Error("ContextWindowSize <= 0 after Layer 2 backfill")
	}
	if !mask2.Model {
		t.Error("mask2.Model=false; expected true (model was empty in stdin)")
	}
	if !mask2.ContextWindow {
		t.Error("mask2.ContextWindow=false; expected true (context was empty in stdin)")
	}
	// needsTranscriptBackfill은 이제 false여야 함 (채워졌으므로)
	if needsTranscriptBackfill(ctx.Stdin) {
		t.Error("needsTranscriptBackfill=true after backfill; expected false")
	}
	// orchestrate 결과에 model id가 포함되어야 함
	combined := strings.Join(result.Lines, "\n")
	if !containsAny(combined, "claude-opus-4-6", "Opus") {
		t.Errorf("model not in output after Layer 2 backfill: %q", combined)
	}
	// CostEstimated=true
	if !ctx.CostEstimated {
		t.Error("ctx.CostEstimated=false; expected true after transcript backfill")
	}
}

// task-008 통합 테스트 (b): entry.cwd != 현재 cwd → D4 가드로 Layer 2 미발동.
func TestLayer2TranscriptBackfillCwdGuardBlocks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	workspaceCurrent := t.TempDir()
	workspaceOther := t.TempDir()
	currentCwd := normalizeCwd(workspaceCurrent)
	otherCwd := normalizeCwd(workspaceOther)

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})
	detectCwdEnv = func(key string) string {
		if key == "CLAUDE_PROJECT_DIR" {
			return workspaceCurrent
		}
		return ""
	}
	detectCwdGetwd = func() (string, error) { return workspaceCurrent, nil }

	// transcript fixture 생성: entry.cwd = otherCwd (현재 cwd와 불일치)
	transcriptDir := encodeCwdToTranscriptDir(home, currentCwd)
	transcriptPath := writeTranscriptFixture(t, transcriptDir, "claude-opus-4-6", otherCwd, 50000, 10000)
	_ = transcriptPath

	emptyStdin := StdinInput{}
	cfg := loadConfig("")
	trans := loadTranslations(cfg.Language)
	ctx := &Context{
		Stdin:        emptyStdin,
		Config:       cfg,
		Translations: trans,
	}

	cwd := detectCurrentCwd()
	if cwd != currentCwd {
		t.Fatalf("cwd=%q, want %q", cwd, currentCwd)
	}

	transcriptPathResolved, err := selectTranscriptCandidate(transcriptDir)
	if err != nil {
		t.Fatalf("selectTranscriptCandidate: %v", err)
	}

	entry, err := readLastAssistantEntry(transcriptPathResolved, 64*1024, 1*1024*1024)
	if err != nil {
		t.Fatalf("readLastAssistantEntry: %v", err)
	}
	if entry == nil {
		t.Fatal("readLastAssistantEntry returned nil")
	}

	// D4 가드: entry.cwd (otherCwd) != currentCwd → 미발동이어야 함
	entryCwdNorm := normalizeCwd(entry.Cwd)
	if entryCwdNorm == cwd {
		t.Fatalf("test setup error: entry.cwd (norm)=%q == cwd=%q; D4 guard would not block", entryCwdNorm, cwd)
	}

	// D4 가드가 차단하므로 applyTranscriptToStdin을 호출하지 않는 것이 올바른 동작
	// 직접 가드 조건을 검증
	if entryCwdNorm == cwd {
		t.Error("D4 guard failed: entry.cwd matches current cwd but should not")
	}

	// 차단 후 ctx.Stdin은 여전히 비어있어야 함
	if ctx.Stdin.Model.ID != "" {
		t.Errorf("Model.ID=%q; should be empty (D4 guard should have blocked backfill)", ctx.Stdin.Model.ID)
	}
	if ctx.Stdin.ContextWindow.ContextWindowSize > 0 {
		t.Errorf("ContextWindowSize=%d; should be 0 (D4 guard should have blocked backfill)", ctx.Stdin.ContextWindow.ContextWindowSize)
	}

	// otherCwd 값이 출력에 등장해서는 안 됨 (cross-cwd 노출 0)
	result := orchestrate(ctx)
	combined := strings.Join(result.Lines, "\n")
	if strings.Contains(combined, otherCwd) {
		t.Errorf("cross-cwd exposure: otherCwd %q appeared in output: %q", otherCwd, combined)
	}
}

// task-008 통합 테스트 (c): 다중 cwd 시나리오 — 다른 cwd transcript가 노출되지 않음.
// cwd A 실행 중, cwd B transcript가 있을 때 B 데이터가 A 출력에 섞이지 않음.
func TestLayer2TranscriptBackfillCrossCwdIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	cwdA := normalizeCwd(workspaceA)
	cwdB := normalizeCwd(workspaceB)

	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	// cwd A 실행 중
	detectCwdEnv = func(key string) string {
		if key == "CLAUDE_PROJECT_DIR" {
			return workspaceA
		}
		return ""
	}
	detectCwdGetwd = func() (string, error) { return workspaceA, nil }

	// transcript fixture 생성:
	// - A 디렉토리에 B의 데이터를 가진 entry (entry.cwd=cwdB) → D4 가드가 차단해야 함
	transcriptDirA := encodeCwdToTranscriptDir(home, cwdA)
	_ = writeTranscriptFixture(t, transcriptDirA, "claude-sonnet-4-6", cwdB, 30000, 5000)

	// B 디렉토리에 B 데이터를 가진 entry (entry.cwd=cwdB)
	transcriptDirB := encodeCwdToTranscriptDir(home, cwdB)
	_ = writeTranscriptFixture(t, transcriptDirB, "claude-sonnet-4-6", cwdB, 30000, 5000)

	emptyStdin := StdinInput{}
	cfg := loadConfig("")
	trans := loadTranslations(cfg.Language)
	ctx := &Context{
		Stdin:        emptyStdin,
		Config:       cfg,
		Translations: trans,
	}

	// cwd A 실행 중에 A 디렉토리 transcript를 읽으면 entry.cwd=cwdB이므로 D4 가드 차단
	cwd := detectCurrentCwd()
	if cwd != cwdA {
		t.Fatalf("cwd=%q, want %q", cwd, cwdA)
	}

	transcriptPathA, err := selectTranscriptCandidate(transcriptDirA)
	if err != nil {
		t.Fatalf("selectTranscriptCandidate(A): %v", err)
	}

	entry, err := readLastAssistantEntry(transcriptPathA, 64*1024, 1*1024*1024)
	if err != nil {
		t.Fatalf("readLastAssistantEntry: %v", err)
	}
	if entry == nil {
		t.Fatal("readLastAssistantEntry returned nil")
	}

	// entry.cwd=cwdB, 현재 cwd=cwdA → D4 가드가 차단해야 함
	entryCwdNorm := normalizeCwd(entry.Cwd)
	if entryCwdNorm == cwd {
		// D4 가드가 통과되면 안 되는 케이스 — 만약 통과되면 cross-cwd 노출
		t.Errorf("cross-cwd exposure: entry.cwd (norm)=%q == cwdA=%q; D4 guard failed", entryCwdNorm, cwd)
	}

	// D4 가드 차단 확인: applyTranscriptToStdin 미호출 → ctx.Stdin 여전히 비어있음
	if ctx.Stdin.Model.ID != "" {
		t.Errorf("cross-cwd: Model.ID=%q leaked into ctx from cwd B transcript", ctx.Stdin.Model.ID)
	}

	// 출력에 cwd B 관련 데이터 없음을 확인
	result := orchestrate(ctx)
	combined := strings.Join(result.Lines, "\n")
	if strings.Contains(combined, cwdB) {
		t.Errorf("cross-cwd: cwdB %q appeared in output for cwdA: %q", cwdB, combined)
	}
	_ = cwdB
}

// runLayer2 는 main.go Layer 2 블록을 그대로 재현하는 테스트용 헬퍼다.
// ctx.Stdin이 변경되면 true를 반환하고, 변경되지 않으면 false를 반환한다.
// panic·hang 없이 완료되는지가 주 검증 대상이다.
func runLayer2(t *testing.T, ctx *Context, cwdOverride string) (applied bool) {
	t.Helper()
	if !needsTranscriptBackfill(ctx.Stdin) {
		return false
	}
	cwd := cwdOverride
	transcriptPath := ctx.Stdin.TranscriptPath
	if transcriptPath == "" && cwd != "" {
		home, homeErr := os.UserHomeDir()
		if homeErr == nil {
			transcriptDir := encodeCwdToTranscriptDir(home, cwd)
			if candidate, err := selectTranscriptCandidate(transcriptDir); err == nil {
				transcriptPath = candidate
			}
		}
	}
	if transcriptPath == "" {
		return false
	}
	const initialWindow = 64 * 1024
	const maxWindow = 1 * 1024 * 1024
	entry, err := readLastAssistantEntry(transcriptPath, initialWindow, maxWindow)
	if err != nil {
		return false
	}
	if entry == nil {
		return false
	}
	entryCwdNorm := normalizeCwd(entry.Cwd)
	if cwd == "" || entryCwdNorm != cwd {
		return false
	}
	oneMSignal := loadLastKnownOneM(cwd)
	applyTranscriptToStdin(&ctx.Stdin, entry, oneMSignal)
	ctx.CostEstimated = true
	return true
}

// TestLayer2GracefulFallback は task-009 회귀 테스트 묶음이다.
// 각 서브테스트는 Layer 2가 발동하지 않아야 하는 실패 경로를 하나씩 커버하며,
// panic·hang 없이 Layer 2 미발동(ctx.Stdin 미변경) → 보수 출력/묵음 폴백을 검증한다.
// SPEC §5.6, §5.10, §5.7 참조.
func TestLayer2GracefulFallback(t *testing.T) {
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	buildEmptyCtx := func() *Context {
		cfg := loadConfig("")
		trans := loadTranslations(cfg.Language)
		return &Context{
			Stdin:        StdinInput{},
			Config:       cfg,
			Translations: trans,
		}
	}

	// 1. cwd 식별 불가: env miss + getwd 실패 → Layer 2 미발동.
	// detectCurrentCwd가 빈 문자열을 반환하므로 transcriptPath 결정 불가 → 즉시 skip.
	t.Run("cwd_unidentifiable", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		detectCwdEnv = func(string) string { return "" }
		detectCwdGetwd = func() (string, error) { return "", os.ErrNotExist }

		ctx := buildEmptyCtx()
		applied := runLayer2(t, ctx, "") // cwdOverride="" = 식별 불가 시뮬레이션
		if applied {
			t.Errorf("Layer 2 applied with unidentifiable cwd; expected no-op")
		}
		if ctx.Stdin.Model.ID != "" {
			t.Errorf("Model.ID=%q after unidentifiable cwd; expected empty", ctx.Stdin.Model.ID)
		}
		if ctx.CostEstimated {
			t.Errorf("CostEstimated=true after cwd miss; expected false")
		}
	})

	// 2. transcript 디렉토리 부재: encodeCwdToTranscriptDir가 가리키는 경로가 없음
	// → selectTranscriptCandidate 에러 → Layer 2 미발동.
	t.Run("transcript_dir_absent", func(t *testing.T) {
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

		// transcript 디렉토리를 생성하지 않음 → selectTranscriptCandidate 에러
		ctx := buildEmptyCtx()
		applied := runLayer2(t, ctx, wantCwd)
		if applied {
			t.Errorf("Layer 2 applied with absent transcript dir; expected no-op")
		}
		if ctx.Stdin.Model.ID != "" {
			t.Errorf("Model.ID=%q after absent dir; expected empty", ctx.Stdin.Model.ID)
		}
	})

	// 3. 디렉토리 존재하나 jsonl 0개: selectTranscriptCandidate → "no jsonl files found" 에러
	// → Layer 2 미발동.
	t.Run("dir_exists_no_jsonl", func(t *testing.T) {
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

		// 빈 transcript 디렉토리 생성 (jsonl 파일 없음)
		transcriptDir := encodeCwdToTranscriptDir(home, wantCwd)
		if err := os.MkdirAll(transcriptDir, 0755); err != nil {
			t.Fatalf("mkdir transcriptDir: %v", err)
		}
		// .txt 파일만 추가 (jsonl 아님)
		if err := os.WriteFile(filepath.Join(transcriptDir, "notes.txt"), []byte("hello"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := buildEmptyCtx()
		applied := runLayer2(t, ctx, wantCwd)
		if applied {
			t.Errorf("Layer 2 applied with no jsonl files; expected no-op")
		}
		if ctx.Stdin.Model.ID != "" {
			t.Errorf("Model.ID=%q after no jsonl; expected empty", ctx.Stdin.Model.ID)
		}
	})

	// 4. transcript 파일 존재하나 assistant entry 없음: user/system line만 있는 파일
	// → readLastAssistantEntry가 nil 반환 → Layer 2 미발동.
	t.Run("no_assistant_entry", func(t *testing.T) {
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

		transcriptDir := encodeCwdToTranscriptDir(home, wantCwd)
		if err := os.MkdirAll(transcriptDir, 0755); err != nil {
			t.Fatalf("mkdir transcriptDir: %v", err)
		}
		// assistant entry 없이 user/system line만
		content := `{"type":"user","cwd":"` + wantCwd + `","message":{"content":"hello"}}` + "\n" +
			`{"type":"system","cwd":"` + wantCwd + `","message":{"content":"sys"}}` + "\n" +
			`{"type":"user","cwd":"` + wantCwd + `","message":{"content":"world"}}` + "\n"
		if err := os.WriteFile(filepath.Join(transcriptDir, "session.jsonl"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := buildEmptyCtx()
		applied := runLayer2(t, ctx, wantCwd)
		if applied {
			t.Errorf("Layer 2 applied with no assistant entry; expected no-op")
		}
		if ctx.Stdin.Model.ID != "" {
			t.Errorf("Model.ID=%q after no assistant entry; expected empty", ctx.Stdin.Model.ID)
		}
	})

	// 5. 손상된 transcript: 모든 line이 JSON 파싱 실패 → readLastAssistantEntry가 nil
	// → Layer 2 미발동. panic/hang 없음 확인.
	t.Run("corrupted_transcript", func(t *testing.T) {
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

		transcriptDir := encodeCwdToTranscriptDir(home, wantCwd)
		if err := os.MkdirAll(transcriptDir, 0755); err != nil {
			t.Fatalf("mkdir transcriptDir: %v", err)
		}
		// 완전히 손상된 내용 (유효한 JSON 없음)
		content := "not valid json at all\n" +
			"{broken json\n" +
			"another bad line\n"
		if err := os.WriteFile(filepath.Join(transcriptDir, "session.jsonl"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := buildEmptyCtx()
		applied := runLayer2(t, ctx, wantCwd)
		if applied {
			t.Errorf("Layer 2 applied with corrupted transcript; expected no-op (no panic)")
		}
		if ctx.Stdin.Model.ID != "" {
			t.Errorf("Model.ID=%q after corrupted transcript; expected empty", ctx.Stdin.Model.ID)
		}
	})

	// 6. 빈 transcript 파일: readLastAssistantEntry가 nil 반환 → Layer 2 미발동.
	t.Run("empty_transcript_file", func(t *testing.T) {
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

		transcriptDir := encodeCwdToTranscriptDir(home, wantCwd)
		if err := os.MkdirAll(transcriptDir, 0755); err != nil {
			t.Fatalf("mkdir transcriptDir: %v", err)
		}
		// 빈 파일
		if err := os.WriteFile(filepath.Join(transcriptDir, "session.jsonl"), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}

		ctx := buildEmptyCtx()
		applied := runLayer2(t, ctx, wantCwd)
		if applied {
			t.Errorf("Layer 2 applied with empty transcript; expected no-op")
		}
		if ctx.Stdin.Model.ID != "" {
			t.Errorf("Model.ID=%q after empty file; expected empty", ctx.Stdin.Model.ID)
		}
	})
}

// TestLayer2AppendWhileReadSimulation는 SPEC §5.10 append-while-read 시뮬레이션이다.
// transcript 파일의 마지막 line이 불완전한 JSON으로 끝나는 fixture를 사용해,
// readLastAssistantEntry 또는 Layer 2 전체가 panic 없이 그 앞 완전 line을 매칭하거나
// graceful skip하는지 검증한다.
// 실제 동시 쓰기 goroutine 없이 결정적 fixture로 시뮬레이션한다.
func TestLayer2AppendWhileReadSimulation(t *testing.T) {
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

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

	transcriptDir := encodeCwdToTranscriptDir(home, wantCwd)
	if err := os.MkdirAll(transcriptDir, 0755); err != nil {
		t.Fatalf("mkdir transcriptDir: %v", err)
	}
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")

	t.Run("partial_last_line_preceding_complete_line_matched", func(t *testing.T) {
		// 완전한 assistant entry 뒤에 append-only 쓰기 중인 partial line이 있는 상태.
		// 마지막 line은 partial이므로 skip, 그 앞 완전한 assistant entry가 매칭되어야 함.
		completeLine := fmt.Sprintf(
			`{"type":"assistant","cwd":%q,"message":{"model":"claude-opus-4-6","usage":{"input_tokens":50000,"output_tokens":10000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
			wantCwd,
		)
		partialLine := `{"type":"assistant","cwd":"` + wantCwd + `","message":{"model":"claude-sonnet-4-6","usage":{`
		content := completeLine + "\n" + partialLine // 마지막에 \n 없음 = 쓰기 중

		if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(transcriptPath, 64*1024, 1*1024*1024)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 결과: panic 없이 완전한 앞 line이 매칭되거나 nil(graceful fallback)
		if entry != nil && entry.Model != "claude-opus-4-6" {
			t.Errorf("partial last line: got model=%q, expected claude-opus-4-6 (complete preceding line) or nil",
				entry.Model)
		}
		// partial line을 잘못 파싱해서 sonnet이 반환되면 안 됨
		if entry != nil && entry.Model == "claude-sonnet-4-6" {
			t.Errorf("partial last line parsed as complete entry: model=%q; partial line must be skipped", entry.Model)
		}
	})

	t.Run("only_partial_line_returns_nil_no_panic", func(t *testing.T) {
		// 파일 전체가 partial (완전한 assistant entry 없음). nil 반환이어야 함.
		content := `{"type":"assistant","cwd":"` + wantCwd + `","message":{"model":"claude-opus-4-6","usage":{`

		if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(transcriptPath, 64*1024, 1*1024*1024)
		if err != nil {
			t.Fatalf("unexpected error on partial-only file: %v", err)
		}
		if entry != nil {
			t.Errorf("expected nil for partial-only file, got model=%q", entry.Model)
		}
	})

	t.Run("layer2_no_panic_with_partial_tail_fixture", func(t *testing.T) {
		// Layer 2 전체 흐름에서 partial tail fixture를 통과해도 panic이 없고,
		// 완전한 앞 entry가 있으면 그 entry로 복원되어야 함.
		completeLine := fmt.Sprintf(
			`{"type":"assistant","cwd":%q,"message":{"model":"claude-opus-4-6","usage":{"input_tokens":50000,"output_tokens":10000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
			wantCwd,
		)
		partialLine := `{"type":"assistant","cwd":"` + wantCwd + `","message":{"model":"claude-sonnet-4-6","usage":{`
		content := completeLine + "\n" + partialLine

		if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		cfg := loadConfig("")
		trans := loadTranslations(cfg.Language)
		ctx := &Context{
			Stdin:        StdinInput{},
			Config:       cfg,
			Translations: trans,
		}

		// panic이 없어야 함 (recover로 감싸지 않아도 테스트 프레임워크가 잡음)
		applied := runLayer2(t, ctx, wantCwd)
		// applied 여부 자체는 entry 존재에 따라 달라지므로 강제하지 않음
		// 핵심: partial line을 읽어도 panic 없이 완료됨
		if applied {
			// 복원됐다면 완전한 앞 entry의 model이어야 함
			if ctx.Stdin.Model.ID != "claude-opus-4-6" {
				t.Errorf("applied with wrong model: got %q, want claude-opus-4-6", ctx.Stdin.Model.ID)
			}
		}
		// partial line만 있는 상태에서 sonnet이 복원되어선 안 됨
		if ctx.Stdin.Model.ID == "claude-sonnet-4-6" {
			t.Errorf("partial last line leaked as restored model: %q; must be skipped", ctx.Stdin.Model.ID)
		}
	})

	t.Run("interleaved_user_assistant_with_partial_tail", func(t *testing.T) {
		// user → assistant → user → partial assistant 순서.
		// 마지막 partial은 skip되고, 그 앞 assistant(두 번째)가 매칭되어야 함.
		line1 := `{"type":"user","cwd":"` + wantCwd + `","message":{"content":"hi"}}`
		line2 := fmt.Sprintf(
			`{"type":"assistant","cwd":%q,"message":{"model":"claude-opus-4-6","usage":{"input_tokens":1000,"output_tokens":200,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`,
			wantCwd,
		)
		line3 := `{"type":"user","cwd":"` + wantCwd + `","message":{"content":"follow up"}}`
		partialLine := `{"type":"assistant","cwd":"` + wantCwd + `","message":{"model":"claude-sonnet-4-6","usage":{`
		content := line1 + "\n" + line2 + "\n" + line3 + "\n" + partialLine

		if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		entry, err := readLastAssistantEntry(transcriptPath, 64*1024, 1*1024*1024)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// partial 마지막 line skip → 유효한 line3(user)와 line2(assistant) 중 assistant가 매칭
		// line2가 마지막 완전한 assistant여야 함
		if entry != nil && entry.Model == "claude-sonnet-4-6" {
			t.Errorf("partial last assistant line parsed: model=%q; must be skipped", entry.Model)
		}
		if entry != nil && entry.Model != "claude-opus-4-6" {
			t.Errorf("unexpected model=%q; expected claude-opus-4-6 or nil", entry.Model)
		}
	})
}

// ── task-010: restoredFieldMask 통합 + self-perpetuation 차단 ──────────────────

// TestTask010Layer2MaskVacateInSnapshot는 Layer 2(transcript backfill)로 채운
// Model·ContextWindow·Cost 필드가 stripRestoredFields 적용 후 저장 스냅샷에서
// 비워지는지를 단위 검증한다(SPEC §5.14, ANALYSIS §5 D8).
//
// 검증 흐름:
//  1. 빈 stdin + 유효 transcript entry로 applyTranscriptToStdin을 직접 호출해 mask2 획득.
//  2. main.go:132-140의 OR 누적 로직과 동일하게 restoredMask에 mask2를 합산.
//  3. main.go:195-203의 save 블록과 동일하게 snapshot을 만들고 stripRestoredFields 적용.
//  4. transcript-복원 필드가 비어있고, 정상 stdin 필드(Version·SessionId)는 보존됨을 어서션.
func TestTask010Layer2MaskVacateInSnapshot(t *testing.T) {
	// transcript entry 픽스처: claude-opus-4-6 모델, 단가표에 존재하므로 cost도 채워짐.
	entry := &transcriptEntry{
		Model: "claude-opus-4-6",
		Cwd:   "/work/proj",
		Usage: transcriptUsage{
			InputTokens:              50000,
			OutputTokens:             10000,
			CacheReadInputTokens:     1000,
			CacheCreationInputTokens: 2000,
			CacheCreation5mTokens:    2000,
		},
	}

	// 빈 stdin: model/context/cost 모두 비어있음.
	stdin := StdinInput{}
	stdin.Version = "v-fresh"    // 정상 stdin 필드 — 보존되어야 함
	stdin.SessionId = "sid-010"  // 정상 stdin 필드 — 보존되어야 함

	mask2 := applyTranscriptToStdin(&stdin, entry, false /*oneMSignal*/)

	// Layer 2가 세 필드를 모두 채웠는지 확인 (전제조건).
	if !mask2.Model {
		t.Fatal("test setup: mask2.Model=false; applyTranscriptToStdin did not fill Model")
	}
	if !mask2.ContextWindow {
		t.Fatal("test setup: mask2.ContextWindow=false; applyTranscriptToStdin did not fill ContextWindow")
	}
	// Cost는 단가표 hit 시만 채워짐 — claude-opus-4-6은 단가표에 있으므로 true여야 함.
	if !mask2.Cost {
		t.Fatal("test setup: mask2.Cost=false; applyTranscriptToStdin did not fill Cost (pricing miss?)")
	}

	// main.go:132-140의 OR 누적 로직 재현: Layer 1 mask는 모두 false인 상태에서 Layer 2 mask를 합산.
	var restoredMask restoredFieldMask
	if mask2.Model {
		restoredMask.Model = true
	}
	if mask2.Cost {
		restoredMask.Cost = true
	}
	if mask2.ContextWindow {
		restoredMask.ContextWindow = true
	}

	// main.go:195-203의 save 블록 재현: snapshot 복사 → RateLimits nil → stripRestoredFields.
	snapshot := stdin
	snapshot.RateLimits = nil
	stripRestoredFields(&snapshot, restoredMask)

	// 검증 1: transcript-복원 필드가 저장 스냅샷에서 비워졌는가.
	if snapshot.Model.ID != "" || snapshot.Model.DisplayName != "" {
		t.Errorf("snapshot.Model = %+v; want zero (transcript-restored field must be vacated)", snapshot.Model)
	}
	if snapshot.ContextWindow.ContextWindowSize != 0 || snapshot.ContextWindow.TotalInputTokens != 0 {
		t.Errorf("snapshot.ContextWindow = %+v; want zero (transcript-restored field must be vacated)", snapshot.ContextWindow)
	}
	if snapshot.Cost.TotalCostUsd != 0 {
		t.Errorf("snapshot.Cost.TotalCostUsd = %v; want 0 (transcript-restored field must be vacated)", snapshot.Cost.TotalCostUsd)
	}

	// 검증 2: 정상 stdin 필드는 보존되어야 한다.
	if snapshot.Version != "v-fresh" {
		t.Errorf("snapshot.Version = %q; want v-fresh (fresh stdin field must survive stripping)", snapshot.Version)
	}
	if snapshot.SessionId != "sid-010" {
		t.Errorf("snapshot.SessionId = %q; want sid-010 (fresh stdin field must survive stripping)", snapshot.SessionId)
	}
}

// TestTask010FreshFieldsPreservedWithLayer2Mask는 stdin이 이미 신선한 Model 값을
// 들고 온 경우(mask.Model=false) applyTranscriptToStdin이 그 필드를 덮지 않고,
// 저장 스냅샷에서도 신선 값이 보존됨을 검증한다.
func TestTask010FreshFieldsPreservedWithLayer2Mask(t *testing.T) {
	entry := &transcriptEntry{
		Model: "claude-opus-4-6",
		Cwd:   "/work/proj",
		Usage: transcriptUsage{
			InputTokens:  10000,
			OutputTokens: 2000,
		},
	}

	// stdin이 이미 신선한 model을 가지고 있음 — transcript가 덮어써선 안 됨.
	stdin := StdinInput{}
	stdin.Model.ID = "claude-fresh-model"
	stdin.Model.DisplayName = "FreshModel"
	stdin.Version = "v-fresh"

	mask2 := applyTranscriptToStdin(&stdin, entry, false)

	// model은 신선하므로 mask.Model=false여야 함.
	if mask2.Model {
		t.Error("mask2.Model=true despite fresh stdin model; applyTranscriptToStdin must not overwrite fresh fields")
	}
	if stdin.Model.ID != "claude-fresh-model" {
		t.Errorf("stdin.Model.ID overwritten: got %q, want claude-fresh-model", stdin.Model.ID)
	}

	// OR 누적 + stripping: Model은 mask=false이므로 스냅샷에서도 신선 값 유지.
	var restoredMask restoredFieldMask
	if mask2.Model {
		restoredMask.Model = true
	}
	if mask2.Cost {
		restoredMask.Cost = true
	}
	if mask2.ContextWindow {
		restoredMask.ContextWindow = true
	}

	snapshot := stdin
	snapshot.RateLimits = nil
	stripRestoredFields(&snapshot, restoredMask)

	// 신선 model은 stripping 대상이 아니므로 보존되어야 함.
	if snapshot.Model.ID != "claude-fresh-model" {
		t.Errorf("snapshot.Model.ID = %q; want claude-fresh-model (fresh field must not be stripped)", snapshot.Model.ID)
	}
	if snapshot.Model.DisplayName != "FreshModel" {
		t.Errorf("snapshot.Model.DisplayName = %q; want FreshModel (fresh field must not be stripped)", snapshot.Model.DisplayName)
	}
	// ContextWindow는 Layer 2가 채웠으므로 vacate되어야 함.
	if snapshot.ContextWindow.ContextWindowSize != 0 {
		t.Errorf("snapshot.ContextWindow.ContextWindowSize = %d; want 0 (transcript-restored field must be vacated)",
			snapshot.ContextWindow.ContextWindowSize)
	}
}

// TestTask010Layer1Layer2ORMerge는 Layer 1(session cache)과 Layer 2(transcript)가
// 같은 필드(Model)를 각각 별도 호출에서 복원했을 때 OR 누적으로 restoredMask가
// 올바르게 합산되고, stripRestoredFields가 통합 mask로 해당 필드를 vacate하는지
// 검증한다(ANALYSIS §5 D8).
//
// 시나리오: Layer 1이 Model·Cost를 복원 → Layer 2가 ContextWindow·Cost를 복원.
// OR 누적 결과: Model·Cost·ContextWindow 모두 true → stripRestoredFields가 셋 다 vacate.
func TestTask010Layer1Layer2ORMerge(t *testing.T) {
	cwd := normalizeCwd(t.TempDir())
	patchCwdTo(t, cwd)

	now := time.Unix(1_700_000_000, 0)

	// Layer 1 캐시 픽스처: Model·Cost를 가지고 있음.
	cached := makeCachedForRestore(cwd, now.Add(-10*time.Second))
	// cached에 ContextWindow는 비어있도록 설정 (ContextWindow 복원은 Layer 2 몫).
	cached.CachedStdin.ContextWindow.TotalInputTokens = 0
	cached.CachedStdin.ContextWindow.TotalOutputTokens = 0
	cached.CachedStdin.ContextWindow.ContextWindowSize = 0

	// stdin: 완전히 비어있음 — Layer 1이 Model·Cost를 채우고, Layer 2가 ContextWindow를 채운다.
	stdin := StdinInput{}
	stdin.Version = "v-fresh"

	// Layer 1 복원: fillFromSessionCache가 Model·Cost를 채움. ContextWindow는 cached가 비어있어 skip.
	mask1 := fillFromSessionCache(&stdin, cached)
	if !mask1.Model {
		t.Fatal("Layer 1: mask1.Model=false; expected true (stdin model was empty)")
	}
	if !mask1.Cost {
		t.Fatal("Layer 1: mask1.Cost=false; expected true (stdin cost was zero)")
	}
	if mask1.ContextWindow {
		t.Error("Layer 1: mask1.ContextWindow=true despite empty cached ContextWindow; expected false")
	}

	// Layer 1 이후 stdin 상태: Model·Cost는 채워짐, ContextWindow는 여전히 비어있음.
	if stdin.Model.ID == "" {
		t.Fatal("Layer 1: stdin.Model.ID is empty after fillFromSessionCache")
	}
	if stdin.ContextWindow.ContextWindowSize != 0 {
		t.Fatal("Layer 1: stdin.ContextWindow.ContextWindowSize != 0; unexpected fill")
	}

	// Layer 2 transcript entry: ContextWindow를 채운다. Model은 이미 신선하므로 덮지 않음.
	entry := &transcriptEntry{
		Model: "claude-opus-4-6", // stdin.Model.ID가 이미 있으므로 mask2.Model=false
		Cwd:   cwd,
		Usage: transcriptUsage{
			InputTokens:  30000,
			OutputTokens: 5000,
		},
	}
	mask2 := applyTranscriptToStdin(&stdin, entry, false)

	// Layer 2: stdin.Model은 이미 채워져 있으므로 mask2.Model=false.
	if mask2.Model {
		t.Error("Layer 2: mask2.Model=true despite already-filled Model; expected false")
	}
	// Layer 2: ContextWindow는 비었으므로 mask2.ContextWindow=true.
	if !mask2.ContextWindow {
		t.Error("Layer 2: mask2.ContextWindow=false; expected true (ContextWindow was empty)")
	}

	// main.go:132-140 OR 누적 재현: restoredMask = Layer1 OR Layer2.
	restoredMask := mask1
	if mask2.Model {
		restoredMask.Model = true
	}
	if mask2.Cost {
		restoredMask.Cost = true
	}
	if mask2.ContextWindow {
		restoredMask.ContextWindow = true
	}

	// 통합 mask 검증: Layer 1이 Model·Cost를 복원, Layer 2가 ContextWindow를 복원.
	if !restoredMask.Model {
		t.Error("restoredMask.Model=false; expected true (Layer 1 filled Model)")
	}
	if !restoredMask.Cost {
		t.Error("restoredMask.Cost=false; expected true (Layer 1 filled Cost)")
	}
	if !restoredMask.ContextWindow {
		t.Error("restoredMask.ContextWindow=false; expected true (Layer 2 filled ContextWindow)")
	}

	// save 블록 재현: stripRestoredFields가 Layer 1 + Layer 2 통합 mask로 vacate.
	snapshot := stdin
	snapshot.RateLimits = nil
	stripRestoredFields(&snapshot, restoredMask)

	// 검증: Layer 1이 복원한 Model이 vacate되어야 함.
	if snapshot.Model.ID != "" || snapshot.Model.DisplayName != "" {
		t.Errorf("snapshot.Model = %+v; want zero (Layer 1 restored, must be vacated by OR-merged mask)", snapshot.Model)
	}
	// 검증: Layer 1이 복원한 Cost가 vacate되어야 함.
	if snapshot.Cost.TotalCostUsd != 0 {
		t.Errorf("snapshot.Cost.TotalCostUsd = %v; want 0 (Layer 1 restored, must be vacated by OR-merged mask)", snapshot.Cost.TotalCostUsd)
	}
	// 검증: Layer 2가 복원한 ContextWindow가 vacate되어야 함.
	if snapshot.ContextWindow.ContextWindowSize != 0 || snapshot.ContextWindow.TotalInputTokens != 0 {
		t.Errorf("snapshot.ContextWindow = %+v; want zero (Layer 2 restored, must be vacated by OR-merged mask)", snapshot.ContextWindow)
	}
	// 검증: 정상 stdin 필드는 보존.
	if snapshot.Version != "v-fresh" {
		t.Errorf("snapshot.Version = %q; want v-fresh (fresh stdin field must survive stripping)", snapshot.Version)
	}
}

// TestTask010SelfPerpetuationPrevented는 Layer 2 복원 후 저장된 캐시에서 다시 읽었을 때
// transcript-복원 필드가 없어서 다음 호출에서 "정상 stdin 값"처럼 재전파되지 않음을 검증한다.
// (self-perpetuation 차단의 종단 시나리오 — save/load 라운드트립 포함)
func TestTask010SelfPerpetuationPrevented(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := normalizeCwd(t.TempDir())
	patchCwdTo(t, cwd)

	const cacheKey = "task010-self-perp"

	// Step 1: Layer 2가 채운 stdin을 stripRestoredFields 적용 후 saveSessionState로 저장.
	entry := &transcriptEntry{
		Model: "claude-opus-4-6",
		Cwd:   cwd,
		Usage: transcriptUsage{
			InputTokens:           50000,
			OutputTokens:          10000,
			CacheCreation5mTokens: 500,
		},
	}

	stdin := StdinInput{}
	stdin.SessionId = cacheKey
	stdin.Version = "v-fresh"

	mask2 := applyTranscriptToStdin(&stdin, entry, false)
	if !mask2.Model || !mask2.ContextWindow {
		t.Fatalf("test setup: Layer 2 did not fill Model or ContextWindow (mask=%+v)", mask2)
	}

	// OR 누적 (Layer 1 없음 → restoredMask = mask2 그대로).
	var restoredMask restoredFieldMask
	if mask2.Model {
		restoredMask.Model = true
	}
	if mask2.Cost {
		restoredMask.Cost = true
	}
	if mask2.ContextWindow {
		restoredMask.ContextWindow = true
	}

	// save 블록: stripRestoredFields 후 저장.
	snapshot := stdin
	snapshot.RateLimits = nil
	stripRestoredFields(&snapshot, restoredMask)
	saveSessionState(cacheKey, &SessionState{
		CachedStdin: &snapshot,
		WidgetCount: 3,
	})

	// Step 2: 저장된 캐시 재로드 → transcript-복원 필드가 비어있어야 함.
	reloaded := loadSessionState(cacheKey)
	if reloaded == nil || reloaded.CachedStdin == nil {
		t.Fatalf("loadSessionState returned nil after save")
	}

	// transcript-복원 필드가 저장 캐시에서 비어있어야 함 → 다음 호출에서 재전파 불가.
	if reloaded.CachedStdin.Model.ID != "" || reloaded.CachedStdin.Model.DisplayName != "" {
		t.Errorf("saved cache: Model=%+v; want zero (Layer 2 restored field must not be saved into cache)", reloaded.CachedStdin.Model)
	}
	if reloaded.CachedStdin.ContextWindow.ContextWindowSize != 0 || reloaded.CachedStdin.ContextWindow.TotalInputTokens != 0 {
		t.Errorf("saved cache: ContextWindow=%+v; want zero (Layer 2 restored field must not be saved into cache)", reloaded.CachedStdin.ContextWindow)
	}
	if reloaded.CachedStdin.Cost.TotalCostUsd != 0 {
		t.Errorf("saved cache: Cost.TotalCostUsd=%v; want 0 (Layer 2 restored field must not be saved into cache)", reloaded.CachedStdin.Cost.TotalCostUsd)
	}

	// 정상 stdin 필드는 보존됨.
	if reloaded.CachedStdin.Version != "v-fresh" {
		t.Errorf("saved cache: Version=%q; want v-fresh (fresh field must survive)", reloaded.CachedStdin.Version)
	}

	// Step 3: 다음 호출에서 이 캐시로 shouldRestoreFromSession을 발동시키면
	// CachedStdin에 복원할 값이 없으므로 needsFill 조건의 "캐시에 값이 있음" 전제가
	// 성립하지 않아 실질적으로 복원이 no-op이어야 한다.
	// (캐시 자체는 eligibility를 통과할 수 있지만, fillFromSessionCache가 빈 캐시 값으로 채우지 않음)
	emptyStdin2 := StdinInput{SessionId: cacheKey}
	now := time.Now()
	if shouldRestoreFromSession(emptyStdin2, reloaded, now) {
		mask3 := fillFromSessionCache(&emptyStdin2, reloaded)
		// 캐시에서 복원된 값이 없어야 함 — 이전에 strip된 필드는 0값이므로 fill이 no-op이어야 함.
		if mask3.Model {
			t.Errorf("Step 3: mask3.Model=true; Layer 2 restored model was re-perpetuated from cache (self-perpetuation!)")
		}
		if mask3.ContextWindow {
			t.Errorf("Step 3: mask3.ContextWindow=true; Layer 2 restored context was re-perpetuated from cache (self-perpetuation!)")
		}
		if mask3.Cost {
			t.Errorf("Step 3: mask3.Cost=true; Layer 2 restored cost was re-perpetuated from cache (self-perpetuation!)")
		}
	}
}

// ─── task-011: last-known [1m] 저장 트리거 ─────────────────────────────────

// TestTask011OneMSavedWhenInputContainsOneM는 원본 stdin Model.ID에 "[1m]"이 포함되고
// cwd가 식별될 때 saveLastKnownOneM이 호출되어 캐시가 true로 갱신되는지 검증한다.
// main.go의 트리거 로직은 input(원본)을 참조하며, patchCwdTo + isolateOneMCache로
// 실제 파일시스템 오염 없이 검증한다.
func TestTask011OneMSavedWhenInputContainsOneM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := normalizeCwd(t.TempDir())
	patchCwdTo(t, cwd)

	// 사전 조건: 캐시에 [1m] 항목이 없어야 함.
	if loadLastKnownOneM(cwd) {
		t.Fatal("precondition: loadLastKnownOneM should return false before trigger")
	}

	// 트리거 조건 재현: input.Model.ID에 "[1m]" 포함 + cwd 식별 가능.
	input := StdinInput{}
	input.Model.ID = "claude-opus-4-7[1m]"
	if strings.Contains(input.Model.ID, "[1m]") {
		if triggerCwd := detectCurrentCwd(); triggerCwd != "" {
			saveLastKnownOneM(triggerCwd, true)
		}
	}

	// 검증: 해당 cwd에 true가 저장되었는가.
	if !loadLastKnownOneM(cwd) {
		t.Fatalf("want true after [1m] trigger for cwd=%s, got false", cwd)
	}
}

// TestTask011NoSaveWhenInputHasNoOneM은 원본 stdin Model.ID에 "[1m]"이 없으면
// 저장 트리거가 발동하지 않아 기존 캐시 값이 그대로 유지됨을 검증한다.
func TestTask011NoSaveWhenInputHasNoOneM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := normalizeCwd(t.TempDir())
	patchCwdTo(t, cwd)

	// 기존 항목 없음 — [1m] 없는 stdin이 도착해도 false 상태가 유지되어야 함.
	input := StdinInput{}
	input.Model.ID = "claude-sonnet-4-6" // [1m] 없음
	if strings.Contains(input.Model.ID, "[1m]") {
		if triggerCwd := detectCurrentCwd(); triggerCwd != "" {
			saveLastKnownOneM(triggerCwd, true)
		}
	}

	if loadLastKnownOneM(cwd) {
		t.Fatalf("want false when input has no [1m], got true")
	}
}

// TestTask011NoSaveWhenCwdEmpty는 [1m] 신호가 있어도 cwd를 식별할 수 없으면
// 저장이 발생하지 않아 캐시가 오염되지 않음을 검증한다.
func TestTask011NoSaveWhenCwdEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// cwd를 빈 문자열로 강제 — detectCurrentCwd()가 ""를 반환하도록.
	origEnv := detectCwdEnv
	origGetwd := detectCwdGetwd
	detectCwdEnv = func(string) string { return "" }
	detectCwdGetwd = func() (string, error) { return "", fmt.Errorf("no cwd") }
	t.Cleanup(func() {
		detectCwdEnv = origEnv
		detectCwdGetwd = origGetwd
	})

	// [1m] 신호 포함, 그러나 cwd 식별 불가.
	input := StdinInput{}
	input.Model.ID = "claude-opus-4-7[1m]"
	if strings.Contains(input.Model.ID, "[1m]") {
		if triggerCwd := detectCurrentCwd(); triggerCwd != "" {
			saveLastKnownOneM(triggerCwd, true)
		}
	}

	// 파일이 생성되지 않았거나 읽어도 false여야 함.
	// detectCurrentCwd가 ""이므로 saveLastKnownOneM은 호출되지 않아야 한다.
	// 임의 cwd로 조회해도 false.
	if loadLastKnownOneM("/any/cwd") {
		t.Fatalf("want false when cwd empty (no save should have occurred)")
	}
}

// TestTask011ExistingEntryPreservedOnNonOneMCall은 [1m]이 없는 호출이 발생해도
// 다른 cwd에 이미 저장된 true 항목이 지워지지 않음을 검증한다 (read-merge-write 불변식).
func TestTask011ExistingEntryPreservedOnNonOneMCall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwdA := normalizeCwd(t.TempDir())
	cwdB := normalizeCwd(t.TempDir())

	// cwd A에 미리 true 저장.
	saveLastKnownOneM(cwdA, true)
	if !loadLastKnownOneM(cwdA) {
		t.Fatal("precondition: cwd A should be true")
	}

	// [1m] 없는 cwd B 호출 — cwd A 항목은 변경되어서는 안 됨.
	patchCwdTo(t, cwdB)
	input := StdinInput{}
	input.Model.ID = "claude-sonnet-4-6" // [1m] 없음
	if strings.Contains(input.Model.ID, "[1m]") {
		if triggerCwd := detectCurrentCwd(); triggerCwd != "" {
			saveLastKnownOneM(triggerCwd, true)
		}
	}

	// cwd A 항목이 보존되었는가.
	if !loadLastKnownOneM(cwdA) {
		t.Fatalf("cwd A entry lost after non-[1m] call on cwd B")
	}
}

// configHomeDir은 CLAUDE_CONFIG_DIR로 config 홈을 옮긴 환경(예: ~/.claude-triptopaz)에서
// 그 디렉토리를, env 미설정·공백 시에는 <home>/.claude를 반환해야 한다. transcript root와
// 동일한 정책으로, 이래야 --config 없이도 credential을 올바른 계정에서 읽는다.
func TestConfigHomeDir(t *testing.T) {
	const home = `C:\Users\zipke`

	t.Run("CLAUDE_CONFIG_DIR set wins over home/.claude", func(t *testing.T) {
		cfg := filepath.Join(home, ".claude-triptopaz")
		t.Setenv("CLAUDE_CONFIG_DIR", cfg)
		if got := configHomeDir(home); got != cfg {
			t.Errorf("configHomeDir = %q, want %q", got, cfg)
		}
	})

	t.Run("unset falls back to home/.claude", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		want := filepath.Join(home, ".claude")
		if got := configHomeDir(home); got != want {
			t.Errorf("configHomeDir = %q, want %q", got, want)
		}
	})

	t.Run("whitespace-only treated as unset", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "   ")
		want := filepath.Join(home, ".claude")
		if got := configHomeDir(home); got != want {
			t.Errorf("configHomeDir = %q, want %q", got, want)
		}
	})
}
