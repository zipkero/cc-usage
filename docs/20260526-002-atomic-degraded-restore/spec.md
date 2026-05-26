# atomic-degraded-restore

## 1. 범위

cc-usage가 degraded stdin(`{}` 또는 workspace/model/cost/context 일부가 비어서 들어온 Claude Code status line 입력)을 받았을 때, on-disk SessionState 캐시로부터 stdin 필드를 복원하는 정책. 구체적으로 `main.go`의 복원 분기(`restoreWorkspace` / `restoreUsageFields` / `costRegressed`)가 다루는 Workspace·Model·Cost·ContextWindow 네 필드의 복원 결정과, 그 결과 status line에 나타나는 출력의 일관성.

## 2. 목표

degraded stdin이 들어왔을 때 status line이 "절반만 멀쩡한" 상태로 노출되지 않게 한다. 사용자 관점에서 한 번의 호출은 다음 중 하나여야 한다.

- 직전 정상 출력과 동등한 수준으로 identity(projectInfo·model)와 usage(cost·context)가 함께 보인다.
- 또는 출력이 비어 있다(혹은 warmup 예외로 rate-limit만 노출된다).

"identity 위젯은 빠지고 cost·context만 살아남은 출력" 같은 중간 상태가 발생하지 않아야 한다. 사용자가 지금 어느 프로젝트·어느 모델에서 얼마를 썼는지 매핑할 수 없는 출력은 정보로서 의미가 없고, 오히려 다른 세션의 누계를 보는 것 같은 오해를 부른다.

## 3. 제약

- zero dependency·단일 `main` 패키지·서브 패키지 금지 규칙을 유지한다 (CLAUDE.md).
- cross-workspace 노출 금지. fallback은 현재 cwd와 normalized exact-match로만 캐시를 매칭한다 (v0.3.7/v0.3.8의 `loadByWorkspaceCwd` + `shouldRestoreWorkspace` 가드 유지).
- `RateLimits`는 SessionState 캐시에서 복원하지 않는다. 항상 API 캐시(`cache-<tokenHash>.json`)에서 fresh하게 가져온다.
- warmup 예외(stdin·캐시 모두에서 identity가 확보되지 않아도 rate-limit 데이터가 있으면 5h/7d만 출력하는 동작)는 유지한다.
- SessionState 캐시 파일 포맷(`session-state-<key>.json` JSON 스키마)을 바꾸지 않는다. 기존 캐시 파일은 그대로 호환되어야 한다.
- 기존 TTL 값(`sessionStateTTL = 300s`)을 유지하거나 더 좁히는 방향으로만 조정한다. 더 늘리지 않는다.
- stdout은 위젯 렌더링 결과 전용. 디버그 로그는 stderr.

## 4. 제외 범위

- API 캐시(rate limit) 관련 정책·스키마·TTL.
- 위젯 추가, 삭제, 또는 렌더 동작 변경.
- 새로운 캐시 파일·새로운 식별자·새로운 영속 저장소 추가.
- Claude Code가 보내는 stdin 자체의 변동성 보정(예: 부분 누락된 fresh stdin이 들어올 때 stdin payload 스키마를 추론·확장하는 일).
- presetCharToWidget·displayPresets 등 출력 레이아웃 설정.

## 5. 완료 조건

1. 캐시 복원이 가능한 조건(현재 cwd가 cached workspace와 normalized exact-match이고, `SavedAt`가 `sessionStateTTL` 이내)을 만족하면, degraded stdin에서 비어 있는 Workspace·Model·Cost·ContextWindow 필드가 **모두 같은 결정에 따라** 캐시 값으로 채워진다. 이때 stdin이 이미 fresh로 들고 온 필드는 캐시 값으로 덮어쓰지 않는다 — 결정의 단위는 "복원할지 말지" 한 번이고, 채움은 빈 자리에 한정된다. 그 결과 projectInfo·model·context·cost 위젯이 직전 정상 상태와 동등하게 함께 렌더된다.
2. 위 복원 조건을 만족하지 못하면(현재 cwd 미식별, cached cwd와 mismatch, `SavedAt`가 TTL 초과 등), Workspace·Model·Cost·ContextWindow 중 어느 한 필드도 캐시에서 복원되지 않는다. projectInfo·model이 빠진 채 cost·context만 표시되는 출력은 발생하지 않는다.
3. 위 (2)의 경우(복원 조건 미충족), stdin 자체에도 identity가 없으면 status line은 빈 문자열을 출력하거나(`shouldSuppressOutput`), 기존 warmup 예외에 따라 rate-limit 위젯만 노출된다. stdin이 일부 필드(예: model 또는 workspace)를 fresh하게 갖고 있으면 그 fresh 값으로 가능한 위젯이 렌더되고, 이 분기에서는 캐시에서 추가 보강이 일어나지 않는다(보강은 §5.1의 결정에 따른다).
4. 어떤 경우에도 현재 cwd가 아닌 다른 워크스페이스의 캐시 값(workspace 경로, model id, cost, context_window)이 출력에 노출되지 않는다.
5. `RateLimits`는 SessionState 캐시에서 복원되지 않으며, status line의 5h/7d/7dSonnet 표시는 항상 API 캐시 경로의 값에서만 파생된다.
6. SessionState 캐시 저장(`saveSessionState`) 시, 캐시에서 복원해 채워진 필드가 다음 저장 본문에 그대로 누적·고착되지 않는다. 즉, degraded 호출이 반복되어도 캐시 본문의 timestamp(`SavedAt`)는 갱신되더라도, 캐시가 한 번 만들어진 시점의 identity·usage 값이 영구히 자기참조로 살아남지는 않는다.
