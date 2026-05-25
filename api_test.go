package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// API가 utilization을 정수가 아닌 부동소수점(예: 12.0, 34.5)으로 보내도
// decode가 성공하고, 위젯이 노출하는 정수 퍼센트는 0~100으로 클램프된다.
func TestDecodeAPIResponseAcceptsDecimalUtilization(t *testing.T) {
	body := strings.NewReader(`{
		"five_hour": {"utilization": 12.0, "resets_at": "2026-04-27T01:02:03Z"},
		"seven_day": {"utilization": 34.5, "resets_at": "2026-04-28T01:02:03Z"},
		"seven_day_sonnet": {"utilization": 101.0, "resets_at": "2026-04-29T01:02:03Z"}
	}`)

	resp, err := decodeAPIResponse(body)
	if err != nil {
		t.Fatalf("decodeAPIResponse failed: %v", err)
	}

	limits := parseUsageLimits(resp)
	if limits == nil {
		t.Fatal("parseUsageLimits returned nil")
	}
	if limits.FiveHour == nil || limits.FiveHour.Utilization != 12 {
		t.Fatalf("five hour utilization = %#v, want 12", limits.FiveHour)
	}
	if limits.SevenDay == nil || limits.SevenDay.Utilization != 34 {
		t.Fatalf("seven day utilization = %#v, want 34 (clamped int from 34.5)", limits.SevenDay)
	}
	if limits.SevenDaySonnet == nil || limits.SevenDaySonnet.Utilization != 100 {
		t.Fatalf("seven day sonnet utilization = %#v, want 100 (clamped from 101.0)", limits.SevenDaySonnet)
	}
}

// task-003: cleanOldCaches가 stale cache-*.json.lock도 정리하고,
// fresh lock과 다른 family(session-state-*.json.lock)는 보존하는지 회귀 검증.
func TestCleanOldCachesHandlesLocks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	prev := lastCleanup
	lastCleanup = time.Time{}
	t.Cleanup(func() {
		lastCleanup = prev
	})

	cacheDir := filepath.Join(home, ".cache", "cc-usage")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}

	writeFixture := func(name string, mtime time.Time) string {
		t.Helper()
		path := filepath.Join(cacheDir, name)
		if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
		return path
	}

	now := time.Now()
	staleJSON := writeFixture("cache-stale.json", now.Add(-2*time.Hour))
	freshJSON := writeFixture("cache-fresh.json", now)
	staleLock := writeFixture("cache-stale.json.lock", now.Add(-2*time.Hour))
	freshLock := writeFixture("cache-fresh.json.lock", now)
	otherFamilyLock := writeFixture("session-state-old.json.lock", now.Add(-(sessionStateTTL + time.Hour)))

	cleanOldCaches()

	if _, err := os.Stat(staleJSON); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale cache .json still exists (err=%v), want removed", err)
	}
	if _, err := os.Stat(staleLock); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale cache .json.lock still exists (err=%v), want removed", err)
	}
	if _, err := os.Stat(freshJSON); err != nil {
		t.Fatalf("fresh cache .json stat: %v, want present", err)
	}
	if _, err := os.Stat(freshLock); err != nil {
		t.Fatalf("fresh cache .json.lock stat: %v, want present", err)
	}
	if _, err := os.Stat(otherFamilyLock); err != nil {
		t.Fatalf("session-state-old.json.lock stat: %v, want present (different family)", err)
	}
}

// task-013: v0.3.4 baseline 보존 — fetchUsageLimits는 cache-hit/miss 어느 쪽에서도
// cleanOldCachesFn을 호출해야 한다. cleanup 호출이 cache-hit short-circuit 뒤로
// 밀리면 cache-*.json.lock이 누적되는 v0.3.4 회귀가 재발한다.
func TestFetchUsageLimitsAlwaysInvokesCleanOldCaches(t *testing.T) {
	cases := []struct {
		name      string
		seedCache bool // true면 cache hit, false면 miss → API 실패 경로
	}{
		{"cache hit", true},
		{"cache miss", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)

			// cleanOldCachesFn을 카운팅 fake로 교체. fetchUsageLimits 안에서
			// goroutine으로 호출되므로 atomic + done 채널로 동기화.
			origFn := cleanOldCachesFn
			t.Cleanup(func() { cleanOldCachesFn = origFn })

			var calls atomic.Int32
			done := make(chan struct{}, 1)
			cleanOldCachesFn = func() {
				calls.Add(1)
				select {
				case done <- struct{}{}:
				default:
				}
			}

			token := "test-token"

			if tc.seedCache {
				// 캐시 hit를 강제하기 위해 fresh cache entry를 디스크에 사전 배치.
				hash := hashToken(token)
				cacheDir := filepath.Join(home, ".cache", "cc-usage")
				if err := os.MkdirAll(cacheDir, 0755); err != nil {
					t.Fatalf("mkdir cache dir: %v", err)
				}
				entry := cacheEntry{
					Data:      &apiUsageResponse{},
					Timestamp: time.Now(),
				}
				data, err := json.Marshal(entry)
				if err != nil {
					t.Fatalf("marshal entry: %v", err)
				}
				path := filepath.Join(cacheDir, "cache-"+hash+".json")
				if err := os.WriteFile(path, data, 0644); err != nil {
					t.Fatalf("write cache file: %v", err)
				}
			}
			// cache miss 경로는 별도 setup 없이 API 호출이 실패(network)하면 됨.
			// fetchUsageLimits는 staleFallback으로 nil을 돌려주지만, 본 테스트는
			// cleanOldCachesFn 호출만 어서션하므로 결과값은 무관.

			cfg := CacheConfig{TTLSeconds: 300}
			_ = fetchUsageLimits(token, cfg)

			// goroutine이 실제로 실행될 때까지 대기 (짧은 timeout).
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("cleanOldCachesFn was not invoked within timeout (calls=%d)", calls.Load())
			}

			if got := calls.Load(); got < 1 {
				t.Fatalf("cleanOldCachesFn invocations = %d, want >= 1", got)
			}
		})
	}
}
