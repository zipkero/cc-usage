# atomic-degraded-restore

## 요약
degraded stdin에서 SessionState 캐시로부터의 필드 복원을 단일 결정으로 묶어, identity(projectInfo·model)와 usage(cost·context)가 함께 살아남거나 함께 빠지도록 만든다. "절반만 멀쩡한 status line" 발생 자체를 구조적으로 제거한다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [x] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-05-26: SPEC 작성
- 2026-05-26: SPEC §5.1·§5.3 E1 해석 명확화 + ANALYSIS 작성
- 2026-05-26: IMPLEMENT 체크리스트 작성
- 2026-05-26: task-001 ~ task-007 구현·검증 완료 (v0.3.9)
