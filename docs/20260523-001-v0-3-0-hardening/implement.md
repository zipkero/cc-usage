# v0-3-0-hardening — IMPLEMENT

## Section: 외부 명령 안전성

- [x] task-001: Keychain `security` 호출 timeout 적용
  - 목적: macOS Keychain이 잠겨 GUI prompt를 띄우는 상황에서도 status line stdout이 1초 이내에 완료되고, 첫 실패 이후 호출은 file fallback으로 즉시 분기한다
  - 접근: `credentials.go`의 `getCredentialFromKeychain`에서 `exec.Command`를 `exec.CommandContext(ctx, ...)`로 교체하고 `context.WithTimeout(parent, 500ms)`를 적용. timeout 또는 오류 시 기존 `keychainBackoffUntil = now + 60s` 분기로 흘러보낸다
  - 검증 조건:
    - 결과: 잠긴 Keychain 환경에서 첫 호출은 500ms 안에 timeout으로 끊기고 backoff에 진입, 후속 호출은 file fallback으로 즉시 반환. 정상 unlock 상태에서는 기존 lookup 동작이 유지된다
    - 확인: 수동으로 Keychain 잠금 상태에서 `cc-usage` 실행 후 stdout 응답 시간 측정. 코드 diff에서 `exec.CommandContext` + 500ms timeout 적용 확인
  - 참조: SPEC §5.1, ANALYSIS §2.2

- [x] task-002: Anthropic Usage API timeout 단축
  - 목적: API endpoint가 응답을 보내지 않는 상황에서도 status line stdout이 3초 이내에 출력되며, 출력에는 stale 캐시 값 또는 placeholder가 포함된다
  - 접근: `api.go`의 `apiTimeout` 상수를 `10 * time.Second`에서 `2 * time.Second`로 단축. HTTP client와 curl fallback 양쪽이 같은 상수를 참조하므로 1곳 수정으로 전파된다. negativeCache / staleFallback 경로는 그대로 둔다(단, task-005에서 in-process 부분은 제거 예정)
  - 검증 조건:
    - 결과: API timeout 발생 시 widget이 nil placeholder 또는 stale 캐시 값으로 fallback하고 stdout이 3초 이내 출력
    - 확인: 수동 테스트(예: 무응답 endpoint로 분기) + 코드 diff에서 `apiTimeout = 2 * time.Second` 확인
  - 참조: SPEC §5.2, ANALYSIS §2.3

- [ ] task-012: API utilization 응답 float 호환
  - 목적: Anthropic Usage API가 `utilization`을 `12.0` 같은 부동소수점으로 보내도 cc-usage가 decode 실패 없이 rate limit 위젯을 0~100 사이로 클램핑된 정수 퍼센트로 렌더한다
  - 접근: `api.go`의 `apiUsageResponse.{FiveHour,SevenDay,SevenDaySonnet}.Utilization`을 `int → float64`로 변경. `parseEntry` 시그니처를 `parseEntry(utilization float64, ...)`로 바꾸고 내부에서 `clampPercent(utilization)`로 정수 변환해 `UsageLimitEntry.Utilization` (int 유지)에 대입. 회귀 방지용 `api_test.go` 신규 생성, `TestDecodeAPIResponseAcceptsDecimalUtilization`이 `12.0`/`34.5`/`101.0` decode + clamp를 확인
  - 검증 조건:
    - 결과: `{"utilization": 12.5}` JSON 입력이 decode 성공하고 위젯이 `12%` (또는 클램핑된 정수)로 렌더. `101.0`은 `100%`로 클램프
    - 확인: `go test -run TestDecodeAPIResponseAcceptsDecimalUtilization` 통과 + 기존 테스트 전체 PASS
  - 참조: SPEC §5.11, ANALYSIS §2.8

## Section: CLI · 빌드 외연

- [x] task-003: `--version` 플래그 추가
  - 목적: `cc-usage --version` 실행 시 stdout에 버전 문자열 한 줄만 출력하고 exit 0으로 종료한다
  - 접근: `main.go`에서 `flag.Bool("version", false, ...)`를 등록하고, `flag.Parse()` 직후 stdin·config·credential 진입 전에 early-exit. 출력은 ldflags 주입 `version` 변수를 `fmt.Println`
  - 검증 조건:
    - 결과: `cc-usage --version` → stdout에 `0.3.0\n`만 출력, stderr 비어있음, exit code 0
    - 확인: 로컬 빌드 후 `./dist/cc-usage --version` 실행하여 출력·exit code 확인
  - 참조: SPEC §5.3, ANALYSIS §2.1

- [x] task-004: Makefile `VERSION` 0.3.0 및 `linux/arm64` 빌드 타깃 추가
  - 목적: cross-build 산출물이 `0.3.0` 버전 문자열을 ldflags로 받고, 5개 플랫폼(darwin/{arm64,amd64}, linux/{amd64,arm64}, windows/amd64) 바이너리가 생성된다
  - 접근: `Makefile`의 `VERSION := 0.2.0`을 `0.3.0`으로 갱신하고, `PLATFORMS` 변수에 `linux/arm64`를 추가한다. `bin/run.sh`는 이미 `uname -m`에서 `aarch64|arm64`를 `arm64`로 정규화하고 `bin/cc-usage-${OS}-${ARCH}`로 분기하므로 무수정
  - 검증 조건:
    - 결과: `make build` 실행 시 `bin/`에 5개 바이너리가 생성되고 각 바이너리의 `--version` 출력이 `0.3.0`. `bin/run.sh`가 linux arm64 호스트에서 `bin/cc-usage-linux-arm64`를 선택
    - 확인: `make build` 실행 후 `ls bin/cc-usage-*` 개수 확인 및 `./bin/cc-usage-darwin-arm64 --version` 출력 확인. run.sh 분기는 코드 inspection
  - 참조: SPEC §5.3, SPEC §5.4, ANALYSIS §2.6

## Section: Config 검증 · Dead state 제거

- [x] task-005: `api.go`의 `memCache` / `negativeCache` 제거
  - 목적: status line 동작은 변경 없이 단일-shot CLI에서 동작하지 않는 in-process 캐시 변수와 분기가 소스에서 사라진다
  - 접근: `api.go`의 package-level `memCache` / `negativeCache` map 선언과 `fetchUsageLimits` / `staleFallback`의 해당 분기·guard 코드를 모두 제거. `fetchUsageLimits` 시그니처는 유지하고 file cache 경로만 남긴다
  - 검증 조건:
    - 결과: `rg memCache|negativeCache` (소스 대상)가 0건. 기존 stdin 입력으로 status line이 정상 렌더되고 file cache 동작이 유지된다
    - 확인: `rg -n 'memCache|negativeCache' *.go` 0건 확인. 기존 stdin 샘플로 렌더 결과가 v0.2.0 출력과 동일한지 수동 비교
  - 참조: SPEC §5.9, ANALYSIS §2.5

- [x] task-006: `Config.DailyBudget` / `Config.Plan` 및 관련 잔존 참조 제거
  - 목적: v0.2.0 cc-usage.json(특히 `dailyBudget`, `plan` 필드를 포함한 것)을 v0.3.0이 입력으로 받아도 status line이 정상 렌더되고 stderr에 관련 경고가 출력되지 않으며, 해당 식별자는 소스에 남지 않는다
  - 접근: `config.go`에서 `Config` struct의 `DailyBudget`, `Plan` 필드 선언과 default 채움 코드를 제거. `main.go:49` 부근의 `Plan`을 참조하는 debugLog format을 함께 제거한다. JSON unmarshal은 `DisallowUnknownFields`를 쓰지 않으므로 v0.2.0 입력의 해당 필드는 silent ignore된다
  - 검증 조건:
    - 결과: `rg 'DailyBudget|\.Plan\b'` (소스 대상)이 0건. v0.2.0 형태 `{"dailyBudget":5.0,"plan":"max",...}` 입력으로 실행 시 stdout은 정상 렌더, stderr는 dailyBudget/plan 관련 메시지 없음
    - 확인: `rg -n 'DailyBudget|\.Plan\b' *.go` 0건 확인. 수동 stdin 테스트로 stderr 비어있음 확인
  - 참조: SPEC §5.7, SPEC §5.9, ANALYSIS §2.5

- [x] task-007: `Translations`의 미사용 라벨 3종 및 locales JSON 키 제거
  - 목적: 위젯 코드가 참조하지 않는 i18n 라벨이 소스와 locales에서 사라지며, 기존 정상 위젯 렌더는 영향받지 않는다
  - 접근: `widget.go`의 `Translations` struct에서 `Errors.NoContext`, `Labels.OneM`, `Labels.SevenDAll` 필드를 제거. `locales/en.json`, `locales/ko.json`에서 대응 키를 제거한다
  - 검증 조건:
    - 결과: `rg 'NoContext|OneM|SevenDAll'`(소스+locales 대상)이 0건. 기존 stdin 샘플로 위젯 렌더 동작 유지
    - 확인: `rg -n 'NoContext|OneM|SevenDAll' . -g '!docs'` 0건 확인. 수동 렌더 결과 비교
  - 참조: SPEC §5.9, ANALYSIS §2.5

- [x] task-008: `validateConfigEnums` 도입 (displayMode / separator / theme / language)
  - 목적: 사용자가 cc-usage.json에 알 수 없는 enum 값을 지정하면 stderr에 어느 필드의 어떤 값이 무효인지 한 줄 경고가 출력되고 stdout 위젯은 해당 필드의 기본값으로 fallback해서 정상 렌더된다. 유효한 값만 사용하면 stderr는 비어있다
  - 접근: `config.go`에 `validateConfigEnums(cfg *Config)` 헬퍼를 추가하고 `loadConfig`의 unmarshal+merge 직후에 호출한다. 각 필드의 허용 집합과 fallback 기본값은 ANALYSIS §2.4 표를 따른다(`displayMode` ∈ {compact, custom} → "compact", `separator` ∈ {pipe, dot, arrow, space, ""} → "", `theme` ∈ themes 키 ∪ {""} → "", `language` ∈ {auto, en, ko} → "auto"). 경고 emitter는 `fmt.Fprintln(os.Stderr, ...)` 직접 호출이며 `debugLog` 사용 금지(DEBUG env 미설정에서도 출력되어야 함)
  - 검증 조건:
    - 결과: 무효 enum 값 입력 시 stderr에 `cc-usage: invalid config: <field>=<value> not in {...}, falling back to <default>` 형태 한 줄 출력 + 해당 필드는 기본값으로 동작. 모든 enum이 유효하면 stderr 비어있음(DEBUG 미설정)
    - 확인: task-010의 `TestLoadConfigWarnsOnUnknownEnum` 등 새 테스트로 자동 검증 + 수동 stdin 테스트
  - 참조: SPEC §5.5, SPEC §5.6, SPEC §5.7, ANALYSIS §2.4

## Section: 회귀 방지 테스트

- [x] task-009: `widget_test.go` 신규 — preset 파싱 및 분석 위젯 GetData
  - 목적: preset 문자열 파싱과 새 분석 위젯의 누락 입력 처리 동작이 다음 릴리스에서 깨지면 `go test ./...`가 즉시 감지한다
  - 접근: `widget_test.go`를 신규 생성하여 다음 케이스를 추가한다 — `TestResolvePresetParsesChars`(`"P|M$C"` → `[["projectInfo"], ["model","cost","context"]]`, `DisplayMode == "custom"`), `TestResolvePresetIgnoresUnknownChars`(`"Mz$"` → `[["model","cost"]]`), `TestBurnRateGetDataNilOnMissingCost`(`(nil, nil)`), `TestSessionDurationGetDataNilOnMissingMs`(`(nil, nil)`)
  - 검증 조건:
    - 결과: 신규 테스트가 의도한 회귀 케이스를 커버하며 모두 PASS
    - 확인: `go test ./... -run 'ResolvePreset|BurnRateGetData|SessionDurationGetData'` 통과
  - 참조: SPEC §5.8, ANALYSIS §2.7

- [x] task-010: `config_test.go` 신규 — loadConfig 머지 / v0.2.0 호환 / enum 경고
  - 목적: config 머지·v0.2.0 호환·enum 검증 경로가 다음 릴리스에서 깨지면 `go test ./...`가 즉시 감지한다
  - 접근: `config_test.go`를 신규 생성하여 다음 케이스를 추가한다 — `TestLoadConfigMergesDefaults`(기본값 머지), `TestLoadConfigAcceptsV020Fields`(`{"dailyBudget":5.0,"plan":"max","language":"ko"}` 입력 시 stderr에 `dailyBudget`/`plan` 미포함, cfg.Language == "ko"), `TestLoadConfigWarnsOnUnknownEnum`(`{"displayMode":"bogus"}` 입력 시 stderr에 `displayMode` 식별자 포함, cfg.DisplayMode == "compact"). stderr 캡처는 `os.Pipe`로 `os.Stderr`를 교체했다가 복원하는 헬퍼를 같은 파일 안에 둔다
  - 검증 조건:
    - 결과: 신규 테스트 3개가 모두 PASS, stderr 캡처가 정상 동작
    - 확인: `go test ./... -run 'LoadConfig'` 통과
  - 참조: SPEC §5.7, SPEC §5.8, ANALYSIS §2.7

## Section: 빌드 산출물 · 릴리스 sync

- [ ] task-011: `make build` 산출물 커밋 및 release 브랜치 sync
  - 목적: main은 v0.3.0 변경분과 5개 플랫폼 바이너리를 포함한 상태가 되고, release 브랜치는 main의 v0.3.0을 sync한 상태가 되며, GitHub default branch는 `release`를 유지한다
  - 접근: 모든 코드 Task와 테스트 통과 확인 후 `make build`로 `bin/` 5개 바이너리를 재생성하고 git에 커밋한다. 이후 release 브랜치 sync 절차는 CLAUDE.md "배포" 섹션을 그대로 따른다(별도 절차 신설 금지)
  - 검증 조건:
    - 결과: main HEAD에 5개 플랫폼 바이너리가 커밋되어 있고 release 브랜치가 main의 v0.3.0 변경을 포함한다. GitHub default branch가 `release`로 남아있다
    - 확인: `ls bin/cc-usage-*` 5개 확인, `git log` / `git branch -a` 로 main·release 동기화 상태 확인, GitHub repo settings에서 default branch 확인
  - 참조: SPEC §5.4, SPEC §5.10
