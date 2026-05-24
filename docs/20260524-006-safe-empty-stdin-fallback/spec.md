# SPEC — safe-empty-stdin-fallback

## 1. 범위

cc-usage가 Claude Code의 status line 호출에서 **빈 또는 거의 빈 stdin**(예: `{}`, 또는 workspace/model/context가 모두 누락된 입력)을 받았을 때 어떤 데이터로 status line을 구성·표시할 것인지 결정한다. 안전한 fallback 메커니즘의 입력·출력·실패 모드만 다룬다.

대상 코드 표면: stdin 파싱 결과 → cache key 결정 → degraded restore → 출력 묵음/표시 판단 경로 (현재 `main.go`, `cache.go`, `stdin.go`).

## 2. 목표

- 빈 stdin이 와도 사용자가 보던 status line이 흔들리지 않고 안정적으로 유지되어, 잦은 partial/묵음 깜빡임을 제거한다.
- **다른 워크스페이스·세션의 데이터가 현재 화면에 표시되는 사건이 0회**가 되도록 정확성을 보장한다 (v0.3.5 회귀의 재발 방지).
- 안전한 fallback이 불가능한 경우엔 잘못된 정보를 추측해서 채우지 않고, v0.3.4 수준의 보수적 출력(cost + rate-limit 또는 묵음)으로 폴백한다.

## 3. 제약

- **정확성 우선 원칙**: 잘못된 정보를 표시할 가능성이 0%보다 큰 fallback은 채택하지 않는다. partial 또는 묵음 출력은 잘못된 정보 출력보다 항상 선호된다.
- zero-dependency 유지: Go 표준 라이브러리 외 의존성을 추가하지 않는다.
- 단일 `main` 패키지 유지: 서브 패키지를 만들지 않는다 (CLAUDE.md §패키지).
- 출력 채널 규칙 유지: stdout은 위젯 출력 전용, stderr는 debug/error 전용 (CLAUDE.md §출력).
- 호출당 추가 오버헤드 상한: status line은 자주 호출되므로 fallback 경로가 추가하는 I/O는 기존 `cacheLockTimeout = 200ms`와 같은 차수 안에서 처리되어야 한다.
- 동시성 안전성: 다중 cc-usage 인스턴스가 같은 캐시 파일에 접근하는 상황에서도 race·panic이 없어야 한다.
- 기존 TTL 정책과 정합: `sessionStateTTL = 300s`, `workspaceRestoreTTL = sessionStateTTL` 정합성을 깨지 않는다.
- 빈 stdin을 보내는 Claude Code 동작은 외부 변수로 가정한다: cc-usage가 이 동작 자체를 fix하지 않으며, 패턴 변경 시에도 본 fallback 로직이 silently 동작해야 한다.
- 워크스페이스 식별을 위해 새로 의존하는 신호(예: Claude Code 환경변수, `os.Getwd()` 등)는 부재해도 안전한 fallback이 보장되어야 한다.

## 4. 제외 범위

- Claude Code가 빈 stdin을 보내는 원인을 추적·수정하는 작업 (Claude Code 본체 영역).
- 새 위젯 추가, 기존 위젯의 표시 옵션 변경, status line UI/UX 개편.
- 캐시 파일 포맷 변경 또는 마이그레이션 (기존 `session-state-<key>.json` 스키마 유지).
- 외부 도구 의존성(예: filesystem watcher, daemon, 글로벌 lockfile)의 도입.
- OAuth credential 캐시 또는 API rate-limit 캐시(`cache-<tokenHash>.json`)의 동작 변경.
- rate-limit 위젯(5h/7d/7d-Sonnet)의 데이터 소스·표시 방식 변경.
- `displayPresets`/`Config.Lines`/`disabledWidgets` 등 사용자 설정 표면의 변경.

## 5. 완료 조건

1. 빈 stdin을 받았을 때 cc-usage가 **현재 워크스페이스를 식별할 수 있고** 그 워크스페이스에 대한 직전 정상 캐시가 TTL 이내로 존재하면, status line은 직전 정상 호출과 동등한 full 출력(projectInfo/model/context/cost + rate-limit)을 표시한다. — UI 관찰: 동일 워크스페이스 내 idle 후 빈 stdin이 와도 사용자가 보던 status line이 그대로 유지된다.

2. 빈 stdin을 받았을 때 cc-usage가 현재 워크스페이스를 식별할 수 없거나, 식별 결과가 보관 중인 어떤 캐시와도 매칭되지 않으면, **다른 워크스페이스의 캐시 데이터는 status line의 어떤 위젯에도 노출되지 않는다.** — UI 관찰: 사용자가 본 화면에 자신의 현재 cwd/projectInfo가 아닌 정보가 한 번도 나타나지 않는다.

3. 현재 워크스페이스 식별 불가 또는 매칭 캐시 부재 시 cc-usage는 v0.3.4와 동등한 보수적 출력 — `shouldSuppressOutput` 통과 시 cost + rate-limit 표시, 모두 미충족 시 묵음 — 으로 폴백한다. — UI 관찰: 빈 stdin + 캐시 미존재 환경에서 partial 또는 묵음만 발생, full 복원은 발생하지 않는다.

4. 정상 stdin이 도착했을 때의 처리는 fallback 도입 이전(v0.3.4)과 동일한 결과를 낸다. fallback 경로가 정상 경로의 출력을 변경하지 않는다. — Library 관찰: 정상 stdin 입력에 대한 기존 회귀 테스트가 모두 PASS 유지된다.

5. RateLimits는 fallback 대상이 아니다. fallback이 발동해도 rate-limit 데이터는 항상 API 캐시(`cache-<tokenHash>.json`)에서만 채워진다. — Library 관찰: fallback 단위 테스트에서 `ctx.RateLimits`가 session-state 캐시로부터 채워지지 않는다.

6. fallback이 발동하거나 발동하지 않는 결정의 근거(예: 매칭에 사용한 cwd, 매칭 실패 사유)가 `DEBUG=cc-usage` 환경에서 stderr 로그로 확인 가능하다. — CLI 관찰: 빈 stdin smoke를 디버그 모드로 실행하면 fallback 결정 로그가 한 줄 이상 기록된다.

7. fallback에 대한 자동 회귀 테스트가 최소 네 가지 경로를 모두 커버한다: (a) 워크스페이스 식별 + 매칭 캐시 존재 → 복원, (b) 식별 가능하지만 매칭 캐시 없음 → 미복원, (c) 워크스페이스 식별 불가 → 미복원, (d) 매칭 캐시 존재하나 TTL 초과 → 미복원. — Library 관찰: `go test ./...`에서 네 경로가 명시적 테스트 케이스로 PASS.

8. fallback 도입이 다음 v0.3.4 동작에 회귀를 만들지 않는다: `shouldSuppressOutput`의 noIdentity + rate-limit OR, `restoreUsageFields`의 Cost/Context 복원, `cleanOldSessionStates`의 glob(임시·legacy 정리 포함), `cleanOldCaches`의 무조건 호출. — Library 관찰: v0.3.4 도입 회귀 테스트가 변경 없이 PASS 유지.

9. 다중 워크스페이스를 빠르게 전환하는 시나리오(워크스페이스 A → B → A 순으로 5분 안에 진입, 각 워크스페이스에서 빈 stdin 다수 발생)에서 어느 시점에도 A의 데이터가 B 세션에, 또는 그 반대로 표시되지 않는다. — UI/Library 관찰: 워크스페이스 전환 시퀀스를 시뮬레이션한 통합 테스트에서 cross-workspace 노출 0회.

10. fallback 도입 후 `SemVer` patch bump 정책(CLAUDE.md §버전 정책)에 따라 `Makefile`, `.claude-plugin/plugin.json`, `api.go`의 `userAgent` 세 곳이 동일한 새 버전으로 갱신되어 `/plugin` UI가 update를 감지할 수 있다. — CLI 관찰: `./dist/cc-usage --version`이 새 버전을 출력하고, 세 파일의 grep 결과가 일치한다.
