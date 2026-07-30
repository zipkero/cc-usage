package main

import (
	"fmt"
	"strings"
	"testing"
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
