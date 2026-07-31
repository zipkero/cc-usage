package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// newContextRenderCtx builds a minimal Context sufficient for
// contextWidget.Render — only Config.Theme is consulted by the renderer.
func newContextRenderCtx() *Context {
	return &Context{
		Config: Config{Theme: "default"},
	}
}

// splitContextRender splits a contextWidget.Render output into
// (barPart, percentPart, tokenPart). The render format is
// "%s %s%d%%%s %s%s%s" → three space-separated segments. bar/percent
// substrings never contain a literal ' ', and formatTokens never emits
// a space either, so a plain split is unambiguous.
func splitContextRender(t *testing.T, out string) (string, string, string) {
	t.Helper()
	parts := strings.SplitN(out, " ", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 space-separated segments, got %d in %q", len(parts), out)
	}
	return parts[0], parts[1], parts[2]
}

// TestContextWidgetGetData pins the placeholder-state judgment added in
// task-001: contextWidget.GetData returns nil when context_window_size <= 0
// regardless of total_input_tokens (SPEC §5.8), and otherwise sets
// contextData.Placeholder from total_input_tokens alone (SPEC §5.2, §5.6).
func TestContextWidgetGetData(t *testing.T) {
	w := contextWidget{}

	cases := []struct {
		name              string
		contextWindowSize int
		totalInputTokens  int
		wantNil           bool
		wantPlaceholder   bool
	}{
		{name: "size<=0, tokens=0 → nil (widget omitted)", contextWindowSize: 0, totalInputTokens: 0, wantNil: true},
		{name: "size<=0, tokens>0 → nil (widget omitted)", contextWindowSize: -1, totalInputTokens: 100, wantNil: true},
		{name: "size>0, tokens=0 → placeholder", contextWindowSize: 200_000, totalInputTokens: 0, wantPlaceholder: true},
		{name: "size>0, tokens>0 → measured", contextWindowSize: 200_000, totalInputTokens: 100, wantPlaceholder: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &Context{Stdin: StdinInput{}, Config: Config{Theme: "default"}}
			ctx.Stdin.ContextWindow.ContextWindowSize = tc.contextWindowSize
			ctx.Stdin.ContextWindow.TotalInputTokens = tc.totalInputTokens

			data, err := w.GetData(ctx)
			if err != nil {
				t.Fatalf("GetData returned error: %v", err)
			}
			if tc.wantNil {
				if data != nil {
					t.Fatalf("GetData = %v, want nil", data)
				}
				return
			}
			if data == nil {
				t.Fatalf("GetData = nil, want non-nil")
			}
			if got := data.(*contextData).Placeholder; got != tc.wantPlaceholder {
				t.Fatalf("Placeholder = %v, want %v", got, tc.wantPlaceholder)
			}
		})
	}
}

// TestContextWidgetRender_Placeholder pins the placeholder render assembly
// (task-001, ANALYSIS §5 D3/D4): "<empty bar> -" with the bar undimmed and
// only the "-" wrapped in dim, using ASCII-only placeholder chars.
// splitContextRender is not used here — it enforces the 3-segment measured
// format, and the placeholder render has only 2 segments.
func TestContextWidgetRender_Placeholder(t *testing.T) {
	w := contextWidget{}
	ctx := newContextRenderCtx()
	theme := themes["default"]

	out := w.Render(&contextData{Placeholder: true}, ctx)

	if !strings.Contains(out, theme.Dim) {
		t.Fatalf("render should contain theme.Dim before stripping ANSI: %q", out)
	}

	stripped := stripANSI(out)
	parts := strings.SplitN(stripped, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 2 space-separated segments (bar, placeholder), got %d in %q", len(parts), stripped)
	}
	bar, placeholder := parts[0], parts[1]

	if placeholder != placeholderChar {
		t.Fatalf("placeholder segment = %q, want %q", placeholder, placeholderChar)
	}
	if strings.ContainsFunc(stripped, func(r rune) bool { return r > '~' && r != '█' && r != '░' }) {
		t.Fatalf("placeholder render should be ASCII-only aside from bar glyphs: %q", stripped)
	}
	if strings.Contains(bar, "█") {
		t.Fatalf("empty bar should have no filled cells: %q", bar)
	}
	wantBarLen := ctx.Config.ContextBarWidth()
	if got := strings.Count(bar, "░"); got != wantBarLen {
		t.Fatalf("bar empty-cell count = %d, want %d in %q", got, wantBarLen, bar)
	}
}

func TestRenderProgressBarWidth(t *testing.T) {
	theme := themes["default"]
	cases := []struct {
		width      int
		percent    int
		wantFilled int
		wantEmpty  int
	}{
		// default width 8
		{width: 8, percent: 0, wantFilled: 0, wantEmpty: 8},
		{width: 8, percent: 50, wantFilled: 4, wantEmpty: 4},
		{width: 8, percent: 100, wantFilled: 8, wantEmpty: 0},
		// custom width honored — param threads through
		{width: 10, percent: 30, wantFilled: 3, wantEmpty: 7},
		{width: 20, percent: 50, wantFilled: 10, wantEmpty: 10},
		{width: 1, percent: 100, wantFilled: 1, wantEmpty: 0},
	}

	for _, tc := range cases {
		bar := stripANSI(renderProgressBar(tc.percent, tc.width, theme))
		filled := strings.Count(bar, "█")
		empty := strings.Count(bar, "░")
		if filled != tc.wantFilled {
			t.Fatalf("width %d percent %d filled = %d, want %d in %q", tc.width, tc.percent, filled, tc.wantFilled, bar)
		}
		if empty != tc.wantEmpty {
			t.Fatalf("width %d percent %d empty = %d, want %d in %q", tc.width, tc.percent, empty, tc.wantEmpty, bar)
		}
		if filled+empty != tc.width {
			t.Fatalf("width %d percent %d total = %d, want %d in %q", tc.width, tc.percent, filled+empty, tc.width, bar)
		}
	}
}

// TestModelWidgetSymbol pins the substring→symbol table added in task-002:
// each known model family keeps its existing symbol, the two new families
// (fable/mythos) get distinct symbols of their own, and an ID matching no
// entry falls back to defaultModelSymbol. It also confirms the pre-existing
// display-name precedence (ID wins over DisplayName unless ID is empty).
func TestModelWidgetSymbol(t *testing.T) {
	w := modelWidget{}
	ctx := newContextRenderCtx()

	cases := []struct {
		name        string
		id          string
		displayName string
		wantSymbol  string
		wantName    string
	}{
		{name: "opus keeps existing symbol", id: "claude-opus-4-6", wantSymbol: "◆", wantName: "claude-opus-4-6"},
		{name: "sonnet keeps existing symbol", id: "claude-sonnet-5", wantSymbol: "◇", wantName: "claude-sonnet-5"},
		{name: "haiku keeps existing symbol", id: "claude-haiku-4-5", wantSymbol: "○", wantName: "claude-haiku-4-5"},
		{name: "fable gets its own symbol", id: "claude-fable-5", wantSymbol: "◈", wantName: "claude-fable-5"},
		{name: "mythos gets its own symbol", id: "claude-mythos-5", wantSymbol: "◎", wantName: "claude-mythos-5"},
		{name: "unknown id falls back to default symbol", id: "claude-unknown-9", wantSymbol: "●", wantName: "claude-unknown-9"},
		{name: "empty id falls back to display name", id: "", displayName: "Custom Model", wantSymbol: "●", wantName: "Custom Model"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := stripANSI(w.Render(&modelData{ID: tc.id, DisplayName: tc.displayName}, ctx))
			if !strings.HasPrefix(out, tc.wantSymbol+" ") {
				t.Fatalf("Render(%q) = %q, want prefix %q", tc.id, out, tc.wantSymbol+" ")
			}
			if !strings.Contains(out, tc.wantName) {
				t.Fatalf("Render(%q) = %q, want it to contain name %q", tc.id, out, tc.wantName)
			}
		})
	}
}

// TestContextWidgetRender_TokenColor pins the token-color branching added
// in task-001: token chunk is wrapped in theme.Warning at >=256K, in
// theme.Danger at >=512K, and left bare below 256K. percent-color
// branching must remain independent of token-count branching.
func TestContextWidgetRender_TokenColor(t *testing.T) {
	w := contextWidget{}
	theme := themes["default"]

	t.Run("below warn threshold leaves token bare", func(t *testing.T) {
		ctx := newContextRenderCtx()
		out := w.Render(&contextData{Percent: 30, TotalTokens: 60_000}, ctx)
		_, _, tokenPart := splitContextRender(t, out)
		if strings.Contains(tokenPart, theme.Warning) {
			t.Errorf("token part should not contain Warning color: %q", tokenPart)
		}
		if strings.Contains(tokenPart, theme.Danger) {
			t.Errorf("token part should not contain Danger color: %q", tokenPart)
		}
	})

	t.Run("warn threshold boundary wraps with Warning", func(t *testing.T) {
		ctx := newContextRenderCtx()
		out := w.Render(&contextData{Percent: 30, TotalTokens: contextTokenWarn}, ctx)
		_, _, tokenPart := splitContextRender(t, out)
		if !strings.Contains(tokenPart, theme.Warning) {
			t.Errorf("token part should contain Warning color: %q", tokenPart)
		}
		if strings.Contains(tokenPart, theme.Danger) {
			t.Errorf("token part should not contain Danger color: %q", tokenPart)
		}
	})

	t.Run("danger threshold boundary wraps with Danger", func(t *testing.T) {
		ctx := newContextRenderCtx()
		out := w.Render(&contextData{Percent: 30, TotalTokens: contextTokenDanger}, ctx)
		_, _, tokenPart := splitContextRender(t, out)
		if !strings.Contains(tokenPart, theme.Danger) {
			t.Errorf("token part should contain Danger color: %q", tokenPart)
		}
		if strings.Contains(tokenPart, theme.Warning) {
			t.Errorf("token part should not contain Warning color: %q", tokenPart)
		}
	})

	t.Run("percent color remains based on percent", func(t *testing.T) {
		ctx := newContextRenderCtx()
		out := w.Render(&contextData{Percent: 90, TotalTokens: 60_000}, ctx)
		_, percentPart, tokenPart := splitContextRender(t, out)
		if !strings.Contains(percentPart, theme.Danger) {
			t.Errorf("percent part should contain Danger color: %q", percentPart)
		}
		if strings.Contains(tokenPart, theme.Danger) {
			t.Errorf("token part should not contain Danger color: %q", tokenPart)
		}
	})
}

// TestRateLimitWidgetsGetData pins the nil-narrowing added in task-002:
// rateLimit5hWidget/rateLimit7dWidget.GetData return measured data whenever
// the corresponding rate limit pointer is present — regardless of
// FirstResponseReceived() — and only consult the placeholder/nil judgment
// when the pointer is absent (SPEC §5.1, §5.4, §5.5; ANALYSIS §5 D2 실측값
// 우선순위). The "tokens=0, rate_limits 존재" case is the one that pins the
// priority itself: without it, a naive implementation could check
// FirstResponseReceived() before the pointer and wrongly show "5h: -" even
// though real data already arrived.
func TestRateLimitWidgetsGetData(t *testing.T) {
	const rateLimitsJSON = `"rate_limits": {"five_hour": {"used_percentage": 42, "resets_at": 0}, "seven_day": {"used_percentage": 69, "resets_at": 0}}`

	cases := []struct {
		name             string
		totalInputTokens int
		withRateLimits   bool
		wantNil          bool
		wantPlaceholder  bool
		want5h           int
		want7d           int
	}{
		{name: "tokens=0, rate_limits 부재 → placeholder", totalInputTokens: 0, withRateLimits: false, wantPlaceholder: true},
		{name: "tokens>0, rate_limits 부재 → nil (칸 생략)", totalInputTokens: 100, withRateLimits: false, wantNil: true},
		{name: "tokens=0, rate_limits 존재 → 실측 (D2 우선순위)", totalInputTokens: 0, withRateLimits: true, want5h: 42, want7d: 69},
		{name: "tokens>0, rate_limits 존재 → 실측", totalInputTokens: 100, withRateLimits: true, want5h: 42, want7d: 69},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts := []string{fmt.Sprintf(`"context_window": {"total_input_tokens": %d}`, tc.totalInputTokens)}
			if tc.withRateLimits {
				parts = append(parts, rateLimitsJSON)
			}
			payload := "{" + strings.Join(parts, ",") + "}"
			stdin := parseStdinReader(strings.NewReader(payload))
			ctx := &Context{Stdin: stdin, Config: Config{Theme: "default"}}

			for _, w := range []struct {
				name        string
				getData     func(*Context) (any, error)
				wantPercent int
			}{
				{name: "rateLimit5h", getData: rateLimit5hWidget{}.GetData, wantPercent: tc.want5h},
				{name: "rateLimit7d", getData: rateLimit7dWidget{}.GetData, wantPercent: tc.want7d},
			} {
				data, err := w.getData(ctx)
				if err != nil {
					t.Fatalf("%s: GetData returned error: %v", w.name, err)
				}
				if tc.wantNil {
					if data != nil {
						t.Fatalf("%s: GetData = %v, want nil", w.name, data)
					}
					continue
				}
				if data == nil {
					t.Fatalf("%s: GetData = nil, want non-nil", w.name)
				}
				rd := data.(*rateLimitData)
				if rd.Placeholder != tc.wantPlaceholder {
					t.Fatalf("%s: Placeholder = %v, want %v", w.name, rd.Placeholder, tc.wantPlaceholder)
				}
				if !tc.wantPlaceholder && rd.Percent != w.wantPercent {
					t.Fatalf("%s: Percent = %d, want %d", w.name, rd.Percent, w.wantPercent)
				}
			}
		})
	}
}

// TestRenderRateLimit_Placeholder pins the placeholder render assembly
// shared by both rate limit widgets (task-002, ANALYSIS §5 D3/D4): "<label>:
// -" with the label AND placeholder wrapped in dim together (unlike
// context, where only the "-" is dimmed) and no "(...)" remaining-time
// suffix, since placeholder state carries no reset time.
func TestRenderRateLimit_Placeholder(t *testing.T) {
	theme := themes["default"]
	ctx := &Context{Config: Config{Theme: "default"}}

	cases := []struct {
		label string
		want  string
	}{
		{label: "5h", want: "5h: -"},
		{label: "7d", want: "7d: -"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			out := renderRateLimit(&rateLimitData{Placeholder: true}, tc.label, ctx)

			if !strings.Contains(out, theme.Dim) {
				t.Fatalf("render should contain theme.Dim before stripping ANSI: %q", out)
			}
			// dim이 라벨까지 감싸야 한다 — Dim 코드가 라벨 문자보다 앞에
			// 와야 한다(placeholder만 감싸는 context와 달리 라벨+": "+
			// placeholder 전체를 감싸는 조립이다).
			dimIdx := strings.Index(out, theme.Dim)
			labelIdx := strings.Index(out, tc.label)
			if dimIdx == -1 || labelIdx == -1 || dimIdx > labelIdx {
				t.Fatalf("dim should wrap the label too, not just the placeholder char: %q", out)
			}

			stripped := stripANSI(out)
			if stripped != tc.want {
				t.Fatalf("stripped render = %q, want %q", stripped, tc.want)
			}
			if strings.ContainsFunc(stripped, func(r rune) bool { return r > '~' }) {
				t.Fatalf("placeholder render should be ASCII-only: %q", stripped)
			}
			if strings.Contains(stripped, "(") {
				t.Fatalf("placeholder render should not have a remaining-time suffix: %q", stripped)
			}
		})
	}
}

// TestRenderRateLimit_RemainingTimeSuffix pins the task-003 judgment move
// from a display-string comparison to a time calculation (SPEC §5.8, §5.10;
// ANALYSIS §5 D6, D7): R0 (resets_at absent/0, mapped by resetsAtTime to the
// Go zero time) and R1 (past, or under 60 seconds away) both suppress the
// "(...)" suffix exactly as before the change, and R2 (comfortably in the
// future) shows it with the same string format as before.
//
// R2 offsets carry a 30s margin past the hour/day boundary (2h+30s, 3d+30s)
// so that the render path's internal time.Now() call — which necessarily
// runs a little later than this test's — cannot shave the elapsed time back
// across the truncation boundary and flip the expected minute/hour digit.
// R1's 59s-future case needs no such margin: any additional delay only
// shrinks the remaining time further below the 60s display threshold, never
// pushes it across into "shown".
func TestRenderRateLimit_RemainingTimeSuffix(t *testing.T) {
	trans := &Translations{}
	trans.Time.Days = "d"
	trans.Time.Hours = "h"
	trans.Time.Minutes = "m"
	ctx := &Context{Config: Config{Theme: "default"}, Translations: trans}

	cases := []struct {
		name       string
		resetsAt   time.Time
		wantSuffix bool
		wantText   string // asserted only when wantSuffix
	}{
		{name: "R0 resets_at absent (zero time)", resetsAt: time.Time{}, wantSuffix: false},
		{name: "R0 resets_at=0 (via resetsAtTime)", resetsAt: resetsAtTime(0), wantSuffix: false},
		{name: "R1 past", resetsAt: time.Now().Add(-1 * time.Hour), wantSuffix: false},
		{name: "R1 59s future", resetsAt: time.Now().Add(59 * time.Second), wantSuffix: false},
		{name: "R2 2h future", resetsAt: time.Now().Add(2*time.Hour + 30*time.Second), wantSuffix: true, wantText: "2h0m"},
		{name: "R2 3d future", resetsAt: time.Now().Add(3*24*time.Hour + 30*time.Second), wantSuffix: true, wantText: "3d 0h"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := stripANSI(renderRateLimit(&rateLimitData{Percent: 42, ResetsAt: tc.resetsAt}, "5h", ctx))

			hasSuffix := strings.Contains(out, "(")
			if hasSuffix != tc.wantSuffix {
				t.Fatalf("suffix present = %v, want %v in %q", hasSuffix, tc.wantSuffix, out)
			}
			if tc.wantSuffix {
				want := fmt.Sprintf("5h: 42%% (%s)", tc.wantText)
				if out != want {
					t.Fatalf("render = %q, want %q", out, want)
				}
			}
		})
	}
}

// TestFormatTimeRemainingLocaleInvariance pins SPEC §5.9: the suppress/show
// judgment is unchanged by the Translations.Time.Minutes string, including
// an empty string — because the judgment now comes from int(diff.Minutes())
// in formatTimeRemaining rather than from comparing against a string built
// from that same locale value (ANALYSIS §5 D7).
func TestFormatTimeRemainingLocaleInvariance(t *testing.T) {
	minutesLabels := []string{"m", "분", " minutes", ""}

	for _, label := range minutesLabels {
		trans := &Translations{}
		trans.Time.Minutes = label
		ctx := &Context{Config: Config{Theme: "default"}, Translations: trans}

		t.Run(fmt.Sprintf("suppressed: absent, label=%q", label), func(t *testing.T) {
			out := stripANSI(renderRateLimit(&rateLimitData{Percent: 10, ResetsAt: time.Time{}}, "5h", ctx))
			if strings.Contains(out, "(") {
				t.Fatalf("suffix should be suppressed regardless of Minutes label %q, got %q", label, out)
			}
		})
		t.Run(fmt.Sprintf("suppressed: 59s future, label=%q", label), func(t *testing.T) {
			out := stripANSI(renderRateLimit(&rateLimitData{Percent: 10, ResetsAt: time.Now().Add(59 * time.Second)}, "5h", ctx))
			if strings.Contains(out, "(") {
				t.Fatalf("suffix should be suppressed regardless of Minutes label %q, got %q", label, out)
			}
		})
		t.Run(fmt.Sprintf("shown: 30m future, label=%q", label), func(t *testing.T) {
			out := stripANSI(renderRateLimit(&rateLimitData{Percent: 10, ResetsAt: time.Now().Add(30 * time.Minute)}, "5h", ctx))
			if !strings.Contains(out, "(") {
				t.Fatalf("suffix should be shown regardless of Minutes label %q, got %q", label, out)
			}
		})
	}
}

// TestFastModeWidgetGetData pins task-003's opt-in-only display rule
// (ANALYSIS §5 D3): fast_mode is rendered only when the pointer is present
// AND true. Both "key absent" (nil pointer) and explicit false omit the
// widget — fast mode defaults off, so neither case has anything to announce.
func TestFastModeWidgetGetData(t *testing.T) {
	w := fastModeWidget{}
	trueVal, falseVal := true, false

	cases := []struct {
		name     string
		fastMode *bool
		wantNil  bool
	}{
		{name: "key absent (nil) omits widget", fastMode: nil, wantNil: true},
		{name: "false omits widget", fastMode: &falseVal, wantNil: true},
		{name: "true renders widget", fastMode: &trueVal, wantNil: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &Context{Stdin: StdinInput{FastMode: tc.fastMode}}

			data, err := w.GetData(ctx)
			if err != nil {
				t.Fatalf("GetData returned error: %v", err)
			}
			if tc.wantNil && data != nil {
				t.Fatalf("GetData = %v, want nil", data)
			}
			if !tc.wantNil && data == nil {
				t.Fatalf("GetData = nil, want non-nil")
			}
		})
	}
}

// TestFastModeWidgetRender pins the label-only render assembly (ANALYSIS §5
// D3): the widget outputs the locale label alone, with no value suffix,
// since the widget's mere presence already carries the meaning.
func TestFastModeWidgetRender(t *testing.T) {
	w := fastModeWidget{}
	ctx := &Context{
		Config:       Config{Theme: "default"},
		Translations: loadTranslations("en"),
	}

	out := stripANSI(w.Render(true, ctx))
	if out != "fast" {
		t.Fatalf("Render = %q, want %q", out, "fast")
	}
}

// TestThinkingWidgetGetData pins task-004's "render whenever the key is
// present" rule (ANALYSIS §5 D3): unlike fastMode, thinking defaults to on,
// so both true and false render — only a nil pointer (key absent) omits the
// widget.
func TestThinkingWidgetGetData(t *testing.T) {
	w := thinkingWidget{}

	cases := []struct {
		name     string
		thinking *struct {
			Enabled bool `json:"enabled"`
		}
		wantNil bool
	}{
		{name: "key absent (nil) omits widget", thinking: nil, wantNil: true},
		{name: "enabled false still renders", thinking: &struct {
			Enabled bool `json:"enabled"`
		}{Enabled: false}, wantNil: false},
		{name: "enabled true renders", thinking: &struct {
			Enabled bool `json:"enabled"`
		}{Enabled: true}, wantNil: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &Context{Stdin: StdinInput{Thinking: tc.thinking}}

			data, err := w.GetData(ctx)
			if err != nil {
				t.Fatalf("GetData returned error: %v", err)
			}
			if tc.wantNil && data != nil {
				t.Fatalf("GetData = %v, want nil", data)
			}
			if !tc.wantNil && data == nil {
				t.Fatalf("GetData = nil, want non-nil")
			}
		})
	}
}

// TestThinkingWidgetRender pins the "<label>: on|off" assembly (ANALYSIS §5
// D3) — on/off are system identifiers and stay untranslated regardless of
// locale.
func TestThinkingWidgetRender(t *testing.T) {
	w := thinkingWidget{}
	ctx := &Context{
		Config:       Config{Theme: "default"},
		Translations: loadTranslations("en"),
	}

	cases := []struct {
		enabled bool
		want    string
	}{
		{enabled: true, want: "think: on"},
		{enabled: false, want: "think: off"},
	}
	for _, tc := range cases {
		out := stripANSI(w.Render(tc.enabled, ctx))
		if out != tc.want {
			t.Fatalf("Render(%v) = %q, want %q", tc.enabled, out, tc.want)
		}
	}
}

// TestEffortWidgetGetData pins task-004's "no off concept" rule (ANALYSIS §5
// D3): the widget renders only when effort.level is a non-empty string. A
// nil pointer (key absent) and an empty level both omit the widget.
func TestEffortWidgetGetData(t *testing.T) {
	w := effortWidget{}

	cases := []struct {
		name    string
		effort  *struct{ Level string }
		wantNil bool
	}{
		{name: "key absent (nil) omits widget", effort: nil, wantNil: true},
		{name: "empty level omits widget", effort: &struct{ Level string }{Level: ""}, wantNil: true},
		{name: "non-empty level renders", effort: &struct{ Level string }{Level: "high"}, wantNil: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &Context{}
			if tc.effort != nil {
				ctx.Stdin.Effort = &struct {
					Level string `json:"level"`
				}{Level: tc.effort.Level}
			}

			data, err := w.GetData(ctx)
			if err != nil {
				t.Fatalf("GetData returned error: %v", err)
			}
			if tc.wantNil && data != nil {
				t.Fatalf("GetData = %v, want nil", data)
			}
			if !tc.wantNil && data == nil {
				t.Fatalf("GetData = nil, want non-nil")
			}
		})
	}
}

// TestEffortWidgetRender pins the "<label>: <level>" assembly (ANALYSIS §5
// D3) — level is a system identifier (low/medium/high/xhigh/max) and stays
// untranslated regardless of locale.
func TestEffortWidgetRender(t *testing.T) {
	w := effortWidget{}
	ctx := &Context{
		Config:       Config{Theme: "default"},
		Translations: loadTranslations("en"),
	}

	out := stripANSI(w.Render("xhigh", ctx))
	if out != "effort: xhigh" {
		t.Fatalf("Render = %q, want %q", out, "effort: xhigh")
	}
}
