package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type SessionState struct {
	LastCost        float64 `json:"last_cost"`
	LastOutput      string  `json:"last_output"`
	WidgetCount     int     `json:"widget_count"`
	AccumulatedCost float64 `json:"accumulated_cost"`
}

func sessionStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "cc-usage", "session-state.json")
}

func loadSessionState() *SessionState {
	path := sessionStatePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		debugLog("cache", "session state read error: %v", err)
		return nil
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		debugLog("cache", "session state parse error: %v", err)
		return nil
	}
	return &state
}

func cachedAccumulatedCost(cached *SessionState) float64 {
	if cached == nil {
		return 0
	}
	return cached.AccumulatedCost
}

func saveSessionState(state *SessionState) {
	path := sessionStatePath()
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		debugLog("cache", "session state dir create failed: %v", err)
		return
	}
	data, err := json.Marshal(state)
	if err != nil {
		debugLog("cache", "session state marshal failed: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		debugLog("cache", "session state write failed: %v", err)
	}
}
