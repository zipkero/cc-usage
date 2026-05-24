# session-state-fixes — SPEC

## 1. 범위

`~/.cache/cc-usage/session-state-*.json` 저장·복원·청소 경로에서 발견된 두 가지 결함을 한 patch release(v0.3.x)로 정리한다.

- **cost cache poisoning**: cost 위젯은 `total_cost_usd=0`이라도 항상 렌더되어 `result.WidgetCount`를 줄이지 않는다. 그 결과 `main.go`의 degraded-input 복원 분기(`usageDegraded := result.WidgetCount < cached.WidgetCount`)가 false가 되어 진입하지 못하고, `cost.TotalCostUsd <= 0` 보강 가드도 같이 우회된다. 그 stdin이 그대로 `saveSessionState`에 넘어가 cache가 `cost=0`으로 영구 오염된다. 한 번 오염되면 이후 cost=0이 들어와도 cache도 0이라 복구할 수 없고, TTL 5분 안에 새 정상 cost stdin이 와야만 회복된다.
- **session-state 파일 디스크 leak**: `api.go`의 `cleanOldCaches`는 `~/.cache/cc-usage/cache-*.json`(API cache)만 청소한다. `session-state-*.json`은 어떤 코드 경로에서도 삭제되지 않는다. `loadSessionState`의 `sessionStateTTL` 검사는 read 시점에 stale 데이터를 무시할 뿐 파일은 디스크에 남는다. session_id가 자주 발급되거나 cwd가 다양한 사용자는 무한 누적된다.

## 2. 목표

- **정상 비용 표시 보존**: 한 번 정상으로 표시되어 cache에 저장된 비용이, 이후 같은 session 식별자로 도착한 `cost.total_cost_usd=0` stdin에 의해 사용자가 보는 status line에서 사라지지 않는다.
- **디스크 위생**: 활성 사용자가 cc-usage를 장기간 사용해도 session-state 파일이 무한 누적되지 않고 staleness 정책에 따라 자동 정리된다.

## 3. 제약

- cc-usage의 출력 형식, CLI 플래그, Config 스키마, locale JSON 키 변경 없음.
- Zero dependency 유지, 단일 `main` 패키지 유지.
- 본 수정의 복원·청소 로직은 cache 부재 상태와 cache의 cost도 0인 상태(첫 세션, 누적 비용이 진짜 0)에는 작동하지 않아야 한다. 즉 stale 복원이 "정상 0달러 상태"를 다른 값으로 잘못 덮어쓰지 않는다.
- cleanup 정책은 기존 staleness 상수(`sessionStateTTL = 300s`)와 일관성 있는 임계값을 사용한다. 무관한 새 상수를 도입하지 않는다.
- cleanup은 `loadSessionState`처럼 read 경로를 막거나 동기 I/O를 늘리지 않는다. 기존 `cleanOldCaches`의 fire-and-forget goroutine 패턴 안에서 확장한다.
- bin/ 재빌드는 본 작업의 소스 변경으로 트리거되며, 산출물은 main + release 브랜치에 함께 commit한다 (CLAUDE.md §배포).

## 4. 제외 범위

- 시나리오 B(workspace cwd 30초 race) — 영향 작고 TTL 내 자동 해소. 별도 spec.
- 시나리오 C(session_id 없는 cwd-fallback cross-pollution) — 일반 Claude Code 세션에서는 session_id가 항상 채워져 발생하지 않음. 별도 spec.
- 시나리오 E(`noIdentity` 가드 완화로 partial 출력 차단) — 정상 출력 경로를 침범할 위험이 있어 별도 trade-off 분석 필요. 별도 spec.
- API cache(`cache-<tokenHash>.json`) 정리 정책 변경 — 본 spec 밖.
- 새 위젯, 새 외부 통합, 새 CLI 플래그 — 본 spec 밖.

## 5. 완료 조건

1. 한 세션에서 정상 cost(>0)가 cache된 뒤, 동일 session 식별자로 `cost.total_cost_usd=0`이고 다른 식별 필드(workspace/model/context)는 살아 있는 stdin이 들어왔을 때 사용자에게 보이는 cost 위젯이 cache의 정상 cost 값을 표시한다. 위젯 개수가 cache와 동일하더라도 복원 분기가 작동한다.

2. 1번 시나리오 직후 `saveSessionState`가 호출될 때, 저장되는 `CachedStdin.Cost.TotalCostUsd`는 들어온 stdin의 0이 아니라 복원 후 값(cache의 정상 cost)이거나, 저장 자체가 skip되어 기존 cache가 보존된다. 둘 중 어느 동작이든 다음 호출에서도 cache의 정상 cost가 살아 있다.

3. 직전 정상 cache가 없거나 cache의 cost도 0인 상태에서 `cost.total_cost_usd=0` stdin이 들어오면 stale 복원이 발생하지 않고 사용자에게 `$0.00`이 그대로 표시된다. "정상 첫 세션"이 stale 복원 false positive로 다른 비용으로 잘못 회복되지 않는다.

4. `~/.cache/cc-usage/session-state-*.json` 중 ModTime이 staleness 임계값을 초과한 파일이 cc-usage 호출 도중 백그라운드로 삭제된다. 단일 호출이 동기적으로 차단되지 않는다.

5. 위 변경 후 `make build`, `make build-local`, `go test ./...` 세 명령이 변경 전과 동일하게 exit 0으로 종료한다.
