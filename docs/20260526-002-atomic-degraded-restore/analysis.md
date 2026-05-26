# atomic-degraded-restore — Analysis

## 근거

실제로 읽은 spec.md 범위
- `docs/20260526-002-atomic-degraded-restore/spec.md` §1–§5 전체 (§5.1·§5.3 문구는 본 ANALYSIS 작성 직전에 E1 해석으로 명확화됨).
- 참조 spec/analysis: `docs/20260524-006-safe-empty-stdin-fallback/`, `docs/20260526-001-degraded-cwd-fallback-relax/`.

확인한 코드 사실
- `main.go` 76–126: `resolveCachedSessionState`로 캐시를 로드한 뒤 `restoreWorkspace` / `usageDegraded` / `costRegressed` 세 개의 **독립된 boolean 분기**가 존재한다. 셋 중 하나라도 true면 `orchestrate`를 한 번 더 호출한다.
- `restoreWorkspace`는 `workspaceRestoreTTL = 60s`(`cache.go:26`) + `shouldRestoreWorkspace(cwd-exact-match)`(`main.go:227`) 가드를 받는다.
- `usageDegraded`는 `result.WidgetCount < cached.WidgetCount` 만족 시 `restoreUsageFields`(`main.go:244`)로 cost와 context를 채운다. **별도 TTL 가드가 없다** — loadSessionState 시점의 `sessionStateTTL = 300s`(`cache.go:17`)만 적용된다.
- `costRegressed`는 `shouldRestoreCost`(`cache.go:365`)로 0→positive 복원을 별도 처리한다. 이쪽도 300s.
- `Model` 필드는 어디서도 캐시 복원되지 않는다. v0.3.5에서 시도되었다가 v0.3.6에서 cross-workspace fallback과 함께 롤백된 흔적은 git log에만 남아 있고 현 소스에는 없음.
- `modelWidget.GetData`(`widgets_core.go:20`): `Model.ID == "" && DisplayName == ""`이면 nil 반환 → orchestrator가 skip하여 WidgetCount에 잡히지 않는다.
- `projectInfoWidget.GetData`(`widgets_project.go:33`): `Workspace.CurrentDir == ""`이면 nil 반환.
- `costWidget`(`widgets_core.go:114`): 항상 렌더(cost=0이라도 `$0.00` 출력). 그래서 `WidgetCount`로 cost 단독 회귀를 잡을 수 없어 `costRegressed`가 별도 신호로 존재한다.
- `contextWidget`(`widgets_core.go:68`): `ContextWindowSize <= 0`이면 nil.
- `saveSessionState`(`cache.go:375`): `result.WidgetCount >= 2`일 때만 호출되며(`main.go:155`), `RateLimits`를 stripping한 stdin 스냅샷을 그대로 저장한다. 저장 직전 `Workspace.CurrentDir`를 normalize.
- `shouldSuppressOutput`(`main.go:261`): workspace·model·context가 전부 비어 있고 API 측 RateLimits도 없으면 stdout 무출력.
- `RateLimits`는 저장 전 stripping(`main.go:156`) + load 후 ctx에 미주입 — invariant 유지.

추정과 사실의 구분
- "60s~300s 구간에서 절반 출력"은 사용자 보고 + 코드 분기 결합으로 도출된 사실: `restoreWorkspace`가 60s에서 끊기면 projectInfo·model은 빠지지만 `usageDegraded`/`costRegressed`가 300s까지 cost/context를 살리기 때문.
- v0.3.5의 model 복원이 어떤 형태였는지는 현 소스에 남아있지 않음 — 본 ANALYSIS는 그 형태를 재구성하지 않고, 현 코드 위에서의 재도입 형태를 §5에서 새로 결정한다.

## 1. 구조

새 모듈·서브 패키지 없음(CLAUDE.md 단일 `main` 패키지 규칙). 기존 `main.go`의 restore 블록과 `cache.go`의 헬퍼군 안에서 경계를 재배치한다.

경계는 두 개로 정리한다.

1. **eligibility 결정 경계** — "이번 호출에서 SessionState 캐시로 stdin을 보강할 자격이 있는가?"라는 한 번의 yes/no를 만든다. 현재 흩어져 있는 세 boolean(`restoreWorkspace`, `usageDegraded`, `costRegressed`)을 하나의 결정으로 흡수한다. 입력은 `(현재 stdin, cached SessionState, now)`이며 출력은 단일 `bool`(+ 디버그 로그용 사유). cwd-exact-match 가드, TTL 검사, RateLimits 제외 invariant가 이 안에 모인다. 이 결정이 SPEC §5.1·§5.2의 "모두 같은 결정에 따라"를 구조적으로 보장한다.
2. **필드별 빈 칸 채우기 경계** — eligibility가 true일 때 stdin의 각 필드를 "stdin이 비어있다 ↔ cached가 채워져 있다"라는 필드 로컬 조건으로 채운다. cost·context·workspace·worktree·model 각각이 독립적이지만, 결정 자체는 공유한다. 결정이 yes면 비어 있는 필드는 다 채우고, no면 어느 한 필드도 캐시에서 채우지 않는다. stdin이 이미 fresh로 들고 온 필드는 덮어쓰지 않음으로써 SPEC §5.1 후반(fresh 값 보호)을 만족.

이 두 경계는 `main.go`의 기존 restore 블록(line 91–126)을 대체한다. `cache.go`에서는 `shouldRestoreWorkspace`(이미 존재) 외에 `shouldRestoreCost`/`restoreUsageFields`의 역할이 §5.A 채택안에 따라 통합·재정의된다.

## 2. 데이터 흐름

```
stdin → parseStdin
     ↓
cacheKey = sessionCacheKey(stdin)
     ↓
cached = resolveCachedSessionState(cacheKey, now)
        └─ primary hit (key 일치)
        └─ primary miss → fallbackByWorkspaceCwd (cwd exact-match + sessionStateTTL)
     ↓
ctx 생성 (Stdin=stdin)
     ↓
result = orchestrate(ctx)                       ← 1차 렌더 (캐시 미보강 상태)
     ↓
[eligibility 결정 경계]                           ← §1.1
     ─ cached == nil?                              → false
     ─ SavedAt 유효 + atomic-TTL 이내?              → no면 false
     ─ shouldRestoreWorkspace(cached.cwd)?         → no면 false
       (현재 cwd 미식별 또는 cached cwd와 mismatch)
     ─ stdin이 어떤 식으로든 보강이 필요한가?         → no면 false
       (모든 대상 필드가 fresh로 채워져 있으면 굳이 보강 X)
     ─ 모두 통과 → true
     ↓
[restore == true 분기]                            ← SPEC §5.1
     ─ stdin.Workspace.CurrentDir == ""           → cached.Workspace로 채움
     ─ stdin.Worktree == nil                      → cached.Worktree로 채움
     ─ stdin.Model.ID/DisplayName 둘 다 ""        → cached.Model로 채움
     ─ stdin.Cost.TotalCostUsd <= 0               → cached.Cost로 채움
     ─ stdin.ContextWindow 합 == 0                → cached.ContextWindow로 채움
     ─ RateLimits 절대 복원 안 함                    SPEC §5.5
     ─ "이 호출에서 cache로 채운 필드 목록" 기록      § 5.D
     ─ result = orchestrate(ctx)                  ← 2차 렌더
     ↓
[restore == false 분기]                           ← SPEC §5.2·§5.3
     ─ stdin 그대로 두고 무보강 (1차 result 그대로 사용)
     ↓
shouldSuppressOutput(ctx.Stdin, ctx.RateLimits)?  ← SPEC §5.3
     ─ true  → 빈 stdout (warmup 예외: rateLimits 존재 시 통과)
     ─ false → fmt.Print(result.Lines join "\n")
     ↓
[자기참조 누적 방지]                                ← SPEC §5.6 / §5.D
     ─ result.WidgetCount >= 2 일 때만 save
     ─ save 직전: §5.D 채택안에 따라 "cache로 채웠던 필드"를
       스냅샷에서 비운 뒤 saveSessionState 호출
     ↓
saveSessionState(cacheKey, ...)
```

핵심 상태 전이: degraded 호출이 반복돼도 `SavedAt`은 갱신될 수 있지만, 캐시 보강만으로 살아난 필드는 다음 save 본문에서 다시 비워져 영구 자기참조 고착이 끊긴다(SPEC §5.6).

동시성: 기존 `withCacheFileLock` 그대로. eligibility/필드 채우기는 모두 in-process 순차이며 추가 동시성 분기 없음.

에러 경로: cached load 실패·corrupt → cached == nil → eligibility false → fresh stdin만으로 렌더 + suppress 판단. 기존 동작과 동일.

## 3. 인터페이스

외부 contract는 변경 없음.
- stdin JSON schema(`stdin.go` `StdinInput`) 불변.
- SessionState on-disk JSON(`session-state-<key>.json`) 불변 (SPEC §3 제약).
- stdout(위젯 렌더 결과) — 사용자 관점 contract는 "절반 출력 사라짐" 한 가지만 바뀐다. format/색/구분자/위젯 순서는 그대로.
- API 캐시(`cache-<tokenHash>.json`) 무관.

내부 경계(`main.go` ↔ `cache.go`)
- 기존 `shouldRestoreCost(stdin, cached, now) bool`, `restoreUsageFields(stdin*, cached*)`, `shouldRestoreWorkspace(cachedCwd) bool` 세 함수는 §5.A 채택안에 따라 한 개의 eligibility 함수와 한 개의 fill 함수로 통합된다. 시그니처는 §5.A에서 commit.
- `workspaceRestoreTTL` 상수는 §5.C 채택안(C2)에 따라 상수 자체는 유지하되 의미가 "atomic restore TTL"로 재정의된다.

## 4. 영향 범위

직접 수정 대상
- `main.go`: `main()`의 restore 블록(line 91–126) 재구성. `shouldSuppressOutput`/`resolveCachedSessionState`/`fallbackByWorkspaceCwd` 본체는 불변. save 직전 자기참조 stripping 블록 추가.
- `cache.go`: `workspaceRestoreTTL` 상수 의미 갱신(주석), `shouldRestoreCost`/`restoreUsageFields` 사용처/시그니처 정리. `saveSessionState` 본문은 §5.D 채택안에서 호출자가 처리하므로 내부 변경 없음(저장 전 stripping은 main.go 쪽에 둠).

테스트
- `main_test.go`, `cache_test.go`: 위 두 파일의 기존 테이블 테스트들이 영향받는다. 새 케이스 추가 필요:
  - 60–300s 구간 degraded stdin → model·projectInfo가 함께 살아남는다.
  - eligibility=true에서 stdin이 부분 fresh로 들어왔을 때 fresh 필드가 cache 값으로 덮이지 않는다.
  - eligibility=false에서 cost/context가 단독으로 채워지지 않는다.
  - degraded 호출을 연속해도 캐시 본문이 cache-복원 값으로 자기참조 누적되지 않는다.
  - cross-workspace 캐시가 다른 cwd 호출에 노출되지 않는다(기존 가드 유지 확인).

해당 없음
- 새 위젯·새 cache schema·새 외부 파일·새 의존성·새 환경변수.
- API 캐시 경로(`api.go`).
- 위젯 렌더링 로직(`widgets_*.go`).
- 로케일(`locales/*.json`).

## 5. Decision Points

### §5.A — atomic restore의 의미

이 feature의 "atomic"이 가리키는 단위.

옵션
- **A1**: 단일 eligibility 결정 + 필드별 빈 칸 채우기. eligibility=true면 stdin의 비어 있는 필드는 모두 cached로 채우고, false면 어느 필드도 채우지 않는다.
- **A2**: 전부-from-cache or 전부-from-stdin. eligibility=true면 `cached.CachedStdin` 통째로 ctx.Stdin에 대입(RateLimits만 빼고).

트레이드오프
- A2는 stdin이 일부 fresh로 들어온 필드를 캐시 값으로 덮어버려 SPEC §5.1 후반(fresh 보호)과 충돌.
- A1은 그 충돌이 없다. 결정은 한 번이고, 채움은 빈 자리에 한정.

채택안: **A1**.

근거: SPEC §5.1·§5.3 둘 다 만족. "절반 출력 금지"는 결정의 atomic성으로 보장되고, fresh 값 보호는 fill의 per-field 가드로 보장된다.

### §5.B — Model 복원 재도입 시 stale model 허용 범위

`Model` 필드를 캐시 복원 대상에 포함할 때 가드 형태.

옵션
- **B1**: cwd-exact-match + atomic-TTL 만으로 model도 함께 복원. Workspace와 동일한 안전 경계.
- **B2**: model에는 추가 가드(더 짧은 TTL 또는 "stdin.Workspace는 있지만 Model만 비어있을 때는 복원 안 함").
- **B3**: model은 복원 대상에서 제외하고 workspace/cost/context만 복원.

트레이드오프
- B3은 SPEC §5.1을 위반(Model이 명시 대상).
- B2는 같은 cwd에서 사용자가 모델을 바꾼 직후 1회 잔상 위험은 있지만, 다음 fresh stdin에 의해 곧 갱신되며 cross-workspace 노출은 아님.
- B1은 단순성·일관성.

채택안: **B1**.

근거: cross-workspace 노출은 cwd-exact-match가 이미 막는다. 같은 cwd 내 모델 잔상은 정보 오해 위험이 낮고 자동 회복된다. SPEC §5.1·§5.4 동시 만족.

### §5.C — workspaceRestoreTTL을 sessionStateTTL로 통합할지

옵션
- **C1**: `workspaceRestoreTTL` 제거, 모든 복원이 `sessionStateTTL(300s)`만 본다.
- **C2**: atomic 결정의 TTL을 `workspaceRestoreTTL(60s)`로 고정. 본 feature 이후 이 상수의 의미는 "atomic restore TTL".
- **C3**: 별도 유지하되 의미는 그대로 — 결정이 atomic이면 결국 가장 좁은 TTL이 지배.

트레이드오프
- C2/C3은 atomic 결정이 사실상 "60s 이내만 복원"으로 제한. idle 후 60s를 넘어 들어온 degraded stdin은 절반 출력이 아니라 무출력으로 끊긴다(warmup 예외 제외). SPEC §3 제약("TTL 유지하거나 더 좁히는 방향") 만족.
- C1은 cwd 가드 통과 + 300s까지 identity 잔상 가능 — SPEC 안에서는 적법하나 본 feature가 만드는 안전 경계와 정합도가 낮음.

채택안: **C2**.

근거: atomic 결정이 가장 좁은 TTL을 따르도록 명시 commit. `workspaceRestoreTTL` 상수는 유지하되 주석에서 의미를 재정의. 부수 효과: `shouldRestoreCost`의 사실상 검사 TTL이 300s→60s로 좁혀짐(현 동작 변경) — IMPLEMENT verification에 명시 포함.

### §5.D — §5.6 자기참조 누적 방지 구현 위치

옵션
- **D1**: 캐시에서 복원해 채워진 필드는 save 시 제외(원래 비어있던 상태로 되돌려 save). RateLimits stripping 패턴(`main.go:156`)을 일반화.
- **D2**: restore가 일어난 호출에서는 save를 skip하거나 SavedAt만 갱신.
- **D3**: 기존 동작(보강된 stdin 그대로 save). TTL에 위임.

트레이드오프
- D3은 매 호출마다 SavedAt이 갱신되면 윈도우가 슬라이드해 영구 자기참조에 가까워진다.
- D2는 그 호출이 fresh로 갖고 온 필드까지 보존하지 못한다.
- D1은 RateLimits stripping과 같은 패턴이므로 코드 일관성·이해 비용 낮음. fresh 필드는 보존하면서 cache-복원 자리만 비운다.

채택안: **D1**.

구체화: restore 분기에서 "이 호출에서 캐시로부터 채워진 필드 목록"을 추적(예: 작은 struct 또는 bitmask). save 직전, 해당 필드를 스냅샷에서 다시 비운 뒤 `saveSessionState` 호출. eligibility=false인 호출은 stdin 원본 그대로 save(현재 동작 유지).

근거: SPEC §5.6 직접 만족. TTL에 위임하지 않고 본문 차원에서 자기참조 종결.

### §5.E — 부분 fresh stdin에서 cache 보강 범위

옵션
- **E1**: eligibility=true면 stdin이 비어 있는 모든 필드를 cache로 채운다.
- **E2**: stdin이 일부라도 fresh면 cache 보강을 아예 건너뛴다.

트레이드오프
- E2는 "model fresh + cost 비어있음" 같은 케이스에서 cost가 채워지지 않아 model 단독 절반 출력이 발생 → §2 목표와 충돌.
- E1은 §5.1·§5.3 명확화 후 문구와 정합.

채택안: **E1**.

근거: spec.md §5.1·§5.3 본문이 E1 해석으로 명확화되었다(본 ANALYSIS 작성 직전). §2 목표(절반 출력 금지)와 정합.
