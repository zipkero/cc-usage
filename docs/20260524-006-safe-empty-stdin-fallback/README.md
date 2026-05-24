# safe-empty-stdin-fallback

## 요약

Claude Code가 cc-usage에 빈 stdin을 지속적으로 보내는 상황에서 status line이 깜빡임 없이 안정적으로 유지되도록 하되, v0.3.5에서 발생한 "다른 워크스페이스 정보 혼합" 회귀가 절대 재발하지 않도록 정확성을 우선하는 안전한 fallback 메커니즘을 정의한다.

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
