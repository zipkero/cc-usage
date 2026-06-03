package main

import (
	"reflect"
	"testing"
)

// TestResolvePresetParsesChars verifies that resolvePreset splits the preset
// string on "|" into lines and maps each char to its widget ID, while also
// forcing DisplayMode to "custom".
func TestResolvePresetParsesChars(t *testing.T) {
	cfg := &Config{Preset: "P|M$C"}
	resolvePreset(cfg)

	want := [][]string{
		{"projectInfo"},
		{"model", "cost", "context"},
	}
	if !reflect.DeepEqual(cfg.Lines, want) {
		t.Fatalf("Lines mismatch: got %v, want %v", cfg.Lines, want)
	}
	if cfg.DisplayMode != "custom" {
		t.Fatalf("DisplayMode: got %q, want %q", cfg.DisplayMode, "custom")
	}
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
