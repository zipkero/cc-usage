# refactor-structure — analysis

## 근거
- spec.md 전체(§1–§5) 정독. 범위는 네 영역(main 분해, api.go 캐시 I/O 이전,
  cache.go 범용 유틸 분리, widget.go i18n 분리)으로 고정되어 있고, §5는 9개의
  완료 조건을 두며 §5.6–§5.8은 식별자 단위로 이동 대상을 명시한다.
- `main.go` 전체 정독. `main()`은 33–235라인을 차지하며 크게 다음 흐름을 인라인으로
  보유한다: (a) flag·version·configPath·configDir 결정, (b) `loadConfig`/
  `parseStdin`/`getCredential`/`fetchUsageLimits`/`cleanOldSessionStates`/
  `loadTranslations`, (c) Layer 1 캐시 복원 (`resolveCachedSessionState` →
  `shouldRestoreFromSession` → `fillFromSessionCache` → 재 `orchestrate`),
  (d) Layer 2 transcript backfill (`needsTranscriptBackfill` 가드 후
  `detectCurrentCwd`/`encodeCwdToTranscriptDir`/`selectTranscriptCandidate`/
  `readLastAssistantEntry`/D4 cwd 가드/`applyTranscriptToStdin` → 재 `orchestrate`),
  (e) suppress·warmup 판정과 출력, (f) last-known [1m] 저장, (g) save snapshot.
  `restoredMask`는 Layer 1과 Layer 2 양쪽에서 같은 인스턴스를 갱신하며 최종
  `stripRestoredFields`에 전달된다.
- `main.go`에는 이미 `resolveCachedSessionState`, `fallbackByWorkspaceCwd`,
  `shouldRestoreWorkspace`, `shouldSuppressOutput`, `isWarmupExceptionPath`,
  `renderRateLimitOnly` 등 main 본체 외 헬퍼가 함께 존재한다. 이들의 의미는
  진입점/플래그/출력보다는 세션 복원·suppress 판정에 가깝다.
- `api.go` 정독. spec.md §5.6 대상 식별자가 실제로 존재함을 확인:
  `cacheEntry` 타입(50–54), `lastCleanup` 변수(57), `cleanOldCachesFn`
  indirection 변수(63), 상수 `staleCacheMaxAge` 등(65–71),
  `cacheFilePath`(239–243), `readFileCache`(245–263), `writeFileCache`
  (265–283), `cleanOldCaches`(321–356). 이 중 `readFileCache`/
  `writeFileCache`는 `withCacheFileLock`, `atomicWriteFile`을 호출한다.
  `cleanOldCachesFn`은 `fetchUsageLimits`(93)에서만 참조된다.
- `cache.go` 정독. spec.md §5.7 대상 식별자가 실제로 존재함을 확인:
  `atomicWriteFile`(140–174), `withCacheFileLock`(176–188),
  `normalizeCwd`(130–138), `detectCurrentCwd`(104–107),
  `detectCurrentCwdWithSource`(114–122), 그리고 그 indirection
  `detectCwdEnv`/`detectCwdGetwd`(92–95)와 상수 `cacheLockTimeout`/
  `cacheLockRetryDelay`(30–33). 세션 도메인 식별자는 `sessionStateTTL`,
  `workspaceRestoreTTL`, `SessionState`, `sessionCacheKey`/`safeCacheKeyPart`/
  `hashCacheKey`, `sessionStatePath`, `loadSessionState`,
  `loadByWorkspaceCwd`, `lastSessionStateCleanup`, `cleanOldSessionStates`,
  `restoredFieldMask`, `shouldRestoreFromSession`, `fillFromSessionCache`,
  `stripRestoredFields`, `saveSessionState`. cwd 탐지(`detectCurrentCwd*`,
  `detectCwdEnv`, `detectCwdGetwd`, `normalizeCwd`)는 세션 캐시 매칭·
  workspace 가드 양쪽에서 쓰이며 transcript 경로(main.go의 Layer 2)에서도
  호출된다.
- `widget.go` 정독. spec.md §5.8 대상이 실제로 존재함을 확인: `localeEN`/
  `localeKO` embed(10–14), `Translations` 타입(17–41), `loadTranslations`
  (44–63), `detectLanguage`(66–74). 나머지(`Context`, `Widget`, `registry`,
  `registerWidget`, `displayPresets`, `presetCharToWidget`, `resolvePreset`,
  `OrchestrateResult`, `orchestrate`)는 위젯 오케스트레이션 본연 책임이다.
- `file_lock_unix.go` / `file_lock_windows.go`는 OS별 `acquireCacheFileLock`
  만 제공하며 `withCacheFileLock`은 `cache.go`가 OS-중립 래퍼로 보유한다.
  이 lock 함수는 API 캐시·세션 캐시 모두가 같은 lock 메커니즘을 공유한다는
  뜻이며, lock 자체는 도메인이 아니라 범용 I/O 유틸이다.
- 이동 대상 식별자의 외부 호출자 탐색은 main 패키지 내부에 한정된다(단일
  `main` 패키지). 따라서 §5.9 grep 검증은 "이 저장소의 모든 `.go` 파일"에서
  새 파일에 정의 1회, 호출은 기존 위치 또는 다른 파일에서 잔존, 으로 환원된다.

## 1. 구조

리팩터 후 파일별 책임 경계는 다음과 같이 분리된다.

- **진입점·출력 경계**: `main.go`. flag 파싱, `--version` 처리, `configPath`/
  `configDir` 결정, suppress·warmup 판정, 최종 stdout 출력, last-known [1m]
  저장 트리거, save snapshot 호출만 담당한다. 세션 복원·transcript backfill
  의 본문은 보유하지 않고 단일 helper 호출로 위임한다.
- **세션 복원 경계**: `session.go` (신규). Layer 1(세션 캐시 복원)과 Layer 2
  (transcript backfill) 흐름을 진입점 함수 한 개로 표면화한다. 이 함수는
  현재 `main()`에 인라인된 두 블록의 본문을 받아 `ctx`, `result`,
  `restoredMask`, `input`(원본 stdin) 갱신을 책임진다. 동반 헬퍼
  (`resolveCachedSessionState`, `fallbackByWorkspaceCwd`,
  `shouldRestoreWorkspace`, `shouldSuppressOutput`, `isWarmupExceptionPath`,
  `renderRateLimitOnly`)도 함께 이전한다 — 이들은 모두 진입점/출력보다는
  세션·suppress·warmup 판정 도메인에 속한다.
- **세션 캐시 도메인**: `cache.go`. 세션 상태 직렬화·저장·로딩·복원 정책에
  한정된다. 보유 식별자: `sessionStateTTL`, `workspaceRestoreTTL`,
  `SessionState`, `sessionCacheKey`, `safeCacheKeyPart`, `hashCacheKey`,
  `sessionStatePath`, `loadSessionState`, `loadByWorkspaceCwd`,
  `lastSessionStateCleanup`, `cleanOldSessionStates`, `restoredFieldMask`,
  `shouldRestoreFromSession`, `fillFromSessionCache`, `stripRestoredFields`,
  `saveSessionState`.
- **API 캐시 도메인**: `cache_api.go` (신규). HTTP 응답 캐시 I/O에 한정된다.
  보유 식별자: `cacheEntry`, `lastCleanup`, `cleanOldCachesFn`,
  `staleCacheMaxAge`, `cacheFilePath`, `readFileCache`, `writeFileCache`,
  `cleanOldCaches`. SPEC §5.6 식별자 일체가 이 파일로 이전된다.
- **HTTP 호출 경계**: `api.go`. UsageLimits 도메인 타입, `fetchUsageLimits`,
  `staleFallback`, `callAPI`/`callAPIWithCurl`/`doAPIRequest`/
  `decodeAPIResponse`, `parseRetryAfter`, `hashToken`, `parseUsageLimits`/
  `parseEntry`, 상수 `apiURL`/`userAgent`/`apiBeta`/`apiTimeout`만 남는다.
- **범용 파일 유틸 경계**: `fsutil.go` (신규). 도메인 무관한 OS-중립
  파일·경로·lock 유틸. 보유 식별자: `atomicWriteFile`, `withCacheFileLock`,
  관련 상수 `cacheLockTimeout`/`cacheLockRetryDelay`, cwd 정규화·탐지
  유틸인 `normalizeCwd`, `detectCurrentCwd`, `detectCurrentCwdWithSource`,
  indirection 변수 `detectCwdEnv`/`detectCwdGetwd`. OS별 구현인
  `file_lock_unix.go` / `file_lock_windows.go`는 그대로 유지된다.
- **위젯 오케스트레이션 경계**: `widget.go`. `Context`, `Widget`,
  `registry`/`registerWidget`, `displayPresets`, `presetCharToWidget`,
  `resolvePreset`, `OrchestrateResult`, `orchestrate`. i18n은 보유하지
  않는다.
- **i18n 경계**: `i18n.go` (신규). `localeEN`/`localeKO` embed,
  `Translations` 타입, `loadTranslations`, `detectLanguage`.

단일 `main` 패키지·zero dependency 제약은 유지된다(SPEC §3). 위 분리는
파일 단위 책임 재분배이며 새 import 경로·새 외부 모듈은 발생하지 않는다.

## 2. 데이터 흐름

진입점에서 출력까지의 흐름은 단계 식별자만 바뀌고 순서는 보존된다.

```
main.go: main()
  ├─ flag.Parse(), --version 처리
  ├─ configPath / configDir 결정
  ├─ loadConfig, parseStdin, getCredential, fetchUsageLimits
  ├─ go cleanOldSessionStates()  (cache.go)
  ├─ loadTranslations(cfg.Language)  (i18n.go)
  ├─ ctx 조립, result = orchestrate(ctx)  (widget.go)
  ├─ resolveSession(ctx, &result, &restoredMask, originalInput)  ← session.go
  │     ├─ Layer 1: cacheKey, resolveCachedSessionState,
  │     │           shouldRestoreFromSession, fillFromSessionCache →
  │     │           restoredMask 갱신 → 재 orchestrate
  │     └─ Layer 2: needsTranscriptBackfill 가드 → detectCurrentCwd /
  │                 transcript 경로 탐색 → readLastAssistantEntry →
  │                 D4 cwd 가드 → applyTranscriptToStdin →
  │                 restoredMask 갱신 → ctx.CostEstimated = true →
  │                 재 orchestrate
  ├─ shouldSuppressOutput → 출력 생략 분기  (session.go)
  ├─ isWarmupExceptionPath → renderRateLimitOnly로 result 교체 (session.go)
  ├─ fmt.Print(partsOutput)
  ├─ last-known [1m] 저장 트리거 (originalInput.Model.ID 기반)
  └─ save snapshot (stripRestoredFields → saveSessionState)  (cache.go)
```

핵심은 두 가지다.

1. `resolveSession`은 단일 진입점이지만 내부적으로 Layer 1과 Layer 2를 순차
   실행한다. 두 layer가 같은 `restoredMask` 인스턴스를 갱신하고 같은
   `result`를 두 번까지 교체할 수 있는 현재 동작을 보존하기 위해, helper는
   포인터로 `result`와 `restoredMask`를 받아 in-place 갱신한다.
2. save snapshot은 `main()`이 직접 호출한다. 이유: save 트리거는 원본
   `input`(backfill 전)을 참조해야 하는 `[1m]` 저장 조건과 묶여 있고, save
   여부 판정(`!warmupOnly && result.WidgetCount >= 2`)이 진입점 흐름의
   마지막 단계라 main 본체가 보유하는 것이 자연스럽다.

상태 전이는 새로 도입되지 않는다. `restoredFieldMask`의 비트 갱신·
`ctx.CostEstimated` 플래그·세션 캐시 파일의 SavedAt TTL 등 기존 상태
의미는 그대로다.

## 3. 인터페이스

이 리팩터는 단일 `main` 패키지 내부 파일 분할이며 외부 경계(공개 API,
CLI, 파일 포맷, 환경 변수)는 변경되지 않는다(SPEC §3).

내부 helper 경계 중 `main()` ↔ `resolveSession`만 기록할 가치가 있다 —
이 helper는 `main()`에서 호출되는 유일한 신규 진입점이며 §5에서 단일
진입점 형태로 commit된다. 다른 모든 helper는 `session.go` 내부 또는
`cache.go`와의 기존 경계를 그대로 따른다.

## 4. 영향 범위

- 직접 수정 파일: `main.go`, `api.go`, `cache.go`, `widget.go`.
- 신규 파일: `session.go`, `cache_api.go`, `fsutil.go`, `i18n.go`.
- 간접 영향: 이동 대상 식별자를 호출하는 모든 위치는 같은 `main` 패키지
  소속이므로 import 변경이 발생하지 않는다.
- 테스트 코드: 같은 `main` 패키지의 `*_test.go` 파일들은 테스트 대상 식별자
  이름이 바뀌지 않으므로 import·호출 모두 그대로 통과한다. 단,
  `cleanOldCachesFn`·`detectCwdEnv`/`detectCwdGetwd`·`fallbackByWorkspaceCwd`
  indirection을 swap하는 테스트가 있다면, 그 변수가 이동 후 파일에서도
  동일하게 패키지 변수로 노출되어야 한다(§5에서 commit).
- 빌드 시스템: Makefile은 파일을 와일드카드로 컴파일하므로 영향 없음.
- 외부 시스템·DB·파일 포맷·캐시 경로·API 헤더: 해당 없음(SPEC §3).

## 5. Decision Points

### 5.1 main() 분해 helper의 형태 — 단일 vs 다중

- 옵션 A (채택): 단일 진입점 `resolveSession(ctx, &result, &restoredMask,
  originalInput)`. Layer 1과 Layer 2를 순차로 실행한다.
- 옵션 B: `restoreLayer1` / `applyLayer2` 두 함수로 분리.
- 트레이드오프: B는 layer별 단위 테스트 표면을 명시화하지만, 두 layer는
  `restoredMask`를 공유하고 한 흐름 안에서 합산된다(main.go 144–155). 두
  함수로 쪼개면 호출자(`main()`)가 다시 layer 간 mask 통합·재 orchestrate
  순서를 보유하게 되어 `main()` 슬림화 목표(SPEC §5.5)와 충돌한다.
- 채택 근거: SPEC §5.5는 "별도 함수 호출 한 번으로 위임된다"를 명시한다.
  단일 진입점이 이 조건을 자연스럽게 만족한다.

### 5.2 main() 분해 helper의 파일명 — `session.go` vs `restore.go`

- 옵션 A (채택): `session.go`. 세션 식별·복원·suppress·warmup 판정을
  포함하는 더 넓은 경계다. `resolveCachedSessionState`,
  `fallbackByWorkspaceCwd`, `shouldRestoreWorkspace`,
  `shouldSuppressOutput`, `isWarmupExceptionPath`, `renderRateLimitOnly`
  모두를 같은 파일에 자연스럽게 수용한다.
- 옵션 B: `restore.go`. 의미가 "복원"에만 한정되어 suppress/warmup 헬퍼를
  같이 두기에 어색하다.
- 채택 근거: 이전 대상 헬퍼 군 전체를 일관된 이름으로 묶기 위해.

### 5.3 api.go 캐시 I/O 이전 위치 — `cache.go` 통합 vs `cache_api.go` 신규

- 옵션 A (채택): 신규 `cache_api.go`. API 캐시(`cache-*.json`)와 세션 캐시
  (`session-state-*.json`)는 도메인이 다르다. 두 캐시는 TTL·cleanup 트리거·
  파일명 prefix·serialize 타입(`cacheEntry` vs `SessionState`)이 모두 다르고,
  각각 `lastCleanup` / `lastSessionStateCleanup`이라는 독립 throttle 변수를
  보유한다.
- 옵션 B: `cache.go`에 통합. SPEC §5.6 본문은 "캐시 도메인 파일"로 두 옵션을
  모두 허용한다.
- 채택 근거: SPEC §2 목표("HTTP 호출과 파일 캐시 책임을 분리해 테스트 단위와
  변경 영향 범위를 좁힌다")가 캐시 도메인 자체의 분리도 함의한다.

### 5.4 cache.go 범용 I/O 분리 파일명 — `ioutils.go` vs `fsutil.go`

- 옵션 A (채택): `fsutil.go`. 파일시스템·경로·lock 유틸이라는 의미를 더
  좁게 잡는다.
- 옵션 B: `ioutils.go`. `io` 표준 패키지와 시각적으로 겹친다.
- 채택 근거: cwd 탐지(`detectCurrentCwd*`)·경로 정규화(`normalizeCwd`)·
  atomic write·file lock 모두 FS 도메인이며 generic `io`가 아니다.

### 5.5 widget.go i18n 분리 파일명 — `i18n.go` vs `translations.go`

- 옵션 A (채택): `i18n.go`. locale embed + 언어 감지 + 번역 로드 전체를
  포괄하는 표준 명칭.
- 옵션 B: `translations.go`. `Translations` 타입은 잘 표현하지만
  `detectLanguage`/`localeEN` embed를 같이 두기에는 의미가 좁다.
- 채택 근거: `detectLanguage`까지 한 파일에 묶이는 점이 결정적.

### 5.6 `cleanOldCachesFn` indirection 변수의 위치

- 옵션 A (채택): `cleanOldCaches`와 같이 `cache_api.go`로 이동.
- 옵션 B: `api.go`에 잔존(`fetchUsageLimits`의 호출 지점 옆).
- 트레이드오프: indirection의 목적은 cleanup 트리거 동작에 대한 테스트
  가능성이다. 트리거 대상 함수가 `cache_api.go`로 이동하면 indirection
  변수의 정의가 함께 이동해 "정의·구현 한 곳" 원칙을 유지하는 편이
  자연스럽다.
- 채택 근거: SPEC §5.6 본문이 `cleanOldCachesFn` indirection을 명시적으로
  api.go에서 제거 대상에 포함시켰다.

### 5.7 cwd 탐지·`normalizeCwd`의 분류

- 옵션 A (채택): `fsutil.go`로 함께 이동. `detectCurrentCwd*`,
  `normalizeCwd`, `detectCwdEnv`/`detectCwdGetwd` indirection을 같이 둔다.
- 옵션 B: `cache.go`에 잔존(세션 캐시 매칭에서만 쓰이는 헬퍼처럼 보일 수
  있음).
- 채택 근거: SPEC §5.7 본문이 `normalizeCwd`, `detectCurrentCwd*`를
  cache.go에서 제거 대상으로 명시한다. 실제 사용처도 세션 캐시 매칭(cache.go),
  fallback(session.go), Layer 2 transcript backfill(main.go 또는 session.go)
  으로 3곳에 걸쳐 있어 어느 한 도메인의 종속물이 아니다.

### 5.8 OS별 lock 파일 처리

- 옵션 A (채택): `file_lock_unix.go` / `file_lock_windows.go`는 그대로 둔다.
  OS-중립 래퍼 `withCacheFileLock`만 `fsutil.go`로 이전한다.
- 옵션 B: OS별 파일을 `fsutil_unix.go` / `fsutil_windows.go`로 리네이밍.
- 채택 근거: SPEC §1은 OS별 lock 파일을 이전 대상으로 지정하지 않는다.
  리네이밍은 §4 "이번 feature 범위 밖"의 의존성 정리 성격에 가까워 보존한다.

### 5.9 save snapshot 단계의 소유자 — `main.go` vs `session.go`

- 옵션 A (채택): `main.go`에 잔존. 호출은 `saveSessionState(cacheKey, ...)`
  한 줄이며 `stripRestoredFields` 호출도 인라인 한 줄.
- 옵션 B: `session.go`의 `resolveSession`이 save까지 책임.
- 채택 근거: save 트리거 조건이 출력 분기(`!warmupOnly && result.WidgetCount
  >= 2`)와 묶여 있고, last-known [1m] 저장 트리거도 같은 출력-후 단계에서
  원본 `input.Model.ID`를 참조한다. 진입점/출력 책임에 해당하므로 main 본체
  에 두는 편이 §5.5의 "진입점·플래그 처리·suppress 판정·최종 출력" 범주에
  부합한다.
