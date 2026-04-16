# cc-usage

Claude Code status line plugin. 모델, 컨텍스트 사용량, 비용, rate limit 등을 status line에 표시한다.

```
◆ Opus │ ████░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%
```

## Features

- Zero dependency (Go 표준 라이브러리만 사용)
- 크로스 플랫폼 (macOS, Linux, Windows)
- 모듈러 위젯 시스템
- 8개 컬러 테마 (default, minimal, catppuccin, dracula, gruvbox, nord, tokyoNight, solarized)
- 다국어 지원 (English, 한국어)
- Display mode: compact, normal, detailed, custom

## Installation

### 1. Plugin Marketplace

```bash
/plugin marketplace add zipkero/cc-usage
/plugin install cc-usage
```

### 2. Manual

바이너리를 다운로드하거나 직접 빌드한 뒤, Claude Code settings에 등록한다.

#### Build

```bash
# 로컬 빌드 (dist/)
make build-local

# 크로스 컴파일 (bin/ — darwin/arm64, darwin/amd64, linux/amd64, windows/amd64)
make build
```

#### Register

`~/.claude/settings.json`에 추가:

```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/bin/run.sh"
  }
}
```

Windows에서 직접 바이너리를 지정할 경우:

```json
{
  "statusLine": {
    "type": "command",
    "command": "C:/path/to/bin/cc-usage-windows-amd64.exe"
  }
}
```

커스텀 프로필 사용 시:

```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/cc-usage --config ~/.claude-triptopaz/cc-usage.json"
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

## License

MIT
