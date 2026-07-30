# stdin-resilience

## 요약

stdin의 한 섹션이 깨져도 status line 전체가 사라지지 않게 한다. 지금은 어떤 decode 오류든 빈 입력으로
떨어져 무출력 조건을 통과하므로 필드 하나의 타입 불일치가 전면 블랙아웃이 된다. 최상위를 섹션 단위로
격리해 깨진 섹션만 버리고, 부분 실패는 stderr로만 알린다. 함께 rate limit의 `resets_at` 부재 판정을
표시 문자열 비교에서 떼어내 시각 계산으로 옮긴다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [ ] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-07-30: SPEC 작성
- 2026-07-30: ANALYSIS 작성
- 2026-07-30: IMPLEMENT 체크리스트 작성
