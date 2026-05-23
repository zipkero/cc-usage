# v0-3-0-hardening — ANALYSIS

## 근거

탐색 범위와 핵심 사실:

- `credentials.go` — `getCredentialFromKeychain`은 `exec.Command("security", ...).Output()`을 timeout 없이 직접 호출한다. Keychain이 잠겨있어 GUI prompt가 뜨면 status line 호출 전체가 그 동안 블록된다. 첫 실패 시 `keychainBackoffUntil = now.Add(60s)`로 backoff에 진입하므로, "첫 호출 timeout → 이후 호출은 file fallback 즉시 분기"라는 SPEC §5.1 시나리오가 timeout만 도입하면 그대로 성립한다.
- `api.go` — `apiTimeout = 10 * time.Second`. HTTP client와 curl fallback 양쪽에 동일 상수가 사용된다. negative cache(30s)와 stale fallback(1h)이 이미 갖춰져 있어 API timeout 단축 시 SPEC §5.2의 placeholder/stale 출력 보장 경로는 코드 변경 없이 유지된다.
- `main.go` — `flag.String("config", ...)`만 등록. `version` 패키지 변수는 ldflags(`-X main.version=...`)로 주입되며, 현재 어디에도 출력되지 않는다. `Makefile`의 `VERSION := 0.2.0`이 진실 원천.
- `config.go` — `loadConfig`는 JSON unmarshal 후 zero-value 필드만 default로 채우고, enum validation은 전혀 없다. `DailyBudget *float64`, `Plan string`은 선언만 존재하고 어떤 위젯에서도 읽지 않는다(grep으로 확인). `Plan`은 `main.go:49`의 debugLog format string에 한 번 등장하나 의미상 fallback 동작 없음.
- `widget.go` — `Translations` struct의 `Errors.NoContext`, `Labels.OneM`, `Labels.SevenDAll`은 widget 코드 어디에서도 참조되지 않는다(grep으로 확인). `locales/{en,ko}.json`에는 값이 들어있다.
- `api.go` — `memCache`/`negativeCache`는 package-level `map`이지만 cc-usage는 Claude Code가 매 status line 호출마다 새 프로세스로 fork-exec하는 one-shot CLI다. 같은 프로세스 안에서 같은 토큰으로 두 번 lookup하는 경로가 없으므로 in-process map은 항상 cold start. file cache(`~/.cache/cc-usage/cache-<hash>.json`)만이 cross-invocation 캐시 역할을 한다. dead state.
- `cache.go` / `cache_test.go` — `last_output` 경로는 이미 코드/JSON에서 모두 사라졌다. `cache_test.go:70-72`는 legacy `session-state.json`이 생성되지 않음을 회귀 보장한다. SPEC §5.9의 `last_output`은 "코드에 더 이상 등장하지 않음을 verify"하는 항목이며, 별도 삭제 작업은 필요 없음.
- `Makefile` — `PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 windows/amd64`. 4개. `linux/arm64` 추가 필요.
- `bin/run.sh` — `uname -m`에서 `arm64|aarch64`를 이미 `arm64`로 정규화한다. `BIN="$DIR/cc-usage-${OS}-${ARCH}"`로 조립하므로, `bin/cc-usage-linux-arm64`만 추가되면 분기 로직 자체는 그대로 두고 통과한다.
- 기존 테스트 — `cache_test.go`만 존재 (cache key 우선순위, atomic write, lock 직렬화). preset 파싱(`resolvePreset`), `loadConfig` 머지, 분석 위젯 GetData에 대한 테스트는 없다.

## 1. 구조

본 릴리스의 변경은 단일 `main` 패키지 안에 머문다.

| 영역 | 파일 | 사유 |
|------|------|------|
| Keychain timeout | `credentials.go` (수정) | 외부 의존 호출의 소유 파일 |
| API timeout 단축 | `api.go` (수정) | 상수 1곳에서 HTTP + curl fallback 양쪽 자동 전파 |
| `--version` 플래그 | `main.go` (수정) | flag 등록·parse·early-exit 모두 `main()` 진입부 |
| `linux/arm64` 추가 | `Makefile` `PLATFORMS` 한 줄, `bin/cc-usage-linux-arm64` 신규 산출물. `bin/run.sh` 무수정 |
| enum validation 경고 | `config.go` 내 새 헬퍼 `validateConfigEnums(cfg)`. 별도 파일 신설 없음 (SPEC §4) |
| Dead field 제거 | `config.go` (`DailyBudget`, `Plan`), `widget.go` (Translations 3개), `locales/{en,ko}.json`, `api.go` (`memCache`/`negativeCache`), `main.go`의 `Plan` debugLog |
| 회귀 방지 테스트 | `config_test.go` 신규, `widget_test.go` 신규. 분할 이유: 서로 다른 소스 단위 |
| Version 상수 단일화 | `Makefile VERSION := 0.3.0`. ldflags 주입만 사용 |

## 2. 데이터 흐름

### 2.1 `--version` early-exit (SPEC §5.3)

```
flag.Parse() → if *showVersion { fmt.Println(version); os.Exit(0) } → 기존 경로
```

stdin·config·credential보다 반드시 앞서 early-exit. 출력은 `version` 한 줄(`0.3.0\n`).

### 2.2 Keychain timeout (SPEC §5.1)

```
getCredentialFromKeychain
  ├── backoffUntil 체크 → file fallback (변경 없음)
  ├── ctx, cancel := context.WithTimeout(parent, 500ms)
  ├── exec.CommandContext(ctx, "security", ...).Output()
  └── err 발생 시 backoffUntil = now + 60s → file fallback
```

500ms 근거: 정상 unlock 상태 lookup은 10–50ms. SPEC §5.1 1초 budget 안에 있고 false-trigger 마진 충분.

### 2.3 API timeout 단축 (SPEC §5.2)

```
const apiTimeout = 10 * time.Second → 2 * time.Second
```

`callAPI`의 `http.Client.Timeout`과 `callAPIWithCurl`의 `context.WithTimeout` 양쪽에 전파. failure path는 `negativeCache → staleFallback(memory or file, ≤1h) → nil → widget placeholder`. `rateLimit5h/7d`는 nil에서 `Unavailable: true`로 `--` placeholder를 그리고, `rateLimit7dSonnet`은 silent skip — SPEC §5.2 자동 만족.

2초 근거: 3초 천장 - (git/Keychain 약 0.7s + 위젯 처리 0.2s) ≈ 2.1s 여유. 1초는 cold-DNS에서 정상 응답 손실.

### 2.4 enum validation 경고 (SPEC §5.5, §5.6, §5.7)

```
loadConfig(path)
  ├── 기존 read+unmarshal+merge
  └── validateConfigEnums(&cfg)
        ├── displayMode ∈ {compact, custom}             아니면 stderr + → "compact"
        ├── separator   ∈ {pipe, dot, arrow, space, ""}  아니면 stderr + → ""
        ├── theme       ∈ themes 키집합 ∪ {""}            아니면 stderr + → ""
        └── language    ∈ {auto, en, ko}                  아니면 stderr + → "auto"
```

경고 형식: `cc-usage: invalid config: <field>=<value> not in {<allowed...>}, falling back to <default>`

emitter: `fmt.Fprintln(os.Stderr, ...)` 직접. `debugLog`는 DEBUG env 의존이라 SPEC §5.5 불만족.

§5.7: JSON unmarshal default가 unknown field silent ignore. `DisallowUnknownFields` 사용 금지. §5.6: 검증 통과 시 무경고 자동.

`displayMode`에 `custom` 포함 이유: `resolvePreset`이 preset 사용 시 `DisplayMode = "custom"`을 강제 세팅(widget.go:153).

### 2.5 Dead state 제거 (SPEC §5.9)

| 식별자 | 위치 | 제거 후 영향 |
|--------|------|---------------|
| `memCache` | `api.go:54,94,102,118,127,134` | `fetchUsageLimits` 1단 분기, `staleFallback` 메모리 분기 제거. one-shot에선 등가 |
| `negativeCache` | `api.go:57,88,112` | guard 자체 제거. 같은 invocation 두 번째 호출 없음 |
| `Config.DailyBudget` | `config.go:23` | 선언만. 사용자 JSON에 있어도 unmarshal 무시 |
| `Config.Plan` + default + main.go debugLog | `config.go:16,31,66-68`, `main.go:49` | 무참조. v0.2.0 cc-usage.json도 unmarshal이 무시 |
| `last_output` | 코드/JSON 잔존 없음 | grep 검증만 |
| `Translations.Errors.NoContext` | `widget.go:37`, en/ko JSON | struct + JSON 양쪽 제거 |
| `Translations.Labels.OneM` | `widget.go:28`, en/ko JSON | 동상 |
| `Translations.Labels.SevenDAll` | `widget.go:26`, en/ko JSON | 동상 |

### 2.6 linux/arm64 빌드 (SPEC §5.4)

```diff
- PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 windows/amd64
+ PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64
```

`bin/run.sh` 무수정. `uname -m → aarch64|arm64 → arm64` 정규화가 이미 있고, `BIN="$DIR/cc-usage-${OS}-${ARCH}"` 조립이 일반화. Makefile chmod·git update-index glob에 자동 포함.

### 2.7 회귀 방지 테스트 (SPEC §5.8)

`config_test.go`
- `TestLoadConfigMergesDefaults`
- `TestLoadConfigAcceptsV020Fields` — `{"dailyBudget":5.0,"plan":"max","language":"ko"}` 입력. stderr에 `dailyBudget`/`plan` 미포함
- `TestLoadConfigWarnsOnUnknownEnum` — `{"displayMode":"bogus"}`. stderr에 식별자 포함, cfg.DisplayMode == "compact"

`widget_test.go`
- `TestResolvePresetParsesChars` — `"P|M$C"` → `[["projectInfo"],["model","cost","context"]]`, `DisplayMode="custom"`
- `TestResolvePresetIgnoresUnknownChars` — `"Mz$"` → `[["model","cost"]]`
- `TestBurnRateGetDataNilOnMissingCost` — `(nil, nil)` 반환
- `TestSessionDurationGetDataNilOnMissingMs` — `(nil, nil)` 반환

SPEC §5.8 최소 3개 vs 실제 7개. stderr 캡처: `os.Pipe`로 `os.Stderr` 교체 후 복원하는 헬퍼를 `config_test.go` 내부에.

## 3. 인터페이스

### 추가
- `--version` (bool, default false). exit 0.
- 내부 헬퍼 `validateConfigEnums(*Config)`.

### 변경
- `apiTimeout` 10s → 2s. 느린 네트워크에서 첫 fetch 실패율 상승 가능, stale cache fallback이 보강.
- Keychain `security` 호출: 무한 대기 → 500ms timeout.

### 제거 (config 입력 호환은 유지)
- `Config.DailyBudget`, `Config.Plan` — JSON 입력에 남아도 silent ignore.
- `Translations` 미사용 라벨 3개 — locales JSON 파일도 함께 키 제거.
- `memCache`, `negativeCache` — 외부 노출 없음. `fetchUsageLimits` 시그니처 무변경.

### 변경 없음
- `cc-usage.json` 스키마, settings.json statusLine 경로, 캐시 파일 형식. v0.3.0 출시 후 GitHub default branch는 `release` 유지.

## 4. 영향 범위

| 변경 | 직접 수정 파일 | 간접 영향 | 검증 |
|------|---------------|----------|------|
| Keychain timeout (§5.1) | `credentials.go` | 없음 | 수동 (잠긴 Keychain) |
| API timeout (§5.2) | `api.go` 상수 1줄 | stale fallback 빈도 증가 | 수동 |
| `--version` (§5.3) | `main.go` | 없음 | 수동 |
| linux/arm64 (§5.4) | `Makefile` + 신규 바이너리 | `run.sh` 무수정 통과 확인 | 수동 cross-build 검사 |
| enum validation (§5.5–5.7) | `config.go` | main.go 무수정 | `config_test.go` |
| dead state 제거 (§5.9) | `api.go`, `config.go`, `widget.go`, locales JSON, `main.go` debugLog | 없음 | grep + `cache_test.go` 통과 |
| 회귀 테스트 (§5.8) | `config_test.go`, `widget_test.go` 신규 | 없음 | `go test ./...` |
| 빌드/배포 (§5.10) | `Makefile VERSION` | `bin/` 5개 git commit, release 브랜치 sync | 수동 (release procedure는 CLAUDE.md) |

새 위젯·새 캐시 키·새 stdin 필드·새 외부 의존 도입 없음. SPEC §4 제외 범위 보호.

## 5. Decision Points

### 5.1 Keychain timeout 값 — 500ms 채택
옵션: 200ms / 500ms / 1s. 정상 lookup 10–50ms, SPEC §5.1 1초 안. 200ms는 I/O 부하 시 false-trigger 위험, 1s는 과다.

### 5.2 API timeout 값 — 2s 채택
옵션: 1s / 2s / 3s. 3초 천장 - 다른 동기 작업(git/Keychain) ≈ 2.1s 여유. 1s는 cold-DNS 위험, 3s는 천장 초과 가능.

### 5.3 enum validation emitter 위치 — `loadConfig` 내 헬퍼 인라인
A) `loadConfig` 내 헬퍼 / B) 별도 파일 / C) `main.go`에서 호출. 채택 A. SPEC §4 "Config validation을 별도 서브커맨드로 분리하는 작업"이 제외 범위에 있어 inline이 일관. 의존성 단방향, 코드 ≈60 라인.

### 5.4 stale `last_output` 캐시 파일 처리 — 별도 작업 없음
식별자 이미 코드/JSON 무참조. `cleanOldCaches`가 1h mtime 초과 prune. 신규 prune 도입 실효 없음. SPEC §5.9는 verify-only.

### 5.5 `--version` 처리 순서 — `flag.Parse()` 직후 early-exit
A) flag.Parse 직후 / B) loadConfig 이후 / C) credential 시도 이후. 채택 A. 외부 의존(Keychain prompt) 노출 전. SPEC §5.3 "다른 출력은 없다" 만족.
