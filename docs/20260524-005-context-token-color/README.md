# context-token-color

## 요약
context 위젯의 토큰 수 표시(`formatTokens`)에 절대값 기반 색 분기 추가 — 256K 노랑 / 512K 빨강. 1M context model에서 clear 시점 시각 cue. percent 색과 분리된 독립 신호. v0.3.3 patch.

샘플 출력:
```
정상     : ████░░░░ 30% 60K          ← 토큰 색 없음
256K 도달: █████░░░ 26% 256K         ← 토큰 노란색
512K 도달: ████████ 51% 512K         ← 토큰 빨간색
```

## 상태
- [x] SPEC
- [x] ANALYSIS
- [ ] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-05-24: SPEC 작성
- 2026-05-24: ANALYSIS 작성
- 2026-05-24: IMPLEMENT 체크리스트 작성
