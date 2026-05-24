package main

import (
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

// v0.3.5: 빈 stdin이 들어와 degraded restore가 발화할 때 cost / context뿐 아니라
// model까지 캐시에서 복원돼야 한다. 그렇지 않으면 fallback 캐시가 살아 있어도
// 모델 위젯이 사라져 partial status line으로 보인다.
func TestRestoreUsageFieldsRestoresModel(t *testing.T) {
	cached := &StdinInput{}
	cached.Model.ID = "claude-opus-4-7"
	cached.Model.DisplayName = "Opus 4.7"
	cached.Cost.TotalCostUsd = 1.25
	cached.ContextWindow.TotalInputTokens = 50000
	cached.ContextWindow.ContextWindowSize = 200000

	t.Run("empty stdin restores all three", func(t *testing.T) {
		stdin := StdinInput{}
		restoreUsageFields(&stdin, cached)
		if stdin.Model.ID != "claude-opus-4-7" {
			t.Fatalf("model.id = %q, want claude-opus-4-7", stdin.Model.ID)
		}
		if stdin.Model.DisplayName != "Opus 4.7" {
			t.Fatalf("model.display_name = %q, want Opus 4.7", stdin.Model.DisplayName)
		}
		if stdin.Cost.TotalCostUsd != 1.25 {
			t.Fatalf("cost = %.4f, want 1.25", stdin.Cost.TotalCostUsd)
		}
		if stdin.ContextWindow.TotalInputTokens != 50000 {
			t.Fatalf("ctx tokens = %d, want 50000", stdin.ContextWindow.TotalInputTokens)
		}
	})

	t.Run("fresh model.id wins", func(t *testing.T) {
		stdin := StdinInput{}
		stdin.Model.ID = "claude-haiku-fresh"
		restoreUsageFields(&stdin, cached)
		if stdin.Model.ID != "claude-haiku-fresh" {
			t.Fatalf("model.id = %q, want fresh claude-haiku-fresh", stdin.Model.ID)
		}
		if stdin.Model.DisplayName != "" {
			t.Fatalf("model.display_name = %q, want empty (fresh model.id present)", stdin.Model.DisplayName)
		}
	})

	t.Run("fresh model.display_name wins", func(t *testing.T) {
		stdin := StdinInput{}
		stdin.Model.DisplayName = "Custom"
		restoreUsageFields(&stdin, cached)
		if stdin.Model.DisplayName != "Custom" {
			t.Fatalf("model.display_name = %q, want fresh Custom", stdin.Model.DisplayName)
		}
		if stdin.Model.ID != "" {
			t.Fatalf("model.id = %q, want empty (fresh model.display_name present)", stdin.Model.ID)
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
		if stdin.Model.ID != "" || stdin.Cost.TotalCostUsd != 0 {
			t.Fatalf("nil cached mutated stdin: %#v", stdin)
		}
	})

	t.Run("nil stdin is no-op", func(t *testing.T) {
		// 패닉만 안 나면 통과.
		restoreUsageFields(nil, cached)
	})
}
