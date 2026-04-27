# cc-usage

Claude Code status line plugin. 모델, 컨텍스트 사용량, 비용, rate limit, 프로젝트 정보를 status line에 표시한다.

```text
my-project (main) | ◆ claude-opus-4-7 | 30% 60K | $1.25 | 5h: 42% | 7d: 69%
```

## Features

- Zero dependency: Go 표준 라이브러리만 사용
- 크로스 플랫폼 지원: macOS, Linux, Windows
- 모듈형 위젯 시스템
- 8개 컬러 테마: `default`, `minimal`, `catppuccin`, `dracula`, `gruvbox`, `nord`, `tokyoNight`, `solarized`
- 다국어 지원: English, 한국어
- Display mode: `compact`, `normal`, `detailed`, `custom`
- `lines` 또는 `preset`으로 위젯 순서와 줄 배치 변경 가능

## Installation

### Plugin Marketplace

```bash
/plugin marketplace add zipkero/cc-usage
/plugin install cc-usage
/cc-usage:cc-usage-install
/reload-plugins
```

### Manual

소스를 직접 빌드해서 Claude Code settings에 등록할 수 있다.

```bash
git clone https://github.com/zipkero/cc-usage.git
cd cc-usage
make build-local
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

Windows에서는 경로에 forward slash를 사용한다.

```json
{
  "statusLine": {
    "type": "command",
    "command": "C:/Users/you/path/to/cc-usage.exe"
  }
}
```

커스텀 프로필을 사용할 때는 `--config`로 해당 프로필의 설정 파일을 지정한다.

```json
{
  "statusLine": {
    "type": "command",
    "command": "C:/path/to/cc-usage.exe --config C:/Users/you/.claude-triptopaz/cc-usage.json"
  }
}
```

## Configuration

설정 파일 기본 경로는 `~/.claude/cc-usage.json`이다. `--config`를 사용하면 다른 경로를 지정할 수 있다.

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

| Field | Default | Description |
|---|---:|---|
| `language` | `"auto"` | `"auto"`, `"en"`, `"ko"` |
| `plan` | `"max"` | `"pro"`, `"max"` |
| `displayMode` | `"compact"` | `"compact"`, `"normal"`, `"detailed"`, `"custom"` |
| `theme` | `"default"` | 사용할 컬러 테마 |
| `separator` | `"pipe"` | `"pipe"`, `"space"`, `"dot"`, `"arrow"` |
| `dailyBudget` | - | 일일 예산 USD |
| `disabledWidgets` | `[]` | 비활성화할 위젯 ID 목록 |
| `lines` | - | `custom` 모드에서 사용할 위젯 줄 배치 |
| `preset` | - | 위젯 단축 문자열. 지정하면 `lines`로 변환되고 `custom` 모드로 동작 |
| `cache.ttlSeconds` | `300` | rate limit API 응답 캐시 TTL |

## Layout

`displayMode`가 `custom`이고 `lines`가 있으면, 위젯은 `lines`에 적힌 순서와 줄 위치 그대로 렌더링된다.

```json
{
  "displayMode": "custom",
  "separator": "space",
  "lines": [
    ["model", "projectInfo", "cost"],
    ["rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"]
  ]
}
```

`projectInfo`도 일반 위젯과 동일하게 배치된다. 예를 들어 아래 설정은 프로젝트 정보를 맨 뒤에 표시한다.

```json
{
  "displayMode": "custom",
  "lines": [
    ["model", "context", "cost", "projectInfo"]
  ]
}
```

`preset`은 짧은 문자열로 위젯 배치를 지정하는 방식이다. `|`는 줄바꿈을 의미한다.

```json
{
  "preset": "MC$P|R7S"
}
```

위 예시는 첫 줄에 `model`, `context`, `cost`, `projectInfo`를 표시하고, 둘째 줄에 `rateLimit5h`, `rateLimit7d`, `rateLimit7dSonnet`을 표시한다.

주요 preset 문자:

| Char | Widget |
|---|---|
| `M` | `model` |
| `C` | `context` |
| `$` | `cost` |
| `R` | `rateLimit5h` |
| `7` | `rateLimit7d` |
| `S` | `rateLimit7dSonnet` |
| `P` | `projectInfo` |
| `D` | `sessionDuration` |
| `B` | `burnRate` |
| `O` | `todoProgress` |
| `V` | `version` |

## Widgets

### Core

| ID | Description |
|---|---|
| `model` | 모델명 |
| `context` | 컨텍스트 사용률과 토큰 수 |
| `cost` | 현재 세션 비용 |
| `rateLimit5h` | 5시간 rate limit |
| `rateLimit7d` | 7일 rate limit |
| `rateLimit7dSonnet` | 7일 Sonnet rate limit |
| `projectInfo` | 디렉터리, git branch, ahead/behind, worktree, subpath |

### Additional

`sessionDuration`, `burnRate`, `cacheHit`, `tokenSpeed`, `todoProgress`, `toolActivity`, `agentStatus`, `configCounts`, `performance`, `tokenBreakdown`, `forecast`, `budget`, `todayCost`, `linesChanged`, `outputStyle`, `version`, `peakHours` 등이 있다.

## Troubleshooting

### Idle 중 `projectInfo`가 사라지는 경우

Claude Code는 status line을 주기적으로 갱신하지 않고 이벤트 기반으로 호출한다. Idle 상태에서 `workspace.current_dir`가 비어 있는 stdin이 들어오면 `projectInfo`가 생략될 수 있다.

이를 완화하기 위해 같은 세션 identity가 확인되는 경우에만 최근 세션 캐시에서 workspace, model, usage 필드를 복원한다. `session_id`, `remote.session_id`, `agent_id`, `transcript_path`, `workspace.current_dir` 등 identity가 전혀 없는 입력은 다른 세션과 섞일 수 있으므로 렌더링하지 않는다.

`DEBUG=cc-usage` 또는 `DEBUG=1`을 설정하면 stdin identity와 캐시 복원 여부를 stderr에서 확인할 수 있다.

### 플러그인 업데이트 중 SSH 인증 오류

```text
git@github.com: Permission denied (publickey).
fatal: Could not read from remote repository.
```

플러그인 업데이트 과정에서 SSH URL로 clone을 시도하면 발생할 수 있다. git 전역 설정으로 GitHub SSH URL을 HTTPS로 우회할 수 있다.

```bash
git config --global url."https://github.com/".insteadOf "git@github.com:"
```

## Privacy

cc-usage는 외부 분석 서버로 데이터를 전송하지 않는다.

- 입력: Claude Code가 stdin으로 전달하는 세션 정보만 읽는다.
- 네트워크: OAuth 토큰으로 Anthropic 공식 API(`api.anthropic.com`)를 호출해 rate limit을 조회한다.
- 저장: `~/.cache/cc-usage/`에 rate limit 응답과 세션 스냅샷을 로컬 캐시한다.
- 텔레메트리: 없음.

## License

MIT
