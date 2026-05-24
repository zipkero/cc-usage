package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sessionStateTTL caps how long a cached SessionState is considered fresh.
// Stale entries prevent cost/currentDir from freezing indefinitely when stdin
// keeps arriving degraded or empty. RateLimit values are not subject to this
// TTL because they are re-fetched from the account-global API cache each run.
const sessionStateTTL = 300 * time.Second

// workspaceRestoreTTL limits how recently cached workspace/worktree fields
// can be restored on degrade. Aligned with sessionStateTTL so that long idle
// periods followed by a degraded stdin still recover cwd; the previous 30s
// ceiling caused the status line to disappear after a few minutes of idle.
const workspaceRestoreTTL = sessionStateTTL

const (
	cacheLockTimeout    = 200 * time.Millisecond
	cacheLockRetryDelay = 10 * time.Millisecond
)

type SessionState struct {
	// CachedStdin is the last stdin payload that rendered at least two widgets.
	// RateLimits is stripped before save so the account-global API cache always
	// supplies fresh 5h/7d values on degrade re-render.
	CachedStdin *StdinInput `json:"cached_stdin,omitempty"`
	WidgetCount int         `json:"widget_count"`
	SavedAt     int64       `json:"saved_at,omitempty"`
}

func sessionCacheKey(input StdinInput) string {
	if key := safeCacheKeyPart(input.SessionId); key != "" {
		return key
	}
	if input.Remote != nil {
		if key := safeCacheKeyPart(input.Remote.SessionId); key != "" {
			return "remote-" + key
		}
	}
	if key := safeCacheKeyPart(input.AgentId); key != "" {
		return "agent-" + key
	}
	if input.TranscriptPath != "" {
		return "transcript-" + hashCacheKey(input.TranscriptPath)
	}
	if input.Workspace.CurrentDir != "" {
		return "cwd-" + hashCacheKey(input.Workspace.CurrentDir)
	}
	return ""
}

func safeCacheKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' {
			continue
		}
		return hashCacheKey(value)
	}
	return value
}

func hashCacheKey(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])[:16]
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	keepTemp = true
	return nil
}

func withCacheFileLock(path string, fn func() error) error {
	unlock, err := acquireCacheFileLock(path+".lock", cacheLockTimeout, cacheLockRetryDelay)
	if err != nil {
		return err
	}
	defer func() {
		if err := unlock(); err != nil {
			debugLog("cache", "cache lock unlock failed: %v", err)
		}
	}()

	return fn()
}

func sessionStatePath(cacheKey string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if cacheKey == "" {
		return ""
	}
	return filepath.Join(home, ".cache", "cc-usage", "session-state-"+cacheKey+".json")
}

func loadSessionState(cacheKey string) *SessionState {
	path := sessionStatePath(cacheKey)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		debugLog("cache", "session state read error: %v", err)
		return nil
	}
	var state SessionState
	if err := withCacheFileLock(path, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &state)
	}); err != nil {
		debugLog("cache", "session state read/parse error: %v", err)
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

// lastSessionStateCleanup throttles session-state cleanup to once per hour.
// Independent of api.go's lastCleanup because the two cache families use
// different staleness thresholds.
var lastSessionStateCleanup time.Time

// cleanOldSessionStates removes session-state files older than sessionStateTTL.
// Fire-and-forget; safe to call on every invocation.
// Handles both session-state-*.json and session-state-*.json.lock — the lock
// family belongs to the same session-state responsibility and uses the same
// staleness threshold. API cache locks are intentionally not touched here
// (see cleanOldCaches).
func cleanOldSessionStates() {
	now := time.Now()
	if now.Sub(lastSessionStateCleanup) < time.Hour {
		return
	}
	lastSessionStateCleanup = now

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".cache", "cc-usage")
	patterns := []string{
		filepath.Join(dir, "session-state-*.json"),
		filepath.Join(dir, "session-state-*.json.lock"),
		// atomicWriteFile leftover when a writer is SIGKILL'd before its
		// defer os.Remove can run. Same TTL — these are short-lived by design.
		filepath.Join(dir, ".session-state-*.json.tmp-*"),
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) > sessionStateTTL {
				_ = os.Remove(f)
			}
		}
	}

	// Legacy zombie: an older release wrote session-state.json (no key suffix).
	// New code never writes this name, so any surviving file is stale. Match
	// by exact path to avoid colliding with the keyed pattern above.
	legacy := filepath.Join(dir, "session-state.json")
	if info, err := os.Stat(legacy); err == nil && now.Sub(info.ModTime()) > sessionStateTTL {
		_ = os.Remove(legacy)
	}
}

// shouldRestoreCost reports whether stdin.cost should be restored from
// cached state. Returns true only when stdin reports cost=0 while the cache
// still holds a positive cost saved within sessionStateTTL. Pure function —
// safe to test in isolation.
func shouldRestoreCost(stdin StdinInput, cached *SessionState, now time.Time) bool {
	if cached == nil || cached.CachedStdin == nil {
		return false
	}
	return stdin.Cost.TotalCostUsd == 0 &&
		cached.CachedStdin.Cost.TotalCostUsd > 0 &&
		cached.SavedAt > 0 &&
		now.Sub(time.Unix(cached.SavedAt, 0)) < sessionStateTTL
}

func saveSessionState(cacheKey string, state *SessionState) {
	path := sessionStatePath(cacheKey)
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
	if err := withCacheFileLock(path, func() error {
		return atomicWriteFile(path, data, 0644)
	}); err != nil {
		debugLog("cache", "session state write failed: %v", err)
	}
}
