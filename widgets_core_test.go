package main

import (
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
