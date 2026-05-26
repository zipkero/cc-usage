# degraded-cwd-fallback-relax

## 요약
session_id 기준 캐시가 비어있어도 같은 cwd의 다른 세션 캐시로 폴백해서, 새 세션 첫 stdin이 degraded여도 status line이 비워지지 않도록 한다.

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
- 2026-05-26: ANALYSIS 작성
- 2026-05-26: IMPLEMENT 체크리스트 작성
- 2026-05-26: IMPLEMENT 완료 (task-001, task-002, task-003 verify approved)
