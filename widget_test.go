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
