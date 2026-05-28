# refactor-structure — implement

- [ ] task-001: fsutil.go 신설 + cache.go의 범용 I/O·cwd 유틸 이전
  - 목적: 도메인 무관한 파일·경로·lock 유틸을 세션 캐시 도메인 파일에서 분리해
    `cache.go`가 세션 상태 도메인만 보유하게 한다.
  - 접근:
    - `fsutil.go` 신설. 다음 식별자를 `cache.go`에서 이전: `atomicWriteFile`,
      `withCacheFileLock`, 상수 `cacheLockTimeout`, `cacheLockRetryDelay`,
      `normalizeCwd`, `detectCurrentCwd`, `detectCurrentCwdWithSource`,
      indirection 변수 `detectCwdEnv`, `detectCwdGetwd`.
    - `file_lock_unix.go` / `file_lock_windows.go`는 변경하지 않는다.
    - 호출자는 같은 `main` 패키지이므로 import 변경 없음.
  - 검증 조건:
    - 결과: `cache.go`에서 위 식별자 정의가 모두 제거되고 `fsutil.go`에 1회씩만
      존재한다.
    - 확인: 각 식별자를 `grep -n`으로 검사해 정의가 새 파일 1회·중복 정의 0회.
      `go vet ./...`, `go build ./...`, `go test ./...` 통과.
  - 참조: SPEC §5.7, §5.9, §5.3, §5.4 / ANALYSIS §1, §5.4, §5.7, §5.8

- [ ] task-002: cache_api.go 신설 + api.go의 파일 캐시 I/O 이전
  - 목적: HTTP 호출과 API 응답 파일 캐시 I/O를 분리해 `api.go`가 순수 HTTP
    레이어만 보유하게 한다.
  - 접근:
    - `cache_api.go` 신설. 다음 식별자를 `api.go`에서 이전: `cacheEntry` 타입,
      `lastCleanup` 변수, `cleanOldCachesFn` indirection 변수, 상수
      `staleCacheMaxAge`, `cacheFilePath`, `readFileCache`, `writeFileCache`,
      `cleanOldCaches`.
    - `api.go`에는 `UsageLimits` 타입, `fetchUsageLimits`, `staleFallback`,
      `callAPI`/`callAPIWithCurl`/`doAPIRequest`/`decodeAPIResponse`,
      `parseRetryAfter`, `hashToken`, `parseUsageLimits`/`parseEntry`, 상수
      `apiURL`/`userAgent`/`apiBeta`/`apiTimeout`만 남긴다.
    - `cleanOldCachesFn`은 패키지 변수로 그대로 노출해 기존 테스트 swap 동작
      유지.
  - 검증 조건:
    - 결과: `api.go`에서 위 식별자 정의가 모두 제거되고 `cache_api.go`에 1회씩만
      존재하며, `fetchUsageLimits`가 `cleanOldCachesFn`을 통해 cleanup을
      트리거하는 기존 동작이 유지된다.
    - 확인: `grep -n`으로 정의 위치 확인. `go vet ./...`, `go build ./...`,
      `go test ./...` 통과(특히 `cleanOldCachesFn` swap 테스트 통과).
  - 참조: SPEC §5.6, §5.9, §5.3, §5.4 / ANALYSIS §1, §5.3, §5.6

- [ ] task-003: i18n.go 신설 + widget.go의 i18n 이전
  - 목적: 위젯 오케스트레이션 파일에서 번역·언어 감지 책임을 분리해 `widget.go`가
    위젯 시스템 본연에만 집중하게 한다.
  - 접근:
    - `i18n.go` 신설. 다음을 `widget.go`에서 이전: `locales` 디렉터리
      `go:embed` 선언(`localeEN`, `localeKO`), `Translations` 타입,
      `loadTranslations`, `detectLanguage`.
    - `widget.go`에는 `Context`, `Widget`, `registry`, `registerWidget`,
      `displayPresets`, `presetCharToWidget`, `resolvePreset`,
      `OrchestrateResult`, `orchestrate`만 남긴다.
  - 검증 조건:
    - 결과: `widget.go`에서 i18n 관련 정의가 모두 제거되고 `i18n.go`에 1회씩만
      존재한다.
    - 확인: `grep -n`으로 정의 위치 확인. `go vet ./...`, `go build ./...`,
      `go test ./...` 통과.
  - 참조: SPEC §5.8, §5.9, §5.3, §5.4 / ANALYSIS §1, §5.5

- [ ] task-004: session.go 신설 + main() 분해, resolveSession 단일 진입점 도입
  - 목적: `main()`을 진입점·플래그·suppress·최종 출력에 한정하고, 세션 복원·
    transcript backfill 본문을 단일 helper로 위임한다.
  - 접근:
    - `session.go` 신설. 단일 진입점 `resolveSession(ctx, &result,
      &restoredMask, originalInput)` 도입 — 내부에서 현재 `main()` 인라인의
      Layer 1(세션 캐시 복원)·Layer 2(transcript backfill) 본문을 순차 실행한다.
      두 layer는 동일 `restoredMask` 인스턴스와 `result`를 in-place 갱신한다.
    - 동반 헬퍼 이전: `resolveCachedSessionState`, `fallbackByWorkspaceCwd`,
      `shouldRestoreWorkspace`, `shouldSuppressOutput`,
      `isWarmupExceptionPath`, `renderRateLimitOnly`.
    - `main()`에는 flag 파싱, `--version` 처리, configPath/configDir 결정,
      `loadConfig`/`parseStdin`/`getCredential`/`fetchUsageLimits`/
      `cleanOldSessionStates`/`loadTranslations` 호출, 초기 `orchestrate`,
      `resolveSession` 호출, suppress 분기, warmup 분기, `fmt.Print`, last-known
      `[1m]` 저장 트리거, save snapshot 호출(`stripRestoredFields` +
      `saveSessionState`)만 남긴다.
    - Layer 1/Layer 2 내부에서 호출되는 기존 함수(`shouldRestoreFromSession`,
      `fillFromSessionCache`, `needsTranscriptBackfill`,
      `encodeCwdToTranscriptDir`, `selectTranscriptCandidate`,
      `readLastAssistantEntry`, `applyTranscriptToStdin` 등)는 위치를 바꾸지
      않는다.
  - 검증 조건:
    - 결과: `main()`이 Layer 1·Layer 2 인라인 블록을 포함하지 않고
      `resolveSession` 호출 한 번으로 위임한다. `session.go`에 위 식별자가 정의
      되고 다른 파일에 중복 정의가 없다. `restoredMask` 비트 갱신 순서와
      `ctx.CostEstimated` 갱신 시점이 리팩터 전과 동일하다.
    - 확인: `main.go`에서 `restoredMask`·Layer 1/Layer 2 키워드 직접 등장
      여부를 `grep -n`으로 검사. `go vet ./...`, `go build ./...`,
      `go test ./...` 통과.
  - 참조: SPEC §5.5, §5.9, §5.3, §5.4 / ANALYSIS §1, §2, §3, §5.1, §5.2, §5.9

- [ ] task-005: 동작 동등성 회귀 검증
  - 목적: 리팩터 전후 외부 동작(stdout/stderr/exit code) 완전 동일성을 확인하고
    빌드·테스트 게이트를 통과시킨다.
  - 접근:
    - 동일 stdin JSON 입력 샘플로 리팩터 전(직전 commit) 바이너리와 리팩터 후
      바이너리의 stdout/stderr/exit code를 비교한다. 비교 입력 케이스:
      1. 정상 stdin (workspace·model·context·cost 모두 채워진 케이스),
      2. degraded stdin (workspace 비어있음 → Layer 1 복원 트리거),
      3. cost 누락 stdin (Layer 2 backfill 트리거),
      4. warmup exception 경로(`isWarmupExceptionPath` 트리거 케이스).
    - `go test ./...`, `go vet ./...`, `go build ./...`, `make build-local`
      성공 확인.
    - SPEC §3 제약 재확인: `go.mod`에 `require` 블록 없음, 단일 `main` 패키지
      유지.
  - 검증 조건:
    - 결과: 위 4개 케이스 모두에서 stdout/stderr/exit code가 리팩터 전과 동일.
      `go.mod`에 `require` 블록 없음. 이동된 모든 식별자에 대해 `grep`이 새 파일
      에서 정의 1회를 보여주며 중복 정의가 없다.
    - 확인: 4개 stdin 샘플을 리팩터 전/후 바이너리에 흘려 출력 바이트 단위 비교
      (PowerShell `Compare-Object` 또는 sha256). 빌드·테스트·vet 명령 종료
      코드 0 확인.
  - 참조: SPEC §5.1, §5.2, §5.3, §5.4, §5.9 / ANALYSIS §4
