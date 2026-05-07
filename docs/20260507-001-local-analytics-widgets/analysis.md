# Local Analytics Widgets Analysis

## 근거
현재 코드는 `StdinInput`에 이번 범위에 필요한 대부분의 입력을 이미 가지고 있다. `context_window.current_usage`에는 input, output, cache creation, cache read 토큰이 있고, `cost`에는 현재 세션 비용과 API duration이 있으며, `rate_limits.five_hour`와 API fallback 결과는 기존 rate limit 위젯에서 사용된다.

위젯 실행 구조는 `Widget` 인터페이스, 전역 registry, `displayPresets`, `presetCharToWidget`로 구성되어 있다. `presetCharToWidget`에는 이번 대상 위젯 문자가 이미 매핑되어 있지만, 실제 `registerWidget` 호출은 core/project 위젯에만 존재한다.

기존 `cache.go`에는 cache key 생성, 파일 잠금, atomic write, session state 저장/로드가 있다. 이 기능은 idle/degraded 입력 복원용 상태를 저장하고 있으므로, 분석 위젯의 세션 시작 시간 저장은 같은 잠금/atomic write 유틸리티를 재사용하되 상태 모델은 분리하는 것이 적합하다.

확인한 명령:
- `rg --files`
- `rg "registerWidget" -n -g "*.go"`
- `go test ./...`
- `Get-Content`로 `stdin.go`, `widget.go`, `cache.go`, `format.go`, `render.go`, 기존 테스트 확인

## SPEC 추적
- SPEC 완료 조건 1: 구조, 데이터 흐름, 저장 경계, 검증 관점
- SPEC 완료 조건 2: 구조, 데이터 흐름, 검증 관점
- SPEC 완료 조건 3: 데이터 흐름, 인터페이스, 검증 관점
- SPEC 완료 조건 4: 데이터 흐름, 인터페이스, 검증 관점
- SPEC 완료 조건 5: 데이터 흐름, 인터페이스, 검증 관점
- SPEC 완료 조건 6: 데이터 흐름, 영향 범위, 리스크, 검증 관점
- SPEC 완료 조건 7: 데이터 흐름, 인터페이스, 검증 관점
- SPEC 완료 조건 8: 인터페이스, 영향 범위, 검증 관점
- SPEC 완료 조건 9: 리스크, 검증 관점
- SPEC 완료 조건 10: 검증 관점

## 구조
분석 위젯은 새 위젯 그룹으로 분리하는 것이 적합하다. 기존 파일 배치 관례상 `widgets_core.go`는 model/context/cost/rate limit, `widgets_project.go`는 projectInfo를 담당하므로, 이번 범위는 `widgets_analytics.go` 같은 별도 파일에 두는 구성이 자연스럽다.

세션 시작 시간 추적은 기존 `SessionState`에 합치지 않는다. `SessionState`는 마지막 정상 stdin과 마지막 출력 복원이라는 목적이 있고 TTL도 idle 복원에 맞춰져 있다. `sessionDuration`, `burnRate`, `depletionTime`, `forecast`는 “세션 시작 기준 시각”만 안정적으로 필요하므로 별도 상태가 더 작고 검증하기 쉽다.

세션 시작 상태는 cache key 기준으로 저장한다. key 생성은 기존 `sessionCacheKey`를 재사용해 `session_id`, remote session, agent, transcript, cwd 순서를 유지한다. 저장 경로는 `~/.cache/cc-usage/sessions/` 아래에 두면 기존 `session-state-*.json`과 목적이 분리된다.

동시 실행 대응은 기존 cache lock과 atomic write 유틸리티를 재사용한다. 최초 생성은 “이미 있으면 읽고, 없으면 생성” 흐름이어야 하며, 경쟁 상태에서 늦게 들어온 실행이 시작 시간을 덮어쓰지 않아야 한다.

## 데이터 흐름
`sessionDuration`은 `sessionCacheKey(ctx.Stdin)`으로 세션 식별자를 만들고, 해당 세션의 시작 Unix timestamp를 읽거나 생성한 뒤 `time.Now()`와의 차이를 표시한다. cache key를 만들 수 없으면 출력하지 않는다.

`burnRate`는 세션 경과 분과 `total_input_tokens + total_output_tokens`를 사용한다. 경과 시간이 1분 미만이면 표시하지 않는다. 계산 결과는 기존 `formatTokens`와 `/m` suffix를 사용하는 형태가 기존 스타일과 맞다.

`cacheHit`은 `cache_read_input_tokens / (input_tokens + cache_creation_input_tokens + cache_read_input_tokens)`를 사용한다. 분모가 0이면 표시하지 않는다. 이 값은 높을수록 좋은 지표이므로 기존 `getColorForPercent`의 “높을수록 위험” 색상 기준을 그대로 쓰면 의미가 반대로 보일 수 있다.

`tokenBreakdown`은 current usage의 input, output, cache creation, cache read 값을 그대로 분리해 렌더링한다. 모든 값이 0이면 표시하지 않는다.

`tokenSpeed`는 output token을 `total_api_duration_ms` 초 단위로 나눈다. output token은 current usage output이 있으면 우선하고, 없으면 total output token fallback을 사용할 수 있다. API duration이 nil 또는 0이면 표시하지 않는다.

`depletionTime`은 5시간 rate limit 사용률과 세션 경과 시간을 사용한다. 사용률은 기존 rate limit 위젯과 같은 우선순위인 stdin `rate_limits.five_hour`, API fallback `UsageLimits.FiveHour` 순서가 맞다. 사용률이 0 이하이거나 경과 시간이 1분 미만이면 추세가 없으므로 표시하지 않는다.

`forecast`는 `cost.total_cost_usd / elapsed_minutes * 60`으로 시간당 비용을 계산한다. 비용이 0 이하이거나 유효한 경과 시간이 없으면 표시하지 않는다.

## 인터페이스
새 사용자 설정 필드는 필요하지 않다. 위젯은 기존 `displayMode`, `lines`, `preset`, `disabledWidgets`를 통해 노출된다.

`normal`은 현재 이미 `sessionDuration`, `burnRate`를 포함하도록 의도되어 있다. 이번 범위에서는 `todoProgress`가 아직 등록되지 않은 상태여도 로컬 분석 위젯이 정상 등록되면 unknown widget 로그가 줄어드는 방향으로 동작한다.

`detailed`은 이번 feature의 주요 진입점이다. 등록되는 로컬 분석 위젯을 detailed preset에 배치해야 사용자가 별도 custom 설정 없이 기능을 확인할 수 있다. 다만 `budget/todayCost`와 transcript 기반 위젯은 이번 범위가 아니므로 detailed에 추가하지 않는다.

`custom`은 위젯 ID를 직접 쓰는 방식이고, `preset`은 기존 문자 매핑을 사용한다. 현재 `D`, `B`, `H`, `N`, `Q`, `E`, `W`가 대상 위젯에 매핑되어 있으므로, 구현 시 registry 등록과 렌더링만 완성되면 preset 경로는 큰 변경이 필요하지 않다.

## 영향 범위
주요 영향 파일은 위젯 등록과 레이아웃을 담당하는 `widget.go`, stdin 입력을 사용하는 새 analytics 위젯 파일, 세션 시작 상태를 저장할 cache 관련 코드, 포매팅 또는 테스트 보조 코드다.

기존 compact 출력은 변경하지 않는 것이 맞다. compact는 현재 MVP status line의 짧은 표시 역할이므로, 분석 위젯은 normal/detailed/custom/preset 경로로 노출하는 편이 SPEC 비범위와 맞다.

기존 rate limit API 호출 흐름은 변경할 필요가 없다. `depletionTime`은 `Context.RateLimits`를 읽기만 해야 하며, 새 API 호출을 만들면 이번 feature의 로컬 분석 범위를 넘어선다.

## 리스크
세션 시작 시간 저장이 기존 idle 복원 cache와 섞이면 오래된 출력 복원 TTL과 분석 기준 시간이 서로 영향을 줄 수 있다. 상태 분리가 이 리스크를 줄인다.

시간 기반 계산은 테스트가 불안정해지기 쉽다. 구현에서는 현재 시각 주입 지점을 두거나, 상태 파일의 timestamp를 테스트에서 직접 제어할 수 있어야 한다.

`cacheHit`은 높을수록 좋은 지표라서 rate limit 색상 함수를 그대로 쓰면 사용자가 의미를 반대로 해석할 수 있다. 렌더링 색상은 별도 기준을 두거나 중립 색상으로 처리하는 판단이 필요하다.

`depletionTime`은 단순 선형 예측이다. 세션 초반에는 값이 크게 흔들릴 수 있으므로 1분 미만 숨김과 사용률 0 숨김은 필수다. 이 예측을 “정확한 reset 시각”처럼 표현하지 않아야 한다.

PowerShell에서 일부 UTF-8 문서가 깨져 보이는 현상이 관찰됐다. 구현 자체의 blocker는 아니지만 문서 확인 명령 출력만으로 텍스트 품질을 판단하면 안 된다.

## 검증 관점
단위 테스트는 위젯별 `GetData`의 nil 조건과 정상 계산을 우선 검증해야 한다. 특히 0 분모, nil API duration, 1분 미만 세션, rate limit 부재를 각각 확인해야 한다.

세션 시작 상태 테스트는 같은 key를 두 번 읽을 때 시작 시간이 유지되는지, 다른 key에서는 별도 시작 시간이 만들어지는지 확인해야 한다.

오케스트레이션 테스트는 custom lines와 preset 문자를 통해 대상 위젯이 registry에서 정상 실행되는지 확인해야 한다. detailed mode는 unknown widget 로그가 아니라 표시 가능한 위젯만 렌더링되는지를 결과로 검증한다.

통합 검증은 `go test ./...`를 기준으로 한다. 필요하면 helper process 방식의 기존 `main_integration_test.go` 패턴을 재사용해 실제 config와 stdin으로 status line 출력을 확인할 수 있다.

## Decision Points
없음.

## 열린 질문
없음.
