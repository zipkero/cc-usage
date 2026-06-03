package main

import (
	"reflect"
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
