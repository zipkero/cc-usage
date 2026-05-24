# session-state-fixes — ANALYSIS

## 근거

### 읽은 자료
- `docs/20260524-002-session-state-fixes/spec.md` 전체 (§1–§5).
- `main.go` 전체. 특히 degraded-input 복원 블록(83–117)과 무출력·저장 블록(118–159).
- `widgets_core.go` cost 위젯(93–111). `widgets_analytics.go` burnRate(75–96).
- `cache.go` 전체. `SessionState`(29–36), `sessionStateTTL`/`workspaceRestoreTTL`(17–22), `sessionCacheKey`(38–57), `loadSessionState`/`saveSessionState`(142–194).
- `api.go` `cleanOldCaches`(310–335), `lastCleanup`(57).
- `CLAUDE.md` §아키텍처, §degraded-input 복원, §경로.

### 코드베이스에서 확인한 사실
- `result.WidgetCount`는 orchestrator가 `GetData`/`Render`를 모두 성공시킨 widget 수다. cost 위젯의 `GetData`는 `total_cost_usd=0`에서도 `(0.0, nil)`을 반환하므로 cost는 widget count에 항상 1을 기여한다.
- 그 결과 `usageDegraded := result.WidgetCount < cached.WidgetCount`는 stdin이 cost 외 영역에서 빈약해질 때만 true가 된다. cost 단독 0 이벤트에서는 false가 되어 복원 블록 안의 `ctx.Stdin.Cost.TotalCostUsd <= 0` 가드 자체가 실행되지 않는다 (main.go:104–108).
- `saveSessionState`는 `result.WidgetCount >= 2` 조건만 본다 (main.go:152). cost가 정상 widget 수를 유지시키므로 0 stdin도 그대로 저장 경로에 들어가 cache의 정상 cost가 0으로 덮어쓰인다.
- cost 위젯과 별개로 `widgets_analytics.go`의 burnRate는 `cost<=0`에서 자체 skip한다. 즉 cost 위젯을 0에서 skip시켜도 burnRate의 동작은 변하지 않는다.
- `cleanOldCaches`의 glob은 `cache-*.json`만 잡는다 (api.go:321). `session-state-*.json`은 어떤 경로에서도 삭제되지 않는다.
- `lastCleanup`은 package-level 변수이며 1시간 throttle을 둔다 (api.go:312). fork-exec 모델이므로 프로세스 시작 시 zero value이고, 한 호출 안에서만 의미가 있다.
- `SessionState.SavedAt`은 `saveSessionState`에서 `time.Now().Unix()`로 채워진다 (cache.go:183). `loadSessionState`는 `SavedAt > 0 && time.Since(...) > sessionStateTTL`이면 nil을 반환한다 (cache.go:166–169) — 즉 load 단계에서 이미 stale entry는 거른다.

### 추정과 미확인
- Claude Code가 `total_cost_usd=0` stdin을 보내는 정확한 트리거(reload, idle, tool 결과 처리 사이 등)는 spec.md §1의 관찰 진술에 의존한다. 코드만으로 검증할 수 없다 — 본 설계는 "그런 stdin이 도착할 수 있다"는 spec.md 전제만 사용한다.
- session-state 파일 누적량과 사용자별 디스크 사용량 수치는 측정하지 않았다. spec.md §1의 "무한 누적" 진술만 근거로 한다.

---

## 1. 구조

본 변경은 새 모듈을 만들지 않고 두 기존 경계 안에서 끝난다.

- **main.go의 degraded-input 복원 블록 (83–117)**: cost 복원을 widget count와 독립된 별도 신호로 분리한다. 기존 `workspaceStale` / `usageDegraded`와 동급 위치에 `costRegressed` 판정을 둔다 (SPEC §5.1). 셋 중 하나라도 true면 fields 복원 → re-orchestrate를 트리거한다 (SPEC §5.1, §5.2). 즉 복원 트리거가 "신호 OR"로 확장되며, 각 신호는 자기 fields에만 책임을 진다.
- **cache.go의 cleanup 책임 분리**: session-state 파일의 stale 청소를 cache.go가 자체 함수로 보유한다. 청소 대상이 cache.go가 만든 파일(`session-state-*.json`)이므로 경계가 그쪽에 속한다. api.go의 `cleanOldCaches`는 자기 책임인 `cache-*.json`만 계속 본다 (SPEC §5.4, 제외 범위).
- **호출 방식**: 새 청소 함수는 `cleanOldCaches`와 동일하게 main.go에서 fire-and-forget goroutine으로 띄운다. throttle 변수는 cache.go 안에 독립적으로 둔다 — api cache cleanup과 session-state cleanup은 다른 staleness 임계값(`time.Hour` vs `sessionStateTTL=300s`)을 쓰므로 throttle도 공유하지 않는 게 자연스럽다 (SPEC §3, §5.4).

cost 위젯 자체는 건드리지 않는다. 위젯을 0에서 skip시키면 `WidgetCount`가 줄어들어 다른 경로(burnRate 진단, save gating)와의 결합이 흔들리기 때문이다 (자세한 비교는 §5의 결정 A 참조).

## 2. 데이터 흐름

degraded-input 복원 경로(cost 영역 한정)는 다음으로 확장된다.

```
stdin 도착
  → loadSessionState(cacheKey) → cached
  → orchestrate(ctx) → result
  → cached가 살아 있으면:
      workspaceStale = stdin.workspace 비었음 && cached.workspace 있음
      usageDegraded  = result.WidgetCount < cached.WidgetCount
      costRegressed  = stdin.cost == 0 && cached.cost > 0 && cached.SavedAt 신선   ← 신규
      ─────────────────────────────────────────────────
      restoreWorkspace = workspaceStale && SavedAt < workspaceRestoreTTL
      if restoreWorkspace: workspace/worktree 복원
      if usageDegraded:    cost/context 복원 (기존 field-level 가드 유지)
      if costRegressed:    cost만 복원                                              ← 신규
      if (restoreWorkspace || usageDegraded || costRegressed):
          result = orchestrate(ctx)                                                ← 신규 분기 포함
  → noIdentity 가드
  → 출력
  → saveSessionState (cost 복원 후 값으로 저장됨 — cache는 정상 값 보존)
```

session-state cleanup 흐름은 기존 fire-and-forget 패턴을 그대로 따른다.

```
main()
  ├─ fetchUsageLimits → go cleanOldCaches()           (기존 — cache-*.json 정리)
  └─ go cleanOldSessionStates()                       (신규 — session-state-*.json 정리)
       └─ ~/.cache/cc-usage/session-state-*.json 글로브
            → ModTime 기준 sessionStateTTL 초과 파일 os.Remove
```

위 두 goroutine은 모두 stdin 출력 경로에 동기 I/O를 추가하지 않는다. 한 호출이 끝나면 OS가 회수한다 (SPEC §3, §5.4).

## 3. 인터페이스

- **외부 contract 변화 없음**. CLI 플래그, config 스키마, locale 키, 출력 형식, `~/.cache/cc-usage/` 디렉터리 레이아웃 모두 그대로 (SPEC §3).
- **세션 cache 파일 포맷 변화 없음**. `SessionState` JSON 구조와 `SavedAt` 의미를 유지한다. 본 변경은 read-time decision만 추가하며 새 필드를 도입하지 않는다 (§5 결정 C 참조).
- **새 함수 시그니처(내부 boundary, 같은 `main` 패키지)**: cache.go에 `cleanOldSessionStates()` 추가. 매개변수 없음, 반환 없음, fire-and-forget. 외부 호출자는 main.go 한 곳.

## 4. 영향 범위

- **`main.go`**: degraded-input 복원 블록(83–117)에 cost-regression 신호와 그에 따른 re-orchestrate 분기 추가. cleanup goroutine 호출 한 줄 추가.
- **`cache.go`**: session-state cleanup 함수 신설. 기존 상수 `sessionStateTTL`을 재사용한다.
- **`api.go`**: 변경 없음. `cleanOldCaches`의 책임은 `cache-*.json`에 한정 유지 (SPEC §4: API cache 정리 정책 변경은 본 spec 밖).
- **`widgets_core.go`**: 변경 없음. cost 위젯의 always-render 동작은 유지(이유는 §5 결정 A).

하위 호환·마이그레이션: 해당 없음. session-state 파일 포맷 변화가 없고, cleanup은 자기 디렉터리의 stale 파일만 지운다. 사용자가 직접 두는 파일이 아니다.

bin/ 재빌드와 release 브랜치 동기화는 본 작업의 소스 변경에 따라 별도 절차로 처리한다 (CLAUDE.md §배포, SPEC §3).

## 5. Decision Points

### A. WidgetCount-tied 트리거를 어떻게 깰 것인가 (SPEC §5.1, §5.2)

옵션:
- **A1 — cost-specific 신호를 복원 분기에 추가**: 기존 `usageDegraded`와 동급의 `costRegressed` 신호를 만들고, `stdin.cost==0 && cached.cost>0 && cached.SavedAt 신선`일 때 true. 트리거되면 cost field만 복원하고 re-orchestrate.
- **A2 — cost 위젯을 0에서 skip**: `costWidget.GetData`가 `cost<=0`이면 `(nil, nil)`을 반환. cost가 widget count에 기여하지 않게 되어 `usageDegraded`가 자동으로 true가 됨.
- **A3 — save 단계 가드**: 복원은 그대로 두고, `saveSessionState` 호출 직전에 "현재 cost < cached cost"면 저장 자체를 skip.

트레이드오프:
- A2는 widget skip이 다른 시스템에 의도치 않은 결합을 만든다. `result.WidgetCount >= 2`라는 save gate(main.go:152)가 흔들리고, 사용자가 cost 위젯을 preset에 명시적으로 둔 상태에서 "정상 첫 세션, 진짜 $0.00" 경우에 `$0.00`이 사라지는 회귀가 생긴다 — 이는 SPEC §5.3을 정면 위반한다.
- A3은 복원 후 다시 저장하는 흐름과 충돌한다. cost 복원이 일어나지 않은 채 cost가 0인 stdin이 그대로 출력으로 흘러간다 — 사용자에게는 여전히 `$0.00`이 보인다. SPEC §5.1("사용자에게 보이는 cost 위젯이 cache의 정상 cost 값을 표시")이 실패한다.
- A1은 복원 흐름과 저장 흐름을 한 번에 해결한다. 복원이 일어나면 `ctx.Stdin.Cost`가 cached 값으로 바뀌고, 이어지는 `saveSessionState`는 그 복원된 값을 저장한다(SPEC §5.2의 두 가지 허용 동작 중 "복원 후 값으로 저장" 케이스).

**채택: A1**. SPEC §5.3의 false-positive 방지 요구를 만족하면서 §5.1/§5.2를 한 분기에서 동시에 처리할 수 있다. 다른 위젯(burnRate)의 동작 보존도 확인됨 — burnRate는 자체적으로 `cost<=0` skip 가드를 가지므로 cost 위젯을 그대로 둬도 변하는 게 없다.

### B. session-state cleanup 배치 (SPEC §5.4)

옵션:
- **B1 — `cleanOldCaches`를 확장해 두 종류의 파일을 동시에 처리**.
- **B2 — main.go에서 별도 goroutine을 띄워 cache.go의 새 함수 호출**.
- **B3 — cache.go에 새 함수를 두지만 호출은 fetchUsageLimits 안에서 함께 띄움** (B1과 B2 사이의 절충).

트레이드오프:
- B1은 함수 하나의 책임이 두 cache 종류로 늘어난다. 두 종류의 staleness 임계값이 다르고 (`time.Hour` vs `sessionStateTTL=300s`) glob 패턴·삭제 기준도 다르다. 한 함수 안에서 if-분기로 처리하면 응집도가 떨어진다. 또한 api.go가 cache.go의 파일 lifecycle을 알게 되어 모듈 경계가 흐려진다.
- B2는 cache.go의 책임이 자기 파일에만 머문다. main.go가 두 goroutine을 띄우게 되지만 cleanup 의도가 호출자 코드에 분명히 드러난다.
- B3은 호출자가 cache.go 함수를 알아야 한다는 점은 B2와 같으면서, "API 호출 트리거"라는 부수 조건에 cleanup 실행 여부가 묶이게 된다. token이 없거나 API 호출이 일어나지 않는 호출에서는 cleanup이 실행되지 않을 수 있어 디스크 위생 요구(SPEC §5.4)가 흔들린다.

**채택: B2**. cache.go가 자기 파일만 책임지는 경계가 가장 깔끔하다. main.go에 한 줄 `go cleanOldSessionStates()`가 추가되며, 이는 기존 `go cleanOldCaches()` 패턴(api.go:92 호출 위치는 다르지만 fire-and-forget 형태는 동일)과 일관된다. throttle 변수는 cache.go 안에 자체 보유.

staleness 임계값은 `sessionStateTTL=300s`를 재사용한다 (SPEC §3: 새 상수 도입 금지). 5분이면 새 stdin이 도착하지 않은 세션은 사실상 종료된 것으로 봐도 안전하다 — `loadSessionState`가 이미 같은 임계값으로 read-time 무시를 하고 있으므로 동일한 의미축에서 file removal도 같은 임계값으로 묶인다.

### C. SavedAt 신선도를 cost 복원에도 사용할 것인가 (SPEC §5.3)

쟁점: 기존 `restoreWorkspace`는 `cached.SavedAt > 0 && time.Since(...) < workspaceRestoreTTL` 가드를 명시적으로 본다 (main.go:93–94). `usageDegraded` 분기는 그 가드 없이 `loadSessionState`의 read-time TTL 검사(`sessionStateTTL=300s`)만 의존한다.

옵션:
- **C1 — costRegressed 트리거가 자체 SavedAt 가드를 본다** (`workspaceRestoreTTL`이 아니라 `sessionStateTTL`).
- **C2 — costRegressed는 load-time TTL 검사만 신뢰하고 별도 가드를 두지 않는다** (`usageDegraded`와 동일 접근).

트레이드오프:
- `loadSessionState`는 stale entry에 nil을 반환하므로(cache.go:166–169), `cached != nil`로 진입한 시점에서 SavedAt이 이미 sessionStateTTL 안임이 보장된다. C2면 추가 가드가 중복이다.
- 그러나 SavedAt이 0인 legacy/edge 케이스가 이론적으로 가능하다 (`SavedAt == 0`이면 load-time TTL 검사가 통과해버린다 — cache.go:166의 조건이 `SavedAt > 0 &&`). 그런 cache의 cost값을 stale 복원에 쓰는 건 false-positive 위험이 있다.

**채택: C1**. `costRegressed`는 `cached.SavedAt > 0 && time.Since(time.Unix(cached.SavedAt, 0)) < sessionStateTTL`을 명시 가드로 본다. load-time 검사와 의미적으로 중복이지만, "복원의 정확성이 SavedAt에 달려 있다"를 코드에 드러내는 게 SPEC §5.3 false-positive 방지 의도와 일치한다. workspaceRestoreTTL(30s)이 아닌 sessionStateTTL(300s)을 쓰는 이유는 cost 누적값이 cwd처럼 빠르게 stale해지지 않기 때문이다 — 5분 안의 cost 값은 사용자에게 여전히 의미 있다.

### D. cost 복원 후 re-orchestrate 필수성 (SPEC §5.1, §5.2)

쟁점: 복원 블록은 `restoreWorkspace || usageDegraded`일 때만 `orchestrate(ctx)`를 다시 호출한다 (main.go:114). costRegressed가 trigger되어도 re-orchestrate이 일어나지 않으면 cost 위젯이 이미 0으로 렌더된 `result.Lines`가 그대로 출력된다.

옵션:
- **D1 — `costRegressed`도 re-orchestrate 트리거에 포함**.
- **D2 — costRegressed는 ctx.Stdin.Cost만 바꾸고 result를 직접 수정** (예: result.Lines에서 cost 라인을 다시 빌드).

트레이드오프:
- D2는 orchestrator의 라인 구성 책임을 main.go에 누설시키며, 위젯 추가 절차(CLAUDE.md §위젯 추가 절차)를 깰 위험이 크다. 본 spec 범위 밖의 리팩터링이다.
- D1은 비용이 한 번의 추가 orchestrate 호출이다 — widget GetData/Render는 stdin/RateLimits만 보는 순수 함수에 가까우므로 부작용 없고, 첫 orchestrate에서 이미 채워진 `ctx.RateLimits` 등도 그대로 재사용된다.

**채택: D1**. re-orchestrate 트리거 조건을 `restoreWorkspace || usageDegraded || costRegressed`로 확장한다. cost field 복원이 사용자가 보는 출력에 반영되는 유일한 경로다. SPEC §5.1이 "사용자에게 보이는 cost 위젯이 cache의 정상 cost 값을 표시"라고 명시하므로 이 분기는 필수다.
