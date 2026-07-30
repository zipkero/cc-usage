# statusline-schema-catchup

## 요약

Claude Code status line 프로토콜의 현재 공식 스펙과 cc-usage 구현 사이의 차이를 좁힌다. 추가된 stdin 필드를 수용·노출하고, 컨텍스트
퍼센트 계산을 공식 문서가 정의한 input-only 의미에 맞추며, 설정 디렉토리를 옮겨 쓰는 환경에서 설치 절차가 잘못된 파일을 고르는 문제를
고친다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [ ] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-07-27: SPEC 작성
- 2026-07-27: ANALYSIS 작성
- 2026-07-30: 승인 전 확인 3건 확정(§5 D1·D7 채택 유지, D3은 필드별 규칙으로 변경) +
  M0·M1·M2 이후 달라진 전제를 SPEC·ANALYSIS에 반영. `ROADMAP.md` 서비스 완료 기준 6번은 git 유래
  캐시를 허용하도록 좁혀 §5 D6의 충돌을 닫았다. 미답 항목 없음 — IMPLEMENT 작성 가능.
- 2026-07-30: IMPLEMENT 체크리스트 작성
