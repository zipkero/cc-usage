# implement: simplify-statusline

데이터 출처를 stdin·git으로 단일화하고 공식 문서에 없는 표시 항목을 제거하는 작업의 실행
체크리스트다. 각 Task는 의존성 순서로 나열되며, 앞 Task가 만든 상태를 뒤 Task가 전제한다.
삭제 대상 파일은 비우거나 dead 함수로 남기지 않고 파일째 삭제하며, 마지막에 orphan 심볼·파일이
없음을 빌드·테스트·grep으로 검증한다.

## Section: 존속 경로 정리 (helper 이전 · stdin 전용화)

- [x] task-001: cwd 탐지 helper를 projectInfo 위젯 파일로 이전
  - 목적: 작업 디렉터리가 비어 있을 때도 프로젝트 이름과 git 브랜치가 그대로 표시된다.
  - 접근: `cache.go`의 cwd 탐지·정규화 helper(`detectCurrentCwd`,
    `detectCurrentCwdWithSource`, 패키지 변수 `detectCwdEnv`/`detectCwdGetwd`, `normalizeCwd`)를
    유일 소비자인 `widgets_project.go`로 옮긴다. 세션캐시·복원·lock 연동 함수는 함께 옮기지
    않는다(task-012에서 `cache.go`와 함께 소멸). 심볼 이름·시그니처는 유지한다.
  - 검증 조건:
    - 결과: cwd helper가 `widgets_project.go`에 존재하고 `cache.go`에서는 사라진다.
    - 확인: `go build ./...` 통과, `widgets_project_test.go`의 cwd 주입 케이스가 green.
  - 참조: SPEC §5.7, ANALYSIS §5 D1, §4

- [x] task-002: 5h/7d 위젯을 stdin 전용으로 축소
  - 목적: 5시간·7일 rate limit 표시가 Claude Code가 보낸 입력값만으로 결정된다.
  - 접근: `widgets_core.go`의 `rateLimit5hWidget`/`rateLimit7dWidget` `GetData`에서 API
    fallback 분기(`ctx.RateLimits` 기반)를 제거하고 `ctx.Stdin.RateLimits`만 읽도록 남긴다.
  - 검증 조건:
    - 결과: 두 위젯이 stdin `rate_limits` 값만으로 렌더되고 API 산출물을 참조하지 않는다.
    - 확인: `go build ./...` 통과. `rate_limits.five_hour`가 담긴 stdin echo 파이프에서 5h가
      렌더되는지 확인(CLAUDE.md §동작 확인).
  - 참조: SPEC §5.1, SPEC §5.2, ANALYSIS §2, §5 D2

- [x] task-003: analytics 위젯 파일 삭제
  - 목적: version·apiDuration·sessionDuration·burnRate·cacheHit·performance 표시 항목이 어떤
    입력·설정에도 출력되지 않는다.
  - 접근: `widgets_analytics.go`를 파일째 삭제한다. 6개 위젯 구현과 `init()` 등록이 함께
    사라진다. preset char·displayPresets·locales의 잔존 참조는 task-006/007에서 정리.
  - 검증 조건:
    - 결과: `widgets_analytics.go`가 존재하지 않는다.
    - 확인: `ls widgets_analytics.go`가 No such file로 실패한다(최종 빌드 green은 task-014).
  - 참조: SPEC §5.4, ANALYSIS §4, §5 D6

- [x] task-004: 5h/7d 데이터 부재 시 위젯 미표시로 전환
  - 목적: stdin에 rate limit 필드가 없으면 해당 항목이 status line에 나타나지 않는다.
  - 접근: `widgets_core.go`에서 `--` placeholder 경로를 제거한다. `rateLimitData.Unavailable`
    필드와 그 분기, `Unavailable: true` 반환을 없애고 데이터 부재 시 `GetData`가 `nil`을
    반환하게 해 orchestrator가 위젯을 skip하게 한다. Render의 `Unavailable` 분기도 정리.
  - 검증 조건:
    - 결과: stdin에 `rate_limits`가 없으면 5h/7d 위젯이 출력에 포함되지 않는다.
    - 확인: `rate_limits`를 뺀 stdin echo 파이프 출력에 `5h`/`7d` 라벨이 없고, 포함한 입력에는
      나타나는지 대조 확인. `go build ./...` 통과.
  - 참조: SPEC §5.2, ANALYSIS §2, §5 D2

- [x] task-005: cost 위젯의 transcript 추정 분기 제거
  - 목적: 비용 표시가 입력의 비용 값만으로 결정되고 `~` 추정 접두 표기가 어떤 입력에도
    나타나지 않는다.
  - 접근: `widgets_core.go`의 cost 위젯에서 `ctx.CostEstimated` 참조 분기와 `estimatedCostMarker`
    상수를 제거한다. cost는 `stdin.cost.total_cost_usd`만으로 렌더한다. (Context의 `CostEstimated`
    필드 제거는 task-009.)
  - 검증 조건:
    - 결과: cost 위젯이 `~` 마커를 생성하는 코드 경로가 남지 않는다.
    - 확인: `grep -rn "estimatedCostMarker\|CostEstimated" widgets_core.go`가 빈 결과.
      `go build ./...` 통과. cost가 담긴 stdin echo 파이프에 `$` 값만 나오고 `~`가 없는지 확인.
  - 참조: SPEC §5.5, ANALYSIS §2, §5 D3

- [x] task-006: 7dSonnet·analytics의 preset char·displayPresets 정리
  - 목적: 제거된 위젯들은 preset 문자나 기본 레이아웃을 통해서도 되살아나지 않는다.
  - 접근: `widgets_core.go`에서 `rateLimit7dSonnetWidget` 구현과 `registerWidget` 등록을
    제거한다. `widget.go`의 `presetCharToWidget`에서 `'S'`(7dSonnet)와
    `'V'/'a'/'D'/'B'/'H'/'F'`(analytics)를 제거하고, `displayPresets["compact"]`에서
    `"rateLimit7dSonnet"`을 제거한다.
  - 검증 조건:
    - 결과: `presetCharToWidget`에 위 7개 char가 없고 `displayPresets`에 7dSonnet이 없으며
      7dSonnet 위젯 등록이 사라진다.
    - 확인: `grep -rn "rateLimit7dSonnet" .`이 소스에서 빈 결과(테스트 잔존은 task-013에서 해소).
      `go build ./...` 통과.
  - 참조: SPEC §5.3, SPEC §5.4, ANALYSIS §5 D6

- [x] task-007: locales와 Translations에서 죽은 라벨·위젯 문자열 제거
  - 목적: 제거된 위젯에 대한 i18n 문자열이 어디에도 남지 않는다.
  - 접근: `locales/en.json`·`locales/ko.json`에서 `labels.sevenDSonnet`과 analytics 관련
    `widgets` 블록 항목(apiDuration·burnRate·cache·performance·session)을 제거하고, `widget.go`의
    `Translations` 구조체에서 대응 필드를 제거해 JSON 구조와 일치시킨다.
  - 검증 조건:
    - 결과: 두 locale 파일과 `Translations` 구조체에 7dSonnet·analytics 문자열이 없다.
    - 확인: `grep -rn "sevenDSonnet\|apiDuration\|burnRate\|performance" locales widget.go`가 빈
      결과. go:embed JSON이 `Translations`로 디코드되며 `go build ./...` 통과.
  - 참조: SPEC §5.3, SPEC §5.4, ANALYSIS §4, §5 D6

## Section: 진입점 단순화 (백엔드 레인 제거)

- [x] task-008: 무출력 판정을 stdin-identity 검사로 축소하고 warmup 예외 제거
  - 목적: model·작업 디렉터리·context가 모두 비어 있으면 빈 status line 없이 출력이 생략되고,
    그 외에는 입력에 담긴 값이 그대로 그려진다.
  - 접근: `main.go`에서 `shouldSuppressOutput`이 rate limit 인자에 의존하지 않게 바꿔
    model(id/display)·`workspace.current_dir`·`context_window_size`만으로 생략을 판정하게 한다.
    warmup 예외(`isWarmupExceptionPath`/`renderRateLimitOnly` 호출·정의)를 제거한다.
  - 검증 조건:
    - 결과: 무출력 판정이 API rate limit 유무에 좌우되지 않고, warmup 예외 함수가 존재하지 않는다.
    - 확인: `grep -rn "isWarmupExceptionPath\|renderRateLimitOnly" .`이 빈 결과. model·dir·context를
      모두 뺀 stdin echo 파이프가 빈 출력을, 채운 입력이 정상 출력을 내는지 대조. `go build ./...`
      통과.
  - 참조: SPEC §5.6, ANALYSIS §2, §5 D5

- [x] task-009: Context에서 죽은 필드 제거
  - 목적: 위젯이 받는 컨텍스트가 입력·설정·i18n만 담아 외부 데이터 경로의 흔적을 남기지 않는다.
  - 접근: `widget.go`의 `Context`에서 `RateLimits`·`CostEstimated`·`ConfigDir`을 제거해
    `Stdin`/`Config`/`Translations`만 남기고, `main.go`에서 이 필드를 채우던 대입을 모두 제거한다.
  - 검증 조건:
    - 결과: `Context`가 세 필드만 보유하고 죽은 필드 대입이 사라진다.
    - 확인: `grep -rn "\.RateLimits\b\|CostEstimated\|\.ConfigDir\b" widget.go main.go widgets_core.go`
      에서 `Stdin.RateLimits`를 제외한 Context 측 참조가 없다. `go build ./...` 통과.
  - 참조: SPEC §5.1, ANALYSIS §3, §5 D4

- [x] task-010: 진입점에서 credential·API·세션캐시·복원 흐름 제거
  - 목적: status line을 그리는 동안 cc-usage가 credential·캐시 파일을 읽거나 쓰지 않고 어떤
    네트워크 엔드포인트에도 접속하지 않는다.
  - 접근: `main.go`를 `config 로드 → stdin 파싱 → orchestrate → 무출력 판정 → 출력`으로
    축소한다. `getCredential`·`fetchUsageLimits` 호출, 세션캐시 cleanup goroutine,
    `loadSessionState`/`saveSessionState`, Layer 1 세션캐시 복원, Layer 2 transcript 복원
    (`recordLastModel`/`[1m]` 신호 저장 포함)과 그에 딸린 debugLog를 제거한다.
  - 검증 조건:
    - 결과: `main.go`가 위 함수들을 호출하지 않고 흐름이 단일 stdin→render 경로다.
    - 확인: `grep -rn "getCredential\|fetchUsageLimits\|loadSessionState\|saveSessionState\|recordLastModel" main.go`가
      빈 결과. 정상 stdin echo 파이프가 model·dir·branch·context·cost·5h·7d를 정상 출력하는지
      확인(소멸 파일 미삭제로 최종 빌드 green은 task-014).
  - 참조: SPEC §5.1, ANALYSIS §1, §2, §5 D5

- [x] task-011: cache.ttlSeconds 설정 처리 제거
  - 목적: 더 이상 쓰이지 않는 캐시 TTL 설정이 설정 스키마와 기본값 머지에서 사라진다.
  - 접근: `config.go`에서 `CacheConfig` 타입, `Config.Cache` 필드, 기본값 머지, `TTLSeconds == 0`
    분기를 제거한다. 기존 설정의 `cache` 블록은 모르는 JSON 키로 무시되어 파싱이 깨지지 않는다.
  - 검증 조건:
    - 결과: `Config`에 `Cache` 필드가 없다.
    - 확인: `grep -rn "CacheConfig\|TTLSeconds\|\.Cache\b" config.go`가 빈 결과. `cache` 블록을
      포함한 `cc-usage.json`으로 실행해도 에러 없이 동작. `go build ./...` 통과.
  - 참조: SPEC §5.1, ANALYSIS §3, §5 D8

## Section: 파일·테스트 삭제 및 검증

- [x] task-012: 소멸 대상 소스 파일 일괄 삭제
  - 목적: 제거된 데이터 경로의 소스 파일이 저장소에 남지 않는다.
  - 접근: 모든 소비자가 제거된 뒤(task-001~011 전제) `credentials.go`, `api.go`, `pricing.go`,
    `transcript.go`, `last_model_cache.go`, `file_lock_unix.go`, `file_lock_windows.go`,
    `cache.go`를 파일째 삭제한다. `cache.go`의 cwd helper는 task-001에서 이미 이전됐다.
    (`widgets_analytics.go`는 task-003에서 삭제됨.)
  - 검증 조건:
    - 결과: 위 8개 파일이 존재하지 않는다.
    - 확인: `ls credentials.go api.go pricing.go transcript.go last_model_cache.go file_lock_unix.go
      file_lock_windows.go cache.go`가 전부 No such file로 실패한다.
  - 참조: SPEC §5.1, SPEC §5.5, ANALYSIS §1, §4, §5 D1

- [x] task-013: 소멸·혼재 테스트 파일 정리
  - 목적: 삭제된 코드에 묶인 테스트가 컴파일을 막지 않고, 존속 동작의 테스트는 남는다.
  - 접근: 소멸 파일에 1:1 대응하는 테스트(`api_test.go`, `cache_test.go`, `pricing_test.go`,
    `transcript_test.go`, `last_model_cache_test.go`, `e2e_task014_test.go`)를 파일째 삭제한다.
    혼재 테스트는 삭제 대상 케이스만 들어낸다 — `main_test.go`(복원/fallback/Layer2/`[1m]` 케이스
    제거, 존속 케이스 유지), `widget_test.go`(burnRate·sessionDuration 케이스 제거),
    `widgets_core_test.go`(`CostEstimated` 마커 케이스 제거). `config_test.go`의 `Cache.TTLSeconds`
    기본값 검증 케이스는 task-011 결정에 따라 제거한다.
  - 검증 조건:
    - 결과: 위 6개 테스트 파일이 없고 혼재 테스트는 존속 케이스만 남는다.
    - 확인: `go vet ./...`와 `make test`가 통과하고 존속 케이스가 green이다.
  - 참조: SPEC §5.1, ANALYSIS §4, §5 D7

- [x] task-014: 전체 빌드·테스트 green 및 orphan 부재 검증
  - 목적: 정상 입력에서 model·project·git branch·context·cost·5h·7d가 정상 출력되고 멀티라인
    설정 시 각 줄이 별도 행으로 나오며, 미사용 파일·심볼이 남지 않는다.
  - 접근: `go build ./...`·`go vet ./...`·`make test`를 green으로 확인하고, 죽은 심볼 부재를
    grep으로 확인한다(`getCredential`, `fetchUsageLimits`, `loadSessionState`,
    `estimatedCostMarker`, `CostEstimated`, `rateLimit7dSonnet`, `isWarmupExceptionPath`,
    `formatDuration`, `UsageLimits`, `cwdWithinRoot` 등). `make build-local` 후 정상 stdin과
    멀티라인(`lines`) 설정으로 echo 파이프 출력을 육안 확인한다.
  - 검증 조건:
    - 결과: 세 명령이 모두 green이고 위 심볼 grep이 전부 빈 결과이며, 정상 입력의 단일/멀티라인
      출력이 의도대로 나온다.
    - 확인: `go build ./... && go vet ./... && make test`가 성공 종료, 심볼 grep 묶음 매치 0건,
      CLAUDE.md §동작 확인의 echo 파이프가 기대 라인을 출력.
  - 참조: SPEC §5.1, SPEC §5.7, ANALYSIS §5 D7

- [x] task-015: 문서에서 제거된 동작 서술 정리
  - 목적: 사용자·개발자 문서에 credential·OAuth API·캐시·degraded 복원·세션 캐시·7d-S·analytics·
    transcript cost 추정에 대한 설명이 남지 않는다.
  - 접근: 루트 `README.md`에서 Configuration의 `.credentials.json` 안내, Widgets의 7dSonnet·
    analytics 행, Troubleshooting의 idle 세션캐시 복원/TTL 설명, Privacy의 네트워크·저장 항목을
    갱신·제거한다(남은 캐시 파일은 더 읽지 않으며 사용자가 직접 지울 수 있음을 반영). 루트
    `CLAUDE.md`에서 아키텍처 표의 삭제 파일 행, Degraded-input 복원 절, 무출력 조건의 warmup
    서술, 경로 표의 인증·API 캐시·세션 캐시 행, 배포 절 userAgent 언급을 갱신·제거한다.
  - 검증 조건:
    - 결과: 두 문서에 제거된 동작·파일에 대한 서술이 없다.
    - 확인: `grep -rin "credential\|oauth\|7d-S\|sevenDSonnet\|burnRate\|transcript\|session-state\|degraded" README.md CLAUDE.md`가
      제거 대상 서술에 매치되지 않는다(유지되는 일반 용어 매치는 검토 후 허용).
  - 참조: SPEC §5.8, ANALYSIS §4

- [x] task-016: 버전 bump 및 release 동기화
  - 목적: 사용자가 `/plugin` UI에서 갱신을 감지하고, 단순화된 동작이 배포본에 반영된다.
  - 접근: `Makefile`의 `VERSION`(현재 `0.3.15`)과 `.claude-plugin/plugin.json`의 `version`을 같은
    새 SemVer 값으로 동시에 올린다(사용자 체감 동작 변화이므로 minor bump 권장 — 최종 값은 사용자
    확인). `api.go`는 task-012에서 삭제되므로 userAgent 동기화 대상은 없다. `make build`로 `bin/`을
    재빌드해 main에 commit한다. release 브랜치 동기화는 되돌리기 어려운 조치이므로 사용자 확인 후
    진행한다.
  - 검증 조건:
    - 결과: `Makefile VERSION`과 `plugin.json version`이 동일한 새 값이고 `bin/`이 재빌드된다.
    - 확인: `grep VERSION Makefile`과 `plugin.json`의 version이 일치하고, release push는 사용자
      승인 게이트를 통과한 뒤에만 수행된다.
  - 참조: SPEC §3, SPEC §5.1, SPEC §5.7
