# cc-usage — Implementation Strategy

PLAN.md의 Exit Criteria를 만족시키기 위한 실행 전략. 진행 상태는 이 문서 기준.

> **규칙:** 구현 완료 → IMPLEMENT 체크. 검증 완료 → PLAN 체크. 둘은 다른 사건.

---

## 아키텍처

```
stdin (JSON) ──→ main.go ──→ stdout (ANSI text)
                   │
                   ├─ stdin.go      : JSON → StdinInput 구조체
                   ├─ config.go     : 파일 → Config 구조체 (기본값 병합, preset 파싱)
                   ├─ widget.go     : 오케스트레이터 (레지스트리 → GetData → Render → 조인)
                   ├─ render.go     : 테마, 세퍼레이터, 프로그레스바, ANSI 코드
                   ├─ format.go     : 토큰/비용/시간/퍼센트 포매팅
                   ├─ credentials.go: OAuth 토큰 추출 (파일 / macOS Keychain)
                   ├─ api.go        : 3-tier 캐시 API 클라이언트
                   ├─ widgets_core.go    : model, context, cost, rateLimit*
                   ├─ widgets_project.go : projectInfo, sessionDuration
                   ├─ widgets_analytics.go: burnRate, cacheHit, tokenSpeed, ...
                   └─ locales/{en,ko}.json: i18n (go:embed)
```

**경계:**
- stdin/stdout은 Claude Code plugin 계약. stdin=JSON, stdout=ANSI text, stderr=debug.
- Config/credentials 경로는 `--config` CLI 인자로 결정되는 configDir 기준.
- 캐시는 `~/.cache/cc-usage/` (전역, configDir 무관).
- 외부 의존성 0 (Go 표준 라이브러리만).

## 실행 흐름

```
main():
  1. flag.Parse() → --config 경로 결정
  2. loadConfig(configPath) → Config (없으면 기본값)
  3. json.Decoder(stdin) → StdinInput
  4. loadTranslations(config.language) → Translations
  5. getCredential(configDir) → token (nullable)
  6. fetchUsageLimits(token, config.cache) → UsageLimits (nullable, 3-tier 캐시)
  7. Context{Stdin, Config, Translations, RateLimits} 조립
  8. orchestrate(ctx) → []string (라인별 렌더링 결과)
  9. fmt.Print(join lines with \n)

orchestrate(ctx):
  for each line in displayPreset[config.displayMode]:
    widgets = filter(line, not in disabledWidgets)
    parts = []
    for each widgetID in widgets:
      widget = registry[widgetID]
      data, err = widget.GetData(ctx)
      if err != nil || data == nil → skip
      rendered = widget.Render(data, ctx)
      if rendered != "" → parts = append(parts, rendered)
    if len(parts) > 0 → output line = join(parts, separator)
  remove empty lines
```

## 상태 모델

| 상태 | 저장 위치 | 수명 | 변화 규칙 |
|------|-----------|------|-----------|
| 메모리 캐시 (API) | 프로세스 글로벌 변수 | 프로세스 | TTL 기반 무효화 |
| 파일 캐시 (API) | `~/.cache/cc-usage/cache-{hash}.json` | 프로세스 간 | TTL(정상=config, 에러=30s, stale=1h) |
| 세션 시작 시간 | `~/.cache/cc-usage/sessions/{id}.json` | 7일 | atomic 생성(O_EXCL), 이후 읽기 전용 |
| 일일 예산 | `~/.cache/cc-usage/budget.json` | 1일 | delta 추적, 날짜 변경 시 리셋 |
| Transcript 파싱 | 프로세스 글로벌 변수 | 프로세스 | offset 기반 incremental, truncation → re-parse |
| Credential 캐시 | 프로세스 글로벌 변수 | 프로세스 | 파일: mtime 기반, Keychain: TTL 10s |

---

## Phase 1: MVP

### I1.1 — stdin 파싱 + config 로드 + 디버그 인프라

선행 조건: 없음 (첫 구현 단위).

- [x] 구현 완료

**목적:** CLI 실행 시 JSON 입력을 구조체로 변환하고, config를 로드하여 이후 모든 위젯의 입력을 준비한다. → PLAN: Phase 1 > T1.1, T1.2

**책임:**
- 입력: stdin JSON 바이트 스트림, `--config <path>` CLI 인자, `DEBUG` 환경변수
- 출력: `StdinInput` 구조체, `Config` 구조체, configDir 문자열
- 경계: stdin 파싱 실패 → stderr 에러 + exit 0 (패닉 금지). config 파일 부재 → 기본값.

**설계:**

구조:
- `main.go`: 엔트리포인트. `flag` 패키지로 `--config` 파싱 → `loadConfig` → `parseStdin` → 위젯 실행 (I1.2 이후).
- `stdin.go`: `StdinInput` 구조체. DESIGN.md의 전체 필드 매핑. 선택 필드는 포인터(`*int`, `*float64`, `*string`) 또는 omitempty. `json.NewDecoder(os.Stdin).Decode(&input)`.
- `config.go`: `Config` 구조체. `loadConfig(path)` → 파일 읽기 시도 → `json.Unmarshal` → 기본값 병합(`language:"auto"`, `plan:"max"`, `displayMode:"compact"`, `cache.ttlSeconds:300`). Preset 문자열은 raw 그대로 `Config.Preset`에 저장 (해석은 I1.2에서).
- `main.go` 상단: `debugLog(context, msg, args...)` — `DEBUG=cc-usage` 또는 `DEBUG=1`이면 `fmt.Fprintf(os.Stderr, "[cc-usage:%s] %s\n", ...)`.

실행 흐름:
1. `flag.String("config", defaultConfigPath(), "config path")` → `flag.Parse()`
2. `configDir = filepath.Dir(configPath)`
3. `config = loadConfig(configPath)` — 파일 없으면 로그 + 기본값 반환
4. `input = parseStdin()` — 실패 시 빈 StdinInput 반환 (패닉 금지)
5. debugLog로 파싱 결과 출력

configDir 추론:
- `--config` 지정 → `filepath.Dir(configPath)`
- 미지정 → `~/.claude/` (기본값)
- `~` 확장: `os.UserHomeDir()` 사용

**선택 이유:** `json.Decoder`는 스트림 파싱으로 stdin에 적합. `flag` 패키지는 zero-dependency 원칙에 부합. 포인터 optional 필드는 Go 관용적 패턴이고 nil 체크로 존재 여부 판별 가능.

**실패/예외:**
- stdin이 빈 문자열/EOF → `Decode` 에러 → 빈 StdinInput 반환, stderr 로그
- stdin이 유효하지 않은 JSON → 동일 처리
- config 파일 권한 문제 → 기본값 폴백, stderr 로그
- `--config` 경로의 `~` 확장 실패 → `os.UserHomeDir` 에러 시 raw 경로 사용

---

### I1.2a — 위젯 프레임워크

선행 조건: I1.1 (StdinInput, Config 구조체 존재).

- [x] 구현 완료

**목적:** Widget 인터페이스, 레지스트리, 오케스트레이터를 구현하여 위젯을 등록하고 실행할 수 있는 프레임워크를 완성한다. → PLAN: Phase 1 > T1.3

**책임:**
- 입력: `Context` (Stdin+Config+Translations+RateLimits), display mode, 위젯 레지스트리
- 출력: 라인별 위젯 렌더링 결과 문자열 배열
- 경계: 위젯 GetData nil/error → skip. 빈 라인 → 자동 제거. 모든 위젯 skip → 빈 출력. disabledWidgets 필터 적용.

**설계:**

구조 (`widget.go`):
- `Widget` 인터페이스: `ID() string`, `GetData(*Context) (any, error)`, `Render(any, *Context) string`
- `Context` 구조체: `{Stdin, Config, Translations, RateLimits}`
- 글로벌 `registry map[string]Widget` + `registerWidget(w Widget)` 함수
- `DisplayPresets map[string][][]string` — compact/normal/detailed 3개 프리셋
- `presetCharToWidget map[byte]string` — Preset 문자→위젯ID 매핑 테이블 (DESIGN.md 전체 매핑)
- `resolvePreset(config *Config)` — Config.Preset이 비어있지 않으면 `|`로 split → 각 문자를 위젯 ID로 매핑 → Config.Lines에 저장 + Config.DisplayMode를 `"custom"`으로 오버라이드
- `orchestrate(ctx *Context) []string` — resolvePreset → preset/custom → 위젯 ID 배열 → disabledWidgets 필터 → GetData → Render → 세퍼레이터 조인 → 빈 라인 제거

실행 흐름: `main.go`에서 `orchestrate(ctx)` 호출 → 결과 라인들을 `\n`으로 조인 → `fmt.Print`

**선택 이유:** 글로벌 레지스트리 + init() 등록은 Go에서 플러그인 패턴의 관용적 방식. 각 위젯 파일의 `init()`에서 `registerWidget` 호출. Preset 해석을 widget.go에 둔 이유는 매핑 테이블과 DisplayPresets가 같은 파일에 있어야 일관성 유지.

**실패/예외:**
- 레지스트리에 없는 위젯 ID → skip + debugLog
- 모든 위젯이 nil → 빈 stdout (정상)
- 알 수 없는 preset 문자 → skip + debugLog

---

### I1.2b — ANSI 렌더링 + 포매팅

선행 조건: I1.2a (Widget 인터페이스, Context 구조체).

- [x] 구현 완료

**목적:** 테마, 세퍼레이터, 프로그레스바, 포매팅 유틸리티를 구현하여 위젯이 ANSI 컬러 출력을 생성할 수 있게 한다. → PLAN: Phase 1 > T1.4, T1.5

**책임:**
- 입력: 테마명, 세퍼레이터 스타일, 퍼센트/토큰/비용/시간 값
- 출력: ANSI 이스케이프 코드가 포함된 문자열
- 경계: 알 수 없는 테마/세퍼레이터 → 기본값 폴백.

**설계:**

구조:

`render.go`:
- `ThemeColors` 구조체 (DESIGN.md 그대로)
- `themes map[string]ThemeColors` — 8개 테마 정의
- `getTheme(name string) ThemeColors` — 미존재 → default 폴백
- `getColorForPercent(percent int, theme ThemeColors) string` — ≤50 Safe, ≤80 Warning, >80 Danger
- `renderSeparator(style string, theme ThemeColors) string` — pipe/space/dot/arrow
- `renderProgressBar(percent int, theme ThemeColors) string` — Width=10, █/░
- `RESET = "\x1b[0m"`

`format.go`:
- `formatTokens(tokens int) string` — <1000: 그대로, <1M: "X.YK", ≥1M: "X.YM"
- `formatCost(cost float64) string` — `$X.XX`
- `formatTimeRemaining(resetAt time.Time, t Translations) string` — days+hours 또는 hours+minutes
- `formatDuration(ms int64, t Translations) string` — 같은 패턴
- `shortenModelName(displayName string) string` — known 패턴 매칭
- `calculatePercent(current, total int) int` — 0-100 클램프
- `truncate(s string, maxLen int) string` — rune 기준, 초과 시 "…" 추가
- `clampPercent(value float64) int` — 0-100 범위
- `osc8Link(url, text string) string` — OSC8 하이퍼링크

**선택 이유:** 테마를 구조체 맵으로 구현하면 런타임 분기 없이 한 번 조회로 끝남. 포매팅 함수를 별도 파일(format.go)로 분리하여 `go test`로 독립 검증 가능.

**실패/예외:**
- 알 수 없는 테마명 → default 폴백
- 알 수 없는 세퍼레이터 → pipe 폴백
- formatTokens(음수) → "0" 반환
- clampPercent(범위 외) → 0 또는 100

---

### I1.3 — 코어 위젯 (model, context, cost)

선행 조건: I1.2a + I1.2b (Widget 인터페이스, 렌더링 인프라, 포매팅 함수 존재).

- [x] 구현 완료

**목적:** compact 모드의 첫 줄을 구성하는 3개 위젯. → PLAN: Phase 1 > T1.6

**책임:**
- 입력: `StdinInput`의 model, context_window, cost 필드
- 출력: `◆ Opus │ ████░░░░ 30% 60K │ $1.25` 형태의 렌더링 문자열
- 경계: 각 위젯은 독립적. 필수 데이터 없으면 nil 반환.

**설계:**

구조 (`widgets_core.go`):

`model` 위젯:
- GetData: `stdin.Model` nil 체크 → `{ID, DisplayName, Effort, Fast}` 반환
- Render: 이모지 선택 (opus→◆, sonnet→◇, haiku→○, 기타→●) + `shortenModelName(displayName)` + effort 접미사(있으면) + fast 접미사(있으면)
- 이모지 매핑: `strings.Contains(strings.ToLower(id), "opus")` 등

`context` 위젯:
- GetData: `stdin.ContextWindow` nil 체크 → percent 계산
  - `used_percentage` 있으면 사용 (Decision Point 1.B 반영)
  - 없으면 `(total_input + total_output) / context_window_size * 100` 폴백
  - `context_window_size`도 없으면 nil (숨김)
- Render: `renderProgressBar(percent)` + ` ` + `percent%` + ` ` + `formatTokens(total)`
  - total = total_input + total_output

`cost` 위젯:
- GetData: `stdin.Cost` nil 체크 → `total_cost_usd` 반환
- Render: `formatCost(cost)`

실행 흐름: 각 위젯은 `init()`에서 `registerWidget` → 오케스트레이터가 compact 프리셋의 순서대로 호출.

**선택 이유:** model 이모지를 ID 문자열 매칭으로 결정하는 이유는 display_name이 Translations에 따라 달라질 수 있지만 ID는 안정적이기 때문.

**실패/예외:**
- model.id가 알 수 없는 패턴 → `●` (기본 이모지)
- context_window 전체 부재 → 위젯 숨김
- cost가 0 → `$0.00` 표시 (숨기지 않음)

---

### I1.4 — OAuth 인증 + API 클라이언트

선행 조건: I1.1 (configDir 결정 로직).

- [x] 구현 완료

**목적:** Claude API에서 rate limit 데이터를 가져오는 인증 + 캐싱 파이프라인. → PLAN: Phase 1 > T1.7

**책임:**
- 입력: configDir (credential 파일 위치), Config.Cache.TTLSeconds
- 출력: `*UsageLimits` (nullable)
- 경계: credential 부재 → nil. API 실패 → stale 캐시 또는 nil. 패닉 금지.

**설계:**

구조:

`credentials.go`:
- `getCredential(configDir string) string` — 토큰 문자열 또는 빈 문자열
- 플랫폼 분기: `runtime.GOOS`
  - `"darwin"`: `exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")` → JSON → `claudeAiOauth.accessToken`. 실패/10초 TTL 만료 시 → 파일 폴백. 실패 시 60초 backoff.
  - 기타: `{configDir}/.credentials.json` → JSON → 같은 경로.
- 파일 방식: mtime 기반 캐시 — `stat` → mtime 변경 시에만 재로드.
- 글로벌 캐시 변수: `cachedToken string`, `cachedMtime time.Time` (파일), `keychainCachedAt time.Time` (macOS).

`api.go`:
- `UsageLimits` 구조체: `FiveHour`, `SevenDay`, `SevenDaySonnet` 각각 `{Utilization int, ResetsAt time.Time}`
- `fetchUsageLimits(token string, cacheCfg CacheConfig) *UsageLimits`

3-tier 캐시 흐름:
```
1. 메모리: memCache[tokenHash] → TTL 이내면 반환
2. 파일: ~/.cache/cc-usage/cache-{hash}.json → TTL 이내면 반환 + 메모리 갱신
3. API: GET https://api.anthropic.com/api/oauth/usage
   - 성공 → 메모리+파일 저장, 반환
   - 429 → retry-after 대기(최대 10초, 1회) → 재시도
   - 403 → curl 서브프로세스 폴백
   - 기타 → negative cache(30초) → stale 캐시(1시간 이내) → nil
```

`hashToken`: SHA-256 앞 16자 hex.

Request deduplication: status line 모드에서는 매 실행이 단일 API 호출이므로 메모리 캐시가 dedup 역할로 충분. 단, Phase 6(check-usage)에서는 동일 프로세스 내 Claude+Codex+Gemini+z.ai를 조회하므로, 그 시점에 동일 토큰 해시 동시 호출 방지가 필요할 수 있음 → Phase 6 진입 시 재평가.

캐시 파일 정리: API 호출 성공 시 fire-and-forget으로 `~/.cache/cc-usage/cache-*.json` 중 1시간+ 파일 삭제. 1시간 간격 throttle (마지막 정리 시간 메모리 기록).

**선택 이유:** 3-tier 캐시는 DESIGN.md 명세. short-lived 프로세스(매 status line 갱신마다 실행)에서 파일 캐시가 프로세스 간 공유를 담당. curl 폴백은 일부 환경에서 Go의 TLS 핑거프린트가 차단되는 경우 대비.

**실패/예외:**
- credential 파일 부재 → 빈 토큰 → API 호출 skip → nil 반환
- macOS Keychain 접근 실패 → 파일 폴백 → 파일도 없으면 nil
- API 타임아웃 → net/http 기본 타임아웃 10초 설정
- 캐시 디렉토리 생성 실패 → 파일 캐시 skip, 메모리만 사용
- JSON 파싱 실패 (API 응답) → negative cache
- 403 curl 폴백 실패 경로: curl 미설치(`exec.LookPath` 실패) 또는 curl도 403/타임아웃 → negative cache(30초) → stale 캐시(1시간 이내) 반환 시도 → stale도 없으면 nil

---

### I1.5 — Rate limit 위젯

선행 조건: I1.4 (API 클라이언트), I1.3 (위젯 등록 패턴).

- [x] 구현 완료

**목적:** stdin rate_limits 우선, API 폴백으로 rate limit 표시. → PLAN: Phase 1 > T1.8

**책임:**
- 입력: `StdinInput.RateLimits` (nullable), `Context.RateLimits` (API, nullable)
- 출력: `5h: 42%`, `7d: 69%`, `7d-S: 15%` 형태
- 경계: 둘 다 없으면 nil. stdin 우선.

**설계:**

구조 (`widgets_core.go` 추가):

데이터 소스 우선순위:
```
rateLimit5h/7d:
  1. stdin.rate_limits.five_hour/seven_day → used_percentage + resets_at(epoch seconds→time.Time)
  2. Context.RateLimits.FiveHour/SevenDay → utilization + resets_at(ISO→time.Time)
  3. 둘 다 없으면 → nil

rateLimit7dSonnet:
  1. Context.RateLimits.SevenDaySonnet (API only, stdin에 없음)
  2. 없으면 → nil (Decision Point 1.C: 데이터 기반 자동 숨김)
```

Render: `{label}: {color}{percent}%{reset}` + ` ({formatTimeRemaining})` (resets_at 있으면)
- label: Translations 사용 (`5h`, `7d`, `7d-S`)
- color: `getColorForPercent(percent)`

**선택 이유:** stdin 우선은 API 호출 없이 즉시 표시 가능하고 더 실시간. API는 stdin에 데이터 없을 때(첫 실행 등)의 폴백.

**실패/예외:**
- stdin resets_at가 과거 시간 → formatTimeRemaining이 빈 문자열 → 시간 표시 생략
- API 필드 타입 불일치 → JSON 파싱에서 zero value → nil 체크로 숨김

---

### I1.6 — projectInfo 위젯

선행 조건: I1.2a (위젯 프레임워크).

- [x] 구현 완료

**목적:** 현재 작업 디렉토리, git 상태를 표시. → PLAN: Phase 1 > T1.9

**책임:**
- 입력: `stdin.workspace`, `stdin.worktree`, git CLI
- 출력: `📁 dirname (branch ↑1↓2)` 형태
- 경계: git 미설치/non-git → 디렉토리명만. git 명령 실패 → graceful 생략.

**설계:**

구조 (`widgets_project.go`):
- GetData:
  1. `workspace.current_dir` → `filepath.Base()` → dirname
  2. `workspace.project_dir` != `current_dir` → subpath 계산
  3. `exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")` → branch (타임아웃 2초)
  4. `exec.Command("git", "rev-list", "--count", "--left-right", "@{u}...HEAD")` → ahead/behind
  5. `stdin.worktree` 존재 → worktree 표시 추가
- Render: `dirname` + `(branch` + `↑ahead` + `↓behind` + `)` + subpath
  - dirname: `theme.Folder` 색상
  - branch: `theme.Branch` 색상
  - ahead/behind: 0이면 생략

git 명령 실행 디렉토리: `cmd.Dir = workspace.current_dir`

**선택 이유:** git CLI 직접 호출은 zero-dependency 원칙 유지. 라이브러리(go-git 등) 대신 2초 타임아웃으로 충분히 안전.

**실패/예외:**
- git 미설치 → `exec.LookPath` 실패 → branch/ahead/behind 전부 생략
- upstream 없음 (`@{u}` 실패) → ahead/behind 생략
- detached HEAD → `rev-parse` 결과가 `HEAD` → 그대로 표시

---

### I1.7 — i18n

선행 조건: I1.2 (Translations 구조체가 Context에 포함).

- [ ] 구현 완료

**목적:** 한국어/영어 라벨 전환. → PLAN: Phase 1 > T1.10

**책임:**
- 입력: `config.language`, `LANG`/`LC_ALL`/`LC_MESSAGES` 환경변수
- 출력: `Translations` 구조체
- 경계: 알 수 없는 언어 → 영어 폴백.

**설계:**

구조:
- `locales/en.json`, `locales/ko.json`: DESIGN.md의 Translations 구조체 필드를 JSON 키로 매핑.
- `widget.go` (또는 별도 i18n 섹션): `//go:embed locales/en.json`, `//go:embed locales/ko.json`
- `loadTranslations(lang string) *Translations`:
  1. `lang == "auto"` → 환경변수 감지: `LC_ALL` → `LC_MESSAGES` → `LANG` 순서. `"ko"`로 시작하면 한국어.
  2. `lang == "ko"` → ko.json
  3. 그 외 → en.json
  4. `json.Unmarshal` → Translations

**선택 이유:** `go:embed`는 빌드 시 바이너리 내장으로 런타임 파일 의존 제거. 환경변수 감지 순서는 POSIX locale 관례.

**실패/예외:**
- JSON 파싱 실패 → 영어 폴백 (embedded이므로 실질적으로 불가능하지만 방어적 처리)

---

### I1.8 — 빌드 + 플러그인 패키징

선행 조건: I1.1~I1.7 (모든 소스 파일 존재).

- [ ] 구현 완료

**목적:** 크로스 컴파일 + 플러그인 매니페스트. → PLAN: Phase 1 > T1.11

**책임:**
- 입력: `make build` 명령
- 출력: `dist/` 디렉토리에 4개 플랫폼 바이너리, `.claude-plugin/` 매니페스트
- 경계: 빌드 실패 → make 에러. 플러그인 매니페스트는 정적 파일.

**설계:**

구조:
- `Makefile`: DESIGN.md 그대로. `build`, `build-local`, `test`, `clean` 타겟. `-ldflags="-s -w -X main.version=VERSION"`.
- `.claude-plugin/plugin.json`: 플러그인 ID, 이름, 설명, statusLine 타입, 빌드 명령.
- `.claude-plugin/marketplace.json`: 마켓플레이스 메타데이터.
- `commands/setup.md`: `/cc-usage:setup` — settings.json에 statusLine 등록 안내.
- `main.go`: `var version = "dev"` (ldflags로 주입).

**선택 이유:** Makefile은 Go 프로젝트 표준. ldflags 버전 주입은 빌드 시 단일 소스.

**실패/예외:**
- 크로스 컴파일 실패 (CGO 의존) → pure Go이므로 발생하지 않음 (`CGO_ENABLED=0` 기본)

---

### I1.9 — Phase 1 통합 검증

선행 조건: I1.1~I1.8 전체.

- [ ] 구현 완료

**목적:** PLAN T1.12의 4가지 Exit Criteria 충족 확인. → PLAN: Phase 1 > T1.12

**책임:** 통합 테스트 실행 + 실제 Claude Code 등록.

**설계:**

검증 항목:
1. `echo '{기본JSON}' | ./dist/cc-usage` → `◆ Opus │ ████░░░░ 30% 60K │ $1.25`
2. `echo '{rate_limits포함JSON}' | ./dist/cc-usage` → `◆ Opus │ ████░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%`
3. `echo '' | ./dist/cc-usage` → exit 0, 패닉 없음
4. `echo '{"model":{"id":"x","display_name":"X"}}' | ./dist/cc-usage` → 최소 출력

settings.json 등록:
```json
{"statusLine":{"type":"command","command":"/path/to/dist/cc-usage --config ~/.claude-triptopaz/cc-usage.json"}}
```

**실패/예외:**
- 검증 실패 → 해당 I1.x로 돌아가 수정

---

## Phase 2: 세션/분석 위젯

### I2.1 — Session tracking 시스템 + 세션/분석 위젯

선행 조건: I1.9 (Phase 1 통합 검증 통과).

- [ ] 구현 완료

**목적:** 세션 시작 시간 파일 기반 추적 + sessionDuration/burnRate/cacheHit 위젯. → PLAN: Phase 2 > T2.1, T2.2

**책임:**
- 입력: `stdin.session_id`, `stdin.context_window`, 시스템 시계
- 출력: 세션 파일(`~/.cache/cc-usage/sessions/{id}.json`), 3개 위젯 렌더링
- 경계: 세션 파일 쓰기 실패 → 현재 시간 기반 폴백. 동시 생성 → O_EXCL로 선착순.

**설계:**

구조:

Session tracking (`widgets_project.go` 또는 `session.go` — 단일 main 패키지이므로 파일명은 자유):
- `getSessionStartTime(sessionID string) (int64, error)`:
  1. 메모리 캐시 조회 (`sessionCache map[string]int64`)
  2. `~/.cache/cc-usage/sessions/{id}.json` 읽기 시도
  3. 없으면 → `os.OpenFile(path, O_CREATE|O_EXCL|O_WRONLY, 0644)` → 현재 시간 기록
  4. O_EXCL 실패(다른 프로세스 선점) → 파일 읽기
  5. 메모리 캐시 갱신
- `getSessionElapsedMinutes(ctx)`: `(now - startTime) / 60000` (ms→minutes)
- 세션 파일 정리: 성공 시 fire-and-forget. `filepath.Glob("sessions/*.json")` → mtime 7일+ → 삭제. 1시간 throttle.
- `session_id` 없으면 `"default"` 사용.

위젯 (`widgets_project.go`, `widgets_analytics.go`):

`sessionDuration`:
- GetData: `getSessionStartTime` → elapsed ms
- Render: `⏱ ` + `formatDuration(elapsed)`

`burnRate`:
- GetData: elapsed < 1분 → nil. `(total_input + total_output) / elapsedMinutes`
- Render: `🔥 ` + `formatTokens(tokensPerMinute)` + `/m`

`cacheHit`:
- GetData: `current_usage.cache_read / (input + cache_creation + cache_read) * 100`. 분모 0 → nil.
- Render: `📦 ` + `{color}{percent}%{reset}`
  - 색상 반전: ≥70 → Safe(초록), 40-70 → Warning(노랑), <40 → Danger(빨강)

**선택 이유:** 세션 파일을 JSON으로 저장하는 이유는 향후 필드 확장 가능성. O_EXCL은 OS 레벨 atomic이라 race condition 없음. cacheHit 색상 반전은 "높을수록 좋다"는 의미를 반영.

**실패/예외:**
- 캐시 디렉토리 생성 실패 → `os.MkdirAll` 에러 → 현재 시간 사용 (파일 없이 동작)
- 세션 파일 corrupt → JSON 파싱 실패 → 현재 시간으로 새로 생성 (기존 파일 덮어쓰기)
- 0으로 나누기 (cacheHit 분모, burnRate 분모) → nil 반환

---

## Phase 3: Transcript + 모니터링 위젯

### I3.1 — Transcript incremental 파서

선행 조건: I2.1 (Phase 2 완료).

- [ ] 구현 완료

**목적:** transcript.jsonl을 offset 기반으로 incremental 파싱하여 도구/에이전트/todo 상태를 추적. → PLAN: Phase 3 > T3.1, T3.2

**책임:**
- 입력: `stdin.transcript_path` (파일 경로)
- 출력: `*ParsedTranscript` 상태 구조체
- 경계: 파일 부재 → nil. 파싱 에러(라인 단위) → skip + 계속.

**설계:**

구조 (`transcript.go`):

```
글로벌 상태:
  transcriptState struct {
    path   string
    offset int64
    data   ParsedTranscript
  }
```

`ParsedTranscript`:
- `ToolUses map[string]ToolInfo` — tool_use ID → {Name, Input, Timestamp}
- `RunningToolIDs map[string]bool`
- `CompletedToolCount int`
- `ActiveAgentIDs map[string]bool`
- `CompletedAgentCount int`
- `Tasks map[string]TaskInfo` — seqId → {Subject, Status}
- `NextTaskID int`
- `PendingTaskCreates map[string]PendingCreate` — tool_use_id → info
- `PendingTaskUpdates map[string]PendingUpdate`
- `LastTodoWriteInput any`
- `SessionStartTime int64`
- `SessionName string`

`parseTranscript(path string) *ParsedTranscript`:
1. `os.Stat(path)` → fileSize
2. path 동일 + offset == fileSize → 캐시 반환 (변경 없음)
3. path 동일 + offset < fileSize → `file.Seek(offset)` → 새 바이트만 읽기
4. path 다르거나 fileSize < offset → 전체 re-parse (offset=0, data 초기화)
5. `bufio.Scanner` → 라인별 `json.Unmarshal` → `processEntry`

`processEntry(entry TranscriptEntry, data *ParsedTranscript)`:
- 첫 timestamp → SessionStartTime
- customTitle → SessionName
- type=="assistant" + tool_use → ToolUses/RunningToolIDs 추가, name별 분기(Task→ActiveAgentIDs, TaskCreate→PendingTaskCreates, TaskUpdate→PendingTaskUpdates)
- type=="user" + tool_result → CompletedToolCount++, RunningToolIDs 제거, Agent/Task/TodoWrite 처리, ToolUses 삭제

**선택 이유:** Incremental 파싱은 transcript가 수 MB까지 커질 수 있으므로 성능 필수. offset 기반은 append-only JSONL의 특성을 활용. 라인 단위 에러 skip은 partial write 대비.

**실패/예외:**
- 파일 부재 → nil 반환
- 라인 JSON 파싱 실패 → debugLog + skip (다음 라인 계속)
- 파일 truncation (offset > fileSize) → 전체 re-parse
- tool_result의 tool_use_id가 ToolUses에 없음 → 무시 (순서 보장 안 될 수 있음)

---

### I3.2 — Transcript 기반 위젯 (todoProgress, toolActivity, agentStatus)

선행 조건: I3.1 (ParsedTranscript 구조체).

- [ ] 구현 완료

**목적:** transcript 데이터를 위젯으로 표시. → PLAN: Phase 3 > T3.3, T3.4

**책임:**
- 입력: `*ParsedTranscript` (nullable)
- 출력: 3개 위젯 렌더링
- 경계: transcript nil → 모든 위젯 nil.

**설계:**

구조 (`widgets_analytics.go`):

`todoProgress`:
- GetData: Tasks 맵 우선 → completed/total 카운트. Tasks 비어있고 LastTodoWriteInput 있으면 → TodoWrite 파싱. 둘 다 없으면 nil.
- TodoWrite 파싱: `input.todos` 배열 → status 정규화 (not_started→pending, running→in_progress, complete/done→completed)
- Render: `✓ {completed}/{total}`

`toolActivity`:
- GetData: RunningToolIDs → 마지막(또는 아무) running tool의 name+target, CompletedToolCount
- Target 추출: Read/Write/Edit → `filepath.Base(input.file_path)`, Glob/Grep → `truncate(input.pattern, 20)`, Bash → `truncate(input.command, 25)`, 기타 → 빈 문자열
- Render: `⚙️ {name}({target}) │ {count} done`. running 없으면 완료 수만.

`agentStatus`:
- GetData: `len(ActiveAgentIDs)` + `CompletedAgentCount`. 둘 다 0이면 nil.
- Render: `🤖 Agent: {running} running, {completed} done`

**선택 이유:** Tasks API 우선은 ROADMAP.md 명세. TodoWrite는 레거시 폴백.

**실패/예외:**
- transcript 없는 display mode에서 이 위젯들이 preset에 포함 → graceful nil

---

### I3.3 — 토큰 분석 위젯 (tokenBreakdown, tokenSpeed, depletionTime)

선행 조건: I1.5 (rate limit 데이터), I2.1 (세션 시간).

- [ ] 구현 완료

**목적:** 토큰 상세 분석. → PLAN: Phase 3 > T3.5

**책임:**
- 입력: `stdin.context_window.current_usage`, `stdin.cost`, 세션 경과 시간, rate limit 데이터
- 출력: 3개 위젯 렌더링
- 경계: 필수 데이터 없으면 각각 nil.

**설계:**

구조 (`widgets_analytics.go`):

`tokenBreakdown`:
- GetData: current_usage → input, output, cache_creation, cache_read. 모두 0이면 nil.
- Render: `📊 In {formatTokens} · Out {formatTokens} · CW {formatTokens} · CR {formatTokens}`

`tokenSpeed`:
- GetData: `total_output_tokens / (total_api_duration_ms / 1000)`. api_duration 없거나 0 → nil.
- Render: `⚡ {int(speed)} tok/s`

`depletionTime`:
- GetData:
  1. 5h/7d 각각: `usedPercent / elapsedMinutes` → ratePerMinute → `(100 - usedPercent) / ratePerMinute` → minutesToLimit
  2. 둘 중 더 짧은 쪽 선택
  3. ratePerMinute 0 또는 세션 1분 미만 → nil
- Render: `⏳ {formatDuration(minutesToLimit * 60000)}`

**선택 이유:** 5h/7d 중 먼저 도달하는 쪽을 표시하는 것은 "가장 임박한 제한"이 사용자에게 가장 유용한 정보이기 때문.

**실패/예외:**
- 0으로 나누기 → nil
- rate limit 데이터 없음 → depletionTime nil

---

## Phase 4: 부가 위젯

### I4.1 — 독립 부가 위젯 (configCounts, forecast, linesChanged)

선행 조건: I3.3 (Phase 3 완료).

- [ ] 구현 완료

**목적:** budget 시스템과 무관한 부가 위젯. → PLAN: Phase 4 > T4.1

**책임:**
- 입력: 파일시스템, stdin, git, 세션 시간
- 출력: 3개 위젯 렌더링
- 경계: 각 위젯 독립. 데이터 없으면 nil.

**설계:**

`configCounts`:
- 탐색 경로: `~/.claude/CLAUDE.md`, `{project_dir}/CLAUDE.md`, `{project_dir}/.claude/CLAUDE.md` (AGENTS.md 동일)
- Rules: `filepath.Glob("{project_dir}/.claude/rules/**")` 파일 수
- MCPs/Hooks: `~/.claude/settings.json` → `mcpServers`/`hooks` JSON 키 수
- +Dirs: `stdin.workspace.added_dirs` 길이
- Render: 0인 항목 생략, `│`로 구분

`forecast`:
- `(currentCost / elapsedMinutes) * 60`. 세션 1분 미만 → nil.
- Render: `📈 ~{formatCost(hourlyCost)}/h`

`linesChanged`:
- stdin `total_lines_added/removed` 우선
- 없으면: `exec.Command("git", "diff", "--stat", "HEAD")` → 파싱
- Render: `+{added} -{removed}`. 변경 없으면 nil.

---

### I4.2 — Budget tracking + budget/todayCost 위젯

선행 조건: I4.1.

- [ ] 구현 완료

**목적:** delta 추적 기반 일일 비용 집계 + 위젯. → PLAN: Phase 4 > T4.2, T4.3

**책임:**
- 입력: `stdin.cost.total_cost_usd`, `stdin.session_id`, 시스템 날짜
- 출력: `~/.cache/cc-usage/budget.json`, 2개 위젯 렌더링
- 경계: budget.json 없으면 생성. 날짜 변경 → 리셋.

**설계:**

Budget tracking:
- `recordCostAndGetDaily(sessionID string, currentCost float64) float64`:
  1. budget.json 로드 → `{date, dailyTotal, sessions}`
  2. date != today → 리셋 (dailyTotal=0, sessions={})
  3. `delta = max(0, currentCost - sessions[sessionID])`
  4. `dailyTotal += delta`, `sessions[sessionID] = currentCost`
  5. fire-and-forget 저장
  6. return dailyTotal

Request dedup: budget + todayCost가 같은 사이클에서 호출 → `sync.Once` 또는 결과 캐시로 1회만 실행.

`budget` 위젯: `config.dailyBudget` nil → nil. 있으면 `💵 {formatCost(dailyTotal)}/{formatCost(budget)}`.
`todayCost` 위젯: `Today: {formatCost(dailyTotal)}`.

---

### I4.3 — 기타 간단 위젯

선행 조건: I4.2.

- [ ] 구현 완료

**목적:** detailed mode 나머지 슬롯. → PLAN: Phase 4 > T4.4

**책임:** 각 위젯은 stdin 단일 필드 또는 간단 계산.

**설계:**

| 위젯 | GetData | Render | nil 조건 |
|------|---------|--------|----------|
| version | `stdin.version` | 그대로 | 빈 문자열 |
| sessionName | `stdin.session_name` 우선, transcript SessionName 폴백 | 그대로 | 둘 다 없음 |
| sessionId | `stdin.session_id` | 앞 8자 | 없음 |
| sessionIdFull | `stdin.session_id` | 전체 | 없음 |
| vimMode | `stdin.vim.mode` | `NORMAL`/`INSERT` | vim 없음 |
| outputStyle | `stdin.output_style.name` | 그대로 | `"default"` 또는 없음 |
| apiDuration | `api_duration_ms / total_duration_ms * 100` | `{percent}%` | 둘 중 하나 없음 |
| peakHours | 시스템 시계 → PT 변환 | `🔴 Peak (Xh left)` / `🟢 Off-Peak (Xh to peak)` | 없음 (항상 표시) |
| performance | cache hit + output ratio | `⚡ Efficient` / `📊 Normal` | 데이터 없음 |
| lastPrompt | transcript 마지막 user text | `truncate(text, 50)` | transcript 없음 |

peakHours 구현:
- `time.LoadLocation("America/Los_Angeles")` → PT 시간
- 평일(Mon-Fri) 5-11 AM → Peak. 나머지 → Off-Peak.
- 남은 시간 계산: Peak 중이면 11시까지, Off-Peak이면 다음 Peak까지.

performance: Decision Point 4.A 미결정 → 구현 보류. 사용자 승인 후 구현.

---

## Phase 5: 외부 CLI 통합

### I5.1 — Provider 감지 + Codex 클라이언트

선행 조건: I4.3 (Phase 4 완료).

- [ ] 구현 완료

**목적:** provider 감지 + Codex CLI 사용량 위젯. → PLAN: Phase 5 > T5.1, T5.2

**책임:**
- 입력: `ANTHROPIC_BASE_URL` 환경변수, `~/.codex/auth.json`
- 출력: provider 문자열, `codexUsage` 위젯
- 경계: 미설치 → nil. API 실패 → negative cache 30초.

**설계:**

`detectProvider()`: `os.Getenv("ANTHROPIC_BASE_URL")` → `strings.Contains` 매칭.

Codex 클라이언트:
- 인증: `~/.codex/auth.json` → `tokens.access_token` + `tokens.account_id`. mtime 기반 캐시.
- API: `GET https://chatgpt.com/backend-api/wham/usage` + `Authorization: Bearer` + `ChatGPT-Account-Id`
- 모델 감지: `~/.codex/config.toml` → 루트 레벨 `model = "value"` (간단 파싱, TOML 라이브러리 없이). mtime 캐시. 없으면 빈 문자열.
- 메모리 캐시만. negative cache 30초.
- Render: `🔷 codex 5h:{primary}% 7d:{secondary}%`

---

### I5.2 — Gemini 클라이언트

선행 조건: I5.1.

- [ ] 구현 완료

**목적:** Gemini CLI OAuth 갱신 + quota 위젯. → PLAN: Phase 5 > T5.3

**책임:**
- 입력: `~/.gemini/oauth_creds.json`, 환경변수
- 출력: `geminiUsage`/`geminiUsageAll` 위젯
- 경계: 토큰 갱신 실패 → nil. 패닉 금지.

**설계:**

OAuth 토큰 lifecycle:
1. `oauth_creds.json` 로드 → `expiry_date` 확인
2. `expiry_date < now + 5분` → refresh 필요
3. `POST https://oauth2.googleapis.com/token` → 새 토큰 → 파일 업데이트 (mode 0600)
4. 갱신 실패 → nil

프로젝트 ID 조회 (우선순위):
1. `GOOGLE_CLOUD_PROJECT` / `GOOGLE_CLOUD_PROJECT_ID`
2. `~/.gemini/settings.json` → `cloudaicompanionProject`
3. API: `POST cloudcode-pa.googleapis.com/v1internal:loadCodeAssist`

Quota API: `POST cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota` → buckets → `usedPercent = round((1 - remainingFraction) * 100)`

모델 감지: `~/.gemini/settings.json` → `selectedModel`

geminiUsage: 현재 모델 버킷만. geminiUsageAll: 전체 버킷 나열.

---

### I5.3 — z.ai/ZHIPU 클라이언트

선행 조건: I5.1 (detectProvider).

- [ ] 구현 완료

**목적:** z.ai/ZHIPU 사용량 위젯. → PLAN: Phase 5 > T5.4

**책임:**
- 입력: `detectProvider()`, `ANTHROPIC_AUTH_TOKEN` 환경변수
- 출력: `zaiUsage` 위젯 (provider가 zai/zhipu일 때만)
- 경계: 다른 provider → nil.

**설계:**

- provider != "zai" && != "zhipu" → nil
- API: `GET {base_url_origin}/api/monitor/usage/quota/limit` + `Authorization: Bearer`
- 사용률 파싱: `percentage` 필드 우선 → `currentValue / (currentValue + remaining) * 100` 폴백
- Render: `Z: 5h {percent}% │ 1m {percent}%`

---

## Phase 6: check-usage CLI

### I6.1 — check 서브커맨드 + 커맨드 파일

선행 조건: I5.3 (Phase 5 완료, 모든 CLI 클라이언트 존재).

- [ ] 구현 완료

**목적:** `cc-usage check`으로 독립 실행 대시보드. → PLAN: Phase 6 > T6.1, T6.2

**책임:**
- 입력: `cc-usage check [--json] [--lang XX]` (stdin 없음)
- 출력: stdout에 대시보드 (pretty 또는 JSON)
- 경계: 각 CLI 조회 실패 → 해당 섹션 null/unavailable.

**설계:**

`main.go` 엔트리포인트 분기:
- `os.Args`에 `"check"` 있으면 → check 모드
- 없으면 → 기존 stdin 모드

check 모드:
1. `--json`, `--lang` 플래그 파싱
2. credential 로드 → Claude 사용량 조회
3. Codex/Gemini/z.ai 각각 조회 (실패 → null)
4. 추천 알고리즘: 각 CLI의 5h 사용률 비교 → 가장 낮은 CLI
5. pretty 출력 (구분선 + 섹션) 또는 JSON 출력

`commands/check-usage.md`: Bash tool로 바이너리 찾아 실행.

**선택 이유:** 서브커맨드를 `os.Args` 기반으로 구현하는 이유는 `flag` 패키지가 서브커맨드를 네이티브 지원하지 않고, 외부 라이브러리(cobra 등)는 zero-dependency 위반이기 때문. `os.Args[1] == "check"` 분기 후 별도 FlagSet 사용.

---

## Phase 7: 추가 커맨드

### I7.1 — update + setup-alias 커맨드 파일

선행 조건: I6.1 (Phase 6 완료).

- [ ] 구현 완료

**목적:** 플러그인 업데이트 + 쉘 별칭 설정. → PLAN: Phase 7 > T7.1, T7.2

**책임:**
- 입력: Claude Code slash command 실행
- 출력: `commands/update.md`, `commands/setup-alias.md`
- 경계: 이 구현 단위는 markdown 커맨드 파일 작성. Go 코드 변경 없음.

**설계:**

`commands/update.md`:
- 플러그인 캐시 디렉토리에서 최신 바이너리 경로 감지
- `settings.json`의 `statusLine.command` 업데이트

`commands/setup-alias.md`:
- 쉘 감지 → `.zshrc`/`.bashrc`/PowerShell profile에 `check-ai` 함수 추가
- 함수 내용: 최신 바이너리 경로 찾아 `check` 서브커맨드 실행

---

## PLAN 매핑 요약

| PLAN Task | IMPLEMENT 단위 | 상태 |
|-----------|---------------|------|
| T1.1 | I1.1 | [ ] |
| T1.2 | I1.1 | [ ] |
| T1.3 | I1.2a | [ ] |
| T1.4 | I1.2b | [ ] |
| T1.5 | I1.2b | [ ] |
| T1.6 | I1.3 | [ ] |
| T1.7 | I1.4 | [ ] |
| T1.8 | I1.5 | [ ] |
| T1.9 | I1.6 | [ ] |
| T1.10 | I1.7 | [ ] |
| T1.11 | I1.8 | [ ] |
| T1.12 | I1.9 | [ ] |
| T2.1 | I2.1 | [ ] |
| T2.2 | I2.1 | [ ] |
| T3.1 | I3.1 | [ ] |
| T3.2 | I3.1 | [ ] |
| T3.3 | I3.2 | [ ] |
| T3.4 | I3.2 | [ ] |
| T3.5 | I3.3 | [ ] |
| T4.1 | I4.1 | [ ] |
| T4.2 | I4.2 | [ ] |
| T4.3 | I4.2 | [ ] |
| T4.4 | I4.3 | [ ] |
| T5.1 | I5.1 | [ ] |
| T5.2 | I5.1 | [ ] |
| T5.3 | I5.2 | [ ] |
| T5.4 | I5.3 | [ ] |
| T6.1 | I6.1 | [ ] |
| T6.2 | I6.1 | [ ] |
| T7.1 | I7.1 | [ ] |
| T7.2 | I7.1 | [ ] |

누락 없음. 모든 PLAN Task가 IMPLEMENT 단위에 매핑됨.
