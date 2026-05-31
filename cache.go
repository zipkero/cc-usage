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

// workspaceRestoreTTL is the single TTL that gates the atomic restore
// eligibility decision — "should this degraded stdin be backfilled from the
// session cache at all?" (ANALYSIS §5.C, adopted C2). When the cache entry's
// age exceeds this bound, no field (workspace, model, cost, context) is
// restored, preventing any half-populated status line. Intentionally narrower
// than sessionStateTTL (300s): the cwd-exact-match guard owns cross-workspace
// correctness; this TTL caps the worst-case stale window within the same
// workspace. v0.3.4 aligned both TTLs at 300s; v0.3.7 tightened this to 60s;
// the atomic-restore rework (v0.3.9) formalises it as the sole eligibility TTL.
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

// cwdWithinRoot reports whether recorded is the same path as root, or a
// descendant of it. The degrade-restore guards compare a recorded cwd (a cached
// workspace.current_dir or a transcript entry's cwd) against the current cwd
// signal, which detectCurrentCwd derives from CLAUDE_PROJECT_DIR — Claude Code
// pins that to the project root and does not move it when the session cd's into
// a subdirectory. Exact equality therefore rejects a perfectly valid session
// whose current_dir drifted below the root (e.g. into node_modules). Containment
// accepts that case while still blocking sibling/foreign workspaces: the trailing
// separator prevents prefix confusion ("/a/bus" must not match "/a/bus-escape").
// Both arguments are expected to be normalizeCwd'd already.
func cwdWithinRoot(recorded, root string) bool {
	if recorded == "" || root == "" {
		return false
	}
	return recorded == root ||
		strings.HasPrefix(recorded, root+string(os.PathSeparator))
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

// restoredFieldMask records which StdinInput fields were backfilled from the
// session cache by fillFromSessionCache in a single call. task-004 uses the
// mask to strip those fields back out of the save snapshot so a degraded-only
// call cannot self-perpetuate cached values via repeated saves.
type restoredFieldMask struct {
	Workspace     bool
	Worktree      bool
	Model         bool
	Cost          bool
	ContextWindow bool
}

// shouldRestoreFromSession returns true when the cached SessionState is
// eligible to backfill missing fields in the current (degraded) stdin.
//
// Checks in order:
//  1. cached != nil and cached.CachedStdin != nil
//  2. SavedAt > 0 and age < workspaceRestoreTTL (atomic-restore TTL, §5.C C2)
//  3. shouldRestoreWorkspace(cached cwd) — cwd-exact-match guard
//  4. At least one backfill-eligible field is actually empty in stdin
//
// Any failing step is logged via debugLog and false is returned immediately.
// Pure function: does not mutate stdin or cached.
func shouldRestoreFromSession(stdin StdinInput, cached *SessionState, now time.Time) bool {
	if cached == nil || cached.CachedStdin == nil {
		debugLog("restore", "eligibility=false: no cached state")
		return false
	}
	if cached.SavedAt <= 0 {
		debugLog("restore", "eligibility=false: SavedAt == 0")
		return false
	}
	if now.Sub(time.Unix(cached.SavedAt, 0)) >= workspaceRestoreTTL {
		debugLog("restore", "eligibility=false: cache age %v >= workspaceRestoreTTL %v",
			now.Sub(time.Unix(cached.SavedAt, 0)), workspaceRestoreTTL)
		return false
	}
	if !shouldRestoreWorkspace(cached.CachedStdin.Workspace.CurrentDir) {
		debugLog("restore", "eligibility=false: cwd mismatch or unknown")
		return false
	}
	// At least one fillable field must be empty in stdin.
	needsFill := stdin.Workspace.CurrentDir == "" ||
		stdin.Worktree == nil ||
		(stdin.Model.ID == "" && stdin.Model.DisplayName == "") ||
		stdin.Cost.TotalCostUsd <= 0 ||
		stdin.ContextWindow.TotalInputTokens+stdin.ContextWindow.TotalOutputTokens == 0
	if !needsFill {
		debugLog("restore", "eligibility=false: stdin has no empty fields requiring fill")
		return false
	}
	return true
}

// fillFromSessionCache backfills empty fields in *stdin from cached.CachedStdin.
// Only fields whose stdin side is empty (per field-local emptiness rules) are
// filled; fresh values that stdin already carries are never overwritten.
// RateLimits is never touched — it must always come from the live API cache.
// Returns a restoredFieldMask describing which fields were actually written.
//
// Callers must check shouldRestoreFromSession before calling this function.
func fillFromSessionCache(stdin *StdinInput, cached *SessionState) restoredFieldMask {
	var mask restoredFieldMask
	if stdin == nil || cached == nil || cached.CachedStdin == nil {
		return mask
	}
	c := cached.CachedStdin

	if stdin.Workspace.CurrentDir == "" && c.Workspace.CurrentDir != "" {
		stdin.Workspace = c.Workspace
		mask.Workspace = true
		debugLog("restore", "filled Workspace.CurrentDir from cache")
	}
	if stdin.Worktree == nil && c.Worktree != nil {
		stdin.Worktree = c.Worktree
		mask.Worktree = true
		debugLog("restore", "filled Worktree from cache")
	}
	if stdin.Model.ID == "" && stdin.Model.DisplayName == "" &&
		(c.Model.ID != "" || c.Model.DisplayName != "") {
		stdin.Model = c.Model
		mask.Model = true
		debugLog("restore", "filled Model from cache")
	}
	if stdin.Cost.TotalCostUsd <= 0 && c.Cost.TotalCostUsd > 0 {
		stdin.Cost = c.Cost
		mask.Cost = true
		debugLog("restore", "filled Cost from cache")
	}
	if stdin.ContextWindow.TotalInputTokens+stdin.ContextWindow.TotalOutputTokens == 0 &&
		c.ContextWindow.TotalInputTokens+c.ContextWindow.TotalOutputTokens > 0 {
		stdin.ContextWindow = c.ContextWindow
		mask.ContextWindow = true
		debugLog("restore", "filled ContextWindow from cache")
	}
	return mask
}

// stripRestoredFields zeros out the fields in snapshot that were backfilled
// from the session cache by fillFromSessionCache in this call, as indicated
// by mask. This prevents degraded-only calls from perpetually refreshing cached
// values via repeated saves — each save carries only data that arrived fresh
// in stdin (SPEC §5.6, ANALYSIS §5.D, adopted D1).
//
// Fields whose mask bit is false are left unchanged. RateLimits is handled
// separately by the caller (snapshot.RateLimits = nil) and is never tracked in
// the mask. SavedAt is unaffected — it is set by saveSessionState itself.
func stripRestoredFields(snapshot *StdinInput, mask restoredFieldMask) {
	if snapshot == nil {
		return
	}
	if mask.Workspace {
		var zero StdinInput
		snapshot.Workspace = zero.Workspace
	}
	if mask.Worktree {
		snapshot.Worktree = nil
	}
	if mask.Model {
		var zero StdinInput
		snapshot.Model = zero.Model
	}
	if mask.Cost {
		var zero StdinInput
		snapshot.Cost = zero.Cost
	}
	if mask.ContextWindow {
		var zero StdinInput
		snapshot.ContextWindow = zero.ContextWindow
	}
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
