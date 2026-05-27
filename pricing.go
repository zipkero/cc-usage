package main

// modelPricing는 단일 모델의 토큰 카테고리별 단가를 보유한다.
// 단위: USD per MTok (백만 토큰당 달러).
//
// 출처: https://platform.claude.com/docs/en/about-claude/pricing
// 조회일: 2026-05-27
type modelPricing struct {
	Input            float64 // 기본 입력 토큰
	Output           float64 // 출력 토큰
	CacheRead        float64 // 캐시 히트(읽기)
	CacheCreation5m  float64 // 5분 캐시 쓰기
	CacheCreation1h  float64 // 1시간 캐시 쓰기
}

// modelPricingTable은 base model ID → 단가의 hardcode embed 단가표다.
//
// 키는 transcript의 message.model 형태 (예: "claude-opus-4-7").
// 가격 변경 시 SemVer patch bump 동반 필요 (CLAUDE.md §버전 정책).
//
// 출처: https://platform.claude.com/docs/en/about-claude/pricing
// 조회일: 2026-05-27
var modelPricingTable = map[string]modelPricing{
	// Claude Opus 4.7 — $5/$25 input/output, 5m write $6.25, 1h write $10, read $0.50
	"claude-opus-4-7": {
		Input:           5.00,
		Output:          25.00,
		CacheRead:       0.50,
		CacheCreation5m: 6.25,
		CacheCreation1h: 10.00,
	},
	// Claude Opus 4.6 — 동일 단가
	"claude-opus-4-6": {
		Input:           5.00,
		Output:          25.00,
		CacheRead:       0.50,
		CacheCreation5m: 6.25,
		CacheCreation1h: 10.00,
	},
	// Claude Opus 4.5 — 동일 단가
	"claude-opus-4-5": {
		Input:           5.00,
		Output:          25.00,
		CacheRead:       0.50,
		CacheCreation5m: 6.25,
		CacheCreation1h: 10.00,
	},
	// Claude Sonnet 4.6 — $3/$15 input/output, 5m write $3.75, 1h write $6, read $0.30
	"claude-sonnet-4-6": {
		Input:           3.00,
		Output:          15.00,
		CacheRead:       0.30,
		CacheCreation5m: 3.75,
		CacheCreation1h: 6.00,
	},
	// Claude Sonnet 4.5 — 동일 단가
	"claude-sonnet-4-5": {
		Input:           3.00,
		Output:          15.00,
		CacheRead:       0.30,
		CacheCreation5m: 3.75,
		CacheCreation1h: 6.00,
	},
	// Claude Haiku 4.5 — $1/$5 input/output, 5m write $1.25, 1h write $2, read $0.10
	"claude-haiku-4-5": {
		Input:           1.00,
		Output:          5.00,
		CacheRead:       0.10,
		CacheCreation5m: 1.25,
		CacheCreation1h: 2.00,
	},
}

// lookupPricing는 modelID에 대응하는 단가를 반환한다.
// 등록된 모델이면 (modelPricing, true), 미등록이면 (modelPricing{}, false).
func lookupPricing(modelID string) (modelPricing, bool) {
	p, ok := modelPricingTable[modelID]
	return p, ok
}

// estimateCost는 transcript usage와 단가표를 받아 추정 비용(USD)을 반환한다.
//
// 합산 공식:
//   input × p.Input + output × p.Output + cacheRead × p.CacheRead
//   + cacheCreation5m × p.CacheCreation5m + cacheCreation1h × p.CacheCreation1h
//
// 모든 토큰 수는 /1_000_000 하여 MTok 단위로 환산 후 단가를 곱한다.
//
// ephemeral 분리 보존 필드(CacheCreation5mTokens, CacheCreation1hTokens)가 모두 0이고
// CacheCreationInputTokens > 0인 경우, transcript.go의 scanLastAssistantEntry가
// 이미 CacheCreation5mTokens에 귀속해두므로 여기서는 그 값을 그대로 소비한다.
func estimateCost(usage transcriptUsage, p modelPricing) float64 {
	const mTok = 1_000_000.0

	cost := float64(usage.InputTokens)/mTok*p.Input +
		float64(usage.OutputTokens)/mTok*p.Output +
		float64(usage.CacheReadInputTokens)/mTok*p.CacheRead +
		float64(usage.CacheCreation5mTokens)/mTok*p.CacheCreation5m +
		float64(usage.CacheCreation1hTokens)/mTok*p.CacheCreation1h

	return cost
}
