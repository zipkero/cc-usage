# cc-usage — Go Claude Code Status Line Plugin

## Overview

Claude Code 플러그인으로 동작하는 Go 기반 status line. stdin JSON을 받아 ANSI 컬러 텍스트를 stdout으로 출력한다.

**핵심 특징:**
- Zero dependency (Go 표준 라이브러리만 사용)
- 단일 바이너리 배포 (크로스 컴파일)
- 모듈러 위젯 시스템
- 테마, i18n, 커스텀 레이아웃 지원

---

## Plugin 계약

| 항목 | 값 |
|------|-----|
| 입력 | stdin JSON (Claude Code가 제공) |
| 출력 | stdout ANSI 텍스트 (멀티라인 가능) |
| CLI 인자 | `--config <path>` (선택, 기본값: `~/.claude/cc-usage.json`) |
| 설정 | `--config`로 지정된 경로 또는 `~/.claude/cc-usage.json` |
| 인증 | `{configDir}/.credentials.json` → `claudeAiOauth.accessToken` |
| API | `GET https://api.anthropic.com/api/oauth/usage` |
| 캐시 | `~/.cache/cc-usage/cache-{tokenHash}.json` |

### Stdin JSON 전체 구조

```json
{
  "model": {
    "id": "claude-opus-4-6",
    "display_name": "Opus"
  },
  "workspace": {
    "current_dir": "/path/to/cwd",
    "project_dir": "/path/to/project",
    "added_dirs": ["/extra/dir1"]
  },
  "worktree": {
    "name": "feature-branch",
    "path": "/tmp/worktree",
    "branch": "feature-branch",
    "original_cwd": "/original/path",
    "original_branch": "main"
  },
  "context_window": {
    "total_input_tokens": 50000,
    "total_output_tokens": 10000,
    "context_window_size": 200000,
    "used_percentage": 30,
    "remaining_percentage": 70,
    "current_usage": {
      "input_tokens": 50000,
      "output_tokens": 10000,
      "cache_creation_input_tokens": 0,
      "cache_read_input_tokens": 0
    }
  },
  "cost": {
    "total_cost_usd": 1.25,
    "total_duration_ms": 300000,
    "total_api_duration_ms": 120000,
    "total_lines_added": 150,
    "total_lines_removed": 30
  },
  "rate_limits": {
    "five_hour": { "used_percentage": 42, "resets_at": 1700000000 },
    "seven_day": { "used_percentage": 69, "resets_at": 1700500000 }
  },
  "output_style": { "name": "concise" },
  "transcript_path": "/path/to/transcript.jsonl",
  "version": "2.1.0",
  "exceeds_200k_tokens": false,
  "session_id": "abc-123",
  "session_name": "my-session",
  "vim": { "mode": "NORMAL" },
  "agent": { "name": "my-agent" },
  "permission_mode": "default",
  "remote": { "session_id": "remote-xyz" },
  "agent_id": "sub-agent-id",
  "agent_type": "Explore"
}
```

**필드 설명:**

| 필드 | 필수 | 설명 |
|------|------|------|
| `model.id` | O | 모델 ID (`claude-opus-4-6` 등) |
| `model.display_name` | O | 표시명 (`Opus`, `Sonnet`, `Haiku`) |
| `workspace.current_dir` | O | 현재 작업 디렉토리 |
| `workspace.project_dir` | X | Claude Code 실행 디렉토리 (CWD와 다를 수 있음) |
| `workspace.added_dirs` | X | `/add-dir`로 추가된 디렉토리 (v2.1.77+) |
| `worktree` | X | `--worktree` 세션 시에만 존재 |
| `context_window.used_percentage` | X | Claude Code가 계산한 공식 사용률 (0-100) |
| `context_window.remaining_percentage` | X | 남은 비율 (0-100) |
| `cost.total_duration_ms` | X | 총 세션 시간 (ms) |
| `cost.total_api_duration_ms` | X | API 호출 시간 총합 (ms) |
| `cost.total_lines_added/removed` | X | 세션 내 변경 라인 수 |
| `rate_limits` | X | 첫 API 응답 이후에만 존재. 각 윈도우 독립적으로 없을 수 있음 |
| `rate_limits.*.resets_at` | - | **Unix epoch seconds** (API 응답의 ISO 문자열과 다름!) |
| `output_style.name` | X | 현재 출력 스타일 (`default` 일 때 숨김) |
| `transcript_path` | X | transcript.jsonl 파일 경로 |
| `exceeds_200k_tokens` | X | 200K 토큰 초과 여부 (고정 임계치) |
| `session_id` | X | 세션 ID (duration tracking에 사용) |
| `session_name` | X | `/rename`으로 설정한 세션명 (v2.1.77+) |
| `vim` | X | Vim 모드 활성화 시에만 존재 |
| `agent` | X | `--agent` 플래그 실행 시에만 존재 |
| `permission_mode` | X | 권한 모드 (`default` 등) |
| `remote` | X | 원격 세션 정보 (`session_id` 포함) |
| `agent_id` | X | 서브에이전트 ID (서브에이전트 컨텍스트에서만) |
| `agent_type` | X | 서브에이전트 타입 (`Explore` 등, 서브에이전트 컨텍스트에서만) |

### API 응답 구조

```
GET https://api.anthropic.com/api/oauth/usage
Authorization: Bearer {token}
User-Agent: cc-usage/{VERSION}
anthropic-beta: oauth-2025-04-20

Response:
{
  "five_hour": { "utilization": 42, "resets_at": "2026-04-14T10:00:00Z" },
  "seven_day": { "utilization": 69, "resets_at": "2026-04-20T00:00:00Z" },
  "seven_day_sonnet": { "utilization": 15, "resets_at": "2026-04-20T00:00:00Z" }
}
```

**주의:** `utilization`은 0-100 퍼센트. stdin의 `rate_limits`도 퍼센트. 단위 혼동 없음.

**API ↔ stdin 필드명 매핑:**

| 의미 | API 응답 필드 | stdin 필드 | 비고 |
|------|--------------|-----------|------|
| 사용률 | `utilization` | `used_percentage` | 둘 다 0-100 퍼센트 |
| 리셋 시간 | `resets_at` (ISO 8601 문자열) | `resets_at` (Unix epoch seconds) | **타입이 다름!** |
| 7d Sonnet | `seven_day_sonnet` | (없음) | API only, stdin에 없음 |

---

## Config 구조

### 경로 결정

```
cc-usage --config ~/.claude-triptopaz/cc-usage.json   # 명시적 지정
cc-usage                                                # 기본값: ~/.claude/cc-usage.json
```

**configDir 추론:** `dirname(configPath)` → 인증 파일 등 관련 경로의 기준 디렉토리로 사용.
- 설정: `{configPath}` (--config 또는 `~/.claude/cc-usage.json`)
- 인증: `{configDir}/.credentials.json`
- 캐시: `~/.cache/cc-usage/` (configDir과 무관, 전역 공유)

### 멀티 프로필 설정 예시

```json
// ~/.claude-triptopaz/settings.json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/cc-usage --config ~/.claude-triptopaz/cc-usage.json"
  }
}

// ~/.claude/settings.json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/cc-usage"
  }
}
```

### Config 파일

```go
type Config struct {
    Language        string     `json:"language"`          // "auto" | "en" | "ko"
    Plan            string     `json:"plan"`              // "pro" | "max"
    DisplayMode     string     `json:"displayMode"`       // "compact" | "normal" | "detailed" | "custom"
    Lines           [][]string `json:"lines,omitempty"`   // custom 모드 전용
    DisabledWidgets []string   `json:"disabledWidgets,omitempty"` // 비활성화할 위젯 ID
    Theme           string     `json:"theme,omitempty"`   // 테마 ID
    Separator       string     `json:"separator,omitempty"` // 세퍼레이터 스타일
    Preset          string     `json:"preset,omitempty"`  // 단축 프리셋 문자열
    DailyBudget     *float64   `json:"dailyBudget,omitempty"` // 일일 예산 (USD)
    Cache           struct {
        TTLSeconds int `json:"ttlSeconds"` // API 캐시 TTL (기본 300)
    } `json:"cache"`
}
```

**기본값:**
```json
{
  "language": "auto",
  "plan": "max",
  "displayMode": "compact",
  "cache": { "ttlSeconds": 300 }
}
```

**disabledWidgets 동작:**
- 어떤 display mode든 적용 (preset, custom 포함)
- 필터링 후 빈 라인은 자동 제거

**Preset 문자열:**
- `"MC$R|BD"` → Line 1: model,context,cost,rateLimit5h / Line 2: burnRate,sessionDuration
- `|`로 라인 구분, 각 문자는 위젯 매핑 (아래 표 참조)
- 설정 시 `displayMode`를 `custom`으로 오버라이드

---

## 프로젝트 구조

```
cc-usage/
├── .claude-plugin/
│   ├── plugin.json          # 플러그인 매니페스트
│   └── marketplace.json     # 마켓플레이스 메타데이터
├── skills/
│   └── cc-usage-install/
│       └── SKILL.md         # 설치 스킬
├── main.go                  # 엔트리포인트 (stdin→위젯→stdout)
├── stdin.go                 # StdinInput 구조체 + 파싱
├── config.go                # Config 구조체 + 로드 + 프리셋
├── widget.go                # Widget 인터페이스 + 레지스트리 + 오케스트레이터
├── render.go                # ANSI 색상, 테마, 세퍼레이터, 프로그레스바
├── api.go                   # OAuth API 클라이언트 (3-tier 캐시)
├── credentials.go           # 인증 토큰 추출 (file + macOS keychain)
├── format.go                # 토큰/비용/시간/퍼센트 포매팅
├── widgets_core.go          # model, context, cost, rateLimit 위젯
├── widgets_project.go       # projectInfo, sessionDuration 등
├── widgets_analytics.go     # burnRate, cacheHit, tokenSpeed 등
├── locales/
│   ├── en.json              # 영어
│   └── ko.json              # 한국어
├── go.mod
├── Makefile                 # 크로스 컴파일 + 로컬 빌드
└── dist/                    # 빌드 산출물
```

**설계 원칙:**
- 단일 `main` 패키지 (플러그인은 라이브러리가 아닌 실행 바이너리)
- 위젯마다 파일 분리하지 않고, 카테고리별 그룹핑 (Go 관용적 패턴)
- 외부 의존성 0

---

## 코어 시스템 설계

### Widget 인터페이스

```go
type Widget interface {
    ID() string
    GetData(ctx *Context) (any, error)
    Render(data any, ctx *Context) string
}

type Context struct {
    Stdin        *StdinInput
    Config       *Config
    Translations *Translations
    RateLimits   *UsageLimits
}
```

**오케스트레이터 흐름:**
1. Config → display mode → 위젯 ID 배열
2. `disabledWidgets` 필터링 적용
3. 각 라인의 위젯 순회: `GetData` → `Render`
4. nil/error 반환 위젯 건너뜀 (graceful degradation)
5. 세퍼레이터로 조인, 빈 라인 제거
6. stdout 출력

### 테마 시스템

```go
type ThemeColors struct {
    // 스타일
    Dim  string
    Bold string

    // Semantic roles
    Model     string  // 모델명
    Folder    string  // 디렉토리명, 비용
    Branch    string  // Git 브랜치
    Safe      string  // 낮은 사용량, 정상 상태
    Warning   string  // 중간 사용량 (51-80%)
    Danger    string  // 높은 사용량 (>80%), 에러
    Secondary string  // 부차적/뮤트 정보
    Accent    string  // 강조 색상 (비용, 라벨)
    Info      string  // 정보성 (파란/시안)

    // 프로그레스바
    BarFilled string  // 채워진 부분
    BarEmpty  string  // 빈 부분

    // 기본 ANSI (항상 사용 가능)
    Red     string
    Green   string
    Yellow  string
    Blue    string
    Magenta string
    Cyan    string
    White   string
    Gray    string
}
```

**8개 테마 (Config.theme 값 = 테이블 첫 열):**

| 테마 | Model | Folder | Branch | Safe | Warning | Danger | Secondary |
|------|-------|--------|--------|------|---------|--------|-----------|
| default | `\x1b[38;5;117m` (pastelCyan) | `\x1b[38;5;222m` (pastelYellow) | `\x1b[38;5;218m` (pastelPink) | `\x1b[38;5;151m` (pastelGreen) | `\x1b[38;5;222m` | `\x1b[38;5;210m` (pastelRed) | `\x1b[38;5;249m` (pastelGray) |
| minimal | `\x1b[37m` (white) | `\x1b[37m` | `\x1b[37m` | `\x1b[90m` (gray) | `\x1b[37m` | `\x1b[1;37m` (bold) | `\x1b[90m` |
| catppuccin | `\x1b[38;2;137;180;250m` (#89b4fa) | `\x1b[38;2;249;226;175m` (#f9e2af) | `\x1b[38;2;245;194;231m` (#f5c2e7) | `\x1b[38;2;166;227;161m` (#a6e3a1) | `\x1b[38;2;250;179;135m` (#fab387) | `\x1b[38;2;243;139;168m` (#f38ba8) | `\x1b[38;2;127;132;156m` (#7f849c) |
| dracula | `\x1b[38;2;189;147;249m` (#bd93f9) | `\x1b[38;2;255;184;108m` (#ffb86c) | `\x1b[38;2;255;121;198m` (#ff79c6) | `\x1b[38;2;80;250;123m` (#50fa7b) | `\x1b[38;2;241;250;140m` (#f1fa8c) | `\x1b[38;2;255;85;85m` (#ff5555) | `\x1b[38;2;98;114;164m` (#6272a4) |
| gruvbox | `\x1b[38;2;215;153;33m` (#d79921) | `\x1b[38;2;250;189;47m` (#fabd2f) | `\x1b[38;2;211;134;155m` (#d3869b) | `\x1b[38;2;184;187;38m` (#b8bb26) | `\x1b[38;2;250;189;47m` | `\x1b[38;2;204;36;29m` (#cc241d) | `\x1b[38;2;168;153;132m` (#a89984) |
| nord | `\x1b[38;2;136;192;208m` (#88c0d0) | `\x1b[38;2;235;203;139m` (#ebcb8b) | `\x1b[38;2;180;142;173m` (#b48ead) | `\x1b[38;2;163;190;140m` (#a3be8c) | `\x1b[38;2;235;203;139m` | `\x1b[38;2;191;97;106m` (#bf616a) | `\x1b[38;2;76;86;106m` (#4c566a) |
| tokyoNight | `\x1b[38;2;122;162;247m` (#7aa2f7) | `\x1b[38;2;224;175;104m` (#e0af68) | `\x1b[38;2;187;154;247m` (#bb9af7) | `\x1b[38;2;158;206;106m` (#9ece6a) | `\x1b[38;2;224;175;104m` | `\x1b[38;2;247;118;142m` (#f7768e) | `\x1b[38;2;86;95;137m` (#565f89) |
| solarized | `\x1b[38;2;38;139;210m` (#268bd2) | `\x1b[38;2;181;137;0m` (#b58900) | `\x1b[38;2;211;54;130m` (#d33682) | `\x1b[38;2;133;153;0m` (#859900) | `\x1b[38;2;181;137;0m` | `\x1b[38;2;220;50;47m` (#dc322f) | `\x1b[38;2;88;110;117m` (#586e75) |

**사용량 기반 색상 선택:**
```go
func getColorForPercent(percent int) string {
    if percent <= 50 { return theme.Safe }
    if percent <= 80 { return theme.Warning }
    return theme.Danger
}
```

### 세퍼레이터

| 스타일 | 문자 | 출력 예시 |
|--------|------|-----------|
| pipe (기본) | `│` | ` {dim}│{reset} ` (양옆 공백) |
| space | ` ` | `  ` (이중 공백) |
| dot | `·` | ` {dim}·{reset} ` |
| arrow | `›` | ` {dim}›{reset} ` |

### 프로그레스바

```go
type ProgressBarConfig struct {
    Width      int    // 기본 10
    FilledChar string // █ (U+2588 Full Block)
    EmptyChar  string // ░ (U+2591 Light Shade)
}

func renderProgressBar(percent int, config ProgressBarConfig) string {
    filled := int(math.Round(float64(percent) / 100.0 * float64(config.Width)))
    empty := config.Width - filled
    color := getColorForPercent(percent)
    return color + strings.Repeat(config.FilledChar, filled) +
           strings.Repeat(config.EmptyChar, empty) + RESET
}
```

### API 클라이언트

**3-tier 캐시:**
1. 메모리 캐시 (글로벌 변수) — 프로세스 내 최속
2. 파일 캐시 (`~/.cache/cc-usage/cache-{hash}.json`) — 프로세스 간 공유
3. API 호출 — 캐시 미스 시 폴백

**토큰 해싱:** SHA-256 앞 16자 hex (64비트 keyspace)
```go
func hashToken(token string) string {
    h := sha256.Sum256([]byte(token))
    return hex.EncodeToString(h[:])[:16]
}
```

**캐시 유효성:**
- 정상 캐시: Config의 `ttlSeconds` (기본 300초)
- 에러 캐시 (negative cache): 30초 고정 → 실패 후 빠른 재시도 억제
- Stale 폴백: 1시간까지 오래된 캐시 허용 (API 장애 시)

**인증:**
- Windows/Linux: `{configDir}/.credentials.json` → `claudeAiOauth.accessToken`
  - `configDir`은 `--config` 경로의 `dirname()` (기본: `~/.claude/`)
  - mtime 기반 캐시 무효화
- macOS: `security find-generic-password -s "Claude Code-credentials" -w` → JSON 파싱 → 같은 경로
  - TTL 기반 캐시 (10초)
  - 실패 시 60초 backoff → 파일 폴백(`{configDir}/.credentials.json`)

**에러 처리:**
- 429 → `retry-after` 헤더 존중 (1회, 최대 10초)
- 403 → `curl` 서브프로세스 폴백 (TLS 핑거프린트 우회)
- 기타 실패 → negative cache(30초) 설정 → stale 캐시 반환 → 없으면 `⚠️`

**Request deduplication:**
- 동일 토큰 해시에 대한 동시 API 호출 방지
- Go에서는 `sync.Once` 또는 channel 패턴으로 구현

**캐시 파일 정리:**
- 1시간 이상 된 `cache-*.json` 파일 삭제
- 최대 1시간 간격으로 실행 (time-based throttling)

### i18n

```go
//go:embed locales/en.json
var enJSON []byte

//go:embed locales/ko.json
var koJSON []byte
```

**언어 감지:** Config `language` → `"auto"`면 `LANG`/`LC_ALL`/`LC_MESSAGES` 환경변수 → `ko`로 시작하면 한국어, 그 외 영어

**Translations 구조:**
```go
type Translations struct {
    Model  struct{ Opus, Sonnet, Haiku string }
    Labels struct{ FiveH, SevenD, SevenDAll, SevenDSonnet, OneM string }
    Time   struct{ Days, Hours, Minutes, Seconds string }
    Errors struct{ NoContext string }
    Widgets struct {
        Tools, Done, Running, Agent, Todos string
        ClaudeMd, AgentsMd, AddedDirs, Rules, Mcps, Hooks string
        BurnRate, Cache, ToLimit, Forecast, Budget string
        Performance, TokenBreakdown, TodayCost string
        ApiDuration, PeakHours, OffPeak string
    }
}
```

### 포매팅 유틸리티

```go
// 토큰: 1500→"1.5K", 150000→"150K", 1500000→"1.5M"
func formatTokens(tokens int) string

// 비용: 0.5→"$0.50", 1.234→"$1.23"
func formatCost(cost float64) string

// 남은 시간: "3d2h", "2h30m", "45m"
func formatTimeRemaining(resetAt time.Time, t Translations) string

// 경과 시간: "1h30m", "5m"
func formatDuration(ms int64, t Translations) string

// 모델명 축약: "Claude 3.5 Sonnet"→"Sonnet"
func shortenModelName(displayName string) string

// 퍼센트 계산 (0-100 클램프)
func calculatePercent(current, total int) int

// 문자열 자르기: "long text..."→"long te…"
func truncate(s string, maxLen int) string

// 퍼센트 클램프 (0-100)
func clampPercent(value float64) int

// OSC8 하이퍼링크: 터미널 링크 생성
// 사용처: projectInfo(디렉토리 링크), transcript_path 등 클릭 가능한 경로 표시
func osc8Link(url, text string) string
```

### 디버그 유틸리티

```go
// DEBUG=cc-usage 또는 DEBUG=1 환경변수로 활성화
func debugLog(context, message string, args ...any)
```

- stderr로 출력 (stdout은 위젯 출력 전용)
- 형식: `[cc-usage:{context}] {message}`

---

## 위젯 목록

### MVP (Phase 1)

| ID | 설명 | 데이터 소스 |
|----|------|-------------|
| model | 모델명 + 이모지 (◆Opus/◇Sonnet/○Haiku) + effort(H/M/L) + fast(↯) | stdin.model |
| context | 프로그레스바 + % + 토큰수 | stdin.context_window |

> **TODO(model):** `effort`와 `fast` 필드는 현재 stdin JSON에 정의되지 않음. 구현 시 Claude Code의 실제 stdin 출력을 덤프하여 필드 존재 여부를 확인하고, 없으면 해당 표시를 생략할 것.
>
> **TODO(context):** `used_percentage`는 optional. 없을 때 폴백 계산: `clampPercent((total_input_tokens + total_output_tokens) / context_window_size * 100)`. `context_window_size`도 없으면 위젯 숨김.
| cost | 세션 비용 ($0.00) | stdin.cost |
| rateLimit5h | 5시간 rate limit % + 리셋시간 | stdin.rate_limits / API |
| rateLimit7d | 7일 rate limit (Max만) | stdin.rate_limits / API |
| rateLimit7dSonnet | 7일 Sonnet limit (Max만) | API only |

> **TODO(plan):** "Max만" = `config.plan == "max"`일 때만 표시인지, 아니면 데이터가 없으면 자동 숨김으로 충분한지 구현 시 결정. 우선 **데이터 없으면 자동 숨김 (graceful skip)** 으로 구현하고, plan 분기는 필요 시 추가.
| projectInfo | 디렉토리 + git branch + ahead/behind(↑↓) + subpath + worktree | stdin.workspace + git |

**MVP 출력 예시:**
```
◆ Opus(H) │ ████░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69% │ 7d-S: 15%
```

**Rate limit 데이터 흐름 (중요):**
1. stdin에 `rate_limits` 있으면 → stdin 사용 (5h, 7d)
2. stdin에 없으면 → API 전체 폴백
3. Max 플랜: stdin(5h,7d) + API(seven_day_sonnet) 하이브리드

### 확장 위젯 (Phase 2~8)

ROADMAP.md에 정의. CHECKLIST.md의 Phase 번호 매핑 참조.

---

## Display Modes

```go
var DisplayPresets = map[string][][]string{
    "compact": {
        {"model", "context", "cost", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"},
    },
    "normal": {
        {"model", "context", "cost", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"},
        {"projectInfo", "sessionDuration", "burnRate", "todoProgress"},
    },
    "detailed": {
        {"model", "context", "cost", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"},
        {"projectInfo", "sessionName", "sessionDuration", "burnRate", "tokenSpeed", "depletionTime", "todoProgress"},
        {"configCounts", "toolActivity", "agentStatus", "cacheHit", "performance"},
        {"tokenBreakdown", "forecast", "budget", "todayCost"},
        {"codexUsage", "geminiUsage", "linesChanged", "outputStyle", "version", "peakHours"},
        {"lastPrompt"},
    },
}
```

### Preset 단축키 전체 매핑

| 문자 | 위젯 | 문자 | 위젯 | 문자 | 위젯 |
|------|------|------|------|------|------|
| `M` | model | `T` | toolActivity | `V` | version |
| `C` | context | `A` | agentStatus | `L` | linesChanged |
| `$` | cost | `O` | todoProgress | `Y` | outputStyle |
| `R` | rateLimit5h | `B` | burnRate | `Q` | tokenSpeed |
| `7` | rateLimit7d | `E` | depletionTime | `J` | sessionName |
| `S` | rateLimit7dSonnet | `H` | cacheHit | `@` | todayCost |
| `P` | projectInfo | `X` | codexUsage | `?` | lastPrompt |
| `I` | sessionId | `G` | geminiUsage | `m` | vimMode |
| `D` | sessionDuration | `Z` | zaiUsage | `a` | apiDuration |
| `K` | configCounts | `N` | tokenBreakdown | `p` | peakHours |
| `F` | performance | `W` | forecast | `g` | geminiUsageAll |
| `U` | budget | `i` | sessionIdFull | | |

---

## 빌드 & 배포

### Makefile

```makefile
BINARY := cc-usage
VERSION := 0.1.0
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 windows/amd64

.PHONY: build build-local test clean

build:
	@for p in $(PLATFORMS); do \
		GOOS=$${p%/*} GOARCH=$${p#*/} \
		go build -ldflags="-s -w -X main.version=$(VERSION)" \
		-o dist/$(BINARY)-$${p%/*}-$${p#*/}$$([ "$${p%/*}" = "windows" ] && echo ".exe") .; \
	done

build-local:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/$(BINARY) .

test:
	go test ./...

clean:
	rm -rf dist/
```

### settings.json 등록

```json
// 기본 프로필 (~/.claude/)
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/cc-usage"
  }
}

// 커스텀 프로필 (~/.claude-triptopaz/ 등)
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/cc-usage --config ~/.claude-triptopaz/cc-usage.json"
  }
}
```

---

## 구현 순서

| 단계 | 파일 | 내용 |
|------|------|------|
| 1 | `go.mod`, `main.go`, `stdin.go`, `config.go` | 프로젝트 초기화, 입출력 파이프라인 |
| 2 | `widget.go`, `render.go` | 위젯 시스템 + ANSI 렌더링 + 테마 |
| 3 | `format.go` | 포매팅 유틸리티 |
| 4 | `widgets_core.go` (model, context, cost) | 첫 3개 위젯 |
| 5 | `credentials.go`, `api.go` | API 인증 + 호출 + 3-tier 캐시 |
| 6 | `widgets_core.go` (rateLimit5h/7d/7dSonnet) | rate limit 위젯 |
| 7 | `widgets_project.go` (projectInfo) | 프로젝트 정보 |
| 8 | `locales/`, i18n 로직 | 다국어 지원 |
| 9 | `Makefile` | 빌드 자동화 |
| 10 | `.claude-plugin/`, `skills/` | 플러그인 매니페스트 + 설치 스킬 |

---

## 검증 방법

```bash
# 빌드
go build -o dist/cc-usage .

# stdin JSON으로 테스트
echo '{"model":{"id":"claude-opus-4-6","display_name":"Opus"},"workspace":{"current_dir":"/tmp"},"context_window":{"total_input_tokens":50000,"total_output_tokens":10000,"context_window_size":200000,"current_usage":{"input_tokens":50000,"output_tokens":10000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"cost":{"total_cost_usd":1.25}}' | ./dist/cc-usage

# 기대 출력 (compact 모드):
# ◆ Opus │ ████░░░░ 30% 60K │ $1.25

# rate limit 포함 테스트
echo '{"model":{"id":"claude-opus-4-6","display_name":"Opus"},"workspace":{"current_dir":"/tmp"},"context_window":{"total_input_tokens":50000,"total_output_tokens":10000,"context_window_size":200000,"current_usage":{"input_tokens":50000,"output_tokens":10000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"cost":{"total_cost_usd":1.25},"rate_limits":{"five_hour":{"used_percentage":42,"resets_at":1700000000},"seven_day":{"used_percentage":69,"resets_at":1700500000}}}' | ./dist/cc-usage

# 기대 출력:
# ◆ Opus │ ████░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%
```
