package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// oneMByCwdPath returns the absolute path to the single shared file that maps
// cwd-hash → bool for last-known [1m] signals.
// Returns "" when os.UserHomeDir fails; callers treat "" as unavailable.
func oneMByCwdPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "cc-usage", "one-m-by-cwd.json")
}

// cwdHashKey returns a stable short hash for the given cwd, ready to use as a
// map key inside one-m-by-cwd.json. The cwd is normalized first via
// normalizeCwd (EvalSymlinks fallback to Clean) so that semantically identical
// paths hash identically. Cross-cwd isolation relies on the uniqueness of
// sha256 truncated to 16 hex characters (hashCacheKey).
func cwdHashKey(cwd string) string {
	return hashCacheKey(normalizeCwd(cwd))
}

// loadLastKnownOneM reads one-m-by-cwd.json and returns whether the given cwd
// was last known to carry a [1m] context window.
//
// Returns false (default 200K path) when:
//   - cwd is empty
//   - the cache file does not exist
//   - the file cannot be parsed
//   - the cwd-hash key is absent in the map
//
// Never panics. Lock is acquired via withCacheFileLock for concurrent-write
// safety.
func loadLastKnownOneM(cwd string) bool {
	if cwd == "" {
		return false
	}
	path := oneMByCwdPath()
	if path == "" {
		return false
	}

	var result bool
	_ = withCacheFileLock(path, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			// File absent or unreadable → default false, not an error to propagate.
			return nil
		}
		var m map[string]bool
		if err := json.Unmarshal(data, &m); err != nil {
			debugLog("cache", "one-m-by-cwd parse error: %v", err)
			return nil
		}
		result = m[cwdHashKey(cwd)]
		return nil
	})
	return result
}

// saveLastKnownOneM persists val for cwd in one-m-by-cwd.json using a
// read-merge-write pattern so other cwd entries are preserved.
//
// If the cache file is absent, a fresh map is created.
// If the file cannot be parsed, the update is dropped silently (degraded
// write; existing data is not clobbered).
// Write is atomic via atomicWriteFile and lock-protected via withCacheFileLock.
func saveLastKnownOneM(cwd string, val bool) {
	if cwd == "" {
		return
	}
	path := oneMByCwdPath()
	if path == "" {
		return
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		debugLog("cache", "one-m-by-cwd mkdir failed: %v", err)
		return
	}

	if err := withCacheFileLock(path, func() error {
		// Read existing map (best-effort; start fresh on any error).
		m := make(map[string]bool)
		if data, err := os.ReadFile(path); err == nil {
			if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
				debugLog("cache", "one-m-by-cwd parse error on write path: %v", jsonErr)
				// Do not clobber existing data — bail without writing.
				return nil
			}
		}

		key := cwdHashKey(cwd)
		m[key] = val

		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return atomicWriteFile(path, data, 0644)
	}); err != nil {
		debugLog("cache", "one-m-by-cwd write failed: %v", err)
	}
}
