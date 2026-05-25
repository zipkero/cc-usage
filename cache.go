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
// can be restored on degrade. Correctness against stale cwd is guaranteed by
// the cwd-match guard (SPEC §5.11); this TTL is the secondary safety bound
// that caps how long a guarded restore can still expose a stale path if the
// guard itself cannot determine the current cwd. v0.3.4 aligned this with
// sessionStateTTL (300s); v0.3.7 shortens it to 60s now that the guard owns
// correctness, narrowing the worst-case stale window.
const workspaceRestoreTTL = 60 * time.Second

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

// detectCwdEnv and detectCwdGetwd are package-level indirection points so
// tests can swap the underlying lookups. Production code routes env reads to
// os.Getenv and process-cwd reads to os.Getwd. PWD env is intentionally not
// consulted (see ANALYSIS §12 D1) because it can lag the actual cwd after
// `cd` inside the same shell, which would defeat the stale-cwd guard.
var (
	detectCwdEnv   = os.Getenv
	detectCwdGetwd = os.Getwd
)

// detectCurrentCwd returns a normalized cwd guess used as the matching key for
// the empty-stdin fallback path. Priority: CLAUDE_PROJECT_DIR env first
// (Claude Code injects this deliberately, so when present it is the authoritative
// signal), then os.Getwd() (always available but its meaning depends on whether
// Claude Code chdir'd into the workspace before invoking the status line). If
// both are absent or fail, returns "" — callers treat empty as "unknown" and
// skip the fallback rather than risk cross-workspace cache exposure.
func detectCurrentCwd() string {
	cwd, _ := detectCurrentCwdWithSource()
	return cwd
}

// detectCurrentCwdWithSource returns the same normalized cwd as detectCurrentCwd
// alongside a short label identifying which signal won. Used by the empty-stdin
// fallback logging (SPEC §5.6, ANALYSIS §7) to distinguish env-driven vs
// getwd-driven matches without changing detectCurrentCwd's existing callers.
// source values: "env" (CLAUDE_PROJECT_DIR), "getwd" (os.Getwd), "" (no signal).
func detectCurrentCwdWithSource() (cwd, source string) {
	if raw := detectCwdEnv("CLAUDE_PROJECT_DIR"); raw != "" {
		return normalizeCwd(raw), "env"
	}
	if raw, err := detectCwdGetwd(); err == nil && raw != "" {
		return normalizeCwd(raw), "getwd"
	}
	return "", ""
}

// normalizeCwd canonicalizes a workspace path so that semantically identical
// paths (e.g. `/var` vs `/private/var` on macOS, trailing slash, `.` segments)
// compare equal during degrade restore matching. EvalSymlinks is preferred
// because it resolves OS-level symlink prefixes; on failure (missing path,
// permission error, etc.) we fall back to Clean so the function never errors
// out. An empty input is returned as-is — callers treat "" as "unknown".
func normalizeCwd(raw string) string {
	if raw == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(raw); err == nil {
		return resolved
	}
	return filepath.Clean(raw)
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

// loadByWorkspaceCwd is the empty-stdin fallback matcher (ANALYSIS §12 D2/D3).
// When stdin is degraded enough that sessionCacheKey returns "", main.go cannot
// look up the per-session cache by key. This function scans the cache directory
// for session-state files whose persisted Workspace.CurrentDir — already
// normalized at save time (see saveSessionState) — exactly matches cwd within
// sessionStateTTL, and returns the newest by mtime.
//
// Matching is strictly exact-equality on normalized cwd: no subpath, substring,
// or case-insensitive comparison. This is the v0.3.5 cross-workspace exposure
// guard (SPEC §5.2). Callers pass cwd already normalized via detectCurrentCwd /
// normalizeCwd; this function does not re-normalize the input.
//
// Returns nil when cwd is empty, dir glob fails, no candidate matches, or all
// candidates are expired. Per-file read errors are logged and skipped rather
// than aborting the whole scan — one corrupt file shouldn't suppress matches
// from siblings. The file lock pattern used by loadSessionState is reused per
// candidate to stay consistent with concurrent writers.
func loadByWorkspaceCwd(dir, cwd string, now time.Time) *SessionState {
	if cwd == "" || dir == "" {
		return nil
	}

	matches, err := filepath.Glob(filepath.Join(dir, "session-state-*.json"))
	if err != nil || len(matches) == 0 {
		return nil
	}

	var bestState *SessionState
	var bestModTime time.Time
	for _, path := range matches {
		// Defensive filter: filepath.Glob with the json suffix already excludes
		// .lock and .tmp-* (dot-prefix) variants, but skip-on-suffix keeps the
		// invariant explicit so future glob changes don't leak temporaries.
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".lock") {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		var state SessionState
		if err := withCacheFileLock(path, func() error {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return json.Unmarshal(data, &state)
		}); err != nil {
			debugLog("cache", "loadByWorkspaceCwd read/parse error for %s: %v", base, err)
			continue
		}
		if state.CachedStdin == nil {
			continue
		}
		if state.SavedAt <= 0 {
			continue
		}
		if now.Sub(time.Unix(state.SavedAt, 0)) > sessionStateTTL {
			continue
		}
		if normalizeCwd(state.CachedStdin.Workspace.CurrentDir) != cwd {
			continue
		}

		if bestState == nil || info.ModTime().After(bestModTime) {
			stateCopy := state
			bestState = &stateCopy
			bestModTime = info.ModTime()
		}
	}
	return bestState
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

	// Normalize Workspace.CurrentDir at serialization time so the on-disk value
	// can be compared by exact-equality against the current cwd during the
	// empty-stdin fallback (ANALYSIS §4.2). We copy the StdinInput by value into
	// a sibling SessionState before mutating to avoid leaking the normalization
	// back into caller-visible state via the shared *StdinInput pointer.
	persisted := state
	if state.CachedStdin != nil {
		stdinCopy := *state.CachedStdin
		stdinCopy.Workspace.CurrentDir = normalizeCwd(stdinCopy.Workspace.CurrentDir)
		stateCopy := *state
		stateCopy.CachedStdin = &stdinCopy
		persisted = &stateCopy
	}

	data, err := json.Marshal(persisted)
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
