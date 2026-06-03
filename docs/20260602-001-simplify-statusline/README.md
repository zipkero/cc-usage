# simplify-statusline

## 요약

cc-usage의 데이터 출처와 표시 항목을 공식 status line 문서 수준으로 단순화한다. OAuth API·
credential·캐시·degraded 복원·transcript cost 추정 등 무거운 백엔드 로직을 걷어내고 stdin·git만으로
동작하게 하며, 공식 문서에 없는 `7d-S`와 analytics 항목을 제거한다. 위젯 구조와 멀티라인은 유지한다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [x] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-06-02: SPEC 작성
- 2026-06-02: ANALYSIS 작성
- 2026-06-02: IMPLEMENT 체크리스트 작성
- 2026-06-03: IMPLEMENT 완료
