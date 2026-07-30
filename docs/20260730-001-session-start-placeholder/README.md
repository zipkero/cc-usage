# session-start-placeholder

## 요약

세션의 첫 API 응답 전에 statusline이 미측정 값을 `0`으로 표시하는 문제를 고친다. context와 5h·7d rate limit 칸을
ASCII placeholder + dim으로 표기해 "아직 측정되지 않음"이 화면에서 구별되게 하고, 첫 응답 이후 동작은 그대로 둔다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [x] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-07-30: SPEC 작성
- 2026-07-30: ANALYSIS 작성
- 2026-07-30: IMPLEMENT 체크리스트 작성
- 2026-07-30: task-001~005 구현·검증 완료 (v0.5.5)
