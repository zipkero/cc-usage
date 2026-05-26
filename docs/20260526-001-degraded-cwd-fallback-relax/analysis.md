# ANALYSIS — degraded-cwd-fallback-relax

## 근거

읽은 spec.md 범위: `docs/20260526-001-degraded-cwd-fallback-relax/spec.md` 전체 (§1–§5).

확인한 코드 사실:

- `main.go:78–79`에서 `cacheKey := sessionCacheKey(input)` 후 `resolveCachedSessionState(cacheKey, time.Now())`로 캐시를 로드한다. 이 함수는 `main.go:181–187`에 정의되어 있다.
- `resolveCachedSessionState`는 현재 두 분기만 가진다: (1) 우선 `loadSessionState(cacheKey)`로 per-session 파일을 시도하고, (2) `cacheKey == "" && cached == nil`일 때만 `fallbackByWorkspaceCwd(now)`를 호출한다. 주석(`main.go:170–172`)도 "cacheKey != "": load the per-session file. No fallback in this branch even if the load misses"로 명시되어 있다 — 이 가드가 spec이 완화하려는 지점이다.
- `sessionCacheKey`(`cache.go:42–61`)는 `SessionId > Remote.SessionId > AgentId > TranscriptPath > Workspace.CurrentDir` 순으로 떨어진다. degraded stdin에서도 `session_id`만은 비어있지 않은 경우가 흔하므로(spec.md §2), 이 키 자체는 비-빈 값이 된다 → 분기 (2)가 절대 트리거되지 않는 게 SPEC §5.1·§5.2의 결손 모드다.
- `fallbackByWorkspaceCwd`(`main.go:196–215`)는 이미 정확하게 필요한 동작을 한다: `detectCurrentCwdWithSource`로 cwd 신호를 얻고, 없으면 nil. 있으면 `loadByWorkspaceCwd(cacheDir, cwd, now)`에 위임.
- `loadByWorkspaceCwd`(`cache.go:247–304`)는 cwd exact-equality 매칭 + `sessionStateTTL` 가드 + newest mtime 동률 결정 + 파일 락 + per-file 에러 격리를 모두 갖춘 상태로 이미 구현되어 있다 — cross-workspace 노출 가드(`safe-empty-stdin-fallback` SPEC §5.2)는 이 매처가 이미 강제한다. spec.md §3 제약 "normalized cwd exact equality"·"sessionStateTTL 적용"·"RateLimits 미복원"은 매처와 그 호출자 체인이 이미 보장한다.
- `loadSessionState`(`cache.go:199–228`)는 파일이 없거나 `SavedAt`이 TTL 초과면 nil을 반환하고, `state.CachedStdin == nil`이면 legacy 캐시로 간주해 nil을 반환한다. v0.3.5 이후의 정상 저장 경로는 항상 `CachedStdin`을 채우므로 "파일은 있는데 안이 비어있다"는 케이스는 legacy/손상 분기로 좁혀진다.
- `main.go:94–125`의 degraded restore 블록은 `cached != nil && cached.CachedStdin != nil`을 진입 조건으로 갖는다. 그 안에서 `workspaceStale` / `usageDegraded` / `costRegressed` 세 신호 중 하나라도 true일 때만 실제 복원이 일어나고 `orchestrate`가 다시 돈다.
- `main.go:138–141`의 `shouldSuppressOutput`은 restore 블록 이후의 ctx.Stdin 기준으로 판단한다. 즉 fallback이 식별 필드를 채워주면 그대로 출력으로 이어진다.
- `main.go:155–162`의 save 경로는 `result.WidgetCount >= 2`이고 `cacheKey != ""`일 때만 동작. degraded 첫 stdin은 `WidgetCount < 2`라 어차피 저장 안 되며, 본 변경이 저장 측에 손댈 이유는 없다.
- 기존 테스트 패턴: `main_test.go`의 `TestResolveCachedSessionState*`, `TestFallbackByWorkspaceCwdEndToEnd`, `TestFallbackFourPaths`, `TestMultiWorkspaceSequence`, `TestFallbackRateLimitsIsolated`가 모두 `detectCwdEnv` / `detectCwdGetwd` / `HOME`·`USERPROFILE` 환경변수를 swap해 격리한다. 신규 테스트도 동일 hook을 그대로 재사용한다.
- 의존자 탐색: `resolveCachedSessionState`는 `main.go:79`(production) + `main_test.go`의 `TestResolveCachedSessionStateEmptyKeyFallback` / `TestResolveCachedSessionStateNormalStdinSkipsFallback` 두 곳에서만 호출된다. 후자는 "cacheKey != "" 일 때 fallback 0회"를 어서션하는데, 본 변경은 이 어서션 의미를 "primary 캐시가 적중하면 fallback 미호출"로 좁힌다.
- `safe-empty-stdin-fallback`(20260524-006) ANALYSIS §3.2(옵션 B 채택)와 §4.3 다중 워크스페이스 시퀀스가 이미 보장한 cross-workspace 0회 노출 정책을 본 feature가 그대로 상속한다. spec.md §3 제약은 그 결정을 약화시키지 말라는 명령일 뿐 새 가드를 도입하라는 요구가 아니다.

## 1. 구조

기존 모듈 안에서 끝나는 변경이다. 새 파일·새 타입·새 함수 시그니처를 만들지 않는다.

- `resolveCachedSessionState`(`main.go`)의 분기 조건만 확장한다. 호출 그래프(`main` → `resolveCachedSessionState` → `loadSessionState` / `fallbackByWorkspaceCwd` → `loadByWorkspaceCwd`)는 유지.
- 캐시 매처(`loadByWorkspaceCwd`)와 cwd 신호(`detectCurrentCwdWithSource`)는 무변경. cross-workspace 노출 가드·TTL·RateLimits 미복원이 모두 매처 안에 이미 박혀 있어 본 변경의 표면이 좁다.
- save 측·degraded restore 블록·`shouldSuppressOutput`도 무변경.

## 2. 데이터 흐름

진입점부터 응답까지의 경로 중 본 변경이 손대는 부분과 인접 단계 (SPEC §5.1·§5.2·§5.5·§5.6):

```
parseStdin
  -> sessionCacheKey(input)               // 보통 SessionId가 있어 비-빈 키
  -> resolveCachedSessionState(cacheKey, now)
       primary := loadSessionState(cacheKey)
       if primary != nil:  return primary                       // SPEC §5.5
       else:               return fallbackByWorkspaceCwd(now)   // SPEC §5.1·§5.2 (신규 진입 조건)
  -> orchestrate(ctx)
  -> degraded restore 블록 (cached != nil && cached.CachedStdin != nil 진입)
       restoreWorkspace / restoreUsageFields / shouldRestoreCost  // 무변경
       조건 부합 시 orchestrate 재실행
  -> shouldSuppressOutput                                       // SPEC §5.6 (캐시 미적중이면 기존 무출력)
  -> stdout
  -> saveSessionState (현재 호출이 ≥2 위젯을 그렸을 때만, 무변경)
```

cwd 폴백 적격 조건: `primary == nil` 단 하나(§5 D1·D2에서 commit). primary가 적중하면 그대로 반환되므로 기존 세션 동작은 본 변경 전후로 동일하다(SPEC §5.5).

cwd 폴백 진입 후 매처 내부의 상태 가드는 그대로 적용된다(존재 + cwd exact match + TTL 통과). 매처가 nil을 반환하면 그대로 nil이 main으로 전달되어 SPEC §5.6의 "캐시 없음 → 기존과 동일 무출력" 분기로 떨어진다.

에러·실패 경로:

- `detectCurrentCwdWithSource`가 빈 값을 돌려주면 fallback은 호출되더라도 nil 반환(`main.go:198–201`). cross-workspace 노출 0회 baseline은 매처 진입 전에 차단.
- `os.UserHomeDir` 실패 → fallback nil.
- glob·read·unmarshal 실패는 per-file 격리(`cache.go:281`)로 다른 후보를 막지 않음.

동시성: `withCacheFileLock`로 매처 내부 read가 직렬화. 본 변경은 락 보유 시간이나 호출 횟수를 늘리지 않음(파일 N개 read 한 번은 기존과 동일).

## 3. 인터페이스

외부 경계 변화 없음.

- stdin/stdout 계약, CLI 플래그, 캐시 파일 스키마(`SessionState` JSON), API 호출, debug 로그 형식 모두 무변경.
- `resolveCachedSessionState`는 같은 main 패키지 안의 내부 helper다. 시그니처 `(cacheKey string, now time.Time) *SessionState`도 유지. 본 변경은 내부 분기 표현만 바꾼다.

## 4. 영향 범위

- `main.go`: `resolveCachedSessionState` 본체와 그 함수 상단 주석. cacheKey != "" 분기의 "No fallback in this branch" 설명을 새 의도에 맞춰 갱신.
- `main_test.go`: `TestResolveCachedSessionStateNormalStdinSkipsFallback` 한 케이스의 의미·셋업 조정 + primary miss + cwd match → fallback 진입을 검증하는 신규 테스트. 기존 매처 단 테스트(`TestFallbackByWorkspaceCwdEndToEnd` / `TestFallbackFourPaths` / `TestMultiWorkspaceSequence` / `TestCdScenarioBlocksStaleWorkspaceRestore` / `TestFallbackRateLimitsIsolated`)는 그대로 유효.
- 버전 갱신: SPEC §5.7에 명시되지 않았으나 사용자 체감 가능한 fix이므로 CLAUDE.md §버전 정책에 따라 `Makefile` VERSION / `.claude-plugin/plugin.json` version / `api.go` userAgent 세 곳을 patch bump 한다. implement.md에서 Task로 펼친다.
- 캐시 파일 포맷·디렉토리 구조·OAuth credential·API 캐시(`cache-<tokenHash>.json`): 해당 없음.
- 외부 contract·하위 호환: 해당 없음 (캐시 파일 스키마 무변경, fallback이 적중하지 않으면 v0.3.7과 정확히 동등한 출력).

## 5. Decision Points

### D1. primary 캐시가 "있지만 쓸모없음"을 어떻게 판정하는가

SPEC §5.2는 "per-session 캐시는 존재하지만 `cached_stdin`이 비어있어 복원에 쓸모가 없을 때" cwd 폴백을 발동하라고 요구한다.

옵션:

- **D1-a. `loadSessionState`가 nil을 반환했을 때만 cwd 폴백.** `loadSessionState`는 파일 부재·TTL 초과·`CachedStdin == nil`(legacy 포맷)에서 nil을 반환한다. 정상 저장 경로(v0.3.5+)는 항상 `WidgetCount >= 2` && non-nil `CachedStdin`을 만족하므로 디스크에 살아남은 파일은 식별 필드가 비어있지 않다.
- **D1-b. 추가로 `primary != nil`이지만 `CachedStdin`의 식별 필드가 모두 비어있으면 폴백 발동.** §5.2 문구를 더 충실히 옮기지만, "식별 필드 모두 비어있음"의 정의가 또 다른 sub-decision을 만들고 코드 분기가 늘어난다.
- **D1-c. primary가 적중해도 항상 cwd 폴백을 시도해 newest mtime을 비교한다.** SPEC §5.5("기존 세션 동작 불변")와 충돌한다 — 같은 cwd의 다른 세션 캐시가 더 newest mtime을 가지면 본인 세션 캐시 대신 그게 선택될 수 있다. 채택 불가.

채택: **D1-a**. 근거:

1. save 경로의 invariant(`WidgetCount >= 2` && non-nil `CachedStdin`)에 의해 "파일이 있는데 안이 비어있는" 케이스는 legacy 포맷 + 손상 케이스로 좁혀지며, 이 둘은 `loadSessionState`가 이미 nil로 정규화한다(`cache.go:219-226`).
2. §5.2의 시나리오 — 새 세션의 첫 stdin이 degraded인 경우 — 는 primary cacheKey의 파일 자체가 디스크에 존재하지 않는 분기(§5.1)와 사실상 합쳐진다. D1-a로도 §5.2가 실현 가능한 모든 케이스를 커버한다.
3. 채택은 코드 표면을 한 분기로 좁혀 SPEC §5.5 보호가 자명해진다.

대안: 미래에 save invariant가 바뀌어 "파일은 있는데 식별 필드는 비어있는" 캐시가 생긴다면 D1-b를 도입한다. 본 feature 범위에서는 두지 않는다.

### D2. `resolveCachedSessionState`의 분기 표현

옵션:

- **D2-a. 분기 가드를 `primary == nil`로 일반화.** `cacheKey == ""`이라는 조건을 떼고 "primary가 비었으면 cwd 폴백"으로 단순화. cacheKey가 있어도(degraded 첫 stdin에서 session_id가 있어 키는 만들어지는 경우) primary 파일이 없으면 폴백 진입.
- **D2-b. cacheKey == "" 분기와 별개로 "cacheKey != "" && primary == nil" 분기를 추가.** 두 진입점이 코드에 명시되나 분기 표현이 늘어남.
- **D2-c. 분기를 그대로 두고 `sessionCacheKey` 측에서 degraded 입력에 대해 빈 키를 반환하도록 동작 변경.** 정상 흐름의 캐시 키 namespace가 바뀌어 SPEC §5.5 회귀 위험. 채택 불가.

채택: **D2-a**. 근거:

1. SPEC §5.1·§5.2 모두 결과적으로 "primary가 없거나 비어있으면 cwd 폴백" 이라는 같은 의미다. D1-a와 합쳐 한 가드로 통합한다.
2. SPEC §5.5는 `primary != nil`이면 항상 그것을 반환한다는 한 줄로 보장된다. 가드 표현이 단순할수록 회귀 보호가 강하다.

부수 효과: `cacheKey != "" && cached == nil` 분기에서 폴백을 막아두던 기존 주석의 의도가 바뀐다. 그 주석이 보호하려던 시나리오 — "알려진 session_id를 가진 호출이 같은 cwd 다른 세션 캐시를 흡수해버리는 위험" — 은 매처(`loadByWorkspaceCwd`)의 cwd exact-match + TTL 가드로 이미 차단된다(같은 cwd sibling 세션 캐시를 채택하는 건 SPEC §5.1·§5.2가 의도한 동작 그 자체다). 주석은 본 변경에서 새 의도를 반영해 갱신한다.

### D3. fallback 매처가 nil을 반환했을 때

옵션:

- **D3-a. nil 그대로 main에 전달.** main은 degraded restore 블록을 skip하고 `shouldSuppressOutput` 평가로 떨어진다. SPEC §5.6과 합치.
- **D3-b. 어떤 폴백이라도 만들어내려 시도(예: 가장 newest mtime).** cross-workspace 노출 가드 위반. 채택 불가.

채택: **D3-a**. 매처의 nil 반환은 그대로 caller로 전달된다. 추가 분기 없음.

### D4. 신규 테스트의 책임 분할

옵션:

- **D4-a. `TestResolveCachedSessionState*` 패밀리에만 새 케이스 추가.** primary miss + cwd match → fallback 진입, primary hit → fallback skip 두 어서션을 같은 파일에 둠.
- **D4-b. 새 테스트 파일을 따로 만든다.** 본 feature 범위가 작아서 분리 비용이 효과보다 크다.

채택: **D4-a**. 기존 테스트와 같은 hook(`detectCwdEnv` / `detectCwdGetwd` / `HOME` swap)을 그대로 쓰는 게 정합적이다.

또한 기존 `TestResolveCachedSessionStateNormalStdinSkipsFallback`은 "cacheKey != "" → fallback 0회"를 어서션하는데, 본 변경 후에는 "primary가 적중하지 못하면 fallback이 호출될 수 있다"가 새 동작이다. 이 테스트의 의도는 "정상 stdin이 primary로 정상 적중하는 경우 fallback 미호출"이어야 하므로, 디스크에 cacheKey 명의의 정상 캐시를 미리 깔아둔 뒤 fallback 0회를 어서션하는 형태로 갱신한다. 이 갱신은 implement.md의 Task로 펼친다.
