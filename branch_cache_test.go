package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// swapBranchCacheRoot points the cache at t.TempDir() instead of the real
// per-user cache directory, and restores the original function afterward.
// Task-008's verification requires never touching the real user cache dir
// while testing (ANALYSIS §5 D6). It returns the actual directory
// branchCacheDir() will resolve to (branchCacheDir joins a fixed
// "cc-usage/branch-cache" suffix onto whatever branchCacheUserCacheDir
// returns), so callers can locate written files directly.
func swapBranchCacheRoot(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	orig := branchCacheUserCacheDir
	branchCacheUserCacheDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { branchCacheUserCacheDir = orig })
	root, err := branchCacheDir()
	if err != nil {
		t.Fatalf("branchCacheDir: %v", err)
	}
	return root
}

// stubGitBranch swaps gitBranchFunc for a counting stub, mirroring the
// package's detectCwdEnv/detectCwdGetwd swap pattern for indirected external
// calls. The returned pointer tracks how many times git would have run.
func stubGitBranch(t *testing.T, branch string) *int {
	t.Helper()
	calls := 0
	orig := gitBranchFunc
	gitBranchFunc = func(dir string) string {
		calls++
		return branch
	}
	t.Cleanup(func() { gitBranchFunc = orig })
	return &calls
}

// resetBranchMemo clears the process-local memo before a test and restores
// its prior contents afterward, so tests don't leak state into each other via
// the shared package-level map.
func resetBranchMemo(t *testing.T) {
	t.Helper()
	orig := branchMemo
	branchMemo = map[string]string{}
	t.Cleanup(func() { branchMemo = orig })
}

func TestCachedGitBranchFirstRun(t *testing.T) {
	swapBranchCacheRoot(t)
	resetBranchMemo(t)
	calls := stubGitBranch(t, "main")

	got := cachedGitBranch("session-1", "/repo/a")
	if got != "main" {
		t.Fatalf("cachedGitBranch = %q, want %q", got, "main")
	}
	if *calls != 1 {
		t.Fatalf("git calls = %d, want 1", *calls)
	}
}

// TestCachedGitBranchProcessMemoReuse covers the same-process, same-key path:
// a second project widget asking about the identical session+directory in
// the same run must not touch the filesystem at all (ANALYSIS §5 D6 "process
// memo hit" branch).
func TestCachedGitBranchProcessMemoReuse(t *testing.T) {
	swapBranchCacheRoot(t)
	resetBranchMemo(t)
	calls := stubGitBranch(t, "main")

	first := cachedGitBranch("session-1", "/repo/a")
	second := cachedGitBranch("session-1", "/repo/a")

	if *calls != 1 {
		t.Fatalf("git calls = %d, want 1 (second call in the same process should hit the memo)", *calls)
	}
	if first != second {
		t.Fatalf("first=%q second=%q, want identical output", first, second)
	}
}

// TestCachedGitBranchAcrossProcessRuns simulates two separate cc-usage
// process invocations (each starting with a cold process-local memo) sharing
// the same session_id and directory. This is what SPEC §5.8's "연속 실행"
// actually means — cc-usage restarts as a new process every status-line
// refresh — so the disk-backed cache, not the memo, is what must dedupe here.
func TestCachedGitBranchAcrossProcessRuns(t *testing.T) {
	swapBranchCacheRoot(t)
	calls := stubGitBranch(t, "main")

	branchMemo = map[string]string{} // run 1: memo-cold
	first := cachedGitBranch("session-1", "/repo/a")

	branchMemo = map[string]string{} // run 2: memo-cold again, only the file cache can help
	second := cachedGitBranch("session-1", "/repo/a")

	if *calls != 1 {
		t.Fatalf("git calls = %d, want 1 (second process run should hit the file cache)", *calls)
	}
	if first != second {
		t.Fatalf("first=%q second=%q, want identical branch across runs", first, second)
	}
}

func TestCachedGitBranchDifferentDirMisses(t *testing.T) {
	swapBranchCacheRoot(t)
	resetBranchMemo(t)
	calls := stubGitBranch(t, "main")

	cachedGitBranch("session-1", "/repo/a")
	branchMemo = map[string]string{} // force the second lookup through the file cache, not the memo
	cachedGitBranch("session-1", "/repo/b")

	if *calls != 2 {
		t.Fatalf("git calls = %d, want 2 (a different directory under the same session must not reuse the cache)", *calls)
	}
}

func TestCachedGitBranchExpiredTTLRefetches(t *testing.T) {
	root := swapBranchCacheRoot(t)
	resetBranchMemo(t)
	calls := stubGitBranch(t, "main")

	cachedGitBranch("session-1", "/repo/a")

	// Age the written entry past branchCacheTTL directly, rather than
	// sleeping in the test.
	path := branchCacheFilePath(root, "session-1", "/repo/a")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var entry branchCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	entry.RecordedAt = time.Now().Add(-branchCacheTTL - time.Second).Unix()
	aged, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, aged, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	branchMemo = map[string]string{}
	got := cachedGitBranch("session-1", "/repo/a")

	if *calls != 2 {
		t.Fatalf("git calls = %d, want 2 (an expired entry must fall back to git)", *calls)
	}
	if got != "main" {
		t.Fatalf("cachedGitBranch = %q, want %q", got, "main")
	}
}

func TestCachedGitBranchCorruptFileRefetches(t *testing.T) {
	root := swapBranchCacheRoot(t)
	resetBranchMemo(t)
	calls := stubGitBranch(t, "main")

	cachedGitBranch("session-1", "/repo/a")

	path := branchCacheFilePath(root, "session-1", "/repo/a")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	branchMemo = map[string]string{}
	got := cachedGitBranch("session-1", "/repo/a")

	if *calls != 2 {
		t.Fatalf("git calls = %d, want 2 (a corrupt cache file must fall back to git)", *calls)
	}
	if got != "main" {
		t.Fatalf("cachedGitBranch = %q, want %q", got, "main")
	}
}

// TestCachedGitBranchNoSessionIDSkipsCache covers SPEC/ANALYSIS §5 D6's
// hardest constraint: an empty session_id must never touch the cache at all,
// not even read it, since a cwd-only key was the deleted cache's
// cross-session pollution bug.
func TestCachedGitBranchNoSessionIDSkipsCache(t *testing.T) {
	root := swapBranchCacheRoot(t)
	resetBranchMemo(t)
	calls := stubGitBranch(t, "main")

	cachedGitBranch("", "/repo/a")
	cachedGitBranch("", "/repo/a")

	if *calls != 2 {
		t.Fatalf("git calls = %d, want 2 (empty session_id must bypass the cache on every call)", *calls)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return // cache dir was never even created — the strongest form of "no file written"
		}
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		t.Fatalf("unexpected cache file written for empty session_id: %s", e.Name())
	}
}

// TestCachedGitBranchCacheRootUnavailableAlwaysRunsGit covers os.UserCacheDir
// failing outright (SPEC §5.8's "캐시 루트를 얻지 못하면 매번 git을 실행한다").
func TestCachedGitBranchCacheRootUnavailableAlwaysRunsGit(t *testing.T) {
	resetBranchMemo(t)
	calls := stubGitBranch(t, "main")

	orig := branchCacheUserCacheDir
	branchCacheUserCacheDir = func() (string, error) { return "", errors.New("no cache dir on this platform") }
	defer func() { branchCacheUserCacheDir = orig }()

	cachedGitBranch("session-1", "/repo/a")
	branchMemo = map[string]string{}
	cachedGitBranch("session-1", "/repo/a")

	if *calls != 2 {
		t.Fatalf("git calls = %d, want 2 (an unavailable cache root must run git every time)", *calls)
	}
}

// TestBranchCacheFileContainsOnlyGitDerivedFields enforces SPEC §3's cache
// scope limit at the file level: the persisted JSON must carry nothing beyond
// the target directory, the branch, and the recorded timestamp — never
// cost/rate_limits or any other account-derived value.
func TestBranchCacheFileContainsOnlyGitDerivedFields(t *testing.T) {
	root := swapBranchCacheRoot(t)
	resetBranchMemo(t)
	stubGitBranch(t, "feature-x")

	cachedGitBranch("session-1", "/repo/a")

	path := branchCacheFilePath(root, "session-1", "/repo/a")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	wantKeys := map[string]bool{"dir": true, "branch": true, "recorded_at": true}
	for k := range raw {
		if !wantKeys[k] {
			t.Fatalf("cache file has unexpected key %q — scope must stay limited to dir/branch/recorded_at (SPEC §3)", k)
		}
	}
	for _, forbidden := range []string{"cost", "rate_limit", "total_cost_usd", "used_percentage"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("cache file contains forbidden substring %q: %s", forbidden, data)
		}
	}
	if raw["dir"] != "/repo/a" || raw["branch"] != "feature-x" {
		t.Fatalf("unexpected cache contents: %v", raw)
	}
}

// TestSweepBranchCacheRemovesAgedFiles covers the age-based cleanup that runs
// on the write path (ANALYSIS §5 D6) — no goroutine, no lock file, just a
// synchronous sweep by mtime, and it must leave unrelated files alone.
func TestSweepBranchCacheRemovesAgedFiles(t *testing.T) {
	root := t.TempDir()

	freshPath := filepath.Join(root, branchCacheFilePrefix+"fresh.json")
	stalePath := filepath.Join(root, branchCacheFilePrefix+"stale.json")
	otherPath := filepath.Join(root, "unrelated.json")

	for _, p := range []string{freshPath, stalePath, otherPath} {
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", p, err)
		}
	}

	oldTime := time.Now().Add(-branchCacheMaxAge - time.Hour)
	if err := os.Chtimes(stalePath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	sweepBranchCache(root)

	remaining, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	names := map[string]bool{}
	for _, e := range remaining {
		names[e.Name()] = true
	}
	if names[filepath.Base(stalePath)] {
		t.Fatalf("stale cache file %s was not swept", stalePath)
	}
	if !names[filepath.Base(freshPath)] {
		t.Fatalf("fresh cache file %s was incorrectly swept", freshPath)
	}
	if !names[filepath.Base(otherPath)] {
		t.Fatalf("unrelated file %s should never be touched by sweepBranchCache", otherPath)
	}
}

// TestBranchCacheNeverWritesLockFiles guards the fourth failure mode of the
// deleted cache: advisory-lock files that never got cleaned up. This cache
// takes no lock at all, so no ".lock" file should ever appear under the
// cache root across a mix of hit/miss/different-directory operations.
func TestBranchCacheNeverWritesLockFiles(t *testing.T) {
	root := swapBranchCacheRoot(t)
	resetBranchMemo(t)
	stubGitBranch(t, "main")

	cachedGitBranch("session-1", "/repo/a")
	branchMemo = map[string]string{}
	cachedGitBranch("session-1", "/repo/a")
	cachedGitBranch("session-1", "/repo/b")

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".lock") {
			t.Fatalf("found lock file %s — this cache must never leave lock files on disk", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
}
