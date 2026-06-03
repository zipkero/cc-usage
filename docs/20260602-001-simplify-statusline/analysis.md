# analysis: simplify-statusline

### 근거

읽은 spec 범위: `spec.md` §1–§5 전체. 범위는 데이터 출처·표시 항목 두 축의 단순화이며
위젯 아키텍처·config/preset/i18n·렌더 비주얼·git 취득 방식은 유지 대상이다(§1, §4).

코드베이스에서 확인한 사실:

- 진입점 `main.go`는 현재 `config 로드 → stdin 파싱 → credential 획득(getCredential) →
  API rate limit 조회(fetchUsageLimits) → 세션캐시 cleanup goroutine → 세션캐시 로드 →
  orchestrate → Layer 1 세션캐시 복원 → Layer 2 transcript 복원 → 무출력/warmup 판정 →
  출력 → [1m] 신호 저장 → 세션캐시 저장` 순으로 동작한다. spec이 말하는 "복잡한 백엔드"는
  이 흐름의 credential·API·캐시·복원 단계 전체에 해당한다.
- spec.md §1이 열거한 삭제 대상(credential·OAuth API·3-tier 캐시·degraded 복원·transcript
  cost 추정)보다 코드의 결합 표면이 넓다. 실제로 다음 파일이 그 백엔드를 구성한다:
  `credentials.go`(OAuth 토큰), `api.go`(`/api/oauth/usage` + 파일캐시 + 403 curl fallback),
  `cache.go`(세션상태 캐시 + degraded 복원 로직), `file_lock_{unix,windows}.go`(캐시 파일
  advisory lock), `transcript.go`(jsonl tail read로 model/context/usage 복원 = Layer 2),
  `pricing.go`(transcript usage → cost 추정 단가표), `last_model_cache.go`(`[1m]` 신호의
  cwd별 보존). spec §1이 명시 열거하지 않은 `transcript.go`·`pricing.go`·`last_model_cache.go`도
  "stdin·git만으로 동작"(SPEC §5.1)과 "transcript 파일을 읽어 비용 추정 금지"(SPEC §5.5)에
  직접 위배되므로 같은 경계로 들어온다.
- 표시 항목: `widgets_core.go`의 `rateLimit5h`/`rateLimit7d`는 stdin 우선 + API fallback의
  2-tier로 데이터를 얻고, `rateLimit7dSonnet`은 API 전용이다. `widgets_analytics.go`는
  version·apiDuration·sessionDuration·burnRate·cacheHit·performance 6개 위젯 전부를 보유한다.
- `widget.go`의 `Context` 구조체는 `RateLimits *UsageLimits`(API 산출물)와 `CostEstimated bool`,
  `ConfigDir string`을 보유한다. `RateLimits`는 `api.go`의 타입, `CostEstimated`는 transcript
  cost 추정 경로(`main.go`에서만 `true` 세팅), `ConfigDir`은 `credentials.go`의 파일 경로에서만
  소비된다 — 셋 다 삭제 대상 경로 전용이다.
- `cache.go`는 전부 죽지 않는다. `detectCurrentCwd`/`detectCwdEnv`/`detectCwdGetwd`/
  `normalizeCwd`는 존속하는 `widgets_project.go`(`GetData`가 빈 `workspace.current_dir`일 때
  `detectCurrentCwd()`로 fallback, SPEC §5.7)와 그 테스트(`widgets_project_test.go`가
  `detectCwdEnv`/`detectCwdGetwd` 주입에 의존)에서 쓰인다. `cwdWithinRoot`·세션캐시 관련 함수는
  복원 경로 전용이라 죽는다.
- `presetCharToWidget`은 삭제 대상 위젯 char를 보유한다: `'S'`(rateLimit7dSonnet),
  `'V'/'a'/'D'/'B'/'H'/'F'`(analytics 6개). `displayPresets["compact"]`는 마지막 항목으로
  `"rateLimit7dSonnet"`을 포함한다.
- i18n: `locales/{en,ko}.json`의 `labels.sevenDSonnet`(`7d-S`/`7일-S`)와 `widgets`
  블록(apiDuration·burnRate·cache·performance·session)이 삭제 대상 위젯 전용 문자열이다.
  `Translations` 구조체(`widget.go`)의 대응 필드도 같이 죽는다.
- 부수 dead code: `format.go`의 `formatDuration`은 analytics 두 위젯에서만 호출되며 제거 후
  미사용이 된다. `truncate`·`osc8Link`는 현재도 비-테스트 호출자가 없다(주석에만 등장).
- 무출력 조건: `shouldSuppressOutput`(model·dir·context 전부 빈 경우 출력 생략)은 SPEC §5.6
  유지 대상이다. 다만 현재 구현은 `RateLimits`(API 산출물)가 있으면 출력하는 warmup 예외
  (`isWarmupExceptionPath`/`renderRateLimitOnly`)와 묶여 있다. API 제거 시 `RateLimits`는 항상
  nil이 되므로 warmup 예외 분기는 의미를 잃는다.
- 테스트 결합: 다음 테스트 파일은 삭제 대상 코드에 통째로 묶여 있다 — `api_test.go`,
  `cache_test.go`, `pricing_test.go`, `transcript_test.go`, `last_model_cache_test.go`,
  `e2e_task014_test.go`(transcript/cost-estimate E2E). `main_test.go`는 복원/fallback/Layer2/
  `[1m]` 테스트가 대부분이며(`TestConfigHomeDir` 등 일부만 존속 가능), `widget_test.go`의
  burnRate·sessionDuration 테스트, `widgets_core_test.go`의 `CostEstimated` 테스트도 삭제 대상에
  묶인다. `widgets_project_test.go`·`config_test.go`(Cache.TTLSeconds 기본값 검증 포함)는 존속.
- 빌드 제약: zero dependency, 단일 `main` 패키지, stdout=위젯+ANSI / stderr=debug (SPEC §3).
  `go.mod`에 `require` 없음을 확인.

추정(코드로 확정 못 한 부분): Claude Code가 stdin `rate_limits.five_hour`/`seven_day`를 어떤
구독·세션 단계에서 채워 보내는지는 외부 동작이라 확인 불가. spec은 "없으면 미표시"를 공식 문서와
동일한 수용 결과로 규정한다(SPEC §4, §5.2).

### 1. 구조

이 변경은 새 모듈을 만들지 않고 **레이어 하나를 통째로 들어내는** 작업이다. 현재 cc-usage는
두 데이터 레인을 가진다 — (a) stdin+git에서 직접 파생하는 렌더 레인, (b) credential·OAuth API·
파일캐시·세션캐시·transcript로 stdin을 보강하는 백엔드 레인. 단순화는 (b) 전체를 제거하고
(a)만 남기는 것이다.

존속 경계:

- **오케스트레이션**: `Widget` 인터페이스 + `registry` + `orchestrate()` + `displayPresets`
  멀티라인 레이아웃 + preset/i18n 해석. 계약(`ID`/`GetData`/`Render`, nil·빈 문자열 skip,
  패닉 금지)은 그대로다(SPEC §3, §4).
- **존속 위젯**: model·context·cost(`widgets_core.go`), 5h/7d rate limit(stdin 전용으로 축소),
  projectInfo(`widgets_project.go`, git porcelain=v2 그대로 — SPEC §4).
- **렌더·포맷·설정**: `render.go`(theme·progress bar·separator), `format.go`(미사용분 정리 후),
  `config.go`, `locales/*.json`(삭제 위젯 문자열 정리 후).

제거 경계(파일 단위 소멸): `credentials.go`, `api.go`, `pricing.go`, `transcript.go`,
`last_model_cache.go`, `widgets_analytics.go`, `file_lock_{unix,windows}.go`. `file_lock`은
캐시 파일 동시성 보호 전용이며, 캐시 쓰기/읽기가 모두 사라지면 호출자가 없어진다(SPEC §5.1).

부분 제거 경계(파일은 존속, 내용 축소):

- `cache.go`: 세션캐시·복원 로직은 소멸하지만 `projectInfo`가 의존하는 cwd 탐지·정규화 helper
  (`detectCurrentCwd`/`detectCwdEnv`/`detectCwdGetwd`/`normalizeCwd`)는 존속해야 한다. 이
  helper 묶음을 어느 파일에 둘지는 §5에서 commit한다.
- `widget.go`: `Context`에서 죽는 필드(`RateLimits`/`CostEstimated`/`ConfigDir`), 죽는
  preset char, `displayPresets`의 `rateLimit7dSonnet` 항목, `Translations`의 죽는 필드를 정리.
- `widgets_core.go`: 5h/7d의 API fallback 분기 제거(stdin 전용), `rateLimit7dSonnet` 위젯과
  그 등록 제거, cost 위젯의 `CostEstimated`/`~` 마커 분기 제거.
- `main.go`: 진입점이 `config 로드 → stdin 파싱 → orchestrate → (무출력 판정) → 출력`으로
  축소된다. credential·API·cleanup goroutine·세션캐시 로드/저장·Layer 1/2 복원·warmup 예외·
  `[1m]` 저장 블록이 모두 사라진다.

### 2. 데이터 흐름

축소 후 단일 경로:

1. `parseStdin()`이 Claude Code JSON을 `StdinInput`으로 디코드(파싱 실패 시 빈 입력).
2. `loadConfig()` + `loadTranslations()`로 표시 설정·i18n 확정.
3. `Context{Stdin, Config, Translations}` 구성. (RateLimits·CostEstimated·ConfigDir 필드
   소멸.)
4. **무출력 판정**: model(id/display) · workspace.current_dir · context_window_size가 모두
   비면 출력 생략(SPEC §5.6). 이 판정은 더 이상 API rate limit 유무에 좌우되지 않으므로
   warmup 예외 없이 순수 stdin-identity 검사로 단순화된다.
5. `orchestrate(ctx)`가 라인별로 위젯 `GetData`/`Render`를 돌려 separator로 조인, 멀티라인
   설정이면 라인마다 별도 행(SPEC §5.7).
6. 결과를 stdout으로 출력(stderr는 debugLog 전용 유지 — SPEC §3).

위젯별 데이터 출처는 전부 stdin(+projectInfo의 git 호출)로 단일화된다:

- 5h/7d rate limit: `stdin.rate_limits.five_hour`/`seven_day`만 사용. 필드 부재 시 위젯 미표시
  (SPEC §5.2). 기존 "stdin 없으면 `--` placeholder" 동작을 유지할지 "필드 없으면 위젯 skip"으로
  바꿀지는 §5에서 결정한다 — SPEC §5.2가 "출력되지 않는다"를 요구하므로 placeholder 동작과
  충돌 여지가 있다.
- cost: `stdin.cost.total_cost_usd`만 사용. 추정 마커 `~`는 어떤 입력에도 안 나온다(SPEC §5.5).
- projectInfo: `stdin.workspace.current_dir`, 비면 `detectCurrentCwd()` fallback → git
  porcelain=v2(SPEC §5.7, §4).

상태·외부 I/O: 제거 후 cc-usage는 status line 렌더 중 stdin·git 외 파일 read/write나 네트워크
접속을 하지 않는다(SPEC §5.1). 캐시 파일도, 세션 상태 파일도, credential 파일도 읽지 않는다.
프로세스 내 메모리 상태(credential/keychain 캐시 등)도 함께 사라진다.

### 3. 인터페이스

- **stdin 입력 스키마**(`StdinInput`): 외부(Claude Code)와의 계약이라 형태를 바꾸지 않는다.
  `rate_limits.five_hour`/`seven_day`·`cost.total_cost_usd`·`context_window.*`·`workspace.*`·
  `model.*`를 그대로 소비한다. 삭제는 cc-usage 내부 소비 측에서만 일어나고, 스키마 필드 자체는
  Claude Code가 보내는 형태를 유지한다. (`transcript_path` 등 복원 전용으로만 읽던 필드는
  소비자가 사라지므로 더는 참조되지 않는다.)
- **`Widget` 인터페이스**: `ID()`/`GetData(*Context) (any, error)`/`Render(any, *Context) string`
  계약 불변(SPEC §3). `Context`에서 제거되는 필드(RateLimits·CostEstimated·ConfigDir)는 존속
  위젯이 참조하지 않으므로 위젯 계약에 영향이 없다.
- **config 스키마**(`cc-usage.json`): preset/lines/disabledWidgets/theme/separator/language는
  유지. `cache.ttlSeconds`는 API 캐시 TTL 전용 설정이었으므로 소비자가 사라진다 — 이 설정 키를
  스키마에서 제거할지(하위 호환 영향), 무시되는 키로 남길지는 §5에서 결정.
- **preset 표기**: 제거되는 char(`S`/`V`/`a`/`D`/`B`/`H`/`F`)는 더 이상 위젯에 매핑되지 않는다.
  사용자 preset에 이 문자가 있으면 `resolvePreset`이 이미 "unknown preset char" 경로로
  무시하므로(존속 동작), 기존 설정이 깨지지는 않고 해당 위젯만 사라진다.

### 4. 영향 범위

건드리는 기존 파일:

- 소멸: `credentials.go`, `api.go`, `pricing.go`, `transcript.go`, `last_model_cache.go`,
  `widgets_analytics.go`, `file_lock_unix.go`, `file_lock_windows.go`.
- 축소: `main.go`(진입점 흐름), `widget.go`(Context·preset·displayPresets·Translations 정리),
  `widgets_core.go`(5h/7d stdin 전용화 + 7dSonnet·cost 마커 제거), `cache.go`(cwd helper만
  존속), `format.go`(`formatDuration` 등 미사용 함수 정리), `config.go`(cache 설정 처리 — §5),
  `locales/en.json`·`locales/ko.json`(죽는 라벨/위젯 문자열 제거).
- 테스트 소멸/축소: `api_test.go`·`cache_test.go`·`pricing_test.go`·`transcript_test.go`·
  `last_model_cache_test.go`·`e2e_task014_test.go`는 삭제. `main_test.go`는 복원/fallback/
  Layer2/`[1m]` 테스트를 제거하고 존속 가능한 케이스(예: config 경로)만 남긴다. `widget_test.go`
  (burnRate·sessionDuration)와 `widgets_core_test.go`(CostEstimated 마커)는 제거 위젯·필드에
  묶인 케이스를 정리한다. `widgets_project_test.go`는 `detectCwd*` helper 존속을 전제로
  유지된다. 처리 방침은 §5에서 commit한다.

직접·간접 의존(탐색으로 확인):

- `detectCurrentCwd`/`normalizeCwd`/`detectCwd*` → `widgets_project.go` + `widgets_project_test.go`
  가 의존 → 존속 필요. (`detectCurrentCwd`는 `detectCurrentCwdWithSource`를 호출하므로 그
  의존 helper까지 같이 존속해야 함을 implement에서 확인한다.)
- `withCacheFileLock`/`atomicWriteFile`/`cwdWithinRoot`/세션캐시·API캐시 함수 → 삭제 대상
  내부에서만 상호 호출 → 함께 소멸.
- `clampPercent`/`calculatePercent`(format.go) → 존속 위젯에서도 사용 → 존속.
- `UsageLimits`/`UsageLimitEntry`(api.go 타입) → `Context.RateLimits`와 5h/7d/7dSonnet API
  fallback에서만 사용 → 소멸.

문서 영향: SPEC §5.8은 `README.md`와 `CLAUDE.md` 모두에서 credential·OAuth API·3-tier 캐시·
degraded 복원·세션 캐시·`7d-S`·analytics·transcript cost 추정 서술을 제거하도록 요구한다.
`README.md`의 Configuration(`.credentials.json` 경로 안내)·Widgets(7dSonnet·analytics 행)·
Troubleshooting(idle 시 세션캐시 복원/TTL 설명)·Privacy(네트워크/저장 항목)가 직접 대상이다.
`CLAUDE.md`는 아키텍처 표·degraded 복원·무출력 조건·경로 표·배포(userAgent) 서술이 대상이다.

하위 호환·마이그레이션: 기존 사용자의 `cc-usage.json`에 `cache.ttlSeconds`나 삭제 위젯 preset
문자가 있을 수 있다. preset 문자는 unknown char로 무시되어 깨지지 않는다(확인됨). `cache` 설정
처리는 §5에서 결정. 디스크에 남은 기존 캐시 파일(`~/.cache/cc-usage/*`)은 더 이상 읽거나 쓰지
않으므로 무해하게 잔존한다 — cc-usage가 능동 삭제하던 cleanup도 함께 사라지므로 사용자가 직접
지울 수 있다(이 동작 변화는 README Privacy/Troubleshooting 갱신에 반영).

### 5. Decision Points

**D1. cwd 탐지 helper의 존속 위치.**
`detectCurrentCwd`/`detectCwdEnv`/`detectCwdGetwd`/`normalizeCwd`는 현재 `cache.go`에 있으나
`cache.go`의 나머지(세션캐시·복원·lock 연동)는 소멸한다. 옵션: (a) `cache.go`를 cwd helper만
남기고 존속, (b) helper를 `widgets_project.go`로 이동(유일 소비자), (c) 별도 작은 파일로 분리.
채택: **(b)** — 유일 소비자가 projectInfo이고 단일 `main` 패키지·파일 분리 원칙(SPEC §3)에
맞다. `cache.go`는 파일째 소멸시켜 "세션캐시" 흔적을 남기지 않는다. 트레이드오프: helper가
위젯 파일에 섞이지만, 소비자가 하나뿐이라 응집도가 오히려 높다. `widgets_project_test.go`의
`detectCwd*` 주입 테스트는 그대로 유효하다.

**D2. 5h/7d 데이터 부재 시 표시 동작.**
SPEC §5.2는 "stdin에 해당 필드가 없으면 그 항목이 출력되지 않는다"를 요구한다. 현재 구현은
데이터 없으면 `--` placeholder(`rateLimitData.Unavailable`)를 그린다. 옵션: (a) placeholder
유지, (b) 필드 부재 시 `GetData`가 nil 반환 → orchestrator가 위젯 skip. 채택: **(b)** —
SPEC §5.2 문구("출력되지 않는다")와 공식 문서 동작에 맞추려면 placeholder가 아니라 미표시여야
한다. 트레이드오프: 세션 초반·무료 구간에 5h/7d가 잠시 사라지는 동작은 SPEC §4가 명시 수용한다.
`rateLimitData.Unavailable` 경로와 그 렌더 분기는 정리 대상이 된다.

**D3. cost 위젯의 추정 분기 제거 범위.**
SPEC §5.5는 `~` 마커가 어떤 입력에도 안 나오게 한다. `CostEstimated`는 transcript 경로에서만
`true`가 되며 그 경로가 소멸하므로 항상 `false`다. 옵션: (a) `Context.CostEstimated` 필드와 cost
위젯의 마커 분기를 모두 제거, (b) 필드는 두고 분기만 죽은 채로 둠. 채택: **(a)** — dead 필드를
남기지 않는다. `estimatedCostMarker` 상수와 cost 위젯의 `~` 분기, `widgets_core_test.go`의
마커 테스트가 함께 정리된다. cost는 `stdin.cost.total_cost_usd`만으로 렌더한다.

**D4. `Context`에서 제거하는 필드 경계.**
`RateLimits`(API 산출 타입)·`CostEstimated`(D3)·`ConfigDir`(credential 경로 전용) 세 필드를
모두 제거한다. 근거: 셋 다 삭제 경로 전용 소비자만 있었고 존속 위젯은 참조하지 않는다(탐색
확인). `Context`는 `Stdin`/`Config`/`Translations`만 보유하게 된다. 이 경계를 commit해
implementer가 "혹시 남길 필드"를 판단하지 않게 한다.

**D5. 무출력 조건과 warmup 예외 처리.**
SPEC §5.6은 model·dir·context 전부 빈 경우 출력 생략을 유지하라고 한다. 현재 그 판정은 API
rate limit 데이터가 있으면 출력하는 warmup 예외(`isWarmupExceptionPath`/`renderRateLimitOnly`)와
얽혀 있다. API가 사라지면 rate limit 데이터는 항상 stdin에서만 오므로 warmup 예외는 의미를
잃는다. 채택: **무출력 판정을 순수 stdin-identity 검사로 축소하고 warmup 예외 분기
(`isWarmupExceptionPath`/`renderRateLimitOnly`)를 제거**한다. 트레이드오프: 빈 stdin에서
계정 전역 5h/7d를 띄우던 동작이 사라지지만, 그 데이터 출처(API)가 제거 대상이므로 동작 자체가
성립하지 않는다. SPEC §5.6의 "model·dir·context 모두 빈 경우 생략"은 그대로 보존된다.

**D6. analytics·7dSonnet의 표면 정리 범위.**
제거 대상 위젯의 흔적을 어디까지 지울지 commit: (a) 위젯 구현·등록(`widgets_analytics.go` 파일
소멸, `widgets_core.go`의 7dSonnet 위젯·등록 제거), (b) `presetCharToWidget`의 `S`/`V`/`a`/`D`/
`B`/`H`/`F` 매핑 제거, (c) `displayPresets["compact"]`에서 `rateLimit7dSonnet` 제거, (d)
`locales/*.json`의 `labels.sevenDSonnet`·`widgets` 블록과 `Translations` 대응 필드 제거, (e)
`format.go`의 미사용이 되는 `formatDuration` 정리. 채택: **(a)–(e) 모두 정리** — SPEC §5.3·§5.4가
"어떤 입력·설정에도 출력되지 않는다"를 요구하므로 매핑·preset·i18n까지 지워야 char를 다시
넣어도 살아나지 않는다. 이미 비-테스트 호출자가 없는 `truncate`/`osc8Link`까지 손댈지는 범위
밖(SPEC §1의 단순화는 데이터·표시 항목 한정)이라 건드리지 않는다.

**D7. 삭제 대상에 묶인 테스트 처리 방침.**
옵션: (a) 삭제 코드와 함께 테스트 파일·케이스도 제거, (b) 테스트를 남기고 skip 처리. 채택:
**(a)** — 대상 코드가 사라지면 컴파일 자체가 불가하므로 skip이 성립하지 않는다. 소멸 파일에
1:1 대응하는 테스트(`api_test.go`·`cache_test.go`·`pricing_test.go`·`transcript_test.go`·
`last_model_cache_test.go`·`e2e_task014_test.go`)는 파일째 제거하고, 혼재 테스트(`main_test.go`·
`widget_test.go`·`widgets_core_test.go`)는 삭제 대상에 묶인 케이스만 들어내고 존속 케이스는
남긴다. 정리 후 `go build ./...`·`go vet ./...`·`make test`가 통과해야 한다(검증은 implement.md
소관).

**D8. `cache.ttlSeconds` 설정 키 처리.**
API 캐시 TTL 전용 설정의 소비자가 사라진다. 옵션: (a) `Config`에서 `Cache` 필드와 기본값 머지·
검증을 제거, (b) 키를 무시되는 호환 필드로 남김. 채택: **(a)** — SPEC §5.8이 캐시 관련 서술을
문서에서 지우라고 요구하고, dead 설정 키를 남기면 단순화 목표(SPEC §2)에 역행한다. 기존 사용자
설정에 `cache` 블록이 있어도 `Config`가 모르는 JSON 키는 디코드 시 무시되므로 파싱이 깨지지
않는다(확인된 동작). `config_test.go`의 `Cache.TTLSeconds` 기본값 검증 케이스는 이 결정에 따라
정리된다.
