# v0-3-x-cleanup

## 요약
v0.3.0 release 이후 발견된 설계 문서 정합성(DESIGN.md의 historical Translations 예시)과 빌드 부산물 위생(.gitignore의 stray binary 패턴) 두 항목을 한 patch release로 정리.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [x] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-05-24: SPEC 작성
- 2026-05-24: ANALYSIS 작성
- 2026-05-24: IMPLEMENT 체크리스트 작성
- 2026-05-24: SPEC §5.4 + task-003 추가 (DESIGN.md / ROADMAP.md 통째 제거 + CLAUDE.md 단일 출처 명시)
- 2026-05-24: IMPLEMENT 완료 (task-001~003 전부 verify 통과)
