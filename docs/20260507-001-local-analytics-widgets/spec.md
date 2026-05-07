# Local Analytics Widgets Spec

## 범위
이 feature는 `cc-usage` status line에 로컬 분석 위젯을 추가하는 작업이다. 대상 위젯은 `sessionDuration`, `burnRate`, `cacheHit`, `tokenBreakdown`, `tokenSpeed`, `depletionTime`, `forecast`이다.

이 범위의 위젯은 Claude Code가 status line stdin으로 전달하는 현재 세션 데이터와 로컬 캐시 디렉터리에 저장되는 세션 시작 상태를 기반으로 동작해야 한다. 외부 CLI 사용량 조회, transcript 파싱, 일일 예산 누적 저장은 이 feature 범위에 포함하지 않는다.

## 목표
사용자는 status line에서 현재 세션이 얼마나 오래 진행되었는지, 토큰과 비용이 어느 속도로 소비되는지, 캐시가 얼마나 활용되는지, 현재 추세라면 rate limit에 언제 도달할 수 있는지를 확인할 수 있어야 한다.

사용자는 `normal`, `detailed`, `custom`, `preset` 설정을 통해 로컬 분석 위젯을 기존 위젯과 함께 배치할 수 있어야 한다.

분석 데이터가 부족한 환경에서도 status line 전체가 실패하지 않고, 해당 위젯만 표시되지 않거나 기존 graceful degradation 규칙에 따라 안정적으로 출력되어야 한다.

## 제약
세션 경과 시간은 로컬 파일 기반으로 추적하며, 동일 세션에 대한 동시 status line 실행이 있어도 세션 시작 시간이 경쟁 상태로 덮어써지지 않아야 한다.

위젯 계산은 status line 실행 경로에 있으므로 느린 파일 I/O나 외부 프로세스 실행을 최소화해야 한다. 이 feature는 새 외부 의존성을 추가하지 않는다.

stdin에 필요한 필드가 없거나 0으로 나누는 계산이 필요한 경우, 추정값을 임의로 만들지 않고 해당 위젯을 숨기거나 의미 있는 fallback만 사용해야 한다.

`budget`과 `todayCost`는 일일 누적 비용 상태를 저장하는 별도 기능이므로 이 feature에서 구현하지 않는다. `todoProgress`, `toolActivity`, `agentStatus`, `lastPrompt`, `sessionName`의 transcript 기반 추적도 이 feature에서 구현하지 않는다.

## 비범위
이 feature는 Codex, Gemini, z.ai 같은 외부 CLI 사용량을 조회하지 않는다.

이 feature는 `cc-usage check` 서브커맨드나 플러그인 커맨드 파일을 추가하지 않는다.

이 feature는 기존 status line의 기본 `compact` 출력 의미를 임의로 바꾸지 않는다.

이 feature는 사용자 설정 파일의 기존 필드 의미를 변경하거나 기존 위젯 ID를 제거하지 않는다.

## 완료 조건
1. `sessionDuration` 위젯은 동일한 `session_id` 입력을 여러 번 실행했을 때 최초 관찰 시점 기준의 경과 시간을 표시하고, 새 `session_id` 입력에서는 별도의 경과 시간을 표시한다.
2. `burnRate` 위젯은 세션 경과 시간이 충분하고 토큰 사용량이 있을 때 분당 토큰 소비량을 표시하며, 세션 경과 시간이 1분 미만이거나 계산할 토큰이 없으면 출력되지 않는다.
3. `cacheHit` 위젯은 `context_window.current_usage`의 cache read/input/cache creation 토큰을 기준으로 캐시 적중률을 표시하고, 입력 토큰 합계가 0이면 출력되지 않는다.
4. `tokenBreakdown` 위젯은 input, output, cache creation, cache read 토큰을 구분해 표시하며, 관련 토큰 값이 모두 0이면 출력되지 않는다.
5. `tokenSpeed` 위젯은 `cost.total_api_duration_ms`와 output token 값이 있을 때 초당 output token 속도를 표시하고, API duration이 없거나 0이면 출력되지 않는다.
6. `depletionTime` 위젯은 5시간 rate limit 사용률과 세션 경과 시간을 기준으로 한도 도달 예상 시간을 표시하고, rate limit 데이터나 유효한 소비 추세가 없으면 출력되지 않는다.
7. `forecast` 위젯은 현재 세션 비용과 세션 경과 시간을 기준으로 시간당 예상 비용을 표시하고, 비용이나 유효한 경과 시간이 없으면 출력되지 않는다.
8. `normal`, `detailed`, `custom`, `preset` 설정에서 위젯 ID 또는 preset 문자를 통해 대상 위젯을 사용할 수 있고, 등록되지 않은 위젯 ID 로그 없이 렌더링된다.
9. 필요한 입력이 빠진 stdin, degraded stdin, rate limit API 부재 상황에서도 프로그램은 panic 없이 종료하며 기존 표시 가능한 위젯은 계속 출력된다.
10. `go test ./...`가 통과하고, 새 위젯의 정상 계산과 데이터 부족 시 숨김 동작을 검증하는 테스트가 포함된다.

## 열린 질문
없음.
