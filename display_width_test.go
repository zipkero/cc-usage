package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestDisplayWidth pins the display-width measurement layer (task-009, SPEC
// §5.7, ANALYSIS §5 D8): ANSI CSI color codes and OSC 8 hyperlinks count as
// 0, Hangul/CJK/fullwidth runes count as 2, and everything else — including
// the East Asian Ambiguous glyphs this renderer already emits — counts as 1.
func TestDisplayWidth(t *testing.T) {
	theme := themes["default"]

	cases := []struct {
		name string
		s    string
		want int
	}{
		{"ASCII text", "hello", 5},
		{"empty string", "", 0},
		{"ANSI SGR color code contributes 0", theme.Model + "AB" + RESET, 2},
		{"OSC 8 link contributes 0, only visible text counts", osc8Link("https://example.com/pr/42", "text"), 4},
		{"Hangul syllables are 2 columns each", "한글", 4},
		{"CJK ideographs are 2 columns each", "中文", 4},
		{"fullwidth form is 2 columns", "Ａ", 2}, // fullwidth "A"
		{"emoji (Emoticons block) is 2 columns", "\U0001F600", 2},
		{"bar glyphs (Ambiguous) are 1 column each", "██░░", 4},
	}

	// The renderer's existing East Asian Ambiguous glyphs must all stay 1
	// column — this is the assumption that keeps their current rendering
	// unchanged (ANALYSIS §5 D8).
	for _, g := range []string{"◆", "◇", "○", "●", "◈", "◎", "█", "░", "│", "↑", "↓", "…", "✓", "✗", "✎"} {
		cases = append(cases, struct {
			name string
			s    string
			want int
		}{"existing render glyph " + g + " is 1 column", g, 1})
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := displayWidth(c.s); got != c.want {
				t.Fatalf("displayWidth(%q) = %d, want %d", c.s, got, c.want)
			}
		})
	}
}

// TestDisplayWidthOSC8Filtering closes a gap TestPullRequestWidget's "번호만"
// case (widgets_project_test.go) left open: asserting rendered contains
// "#42" doesn't prove the OSC 8 wrapper is actually absent, since "#42" is a
// substring of the wrapped form too. This locks both directions directly —
// the no-URL render must not contain an OSC 8 marker at all, the with-URL
// render must, and displayWidth treats both as exactly the width of the
// visible "#<number>" text either way, confirming the wrapper itself never
// contributes to display width (task-007/task-009, SPEC §5.7).
func TestDisplayWidthOSC8Filtering(t *testing.T) {
	w := pullRequestWidget{}

	noURLCtx := &Context{Stdin: StdinInput{}, Config: Config{Theme: "default"}}
	noURLCtx.Stdin.PR = &prField{Number: 42}
	noURLData, err := w.GetData(noURLCtx)
	if err != nil {
		t.Fatalf("GetData (no URL) error: %v", err)
	}
	noURLRendered := w.Render(noURLData, noURLCtx)
	if strings.Contains(noURLRendered, "\x1b]8;;") {
		t.Fatalf("Render without URL = %q, must not contain an OSC 8 sequence", noURLRendered)
	}

	withURLCtx := &Context{Stdin: StdinInput{}, Config: Config{Theme: "default"}}
	withURLCtx.Stdin.PR = &prField{Number: 42, URL: "https://github.com/zipkero/cc-usage/pull/42"}
	withURLData, err := w.GetData(withURLCtx)
	if err != nil {
		t.Fatalf("GetData (with URL) error: %v", err)
	}
	withURLRendered := w.Render(withURLData, withURLCtx)
	if !strings.Contains(withURLRendered, "\x1b]8;;") {
		t.Fatalf("Render with URL = %q, want it to contain an OSC 8 sequence", withURLRendered)
	}

	wantWidth := displayWidth("#42")
	if got := displayWidth(noURLRendered); got != wantWidth {
		t.Fatalf("displayWidth(no URL render) = %d, want %d (visible text only)", got, wantWidth)
	}
	if got := displayWidth(withURLRendered); got != wantWidth {
		t.Fatalf("displayWidth(with URL render) = %d, want %d (OSC 8 wrapper must contribute 0)", got, wantWidth)
	}
}

// TestDisplayWidthKoreanNameAndBranch covers a Korean project name + branch
// rendered through the real projectName widget, so the 3-OS CI case SPEC
// §5.7's verification calls for is backed by an actual widget render, not
// just a raw string literal. The expected width is hand-derived (not routed
// through displayWidth itself, so the test doesn't just check the function
// against its own output) and truncation is checked for valid UTF-8 and a
// closing RESET.
func TestDisplayWidthKoreanNameAndBranch(t *testing.T) {
	w := projectNameWidget{}
	ctx := &Context{Config: Config{Theme: "default"}}
	data := &projectNameData{Name: "한글프로젝트", Branch: "메인-브랜치"}
	rendered := w.Render(data, ctx)

	// Visible text: "한글프로젝트" (6 Hangul syllables × 2 = 12) + " (" (2) +
	// "메인-브랜치" (5 Hangul × 2 + 1 hyphen = 11) + ")" (1) = 26.
	const want = 26
	if got := displayWidth(rendered); got != want {
		t.Fatalf("displayWidth(%q) = %d, want %d", rendered, got, want)
	}

	truncated := truncateToWidth(rendered, 10)
	if !utf8.ValidString(truncated) {
		t.Fatalf("truncateToWidth produced invalid UTF-8: %q", truncated)
	}
	if !strings.HasSuffix(truncated, RESET) {
		t.Fatalf("truncateToWidth result = %q, want it to end with RESET", truncated)
	}
	if got := displayWidth(truncated); got > 10 {
		t.Fatalf("displayWidth(truncated) = %d, want <= 10", got)
	}
}

// TestFitLineWidth pins the line-fitting layer's four cases (SPEC §5.7;
// ANALYSIS §5 D7): ample space (no change), dropping one widget from the
// right, dropping several, and — when a single remaining widget still
// doesn't fit — truncating it by display width with a trailing RESET so no
// color code is left open.
func TestFitLineWidth(t *testing.T) {
	theme := themes["default"]

	cases := []struct {
		name      string
		parts     []string
		sep       string
		maxWidth  int
		wantLine  string
		wantCount int
	}{
		{
			name:      "ample space: nothing dropped",
			parts:     []string{"A", "B", "C"},
			sep:       "|",
			maxWidth:  5, // exact width of "A|B|C"
			wantLine:  "A|B|C",
			wantCount: 3,
		},
		{
			name:      "drop one widget from the right",
			parts:     []string{"AAAA", "BBBB", "CCCC"},
			sep:       "|",
			maxWidth:  9, // exact width of "AAAA|BBBB"
			wantLine:  "AAAA|BBBB",
			wantCount: 2,
		},
		{
			name:      "drop several widgets from the right",
			parts:     []string{"AAAA", "BBBB", "CCCC", "DDDD"},
			sep:       "|",
			maxWidth:  9, // only the leftmost two survive
			wantLine:  "AAAA|BBBB",
			wantCount: 2,
		},
		{
			name:      "single remaining widget still overflows: truncated with ellipsis + RESET",
			parts:     []string{"ABCDEFGH"},
			sep:       "|",
			maxWidth:  5,
			wantLine:  "ABCD…" + RESET,
			wantCount: 1,
		},
		{
			name:      "truncation closes a color left open by the cut widget",
			parts:     []string{theme.Model + "ABCDEFGH" + RESET},
			sep:       "|",
			maxWidth:  5,
			wantLine:  theme.Model + "ABCD…" + RESET,
			wantCount: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotLine, gotCount := fitLineWidth(c.parts, c.sep, c.maxWidth)
			if gotLine != c.wantLine {
				t.Fatalf("fitLineWidth() line = %q, want %q", gotLine, c.wantLine)
			}
			if gotCount != c.wantCount {
				t.Fatalf("fitLineWidth() count = %d, want %d", gotCount, c.wantCount)
			}
			if got := displayWidth(gotLine); got > c.maxWidth {
				t.Fatalf("displayWidth(result) = %d, want <= %d", got, c.maxWidth)
			}
		})
	}
}

// TestOrchestrateColumnsFitting wires Context.Columns through orchestrate()
// end to end using real widgets (model/context/cost), confirming the
// integration — not just fitLineWidth in isolation — drops widgets from the
// right in priority order and truncates only as a last resort (SPEC §5.7;
// ANALYSIS §2 step 8, §5 D7). Thresholds are derived from the widgets' own
// rendered widths (via orchestrate with Columns=0, the untouched baseline
// path) rather than hardcoded, since exact ANSI-wrapped widths depend on the
// theme's color codes.
func TestOrchestrateColumnsFitting(t *testing.T) {
	newBaseCtx := func(lines [][]string) *Context {
		ctx := &Context{
			Config: Config{
				Theme: "default", Separator: "space", DisplayMode: "custom",
				Lines: lines,
			},
			Translations: loadTranslations("en"),
		}
		ctx.Stdin.Model.ID = "claude-opus-4-6"
		ctx.Stdin.ContextWindow.ContextWindowSize = 200000
		ctx.Stdin.ContextWindow.TotalInputTokens = 50000
		ctx.Stdin.Cost.TotalCostUsd = 1.25
		return ctx
	}

	all := orchestrate(newBaseCtx([][]string{{"model", "context", "cost"}}))
	modelContext := orchestrate(newBaseCtx([][]string{{"model", "context"}}))
	modelOnly := orchestrate(newBaseCtx([][]string{{"model"}}))
	if len(all.Lines) != 1 || len(modelContext.Lines) != 1 || len(modelOnly.Lines) != 1 {
		t.Fatalf("expected one rendered line per baseline config")
	}
	lineAll, lineMC, lineM := all.Lines[0], modelContext.Lines[0], modelOnly.Lines[0]

	t.Run("ample: Columns exactly fits the full line", func(t *testing.T) {
		ctx := newBaseCtx([][]string{{"model", "context", "cost"}})
		ctx.Columns = displayWidth(lineAll)
		result := orchestrate(ctx)
		if result.Lines[0] != lineAll {
			t.Fatalf("Lines[0] = %q, want unchanged %q", result.Lines[0], lineAll)
		}
		if result.WidgetCount != 3 {
			t.Fatalf("WidgetCount = %d, want 3", result.WidgetCount)
		}
	})

	t.Run("drop rightmost widget (cost)", func(t *testing.T) {
		ctx := newBaseCtx([][]string{{"model", "context", "cost"}})
		ctx.Columns = displayWidth(lineMC) // too narrow for cost, exact fit without it
		result := orchestrate(ctx)
		if result.Lines[0] != lineMC {
			t.Fatalf("Lines[0] = %q, want %q (cost dropped)", result.Lines[0], lineMC)
		}
		if result.WidgetCount != 2 {
			t.Fatalf("WidgetCount = %d, want 2", result.WidgetCount)
		}
		if got := displayWidth(result.Lines[0]); got > ctx.Columns {
			t.Fatalf("displayWidth(result) = %d, want <= %d", got, ctx.Columns)
		}
	})

	t.Run("drop several widgets (context and cost)", func(t *testing.T) {
		ctx := newBaseCtx([][]string{{"model", "context", "cost"}})
		ctx.Columns = displayWidth(lineM) // only model fits
		result := orchestrate(ctx)
		if result.Lines[0] != lineM {
			t.Fatalf("Lines[0] = %q, want %q (context and cost dropped)", result.Lines[0], lineM)
		}
		if result.WidgetCount != 1 {
			t.Fatalf("WidgetCount = %d, want 1", result.WidgetCount)
		}
	})

	t.Run("single widget still overflows: truncated with trailing RESET", func(t *testing.T) {
		ctx := newBaseCtx([][]string{{"model", "context", "cost"}})
		ctx.Columns = displayWidth(lineM) - 3 // narrower than even model alone
		result := orchestrate(ctx)
		if got := displayWidth(result.Lines[0]); got > ctx.Columns {
			t.Fatalf("displayWidth(result) = %d, want <= %d", got, ctx.Columns)
		}
		if !strings.HasSuffix(result.Lines[0], RESET) {
			t.Fatalf("Lines[0] = %q, want it to end with RESET", result.Lines[0])
		}
		if result.WidgetCount != 1 {
			t.Fatalf("WidgetCount = %d, want 1 (truncated, not dropped)", result.WidgetCount)
		}
	})

	t.Run("Columns unset (0) matches the untouched baseline path byte for byte", func(t *testing.T) {
		ctx := newBaseCtx([][]string{{"model", "context", "cost"}})
		ctx.Columns = 0
		result := orchestrate(ctx)
		if result.Lines[0] != lineAll {
			t.Fatalf("Lines[0] = %q, want %q (Columns=0 must not engage fitting)", result.Lines[0], lineAll)
		}
	})
}

// TestOrchestrateColumnsPreservesExplicitBarWidth pins ANALYSIS §5 D7: an
// explicit widgets.context.barWidth is never resized by Columns fitting.
// fitLineWidth only ever shrinks or truncates the already-rendered line —
// it never re-invokes GetData/Render — so the bar's cell count is identical
// whether Columns is unset or set to an ample (non-constraining) value.
func TestOrchestrateColumnsPreservesExplicitBarWidth(t *testing.T) {
	newCtx := func(columns int) *Context {
		ctx := &Context{
			Config: Config{
				Theme: "default", Separator: "space", DisplayMode: "custom",
				Lines:   [][]string{{"context"}},
				Widgets: WidgetConfig{Context: ContextWidgetConfig{BarWidth: 20}},
			},
			Translations: loadTranslations("en"),
			Columns:      columns,
		}
		ctx.Stdin.ContextWindow.ContextWindowSize = 200000
		ctx.Stdin.ContextWindow.TotalInputTokens = 50000
		return ctx
	}

	barCellCount := func(line string) int {
		stripped := stripANSI(line)
		return strings.Count(stripped, "█") + strings.Count(stripped, "░")
	}

	withoutColumns := orchestrate(newCtx(0))
	if len(withoutColumns.Lines) != 1 {
		t.Fatalf("Lines = %v, want one line", withoutColumns.Lines)
	}
	if got := barCellCount(withoutColumns.Lines[0]); got != 20 {
		t.Fatalf("bar cell count (Columns=0) = %d, want 20", got)
	}

	ample := orchestrate(newCtx(displayWidth(withoutColumns.Lines[0]) + 10))
	if got := barCellCount(ample.Lines[0]); got != 20 {
		t.Fatalf("bar cell count (ample Columns) = %d, want 20 — explicit barWidth must survive width fitting", got)
	}
}
