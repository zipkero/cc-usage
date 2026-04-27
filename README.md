# cc-usage

Claude Code status line plugin. Claude Code가 status line command에 전달하는 stdin JSON을 읽어 모델, 프로젝트, 컨텍스트 사용량, 비용, rate limit 정보를 한 줄 또는 여러 줄로 렌더링한다.

현재 버전: `0.1.7`

```text
C:\Users\you\project (main)  ◆ claude-opus-4-7  $1.23  30% 60K  5h: 42%  7d: 69%
```

## Features

- Go 표준 라이브러리 기반, 별도 런타임 의존성 없음
- macOS, Linux, Windows 빌드 지원
- `compact`, `normal`, `detailed`, `custom` display mode 지원
- `lines` 또는 `preset`으로 위젯 순서와 줄 배치 변경 가능
- `projectInfo`도 일반 위젯처럼 원하는 위치에 배치 가능
- Claude OAuth credential을 이용한 rate limit API fallback 지원
- idle 또는 degraded stdin 입력 시 최근 정상 렌더를 캐시에서 복원
- English / Korean locale 지원

## Installation

### Plugin Marketplace

```bash
/plugin marketplace add zipkero/cc-usage
/plugin install cc-usage
/cc-usage:cc-usage-install
/reload-plugins
```

### Manual

소스를 직접 빌드해서 Claude Code `settings.json`의 `statusLine.command`에 등록할 수 있다.

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

Windows에서는 forward slash 경로를 사용하는 것이 안전하다.

```json
{
  "statusLine": {
    "type": "command",
    "command": "C:/Users/you/path/to/cc-usage.exe"
  }
}
```

별도 Claude Code 프로필을 사용한다면 `--config`로 해당 프로필의 설정 파일을 지정한다.

```json
{
  "statusLine": {
    "type": "command",
    "command": "C:/path/to/cc-usage.exe --config C:/Users/you/.claude-triptopaz/cc-usage.json"
  }
}
```

## Configuration

기본 설정 파일 경로는 `~/.claude/cc-usage.json`이다. `--config`를 사용하면 다른 경로를 사용할 수 있다.

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
| `theme` | `"default"` | ANSI 색상 테마 |
| `separator` | `"pipe"` | `"pipe"`, `"space"`, `"dot"`, `"arrow"` |
| `dailyBudget` | - | 일일 예산 USD. 현재 코어 위젯에는 표시되지 않음 |
| `disabledWidgets` | `[]` | 비활성화할 위젯 ID 목록 |
| `lines` | - | `custom` 모드에서 사용할 위젯 줄 배치 |
| `preset` | - | 짧은 문자 조합으로 위젯 배치 지정. 지정 시 `lines`로 변환되고 `custom` 모드로 동작 |
| `cache.ttlSeconds` | `300` | rate limit API 응답 캐시 TTL |

## Layout

`displayMode`가 `custom`이고 `lines`가 있으면 위젯은 `lines` 배열의 순서와 줄 위치 그대로 렌더링된다.

```json
{
  "displayMode": "custom",
  "separator": "space",
  "lines": [
    ["projectInfo"],
    ["model", "cost", "context", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"]
  ]
}
```

`projectInfo`는 고정 위치가 아니라 일반 위젯이다. 아래처럼 마지막 위치에도 둘 수 있다.

```json
{
  "displayMode": "custom",
  "lines": [
    ["model", "context", "cost", "projectInfo"]
  ]
}
```

`projectInfo` 표시 방식은 설정된 줄 수에 따라 다르다.

- 설정상 한 줄이면 프로젝트 디렉터리 이름만 표시한다.
- 설정상 여러 줄이면 전체 경로를 표시한다.

예를 들어 위젯 전체가 한 줄이면 `cc-usage`처럼 짧게 표시되고, `projectInfo`를 별도 줄로 분리하면 `C:\Users\you\GolandProjects\cc-usage`처럼 전체 경로가 표시된다.

`preset`은 짧은 문자로 위젯 배치를 지정하는 방식이다. `|`는 줄바꿈을 의미한다.

```json
{
  "preset": "MC$P|R7S"
}
```

위 예시는 첫 줄에 `model`, `context`, `cost`, `projectInfo`를 표시하고, 둘째 줄에 `rateLimit5h`, `rateLimit7d`, `rateLimit7dSonnet`을 표시한다.

현재 구현된 주요 preset 문자:

| Char | Widget |
|---|---|
| `M` | `model` |
| `C` | `context` |
| `$` | `cost` |
| `R` | `rateLimit5h` |
| `7` | `rateLimit7d` |
| `S` | `rateLimit7dSonnet` |
| `P` | `projectInfo` |

등록되지 않은 위젯 ID나 아직 구현되지 않은 preset 문자는 렌더링 시 건너뛴다.

## Widgets

현재 등록되어 실제 렌더링되는 위젯은 다음과 같다.

| ID | Description |
|---|---|
| `projectInfo` | 현재 디렉터리, git branch, ahead/behind, worktree, subpath |
| `model` | Claude 모델명 |
| `context` | 컨텍스트 사용률과 토큰 수 |
| `cost` | 현재 세션 비용 |
| `rateLimit5h` | 5시간 rate limit |
| `rateLimit7d` | 7일 rate limit |
| `rateLimit7dSonnet` | 7일 Sonnet rate limit. API 전용 fallback |

## Idle / Degraded Input

Claude Code는 status line을 주기적으로 고정 갱신하지 않고 이벤트 기반으로 호출한다. 오래 idle 상태였다가 다시 호출될 때 `workspace.current_dir`, `model`, `context_window`가 비어 있는 stdin이 들어올 수 있다.

`0.1.7`부터는 이 경우 status line이 지워지지 않도록 최근 정상 렌더를 캐시에 저장한다.

- 정상 렌더가 2개 이상 위젯을 출력하면 `~/.cache/cc-usage/session-state-*.json`에 stdin snapshot과 `last_output`을 저장한다.
- `session_id`, `remote.session_id`, `agent_id`, `transcript_path`, `workspace.current_dir` 순서로 세션 캐시 키를 만든다.
- 정상 렌더 시 현재 작업 디렉터리 기준 `cwd-*` 보조 캐시도 함께 저장한다.
- identity가 전혀 없는 stdin이 들어와도 같은 cwd 캐시가 있으면 빈 stdout 대신 마지막 정상 렌더를 다시 출력한다.
- 캐시는 24시간 동안 유효하다.
- 전역 최신 세션 fallback은 사용하지 않는다. 서로 다른 프로젝트나 여러 세션 출력이 섞이는 것을 피하기 위해서다.

이 동작은 `main_integration_test.go`의 통합 테스트로 검증한다.

## Cache

저장 위치는 모두 로컬이다.

| Cache | Path | Purpose |
|---|---|---|
| Rate limit API | `~/.cache/cc-usage/cache-*.json` | Anthropic API rate limit 응답 캐시 |
| Session state | `~/.cache/cc-usage/session-state-*.json` | 마지막 정상 stdin snapshot, 위젯 수, 마지막 렌더 문자열 |
| Session duration | `~/.cache/cc-usage/sessions/*.json` | 세션 시작 시간 추적 |

캐시 파일은 lock 파일을 사용해 동시 접근을 직렬화한다.

## Troubleshooting

### Idle 후 status line이 사라지는 경우

`0.1.7` 이상인지 먼저 확인한다. `~/.claude-triptopaz` 같은 별도 프로필을 쓰는 경우, `settings.json`의 `statusLine.command`가 실제로 최신 플러그인 캐시 경로를 가리키는지 확인한다.

```json
{
  "statusLine": {
    "command": "C:/Users/you/.claude-triptopaz/plugins/cache/zipkero-cc-usage/cc-usage/0.1.7/bin/cc-usage-windows-amd64.exe --config C:/Users/you/.claude-triptopaz/cc-usage.json"
  }
}
```

`DEBUG=cc-usage` 또는 `DEBUG=1`을 설정하면 stdin identity, cache key, 복원 여부를 stderr에서 확인할 수 있다.

### 플러그인 업데이트 중 SSH 인증 오류

```text
git@github.com: Permission denied (publickey).
fatal: Could not read from remote repository.
```

플러그인 업데이트 과정에서 SSH URL로 clone을 시도하면 발생할 수 있다. 전역 git 설정으로 GitHub SSH URL을 HTTPS로 우회할 수 있다.

```bash
git config --global url."https://github.com/".insteadOf "git@github.com:"
```

## Development

테스트:

```bash
go test ./...
```

Windows PowerShell에서 전체 플랫폼 빌드:

```powershell
$version = "0.1.7"
$platforms = @(
  @("darwin", "arm64"),
  @("darwin", "amd64"),
  @("linux", "amd64"),
  @("windows", "amd64")
)
foreach ($p in $platforms) {
  $env:GOOS = $p[0]
  $env:GOARCH = $p[1]
  $ext = if ($p[0] -eq "windows") { ".exe" } else { "" }
  go build -ldflags="-s -w -X main.version=$version" -o "bin/cc-usage-$($p[0])-$($p[1])$ext" .
}
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
```

## Privacy

cc-usage는 임의 분석 서버로 데이터를 전송하지 않는다.

- 입력: Claude Code가 stdin으로 전달하는 status line JSON만 읽는다.
- 네트워크: OAuth credential이 있으면 Anthropic API에서 rate limit 정보를 조회한다.
- 저장: `~/.cache/cc-usage/`에 rate limit 응답과 세션 snapshot을 로컬 캐시한다.
- telemetry 없음.

## License

MIT
