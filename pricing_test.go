package main

import (
	"math"
	"testing"
)

// TestLookupPricing_KnownModels는 등록된 모델이 lookup에서 성공 반환되는지 검증한다.
func TestLookupPricing_KnownModels(t *testing.T) {
	knownModels := []string{
		"claude-opus-4-7",
		"claude-opus-4-6",
		"claude-opus-4-5",
		"claude-sonnet-4-6",
		"claude-sonnet-4-5",
		"claude-haiku-4-5",
	}

	for _, modelID := range knownModels {
		t.Run(modelID, func(t *testing.T) {
			p, ok := lookupPricing(modelID)
			if !ok {
				t.Fatalf("lookupPricing(%q) = false, want true", modelID)
			}
			if p.Input <= 0 {
				t.Errorf("Input price <= 0 for %q", modelID)
			}
			if p.Output <= 0 {
				t.Errorf("Output price <= 0 for %q", modelID)
			}
			if p.CacheRead <= 0 {
				t.Errorf("CacheRead price <= 0 for %q", modelID)
			}
			if p.CacheCreation5m <= 0 {
				t.Errorf("CacheCreation5m price <= 0 for %q", modelID)
			}
			if p.CacheCreation1h <= 0 {
				t.Errorf("CacheCreation1h price <= 0 for %q", modelID)
			}
			// 1h cache write가 5m보다 비싸야 한다
			if p.CacheCreation1h <= p.CacheCreation5m {
				t.Errorf("CacheCreation1h (%.4f) should be > CacheCreation5m (%.4f) for %q",
					p.CacheCreation1h, p.CacheCreation5m, modelID)
			}
			// cache read가 input보다 저렴해야 한다
			if p.CacheRead >= p.Input {
				t.Errorf("CacheRead (%.4f) should be < Input (%.4f) for %q",
					p.CacheRead, p.Input, modelID)
			}
		})
	}
}

// TestLookupPricing_UnknownModel는 미등록 모델이 false를 반환하는지 검증한다.
func TestLookupPricing_UnknownModel(t *testing.T) {
	cases := []string{
		"claude-unknown-model",
		"gpt-4o",
		"",
		"claude-opus-4-7-extra",
		"CLAUDE-OPUS-4-7", // 대소문자 불일치
	}

	for _, modelID := range cases {
		t.Run("miss/"+modelID, func(t *testing.T) {
			p, ok := lookupPricing(modelID)
			if ok {
				t.Fatalf("lookupPricing(%q) = true, want false", modelID)
			}
			// miss일 때 반환된 구조체는 zero value여야 한다
			if p != (modelPricing{}) {
				t.Errorf("lookupPricing(%q) returned non-zero modelPricing on miss: %+v", modelID, p)
			}
		})
	}
}

// TestEstimateCost_Sonnet46는 claude-sonnet-4-6의 단가로 합산 정확성을 검증한다.
//
// 단가: Input $3, Output $15, CacheRead $0.30, Cache5m $3.75, Cache1h $6.00
// 입력: input=1_000_000, output=500_000, cacheRead=200_000, cache5m=100_000, cache1h=50_000
// 기대:
//   1.0 × 3.00 = $3.00
//   0.5 × 15.00 = $7.50
//   0.2 × 0.30 = $0.06
//   0.1 × 3.75 = $0.375
//   0.05 × 6.00 = $0.30
//   합계 = $11.235
func TestEstimateCost_Sonnet46(t *testing.T) {
	p, ok := lookupPricing("claude-sonnet-4-6")
	if !ok {
		t.Fatal("lookupPricing(claude-sonnet-4-6) returned false")
	}

	usage := transcriptUsage{
		InputTokens:              1_000_000,
		OutputTokens:             500_000,
		CacheReadInputTokens:     200_000,
		CacheCreation5mTokens:    100_000,
		CacheCreation1hTokens:    50_000,
		CacheCreationInputTokens: 150_000, // 5m+1h 합산 (estimateCost에서 미사용)
	}

	want := 11.235
	got := estimateCost(usage, p)

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("estimateCost() = %.9f, want %.9f", got, want)
	}
}

// TestEstimateCost_Opus47는 claude-opus-4-7 단가로 합산 정확성을 검증한다.
//
// 단가: Input $5, Output $25, CacheRead $0.50, Cache5m $6.25, Cache1h $10.00
// 입력: input=2_000_000, output=1_000_000, cacheRead=500_000, cache5m=300_000, cache1h=100_000
// 기대:
//   2.0 × 5.00 = $10.00
//   1.0 × 25.00 = $25.00
//   0.5 × 0.50 = $0.25
//   0.3 × 6.25 = $1.875
//   0.1 × 10.00 = $1.00
//   합계 = $38.125
func TestEstimateCost_Opus47(t *testing.T) {
	p, ok := lookupPricing("claude-opus-4-7")
	if !ok {
		t.Fatal("lookupPricing(claude-opus-4-7) returned false")
	}

	usage := transcriptUsage{
		InputTokens:           2_000_000,
		OutputTokens:          1_000_000,
		CacheReadInputTokens:  500_000,
		CacheCreation5mTokens: 300_000,
		CacheCreation1hTokens: 100_000,
	}

	want := 38.125
	got := estimateCost(usage, p)

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("estimateCost() = %.9f, want %.9f", got, want)
	}
}

// TestEstimateCost_Haiku45는 claude-haiku-4-5 단가로 합산 정확성을 검증한다.
//
// 단가: Input $1, Output $5, CacheRead $0.10, Cache5m $1.25, Cache1h $2.00
// 입력: input=500_000, output=100_000, cacheRead=0, cache5m=50_000, cache1h=0
// 기대:
//   0.5 × 1.00 = $0.50
//   0.1 × 5.00 = $0.50
//   0.0 × 0.10 = $0.00
//   0.05 × 1.25 = $0.0625
//   0.0 × 2.00 = $0.00
//   합계 = $1.0625
func TestEstimateCost_Haiku45(t *testing.T) {
	p, ok := lookupPricing("claude-haiku-4-5")
	if !ok {
		t.Fatal("lookupPricing(claude-haiku-4-5) returned false")
	}

	usage := transcriptUsage{
		InputTokens:           500_000,
		OutputTokens:          100_000,
		CacheReadInputTokens:  0,
		CacheCreation5mTokens: 50_000,
		CacheCreation1hTokens: 0,
	}

	want := 1.0625
	got := estimateCost(usage, p)

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("estimateCost() = %.9f, want %.9f", got, want)
	}
}

// TestEstimateCost_ZeroUsage는 토큰이 모두 0일 때 cost가 0인지 검증한다.
func TestEstimateCost_ZeroUsage(t *testing.T) {
	p, ok := lookupPricing("claude-sonnet-4-6")
	if !ok {
		t.Fatal("lookupPricing(claude-sonnet-4-6) returned false")
	}

	usage := transcriptUsage{}
	got := estimateCost(usage, p)
	if got != 0 {
		t.Errorf("estimateCost(zero usage) = %f, want 0", got)
	}
}

// TestEstimateCost_FallbackCase는 CacheCreation5mTokens만 채워진 폴백 케이스(5m 귀속)를 검증한다.
// transcript.go의 scanLastAssistantEntry가 ephemeral 합산이 0이면 top-level을 5m에 귀속시키므로,
// estimateCost는 CacheCreation5mTokens만 사용해 합산해야 한다.
//
// 단가: claude-sonnet-4-6, Input $3, Cache5m $3.75
// 입력: input=1_000_000, output=0, cacheRead=0, cache5m=400_000(폴백 귀속), cache1h=0
// 기대: 1.0 × 3.00 + 0.4 × 3.75 = $3.00 + $1.50 = $4.50
func TestEstimateCost_FallbackCase(t *testing.T) {
	p, ok := lookupPricing("claude-sonnet-4-6")
	if !ok {
		t.Fatal("lookupPricing(claude-sonnet-4-6) returned false")
	}

	usage := transcriptUsage{
		InputTokens:              1_000_000,
		CacheCreationInputTokens: 400_000, // 원본 top-level (estimateCost에서 미사용)
		CacheCreation5mTokens:    400_000, // scanLastAssistantEntry가 귀속한 값
		CacheCreation1hTokens:    0,
	}

	want := 4.50
	got := estimateCost(usage, p)

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("estimateCost() = %.9f, want %.9f (fallback 5m case)", got, want)
	}
}

// TestPricingTable_Consistency는 단가표 내 모든 모델에 대해 가격 관계 일관성을 검증한다.
// cache read < input, 1h write > 5m write 관계가 모든 모델에서 성립해야 한다.
func TestPricingTable_Consistency(t *testing.T) {
	for modelID, p := range modelPricingTable {
		if p.CacheRead >= p.Input {
			t.Errorf("[%s] CacheRead (%.4f) should be < Input (%.4f)", modelID, p.CacheRead, p.Input)
		}
		if p.CacheCreation1h <= p.CacheCreation5m {
			t.Errorf("[%s] CacheCreation1h (%.4f) should be > CacheCreation5m (%.4f)", modelID, p.CacheCreation1h, p.CacheCreation5m)
		}
		if p.CacheCreation5m <= p.Input {
			t.Errorf("[%s] CacheCreation5m (%.4f) should be > Input (%.4f)", modelID, p.CacheCreation5m, p.Input)
		}
	}
}
