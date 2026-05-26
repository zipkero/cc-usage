# IMPLEMENT — degraded-cwd-fallback-relax

> 선행: spec.md §5, analysis.md §1·§2·§4·§5(D1–D4).
> 모든 Task는 분기 가드 완화(`primary == nil`) 한 줄에 수렴한다.

- [x] task-001: resolveCachedSessionState 분기 가드를 primary == nil로 완화
  - 목적: session_id로 만든 cacheKey가 비어있지 않아도 해당 세션의 디스크 캐시가 없거나 legacy 포맷이면 같은 cwd의 다른 세션 캐시로 폴백해 직전 정상 status line이 그대로 다시 출력된다
  - 접근: main.go의 resolveCachedSessionState에서 `cacheKey == "" && cached == nil` 가드를 `cached == nil` 단일 가드로 바꾸고, 함수 상단 doc 주석을 새 분기 의미(primary 적중 시 그대로 반환 / primary 미적중 시 cwd 폴백 시도, cross-workspace 노출은 loadByWorkspaceCwd 매처의 exact-equality + TTL 가드가 차단)로 갱신한다. fallbackByWorkspaceCwd / loadByWorkspaceCwd / 캐시 스키마 / save 경로 / shouldSuppressOutput은 무변경.
  - 검증 조건:
    - 결과: (a) cacheKey가 비-빈 값이고 디스크에 그 키의 캐시가 없을 때 fallbackByWorkspaceCwd가 호출되어 그 반환값이 그대로 caller에 전달된다. (b) cacheKey와 무관하게 primary 캐시가 적중하면 fallback은 호출되지 않는다. (c) 매처가 nil을 반환하면 caller에도 nil이 그대로 전달되어 기존 무출력 경로(shouldSuppressOutput)로 떨어진다.
    - 확인: `go vet ./...` / `go build ./...` 통과. task-002의 갱신·신규 테스트가 모두 통과. 함수 doc 주석이 새 분기 의도와 cross-workspace 가드 위임 위치(loadByWorkspaceCwd)를 명시한다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.4, SPEC §5.5, SPEC §5.6, ANALYSIS §4, ANALYSIS §5(D1·D2·D3)

- [x] task-002: resolveCachedSessionState 분기 변경에 대한 테스트 갱신·추가
  - 목적: 분기 가드 완화가 (a) primary 미적중 + cwd 매칭 시 fallback 진입, (b) primary 적중 시 fallback 미호출 두 동작을 모두 보장하며 기존 cross-workspace·TTL 회귀를 일으키지 않는다
  - 접근: 기존 `TestResolveCachedSessionStateNormalStdinSkipsFallback`을 새 의미로 갱신한다 — "정상 cacheKey + 디스크에 해당 키의 정상 캐시가 깔려 있을 때 primary 적중으로 fallback 미호출"을 어서션하도록 saveSessionState로 사전 캐시를 깔거나 디스크에 SessionState JSON을 직접 써둔다(HOME swap으로 격리). 그리고 신규 테스트를 추가한다 — "cacheKey가 비-빈 값이지만 그 키의 디스크 캐시가 부재하고 같은 cwd의 다른 세션 캐시는 존재할 때 fallback이 호출되고 그 반환값이 그대로 전달됨"을 fallbackByWorkspaceCwd hook으로 어서션. 기존 hook(`fallbackByWorkspaceCwd` 변수 교체, `detectCwdEnv` / `detectCwdGetwd` / `HOME`·`USERPROFILE` Setenv)을 그대로 재사용한다. 매처 단(`TestFallbackByWorkspaceCwdEndToEnd` / `TestFallbackFourPaths` / `TestMultiWorkspaceSequence` / `TestCdScenarioBlocksStaleWorkspaceRestore` / `TestFallbackRateLimitsIsolated`)은 무변경.
  - 검증 조건:
    - 결과: 갱신된 `TestResolveCachedSessionStateNormalStdinSkipsFallback`이 "primary 적중 → fallback 0회"를 검증한다. 신규 테스트가 "primary 미적중 + cwd 매칭 → fallback 1회 + 반환값 전달"을 검증한다. 기존 매처 단 테스트(cross-workspace 0회, TTL 초과 0회, 정규화 동치)는 변경 없이 통과한다.
    - 확인: `go test ./...` 전부 통과. `go test -run TestResolveCachedSessionState ./...`로 두 테스트가 개별 통과.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.4, SPEC §5.5, SPEC §5.7, ANALYSIS §4, ANALYSIS §5(D4)

- [x] task-003: 사용자 체감 fix patch bump (0.3.7 → 0.3.8)
  - 목적: `/plugin` UI가 본 fix를 update로 감지해 설치된 사용자에게 새 바이너리가 전파된다
  - 접근: `Makefile`의 `VERSION := 0.3.7`을 `0.3.8`로, `.claude-plugin/plugin.json`의 `"version": "0.3.7"`을 `"0.3.8"`로, `api.go`의 `userAgent = "cc-usage/0.3.7"`을 `"cc-usage/0.3.8"`로 동시 갱신. 세 곳 값이 일치한다.
  - 검증 조건:
    - 결과: 세 파일의 버전 문자열이 모두 `0.3.8`로 일치하며 다른 의미적 변경은 없다.
    - 확인: `grep -n 0.3.8 Makefile .claude-plugin/plugin.json api.go`가 세 위치를 모두 잡고, `0.3.7` 잔재가 소스에 남아있지 않다. `go build ./...` 통과.
  - 참조: SPEC §5.7, ANALYSIS §4
