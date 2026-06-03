package main

import (
	"strings"
	"testing"
)

// TestStdinFractionalPercentages는 Claude Code가 소수 백분율을 보낼 때
// (공식 status line 문서 스키마 예: rate_limits 23.5/41.2, context 8.4)
// 디코딩이 실패하지 않고 값이 보존되는지 검증한다. 과거 used_percentage를
// int로 모델링했을 때는 소수 입력이 전체 stdin 파싱을 실패시켜 상태줄이
// 통째로 사라졌다.
func TestStdinFractionalPercentages(t *testing.T) {
	payload := `{
		"model": {"id": "claude-opus-4-8", "display_name": "Opus"},
		"workspace": {"current_dir": "/tmp"},
		"context_window": {"context_window_size": 200000, "used_percentage": 8.4, "remaining_percentage": 91.6},
		"rate_limits": {
			"five_hour": {"used_percentage": 23.5, "resets_at": 0},
			"seven_day": {"used_percentage": 41.2, "resets_at": 0}
		}
	}`

	in := parseStdinReader(strings.NewReader(payload))

	if in.ContextWindow.UsedPercentage == nil || *in.ContextWindow.UsedPercentage != 8.4 {
		t.Fatalf("context used_percentage not preserved: %+v", in.ContextWindow.UsedPercentage)
	}
	if in.RateLimits == nil || in.RateLimits.FiveHour == nil {
		t.Fatal("rate_limits.five_hour lost after decode")
	}
	if in.RateLimits.FiveHour.UsedPercentage != 23.5 {
		t.Fatalf("five_hour used_percentage = %v, want 23.5", in.RateLimits.FiveHour.UsedPercentage)
	}
	if in.RateLimits.SevenDay.UsedPercentage != 41.2 {
		t.Fatalf("seven_day used_percentage = %v, want 41.2", in.RateLimits.SevenDay.UsedPercentage)
	}

	// 소수 입력에도 정체성 신호(model/workspace/context)가 살아있어 출력이
	// 억제되지 않아야 한다.
	if shouldSuppressOutput(in) {
		t.Fatal("fractional percentages must not blank the status line")
	}
}

// TestContextWidgetFractionalPercent는 소수 used_percentage가 정수로
// 절삭되어 위젯 데이터에 반영되는지 검증한다(문서 bash 예제의 cut -d. -f1과 동일).
func TestContextWidgetFractionalPercent(t *testing.T) {
	in := parseStdinReader(strings.NewReader(`{
		"context_window": {"context_window_size": 200000, "used_percentage": 8.9}
	}`))
	ctx := &Context{Stdin: in, Config: Config{Theme: "default"}}

	data, err := contextWidget{}.GetData(ctx)
	if err != nil || data == nil {
		t.Fatalf("GetData() = %v, %v", data, err)
	}
	if got := data.(*contextData).Percent; got != 8 {
		t.Fatalf("Percent = %d, want 8 (truncated from 8.9)", got)
	}
}
