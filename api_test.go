package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
