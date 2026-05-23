package main

import (
	"strings"
	"testing"
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
