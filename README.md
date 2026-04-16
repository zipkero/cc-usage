# cc-usage

Claude Code status line plugin. 모델, 컨텍스트 사용량, 비용, rate limit 등을 status line에 표시한다.

```
my-project (main) │ ◆ Opus │ ████░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%
```

## Features

- Zero dependency (Go 표준 라이브러리만 사용)
- 크로스 플랫폼 (macOS, Linux, Windows)
- 모듈러 위젯 시스템
- 8개 컬러 테마 (default, minimal, catppuccin, dracula, gruvbox, nord, tokyoNight, solarized)
- 다국어 지원 (English, 한국어)
- Display mode: compact, normal, detailed, custom

## Installation

### Plugin Marketplace (권장)

```bash
# 1. marketplace 등록
/plugin marketplace add zipkero/cc-usage

# 2. 플러그인 설치
/plugin install cc-usage

# 3. status line 설정 적용
/cc-usage:cc-usage-install

# 4. 적용
/reload-plugins
```

### Manual

소스를 클론하고 빌드한 뒤 settings에 직접 등록한다.

```bash
git clone https://github.com/zipkero/cc-usage.git
cd cc-usage
make build-local   # dist/cc-usage 생성
```

`~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/dist/cc-usage"
  }
}
```

> **Windows**: 경로에 forward slash 사용. (`"command": "C:/Users/.../dist/cc-usage.exe"`)

커스텀 프로필 사용 시:

```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/dist/cc-usage --config ~/.claude-triptopaz/cc-usage.json"
  }
}
```

## Configuration

설정 파일: `~/.claude/cc-usage.json` (또는 `--config`로 지정)

```json
{
  "language": "auto",
  "plan": "max",
  "displayMode": "compact",
  "theme": "default",
  "separator": "pipe",
  "dailyBudget": 10.0,
  "disabledWidgets": [],
  "cache": { "ttlSeconds": 300 }
}
```

| 필드 | 기본값 | 설명 |
|------|--------|------|
| `language` | `"auto"` | `"auto"`, `"en"`, `"ko"` |
| `plan` | `"max"` | `"pro"`, `"max"` |
| `displayMode` | `"compact"` | `"compact"`, `"normal"`, `"detailed"`, `"custom"` |
| `theme` | `"default"` | 8개 테마 중 선택 |
| `separator` | `"pipe"` | `"pipe"`, `"space"`, `"dot"`, `"arrow"` |
| `dailyBudget` | - | 일일 예산 (USD) |
| `disabledWidgets` | `[]` | 비활성화할 위젯 ID 목록 |
| `preset` | - | 위젯 단축 문자열 (예: `"MC$R\|BD"`) |

## Widgets

### Core

| ID | 설명 |
|----|------|
| `model` | 모델명 + 아이콘 (◆Opus/◇Sonnet/○Haiku) |
| `context` | 프로그레스바 + 사용률 + 토큰 수 |
| `cost` | 세션 비용 |
| `rateLimit5h` | 5시간 rate limit |
| `rateLimit7d` | 7일 rate limit |
| `rateLimit7dSonnet` | 7일 Sonnet rate limit |
| `projectInfo` | 디렉토리 + git branch |

### Analytics (normal/detailed mode)

`sessionDuration`, `burnRate`, `cacheHit`, `tokenSpeed`, `todoProgress`, `toolActivity`, `agentStatus`, `configCounts`, `performance`, `tokenBreakdown`, `forecast`, `budget`, `todayCost`, `linesChanged`, `outputStyle`, `version`, `peakHours` 등

## Troubleshooting

### 플러그인 업데이트 시 SSH 인증 오류 (Windows)

```
git@github.com: Permission denied (publickey).
fatal: Could not read from remote repository.
```

플러그인 업데이트 과정에서 SSH URL로 clone을 시도하면서 발생할 수 있다. git 글로벌 설정으로 SSH를 HTTPS로 우회하면 해결된다.

```bash
git config --global url."https://github.com/".insteadOf "git@github.com:"
```

## License

MIT
