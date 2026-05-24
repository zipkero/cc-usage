# v0-3-0-hardening

## 요약
cc-usage v0.2.0 운영 중 식별된 외부 명령 stall 위험, dead code, 광고/동작 불일치, 배포 외연 한계를 단일 v0.3.0 릴리스로 묶어 정리한다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [x] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-05-23: SPEC 작성
- 2026-05-23: ANALYSIS 작성
- 2026-05-23: IMPLEMENT 체크리스트 작성
- 2026-05-23: SPEC §5.11 + ANALYSIS §2.8 + task-012 추가 (origin/main의 utilization float 호환 fix 도입)
- 2026-05-24: IMPLEMENT 완료 (task-001~012 전부 verify 통과, v0.3.0 main + release push)
