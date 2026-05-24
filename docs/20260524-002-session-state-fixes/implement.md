# session-state-fixes — IMPLEMENT

- [x] task-001: `main.go` degraded-input 복원 블록에 cost-regression 신호 추가
  - 목적: 한 세션의 cache에 정상 cost(>0)가 살아 있는 상태에서 `cost.total_cost_usd=0` stdin이 들어오면, 사용자에게 보이는 cost 위젯이 cache의 정상 cost 값을 표시하고, 같은 호출의 `saveSessionState`는 그 복원 후 값을 저장한다 (cache가 0으로 덮어써지지 않음). 직전 정상 cache가 없거나 cache의 cost도 0이거나 `SavedAt`이 sessionStateTTL을 초과한 상태에서는 복원이 발생하지 않고 `$0.00`이 그대로 표시된다
  - 접근: `main.go` 83–117의 복원 블록 안에 `costRegressed := ctx.Stdin.Cost.TotalCostUsd == 0 && cached.CachedStdin.Cost.TotalCostUsd > 0 && cached.SavedAt > 0 && time.Since(time.Unix(cached.SavedAt, 0)) < sessionStateTTL` 판정을 추가하고, true일 때 `ctx.Stdin.Cost = cached.CachedStdin.Cost`로 cost field만 복원한다. `usageDegraded` 분기는 그대로 둔다 (cost 외 케이스도 cost 복원을 자기 분기 안에서 계속 처리). re-orchestrate 트리거 조건을 `restoreWorkspace || usageDegraded || costRegressed`로 확장한다
  - 검증 조건:
    - 결과:
      - cache에 cost=5.20인 SessionState가 있고 동일 cacheKey의 stdin이 cost=0(다른 필드 정상)으로 들어오면 출력의 cost 위젯이 `$5.20`(또는 그에 해당하는 포맷 결과)을 표시한다
      - 동일 호출 후 cache의 `CachedStdin.Cost.TotalCostUsd`는 5.20을 유지한다 (0으로 덮어써지지 않음)
      - cache가 없거나 cache의 cost가 0인 상태에서 stdin cost=0이 들어오면 출력은 `$0.00`이고 cache는 (없으면 새로 생성되더라도 cost=0로) 어느 비용 값도 발명되지 않는다
      - `SavedAt > 0 && time.Since(SavedAt) < sessionStateTTL`이 거짓이면 복원이 발생하지 않는다
    - 확인: `go vet ./...`, `go build ./...`, `make build`, `make build-local`, `go test ./...` 모두 exit 0. task-003 회귀 테스트가 모두 통과
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.5, ANALYSIS §5.A, ANALYSIS §5.C, ANALYSIS §5.D

- [x] task-002: `cache.go`에 `cleanOldSessionStates` 추가하고 `main.go`에서 fire-and-forget으로 호출
  - 목적: cc-usage가 호출될 때 `~/.cache/cc-usage/session-state-*.json` 중 ModTime이 sessionStateTTL(300s)을 초과한 파일이 백그라운드로 삭제된다. 단일 호출의 출력 경로는 동기 I/O로 차단되지 않으며, `cache-*.json`(API cache)은 영향받지 않는다
  - 접근: `cache.go`에 throttle 변수(예: package-level `lastSessionStateCleanup time.Time`)와 `cleanOldSessionStates()` 함수를 추가한다. 함수 본문은 `api.go`의 `cleanOldCaches` 패턴(home dir 조회 → glob → ModTime 검사 → `os.Remove`)을 따르되 glob 패턴은 `session-state-*.json`, 임계값은 `sessionStateTTL`을 사용한다. throttle은 자체 변수로 1시간 간격을 둔다 (API cache cleanup과 공유하지 않음). `main.go`에서 fetchUsageLimits 인근에 `go cleanOldSessionStates()` 한 줄을 추가한다 — 호출 위치는 token 유무에 의존하지 않도록 `fetchUsageLimits`의 if 분기 밖에 둔다
  - 검증 조건:
    - 결과:
      - cc-usage 호출 후 ModTime이 sessionStateTTL을 초과한 `session-state-*.json` 파일은 삭제되어 있다
      - `cache-*.json`(API cache) 파일은 본 함수에 의해 영향받지 않는다
      - 단일 호출이 cleanup 때문에 동기적으로 느려지지 않는다 (fire-and-forget goroutine)
    - 확인: `go vet ./...`, `go build ./...`, `make build`, `make build-local`, `go test ./...` 모두 exit 0. task-004 회귀 테스트가 통과. 수동 확인 — 더미 `session-state-stale.json`을 `~/.cache/cc-usage/`에 만들고 ModTime을 6분 전으로 설정한 뒤 cc-usage 호출, 잠시 후 파일 부재 확인
  - 참조: SPEC §5.4, SPEC §5.5, ANALYSIS §5.B

- [x] task-003: cost cache poisoning 복원 회귀 테스트 작성
  - 목적: task-001의 cost-regression 복원 로직이 의도한 세 가지 케이스(복원 발동 / cache 보존 / false-positive 방지)에 대해 회귀하지 않음을 자동 검증한다
  - 접근: `main_test.go` 또는 `cache_test.go`(새 파일이라면 후자)에 unit test를 추가한다. SessionState fixture를 만들어 시나리오 3종을 직접 검증한다 — (a) cache cost>0, stdin cost=0, SavedAt 신선 → 복원 후 stdin.Cost.TotalCostUsd가 cache 값과 같음, (b) cache 부재 또는 cache cost=0 → 복원이 일어나지 않고 stdin.Cost.TotalCostUsd가 0 유지, (c) cache cost>0이지만 SavedAt이 sessionStateTTL 초과 → 복원이 일어나지 않음. 가능하다면 main의 복원 로직을 함수로 분리하지 않고도 `decideCostRestore` 같은 작은 헬퍼나 인라인 조건을 직접 호출해 검증한다. 분리 비용이 너무 크면 SessionState/StdinInput만 직접 만들어 조건식이 의도대로 평가되는지의 단위 테스트로 갈음
  - 검증 조건:
    - 결과: 세 시나리오의 테스트 케이스가 모두 정의되어 있고 통과한다
    - 확인: `go test -run TestCostRegress ./...` 또는 해당 테스트 이름으로 실행 시 PASS
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.5

- [x] task-004: session-state cleanup 회귀 테스트 작성
  - 목적: task-002의 cleanup 함수가 stale 파일만 삭제하고 fresh 파일과 다른 prefix(`cache-*.json`)는 보존함을 자동 검증한다
  - 접근: `cache_test.go`에 unit test를 추가한다. `t.TempDir()` 기반 임시 cache 디렉터리에 fixture 3종을 만든다 — (a) ModTime이 sessionStateTTL+여유 전인 `session-state-stale.json`, (b) ModTime이 방금 전인 `session-state-fresh.json`, (c) ModTime이 1시간+여유 전인 `cache-old.json`. `cleanOldSessionStates`를 직접 호출하거나, home dir 의존이 있으면 home을 임시 디렉터리로 가리키도록 `HOME` 환경변수를 `t.Setenv`로 임시 변경한 뒤 호출한다. throttle 변수가 테스트 간 leak되지 않도록 테스트 시작 시 throttle을 zero value로 리셋한다
  - 검증 조건:
    - 결과: stale 파일은 삭제, fresh 파일과 `cache-old.json`은 보존됨
    - 확인: `go test -run TestCleanOldSessionStates ./...` 또는 해당 테스트 이름으로 실행 시 PASS
  - 참조: SPEC §5.4, SPEC §5.5
