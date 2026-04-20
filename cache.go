package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// sessionStateTTL caps how long a cached SessionState is considered fresh.
// Stale entries prevent cost/currentDir from freezing indefinitely when stdin
// keeps arriving degraded or empty. RateLimit values are not subject to this
// TTL because they are re-fetched from the account-global API cache each run.
const sessionStateTTL = 300 * time.Second

// workspaceRestoreTTL limits how recently cached workspace/worktree fields
// can be restored on degrade. Shorter than sessionStateTTL because a stale
// cwd is more user-visible than stale cost/context numbers.
const workspaceRestoreTTL = 30 * time.Second

type SessionState struct {
	// CachedStdin is the last stdin payload that rendered at least two widgets.
	// RateLimits is stripped before save so the account-global API cache always
	// supplies fresh 5h/7d values on degrade re-render.
	CachedStdin *StdinInput `json:"cached_stdin,omitempty"`
	WidgetCount int         `json:"widget_count"`
	SavedAt     int64       `json:"saved_at,omitempty"`
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
	if state.CachedStdin == nil {
		debugLog("cache", "ignoring legacy cache format")
		return nil
	}
	if state.SavedAt > 0 && time.Since(time.Unix(state.SavedAt, 0)) > sessionStateTTL {
		debugLog("cache", "session state expired (age > %s)", sessionStateTTL)
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
	state.SavedAt = time.Now().Unix()
	data, err := json.Marshal(state)
	if err != nil {
		debugLog("cache", "session state marshal failed: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		debugLog("cache", "session state write failed: %v", err)
	}
}
