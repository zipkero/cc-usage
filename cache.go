package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type SessionState struct {
	CachedParts string `json:"cached_parts"`
	WidgetCount int    `json:"widget_count"`
	CurrentDir  string `json:"current_dir,omitempty"`
}

func sessionStatePath(sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if sessionID == "" {
		return filepath.Join(home, ".cache", "cc-usage", "session-state.json")
	}
	return filepath.Join(home, ".cache", "cc-usage", "session-state-"+sessionID+".json")
}

func loadSessionState(sessionID string) *SessionState {
	path := sessionStatePath(sessionID)
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
	// Ignore legacy format (had last_output instead of cached_parts)
	if state.CachedParts == "" {
		debugLog("cache", "ignoring legacy cache format")
		return nil
	}
	return &state
}

func saveSessionState(sessionID string, state *SessionState) {
	path := sessionStatePath(sessionID)
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
