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

// branchCacheTTL bounds how long a cached branch answer is considered fresh.
// It only needs to survive the handful of near-simultaneous status-line
// invocations Claude Code issues per turn, not to model actual branch-change
// latency, so a short fixed value is enough. It is a code constant, not a
// config key — ANALYSIS §5 D6 rejected exact `.git/HEAD`-based invalidation
// (walking up for the git dir, resolving worktree/submodule `gitdir:`
// pointers, honoring GIT_DIR) as reimplementing git's own directory
// resolution under the zero-dependency constraint, which is more risk than
// the caching is worth.
const branchCacheTTL = 5 * time.Second

// branchCacheMaxAge is the age past which a cache file is swept on the next
// write, regardless of which session wrote it. Running this sweep
// synchronously on every write — instead of a cleanup goroutine or a
// throttled background pass — is what keeps the cache directory bounded
// without introducing the concurrency the deleted cache needed a lock file
// for. The deleted session-state cache leaked because nothing ever removed
// its files (ANALYSIS §5 D6).
const branchCacheMaxAge = 1 * time.Hour

// branchCacheFilePrefix deliberately does not match the deleted session-state
// cache's `session-state-*.json` naming, so a leftover file from that removed
// feature is never mistaken for (or swept alongside) one of these (ANALYSIS §5
// D6).
const branchCacheFilePrefix = "branch-"

// branchCacheEntry is the entire persisted shape. It holds nothing beyond the
// target directory, the branch string, and when it was recorded — no
// cost/rate_limits or other account-derived value, no widget render output,
// no raw stdin (SPEC §3's cache-scope limit; ANALYSIS §5 D6).
type branchCacheEntry struct {
	Dir        string `json:"dir"`
	Branch     string `json:"branch"`
	RecordedAt int64  `json:"recorded_at"`
}

// branchCacheUserCacheDir indirects os.UserCacheDir so tests can point the
// cache at t.TempDir() instead of the real per-user cache directory (same
// package-level-variable-swap pattern as detectCwdEnv/detectCwdGetwd in
// widgets_project.go).
var branchCacheUserCacheDir = os.UserCacheDir

// gitBranchFunc indirects the raw subprocess lookup so tests can replace it
// and count invocations without shelling out to git. gitBranch itself is left
// untouched — its timeout, "(detached)" handling, and failure-degrades-to-""
// behavior all stay exactly as they were before this cache existed.
var gitBranchFunc = gitBranch

// branchMemo is the process-local half of the cache: within a single cc-usage
// run, a second project widget asking about the same session+directory reuses
// the first widget's answer instead of doing another file read. cc-usage
// never spawns goroutines, so a plain map needs no mutex.
var branchMemo = map[string]string{}

// branchCacheDir returns the directory this cache writes under, or an error
// when the platform cache directory can't be resolved. Every caller treats
// an error here as "cache unavailable" and falls back to running git
// directly. os.TempDir() and the settings directory were both considered and
// rejected (ANALYSIS §5 D6): the former has unpredictable per-OS cleanup
// policies, and the latter would plant cache state inside a directory users
// partition per profile via CLAUDE_CONFIG_DIR.
func branchCacheDir() (string, error) {
	base, err := branchCacheUserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "cc-usage", "branch-cache"), nil
}

// branchCacheKey folds sessionID and dir into a fixed-length hex digest via a
// standard-library hash. sessionID arrives over stdin and is not trustworthy
// input, so it is never spliced into a path directly — hashing removes any
// path-traversal surface (ANALYSIS §5 D6).
func branchCacheKey(sessionID, dir string) string {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + dir))
	return hex.EncodeToString(sum[:])
}

func branchCacheFilePath(root, sessionID, dir string) string {
	return filepath.Join(root, branchCacheFilePrefix+branchCacheKey(sessionID, dir)+".json")
}

// cachedGitBranch is the entry point both project widgets call instead of
// gitBranch directly. Every fallback path here — absent session_id, an
// unavailable cache root, a miss, an expired or corrupt entry — resolves to
// gitBranchFunc(dir), the same call the widgets made before this cache
// existed. So losing the cache for any reason degrades display to exactly
// the pre-cache behavior; it never blanks or wrongs the branch shown
// (SPEC §5.8, ROADMAP 서비스 완료 기준 6).
func cachedGitBranch(sessionID, dir string) string {
	// A cross-session, cwd-only key was the deleted cache's cross-pollution
	// bug: one session's cached branch could be served to a different
	// session that happened to look at the same directory. Refusing to
	// cache at all when session_id is empty removes that key space
	// entirely instead of trying to patch around it (ANALYSIS §5 D6).
	if sessionID == "" {
		return gitBranchFunc(dir)
	}

	memoKey := sessionID + "\x00" + dir
	if branch, ok := branchMemo[memoKey]; ok {
		return branch
	}

	if branch, ok := readBranchCache(sessionID, dir); ok {
		branchMemo[memoKey] = branch
		return branch
	}

	branch := gitBranchFunc(dir)
	branchMemo[memoKey] = branch
	writeBranchCache(sessionID, dir, branch)
	return branch
}

// readBranchCache reports the cached branch for sessionID+dir. It returns
// ok=false for every condition that should fall back to running git: no
// cache root, missing file, unreadable file, malformed JSON, a recorded dir
// that no longer matches (a stale hash collision or reused key), or an entry
// older than branchCacheTTL. None of these are surfaced as errors upstream —
// they all just mean "ask git" (ANALYSIS §5 D6).
func readBranchCache(sessionID, dir string) (string, bool) {
	root, err := branchCacheDir()
	if err != nil {
		debugLog("branchCache", "cache dir unavailable, skipping cache: %v", err)
		return "", false
	}

	data, err := os.ReadFile(branchCacheFilePath(root, sessionID, dir))
	if err != nil {
		return "", false
	}

	var entry branchCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		debugLog("branchCache", "cache file corrupt, ignoring: %v", err)
		return "", false
	}
	if entry.Dir != dir {
		debugLog("branchCache", "cache dir mismatch, ignoring")
		return "", false
	}
	if entry.RecordedAt <= 0 || time.Since(time.Unix(entry.RecordedAt, 0)) > branchCacheTTL {
		debugLog("branchCache", "cache entry expired, ignoring")
		return "", false
	}
	return entry.Branch, true
}

// writeBranchCache persists branch for sessionID+dir via a temp-file-then-
// rename swap, sweeping this cache's aged-out files first. Both steps run
// synchronously on this call — no goroutine, no advisory lock file — so
// there is no cleanup queue to fall behind and no lock file left on disk,
// the other two failure modes of the deleted cache (ANALYSIS §5 D6). Any
// failure here is logged and swallowed: a write that doesn't happen just
// means the next run asks git again, the same outcome as a cache miss.
func writeBranchCache(sessionID, dir, branch string) {
	root, err := branchCacheDir()
	if err != nil {
		debugLog("branchCache", "cache dir unavailable, skipping write: %v", err)
		return
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		debugLog("branchCache", "cache dir create failed, skipping write: %v", err)
		return
	}

	sweepBranchCache(root)

	entry := branchCacheEntry{Dir: dir, Branch: branch, RecordedAt: time.Now().Unix()}
	data, err := json.Marshal(entry)
	if err != nil {
		debugLog("branchCache", "marshal failed, skipping write: %v", err)
		return
	}

	if err := atomicWriteFile(branchCacheFilePath(root, sessionID, dir), data); err != nil {
		debugLog("branchCache", "write failed: %v", err)
	}
}

// sweepBranchCache removes this cache's own files older than
// branchCacheMaxAge from root. It only inspects names matching
// branchCacheFilePrefix+".json", so it never touches another feature's files
// or its own in-flight temp files (atomicWriteFile's temp names carry a
// ".tmp-*" suffix, not ".json").
func sweepBranchCache(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-branchCacheMaxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, branchCacheFilePrefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(root, name))
		}
	}
}

// atomicWriteFile writes data to a temp file in path's directory and renames
// it into place, so a reader never observes a partially written file.
// os.Rename replaces an existing destination on every platform this project
// builds for (SPEC §3 zero dependency — Go's standard-library guarantee is
// relied on instead of a third-party atomic-write package). No advisory lock
// guards this: losing a race between two writers just means the last rename
// wins, an acceptable outcome for a best-effort cache that already re-derives
// its value from git on any miss. The deleted cache's lock file was itself
// the thing that leaked, so this cache does not add one (ANALYSIS §5 D6).
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
