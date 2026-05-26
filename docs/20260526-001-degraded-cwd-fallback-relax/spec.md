# spec.md — degraded-cwd-fallback-relax

## 1. 범위
- cc-usage 바이너리의 degraded stdin 처리 경로 한정. 구체적으로 main.go의 캐시 로딩·복원 분기와 cache.go의 캐시 조회 분기 사이의 매칭 정책.
- 영향 범위는 stdout 출력 결정 로직과 그에 영향을 주는 캐시 조회 우선순위.

## 2. 목표
- 새 세션의 첫 stdin이 degraded(workspace/model/context 비어있음)로 들어와 per-session 캐시가 만들어진 적이 없는 상황에서도, 같은 cwd의 다른 세션이 남긴 캐시를 폴백으로 사용해 직전 정상 status line을 그대로 다시 출력한다.
- 결과적으로 사용자는 Claude Code가 degraded stdin을 보내는 워밍업·재로드 구간에서도 status line이 통째로 비워지는 현상을 더 이상 보지 않는다.

## 3. 제약
- zero dependency 원칙 유지 (`go.mod`에 require 블록 추가 금지).
- 단일 `main` 패키지 유지 — 서브 패키지 생성 금지.
- stdout은 위젯 렌더 결과와 ANSI 코드만 출력한다. 디버그/에러는 stderr.
- 캐시 cross-workspace 노출 가드(SPEC §5.2 of `safe-empty-stdin-fallback`)를 약화시키지 않는다. cwd 폴백은 normalized cwd의 exact equality 매칭만 사용한다.
- 캐시 stale 가드(sessionStateTTL, workspaceRestoreTTL)는 폴백 경로에도 동일하게 적용한다.
- RateLimits는 캐시에서 복원하지 않는다 (기존 정책 유지).

## 4. 제외 범위
- Claude Code statusline 프로토콜 자체의 변경 요청. "빈 stdout = 이전 줄 유지" 같은 새 신호 도입은 외부 의존이라 다루지 않는다.
- `shouldSuppressOutput` 정책 자체의 완화 — `$0.00 │ 5h: --` 같은 알맹이 없는 라인을 강제로 그릴지 여부는 별도 결정 사안. 이 feature는 "캐시가 있으면 캐시로 그린다"의 적중률만 끌어올린다.
- 전역 단일 "마지막 출력" 파일 도입. cross-workspace 노출 위험 때문에 채택하지 않는다.
- 위젯 단위 부분 렌더 (모델만, cwd만 등). 이 feature는 stdin payload 복원 경로에만 손댄다.
- API 캐시(`cache-<tokenHash>.json`) 및 OAuth credential 처리 경로.

## 5. 완료 조건
1. session_id가 있고 그 session_id 기준의 session-state 캐시 파일이 존재하지 않을 때, 같은 cwd로 저장된 다른 세션의 session-state 캐시가 존재하면 cc-usage는 그 캐시를 사용해 직전 정상 status line과 동등한 stdout을 출력한다.
2. session_id가 있고 그 session_id 기준의 session-state 캐시는 존재하지만 그 캐시의 `cached_stdin`이 비어있어 복원에 쓸모가 없을 때, 같은 cwd의 다른 세션 캐시가 있으면 그 캐시를 폴백으로 사용해 stdout을 출력한다.
3. cwd 폴백이 적중하더라도 후보 캐시의 normalized cwd가 현재 cwd와 exact equality로 일치하지 않으면 사용하지 않는다. 즉 다른 워크스페이스의 status line이 새어 나오지 않는다.
4. cwd 폴백 후보가 `sessionStateTTL`을 초과한 경우 사용하지 않는다. 만료된 캐시로 복원한 결과가 stdout에 나가지 않는다.
5. session_id 기준의 캐시 적중으로 정상 복원이 가능한 케이스에서는 cwd 폴백을 거치지 않는다. 즉 기존 세션의 동작은 변경되지 않으며, 기존 세션의 stdout은 이 변경 전후로 동일하다.
6. degraded stdin이 와도 같은 cwd의 캐시가 어디에도 없고 RateLimits도 없으면 기존과 동일하게 무출력으로 종료한다 (이 feature는 캐시가 존재할 때의 적중률만 끌어올린다).
7. `go test ./...`가 통과한다. 신규 동작(§5.1–§5.5)을 검증하는 테스트가 추가되어 같이 통과한다.
