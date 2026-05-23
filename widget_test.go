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

// TestBurnRateGetDataNilOnMissingCost verifies that burnRateWidget.GetData
// returns (nil, nil) when cost is non-positive or total_duration_ms is nil.
func TestBurnRateGetDataNilOnMissingCost(t *testing.T) {
	w := burnRateWidget{}

	// Case 1: cost == 0, duration nil.
	ctx := &Context{Stdin: StdinInput{}}
	data, err := w.GetData(ctx)
	if err != nil || data != nil {
		t.Fatalf("zero cost + nil duration: got (%v, %v), want (nil, nil)", data, err)
	}

	// Case 2: cost > 0 but duration nil.
	ctx2 := &Context{Stdin: StdinInput{}}
	ctx2.Stdin.Cost.TotalCostUsd = 1.25
	data, err = w.GetData(ctx2)
	if err != nil || data != nil {
		t.Fatalf("positive cost + nil duration: got (%v, %v), want (nil, nil)", data, err)
	}

	// Case 3: cost == 0 but duration positive.
	ctx3 := &Context{Stdin: StdinInput{}}
	ms := int64(60_000)
	ctx3.Stdin.Cost.TotalDurationMs = &ms
	data, err = w.GetData(ctx3)
	if err != nil || data != nil {
		t.Fatalf("zero cost + positive duration: got (%v, %v), want (nil, nil)", data, err)
	}

	// Case 4: negative cost.
	ctx4 := &Context{Stdin: StdinInput{}}
	ctx4.Stdin.Cost.TotalCostUsd = -0.5
	ctx4.Stdin.Cost.TotalDurationMs = &ms
	data, err = w.GetData(ctx4)
	if err != nil || data != nil {
		t.Fatalf("negative cost: got (%v, %v), want (nil, nil)", data, err)
	}
}

// TestSessionDurationGetDataNilOnMissingMs verifies that
// sessionDurationWidget.GetData returns (nil, nil) when total_duration_ms is
// nil or non-positive.
func TestSessionDurationGetDataNilOnMissingMs(t *testing.T) {
	w := sessionDurationWidget{}

	// Case 1: ms == nil.
	ctx := &Context{Stdin: StdinInput{}}
	data, err := w.GetData(ctx)
	if err != nil || data != nil {
		t.Fatalf("nil ms: got (%v, %v), want (nil, nil)", data, err)
	}

	// Case 2: ms == 0.
	ctx2 := &Context{Stdin: StdinInput{}}
	zero := int64(0)
	ctx2.Stdin.Cost.TotalDurationMs = &zero
	data, err = w.GetData(ctx2)
	if err != nil || data != nil {
		t.Fatalf("zero ms: got (%v, %v), want (nil, nil)", data, err)
	}

	// Case 3: ms < 0.
	ctx3 := &Context{Stdin: StdinInput{}}
	neg := int64(-1)
	ctx3.Stdin.Cost.TotalDurationMs = &neg
	data, err = w.GetData(ctx3)
	if err != nil || data != nil {
		t.Fatalf("negative ms: got (%v, %v), want (nil, nil)", data, err)
	}
}
