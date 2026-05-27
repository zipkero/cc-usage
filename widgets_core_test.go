package main

import (
	"strings"
	"testing"
)

// newCostRenderCtx builds a minimal Context for costWidget.Render.
func newCostRenderCtx(estimated bool) *Context {
	return &Context{
		Config:        Config{Theme: "default"},
		CostEstimated: estimated,
	}
}

// TestCostWidgetRender_EstimatedMarker verifies the three cost rendering paths:
// (a) CostEstimated=true with valid cost → marker prefix included
// (b) CostEstimated=false (normal stdin) → no marker, output unchanged
// (c) CostEstimated=true with cost<=0 (pricing miss) → empty string (widget skip)
func TestCostWidgetRender_EstimatedMarker(t *testing.T) {
	w := costWidget{}
	theme := themes["default"]

	t.Run("(a) estimated with valid cost shows marker", func(t *testing.T) {
		ctx := newCostRenderCtx(true)
		out := w.Render(float64(3.14), ctx)
		if out == "" {
			t.Fatal("expected non-empty output for estimated cost")
		}
		if !strings.Contains(out, estimatedCostMarker) {
			t.Errorf("expected %q marker in output, got %q", estimatedCostMarker, out)
		}
		if !strings.Contains(out, "$3.14") {
			t.Errorf("expected formatted cost in output, got %q", out)
		}
		if !strings.HasPrefix(out, theme.Accent) {
			t.Errorf("output should start with theme.Accent color, got %q", out)
		}
		if !strings.HasSuffix(out, RESET) {
			t.Errorf("output should end with RESET, got %q", out)
		}
	})

	t.Run("(b) normal stdin (not estimated) no marker", func(t *testing.T) {
		ctx := newCostRenderCtx(false)
		out := w.Render(float64(3.14), ctx)
		if out == "" {
			t.Fatal("expected non-empty output for normal cost")
		}
		if strings.Contains(out, estimatedCostMarker) {
			t.Errorf("normal cost should not contain %q marker, got %q", estimatedCostMarker, out)
		}
		expected := theme.Accent + "$3.14" + RESET
		if out != expected {
			t.Errorf("expected %q, got %q", expected, out)
		}
	})

	t.Run("(b) normal stdin zero cost no marker", func(t *testing.T) {
		ctx := newCostRenderCtx(false)
		out := w.Render(float64(0), ctx)
		if strings.Contains(out, estimatedCostMarker) {
			t.Errorf("normal zero cost should not contain marker, got %q", out)
		}
		if out == "" {
			t.Error("normal zero cost should not skip widget")
		}
	})

	t.Run("(c) estimated with zero cost (miss) returns empty (widget skip)", func(t *testing.T) {
		ctx := newCostRenderCtx(true)
		out := w.Render(float64(0), ctx)
		if out != "" {
			t.Errorf("pricing miss should return empty string, got %q", out)
		}
	})

	t.Run("(c) estimated with negative cost (miss after clamp) returns empty", func(t *testing.T) {
		// GetData normalizes negative to 0; Render receives the normalized value.
		// Simulate what orchestrator sees after GetData runs.
		ctx := newCostRenderCtx(true)
		out := w.Render(float64(0), ctx)
		if out != "" {
			t.Errorf("estimated+zero should return empty string, got %q", out)
		}
	})

	t.Run("marker format is ~$X.XX", func(t *testing.T) {
		ctx := newCostRenderCtx(true)
		out := w.Render(float64(1.50), ctx)
		if !strings.Contains(out, "~$1.50") {
			t.Errorf("expected ~$1.50 in output, got %q", out)
		}
	})
}

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
		out := w.Render(&contextData{Percent: 30, TotalTokens: 256_000}, ctx)
		_, _, tokenPart := splitContextRender(t, out)
		if !strings.HasPrefix(tokenPart, theme.Warning) {
			t.Errorf("token part should start with Warning color, got %q", tokenPart)
		}
		if !strings.HasSuffix(tokenPart, RESET) {
			t.Errorf("token part should end with RESET, got %q", tokenPart)
		}
		if strings.Contains(tokenPart, theme.Danger) {
			t.Errorf("token part should not contain Danger color: %q", tokenPart)
		}
	})

	t.Run("warn interior wraps with Warning", func(t *testing.T) {
		ctx := newContextRenderCtx()
		out := w.Render(&contextData{Percent: 50, TotalTokens: 400_000}, ctx)
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
		out := w.Render(&contextData{Percent: 70, TotalTokens: 512_000}, ctx)
		_, _, tokenPart := splitContextRender(t, out)
		if !strings.HasPrefix(tokenPart, theme.Danger) {
			t.Errorf("token part should start with Danger color, got %q", tokenPart)
		}
		if strings.Contains(tokenPart, theme.Warning) {
			t.Errorf("token part should not contain Warning color: %q", tokenPart)
		}
	})

	t.Run("danger interior wraps with Danger", func(t *testing.T) {
		ctx := newContextRenderCtx()
		out := w.Render(&contextData{Percent: 80, TotalTokens: 800_000}, ctx)
		_, _, tokenPart := splitContextRender(t, out)
		if !strings.Contains(tokenPart, theme.Danger) {
			t.Errorf("token part should contain Danger color: %q", tokenPart)
		}
		if strings.Contains(tokenPart, theme.Warning) {
			t.Errorf("token part should not contain Warning color: %q", tokenPart)
		}
	})

	t.Run("percent color independent of token color", func(t *testing.T) {
		ctx := newContextRenderCtx()
		low := w.Render(&contextData{Percent: 30, TotalTokens: 60_000}, ctx)
		high := w.Render(&contextData{Percent: 30, TotalTokens: 400_000}, ctx)
		_, lowPercent, _ := splitContextRender(t, low)
		_, highPercent, _ := splitContextRender(t, high)
		if lowPercent != highPercent {
			t.Errorf("percent part should not depend on TotalTokens, low=%q high=%q", lowPercent, highPercent)
		}
	})

	t.Run("200K model fixture stays bare", func(t *testing.T) {
		ctx := newContextRenderCtx()
		// percent=90 (Danger zone for getColorForPercent) but totalTokens
		// well below contextTokenWarn — token part must not pick up
		// Warning/Danger because of the percent color.
		out := w.Render(&contextData{Percent: 90, TotalTokens: 180_000}, ctx)
		_, _, tokenPart := splitContextRender(t, out)
		if strings.Contains(tokenPart, theme.Warning) {
			t.Errorf("token part should not contain Warning color: %q", tokenPart)
		}
		// theme.Danger == theme.Danger; the percent chunk uses Danger but
		// that's in percentPart, not tokenPart. The split isolates them.
		if strings.Contains(tokenPart, theme.Danger) {
			t.Errorf("token part should not contain Danger color: %q", tokenPart)
		}
	})
}
