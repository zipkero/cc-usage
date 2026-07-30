package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestResolvePresetParsesChars verifies that resolvePreset splits the preset
// string on "|" into lines and maps each char to its widget ID, while also
// forcing DisplayMode to "custom".
func TestResolvePresetParsesChars(t *testing.T) {
	cfg := &Config{Preset: "PN|M$C"}
	resolvePreset(cfg)

	want := [][]string{
		{"projectInfo", "projectName"},
		{"model", "cost", "context"},
	}
	if !reflect.DeepEqual(cfg.Lines, want) {
		t.Fatalf("Lines mismatch: got %v, want %v", cfg.Lines, want)
	}
	if cfg.DisplayMode != "custom" {
		t.Fatalf("DisplayMode: got %q, want %q", cfg.DisplayMode, "custom")
	}
}

func TestProjectNameSelectionMechanisms(t *testing.T) {
	t.Setenv("PATH", "")

	t.Run("preset N renders projectName", func(t *testing.T) {
		ctx := &Context{
			Stdin: StdinInput{},
			Config: Config{
				Preset:    "N",
				Theme:     "default",
				Separator: "space",
			},
			Translations: loadTranslations("en"),
		}
		ctx.Stdin.Workspace.CurrentDir = "/tmp/cc-usage"

		result := orchestrate(ctx)
		if result.WidgetCount != 1 {
			t.Fatalf("WidgetCount = %d, want 1", result.WidgetCount)
		}
		if len(result.Lines) != 1 {
			t.Fatalf("Lines = %v, want one rendered line", result.Lines)
		}
		if got := stripANSI(result.Lines[0]); got != "cc-usage" {
			t.Fatalf("preset N output = %q, want %q", got, "cc-usage")
		}
	})

	t.Run("disabledWidgets removes projectName", func(t *testing.T) {
		ctx := &Context{
			Stdin: StdinInput{},
			Config: Config{
				DisplayMode:     "custom",
				Lines:           [][]string{{"projectName"}},
				DisabledWidgets: []string{"projectName"},
				Theme:           "default",
				Separator:       "space",
			},
			Translations: loadTranslations("en"),
		}
		ctx.Stdin.Workspace.CurrentDir = "/tmp/cc-usage"

		result := orchestrate(ctx)
		if result.WidgetCount != 0 {
			t.Fatalf("WidgetCount = %d, want 0", result.WidgetCount)
		}
		if len(result.Lines) != 0 {
			t.Fatalf("Lines = %v, want no rendered lines", result.Lines)
		}
	})

	t.Run("compact keeps projectInfo and omits projectName", func(t *testing.T) {
		compact := displayPresets["compact"]
		if len(compact) != 1 {
			t.Fatalf("compact preset = %v, want one line", compact)
		}
		want := []string{"projectInfo", "model", "context", "cost", "rateLimit5h", "rateLimit7d"}
		if !reflect.DeepEqual(compact[0], want) {
			t.Fatalf("compact preset line = %v, want %v", compact[0], want)
		}
	})
}

// TestResolvePresetIgnoresUnknownChars verifies that unmapped preset chars
// are silently dropped without aborting the rest of the line.
func TestResolvePresetIgnoresUnknownChars(t *testing.T) {
	// 'z' is not in presetCharToWidget; M -> model, $ -> cost.
	if _, ok := presetCharToWidget['z']; ok {
		t.Fatalf("precondition: 'z' must be unmapped in presetCharToWidget")
	}

	cfg := &Config{Preset: "Mz$"}
	resolvePreset(cfg)

	want := [][]string{{"model", "cost"}}
	if !reflect.DeepEqual(cfg.Lines, want) {
		t.Fatalf("Lines mismatch: got %v, want %v", cfg.Lines, want)
	}
	if cfg.DisplayMode != "custom" {
		t.Fatalf("DisplayMode: got %q, want %q", cfg.DisplayMode, "custom")
	}
}

func TestRemovedPresetCharsAreUnmapped(t *testing.T) {
	for _, ch := range []byte{'S', 'V', 'a', 'D', 'B', 'H', 'F'} {
		if got, ok := presetCharToWidget[ch]; ok {
			t.Fatalf("preset char %q maps to %q, want unmapped", ch, got)
		}
	}
}

// TestOrchestrateSessionStartLine locks the actual stdout line for the two
// session-start moments SPEC §5.1-§5.4 describe: the first render (before
// Claude Code's first API response) and the render right after that response
// arrives for an account without rate_limits (rate_limits stays absent all
// session for non-subscription accounts). Only context/cost/rateLimit5h/
// rateLimit7d are on the line so projectInfo/projectName git lookups can't
// affect it; PATH is cleared as a second guard against git subprocess calls.
func TestOrchestrateSessionStartLine(t *testing.T) {
	t.Setenv("PATH", "")

	baseConfig := Config{
		Theme:       "default",
		Separator:   "space",
		DisplayMode: "custom",
		Lines:       [][]string{{"context", "cost", "rateLimit5h", "rateLimit7d"}},
	}

	t.Run("first render: no rate_limits, total_input_tokens 0", func(t *testing.T) {
		ctx := &Context{
			Config:       baseConfig,
			Translations: loadTranslations("en"),
		}
		ctx.Stdin.ContextWindow.ContextWindowSize = 200000

		result := orchestrate(ctx)
		if len(result.Lines) != 1 {
			t.Fatalf("Lines = %v, want exactly one rendered line", result.Lines)
		}

		emptyBar := strings.Repeat("░", ctx.Config.ContextBarWidth())
		want := emptyBar + " -  $0.00  5h: -  7d: -"
		if got := stripANSI(result.Lines[0]); got != want {
			t.Fatalf("line = %q, want %q", got, want)
		}
	})

	t.Run("after first response: total_input_tokens positive, no rate_limits", func(t *testing.T) {
		ctx := &Context{
			Config:       baseConfig,
			Translations: loadTranslations("en"),
		}
		ctx.Stdin.ContextWindow.ContextWindowSize = 200000
		ctx.Stdin.ContextWindow.TotalInputTokens = 50000
		ctx.Stdin.ContextWindow.TotalOutputTokens = 10000

		result := orchestrate(ctx)
		if len(result.Lines) != 1 {
			t.Fatalf("Lines = %v, want exactly one rendered line", result.Lines)
		}

		got := stripANSI(result.Lines[0])
		if strings.Contains(got, "5h") || strings.Contains(got, "7d") {
			t.Fatalf("line = %q, want no 5h/7d fragment when rate_limits stays absent after the first response", got)
		}
	})
}

// TestOrchestrateSectionIsolation locks the stdout line for stdin with
// exactly one corrupted top-level section, per SPEC §5.1-§5.3: the widgets
// fed by other sections keep rendering normally, and only the widget(s) fed
// by the broken section disappear — no partial or placeholder value takes
// their place. Follows TestOrchestrateSessionStartLine's PATH="" +
// Separator + explicit Lines pattern so git subprocess calls can't affect
// the line.
func TestOrchestrateSectionIsolation(t *testing.T) {
	t.Setenv("PATH", "")

	t.Run("rate_limits broken (SPEC §5.1): model/context/cost survive, 5h/7d drop", func(t *testing.T) {
		in := parseStdinReader(strings.NewReader(`{
			"model": {"id": "claude-opus-4-6", "display_name": "Opus"},
			"workspace": {"current_dir": "/tmp"},
			"context_window": {"total_input_tokens": 50000, "total_output_tokens": 10000, "context_window_size": 200000},
			"cost": {"total_cost_usd": 1.25},
			"rate_limits": {"five_hour": {"used_percentage": "high", "resets_at": 0}, "seven_day": {"used_percentage": 69, "resets_at": 0}}
		}`))
		ctx := &Context{
			Stdin: in,
			Config: Config{
				Theme:       "default",
				Separator:   "space",
				DisplayMode: "custom",
				Lines:       [][]string{{"model", "context", "cost", "rateLimit5h", "rateLimit7d"}},
			},
			Translations: loadTranslations("en"),
		}

		result := orchestrate(ctx)
		if len(result.Lines) != 1 {
			t.Fatalf("Lines = %v, want exactly one rendered line", result.Lines)
		}
		got := stripANSI(result.Lines[0])
		for _, want := range []string{"claude-opus-4-6", "30%", "60K", "$1.25"} {
			if !strings.Contains(got, want) {
				t.Fatalf("line = %q, want it to contain %q", got, want)
			}
		}
		if strings.Contains(got, "5h") || strings.Contains(got, "7d") {
			t.Fatalf("line = %q, want no 5h/7d fragment when rate_limits is corrupted", got)
		}
	})

	t.Run("context_window broken (SPEC §5.2): model/cost survive, context drops", func(t *testing.T) {
		in := parseStdinReader(strings.NewReader(`{
			"model": {"id": "claude-opus-4-6", "display_name": "Opus"},
			"context_window": [1, 2, 3],
			"cost": {"total_cost_usd": 1.25}
		}`))
		ctx := &Context{
			Stdin: in,
			Config: Config{
				Theme:       "default",
				Separator:   "space",
				DisplayMode: "custom",
				Lines:       [][]string{{"model", "context", "cost"}},
			},
			Translations: loadTranslations("en"),
		}

		result := orchestrate(ctx)
		if len(result.Lines) != 1 {
			t.Fatalf("Lines = %v, want exactly one rendered line", result.Lines)
		}
		if result.WidgetCount != 2 {
			t.Fatalf("WidgetCount = %d, want 2 (model, cost only)", result.WidgetCount)
		}
		got := stripANSI(result.Lines[0])
		for _, want := range []string{"claude-opus-4-6", "$1.25"} {
			if !strings.Contains(got, want) {
				t.Fatalf("line = %q, want it to contain %q", got, want)
			}
		}
		if strings.Contains(got, "%") {
			t.Fatalf("line = %q, want no percent fragment when context_window is corrupted", got)
		}
	})

	t.Run("workspace broken (SPEC §5.3): model/context/cost survive", func(t *testing.T) {
		in := parseStdinReader(strings.NewReader(`{
			"model": {"id": "claude-opus-4-6", "display_name": "Opus"},
			"workspace": "nope",
			"context_window": {"total_input_tokens": 50000, "total_output_tokens": 10000, "context_window_size": 200000},
			"cost": {"total_cost_usd": 1.25}
		}`))
		ctx := &Context{
			Stdin: in,
			Config: Config{
				Theme:       "default",
				Separator:   "space",
				DisplayMode: "custom",
				Lines:       [][]string{{"model", "context", "cost"}},
			},
			Translations: loadTranslations("en"),
		}

		result := orchestrate(ctx)
		if len(result.Lines) != 1 {
			t.Fatalf("Lines = %v, want exactly one rendered line", result.Lines)
		}
		got := stripANSI(result.Lines[0])
		for _, want := range []string{"claude-opus-4-6", "30%", "60K", "$1.25"} {
			if !strings.Contains(got, want) {
				t.Fatalf("line = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("scalar section broken (version): all other columns survive", func(t *testing.T) {
		in := parseStdinReader(strings.NewReader(`{
			"model": {"id": "claude-opus-4-6", "display_name": "Opus"},
			"workspace": {"current_dir": "/tmp"},
			"context_window": {"total_input_tokens": 50000, "total_output_tokens": 10000, "context_window_size": 200000},
			"cost": {"total_cost_usd": 1.25},
			"rate_limits": {"five_hour": {"used_percentage": 42, "resets_at": 0}, "seven_day": {"used_percentage": 69, "resets_at": 0}},
			"version": 7
		}`))
		if in.Version != "" {
			t.Fatalf("Version = %q, want zero value when the section is corrupted", in.Version)
		}
		ctx := &Context{
			Stdin: in,
			Config: Config{
				Theme:       "default",
				Separator:   "space",
				DisplayMode: "custom",
				Lines:       [][]string{{"model", "context", "cost", "rateLimit5h", "rateLimit7d"}},
			},
			Translations: loadTranslations("en"),
		}

		result := orchestrate(ctx)
		if len(result.Lines) != 1 {
			t.Fatalf("Lines = %v, want exactly one rendered line", result.Lines)
		}
		got := stripANSI(result.Lines[0])
		for _, want := range []string{"claude-opus-4-6", "30%", "60K", "$1.25", "5h: 42%", "7d: 69%"} {
			if !strings.Contains(got, want) {
				t.Fatalf("line = %q, want it to contain %q", got, want)
			}
		}
	})
}

// TestOrchestrateStdoutHasNoDiagnosticCharsForBrokenSections pins SPEC §5.7:
// stdout for a payload with two corrupted sections and unknown top-level
// keys (task-002's stderrBrokenSectionsPayload, defined in stdin_test.go)
// carries no diagnostic marker — only the surviving widgets' values. cost
// is corrupted (so it shows the existing "$0.00" degraded value per
// ANALYSIS §2/D5 — this feature doesn't change that) and rate_limits is
// corrupted (so 5h/7d are dropped outright, since context_window is valid
// and FirstResponseReceived() is true).
func TestOrchestrateStdoutHasNoDiagnosticCharsForBrokenSections(t *testing.T) {
	t.Setenv("PATH", "")

	in := parseStdinReader(strings.NewReader(stderrBrokenSectionsPayload))
	ctx := &Context{
		Stdin: in,
		Config: Config{
			Theme:       "default",
			Separator:   "space",
			DisplayMode: "custom",
			Lines:       [][]string{{"model", "context", "cost", "rateLimit5h", "rateLimit7d"}},
		},
		Translations: loadTranslations("en"),
	}

	result := orchestrate(ctx)
	if len(result.Lines) != 1 {
		t.Fatalf("Lines = %v, want exactly one rendered line", result.Lines)
	}
	got := stripANSI(result.Lines[0])

	for _, want := range []string{"claude-opus-4-6", "30%", "60K", "$0.00"} {
		if !strings.Contains(got, want) {
			t.Fatalf("line = %q, want it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{"5h", "7d", "corrupted", "unknown", "ignored", "cc-usage:"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("line = %q, must not contain diagnostic/dropped-section fragment %q", got, unwanted)
		}
	}
}

// TestOrchestrateStdoutUnaffectedByUnknownTopLevelKeys pins SPEC §5.5:
// unknown top-level keys are silently ignored, so stdout for a payload with
// stray keys mixed in matches stdout for the same payload without them.
func TestOrchestrateStdoutUnaffectedByUnknownTopLevelKeys(t *testing.T) {
	t.Setenv("PATH", "")

	basePayload := `{
		"model": {"id": "claude-opus-4-6", "display_name": "Opus"},
		"workspace": {"current_dir": "/tmp"},
		"context_window": {"total_input_tokens": 50000, "total_output_tokens": 10000, "context_window_size": 200000},
		"cost": {"total_cost_usd": 1.25},
		"rate_limits": {"five_hour": {"used_percentage": 42, "resets_at": 0}, "seven_day": {"used_percentage": 69, "resets_at": 0}}
	}`
	withUnknownKeys := `{
		"model": {"id": "claude-opus-4-6", "display_name": "Opus"},
		"workspace": {"current_dir": "/tmp"},
		"context_window": {"total_input_tokens": 50000, "total_output_tokens": 10000, "context_window_size": 200000},
		"cost": {"total_cost_usd": 1.25},
		"rate_limits": {"five_hour": {"used_percentage": 42, "resets_at": 0}, "seven_day": {"used_percentage": 69, "resets_at": 0}},
		"zzz_unknown": true,
		"aaa_unknown": {"nested": 1}
	}`

	newCtx := func(payload string) *Context {
		return &Context{
			Stdin: parseStdinReader(strings.NewReader(payload)),
			Config: Config{
				Theme:       "default",
				Separator:   "space",
				DisplayMode: "custom",
				Lines:       [][]string{{"model", "context", "cost", "rateLimit5h", "rateLimit7d"}},
			},
			Translations: loadTranslations("en"),
		}
	}

	base := orchestrate(newCtx(basePayload))
	withUnknown := orchestrate(newCtx(withUnknownKeys))

	if len(base.Lines) != 1 || len(withUnknown.Lines) != 1 {
		t.Fatalf("Lines = %v / %v, want exactly one rendered line each", base.Lines, withUnknown.Lines)
	}
	if got, want := stripANSI(withUnknown.Lines[0]), stripANSI(base.Lines[0]); got != want {
		t.Fatalf("stdout with unknown keys = %q, want same as without: %q", got, want)
	}
}
