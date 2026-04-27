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

// sessionStateTTL caps how long a cached SessionState with a strong session
// identity is considered fresh. This needs to survive long idle gaps because
// Claude Code may resume by sending a degraded status-line payload first.
const sessionStateTTL = 24 * time.Hour

// cwdSessionStateTTL is used for cwd-only cache keys. It is intentionally
// profile-local and project-scoped so an identity-less idle refresh can keep
// the last visible render without falling back to a different project.
const cwdSessionStateTTL = 24 * time.Hour

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
	LastOutput  string      `json:"last_output,omitempty"`
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

func sessionStateTTLForKey(cacheKey string) time.Duration {
	if strings.HasPrefix(cacheKey, "cwd-") {
		return cwdSessionStateTTL
	}
	return sessionStateTTL
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
	ttl := sessionStateTTLForKey(cacheKey)
	if state.SavedAt > 0 && time.Since(time.Unix(state.SavedAt, 0)) > ttl {
		debugLog("cache", "session state expired (age > %s)", ttl)
		return nil
	}
	return &state
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
