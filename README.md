# cc-usage

Claude Code status line plugin. 모델, 컨텍스트 사용량, 비용, rate limit 등을 status line에 표시한다.

```
my-project (main) │ ◆ Opus │ ████░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%
```

> **브랜치 안내**: 이 `master` 브랜치는 소스 코드와 개발 환경입니다. 마켓플레이스 배포는 [`release` 브랜치](https://github.com/zipkero/cc-usage/tree/release)(GitHub default)에서 이뤄지며 미리 빌드된 바이너리만 포함합니다. 단순 설치만 원한다면 아래 *Plugin Marketplace* 또는 release 브랜치의 README를 참고하세요.

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

### Manual (소스 빌드)

이 `master` 브랜치를 클론하고 직접 빌드한 뒤 settings에 등록한다. Go 1.20+ 필요.

```bash
git clone --branch master https://github.com/zipkero/cc-usage.git
cd cc-usage
make build-local   # dist/cc-usage 생성
```

> **빌드 없이 미리 빌드된 바이너리만** 원한다면 release 브랜치에서 받으세요: `git clone --branch release --depth 1 https://github.com/zipkero/cc-usage.git`. 자세한 절차는 [release 브랜치 README](https://github.com/zipkero/cc-usage/tree/release#manual)에 있습니다.

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
| `displayMode` | `"compact"` | `"compact"` 또는 `"custom"` (custom은 `preset`/`lines` 정의 시 자동 적용) |
| `theme` | `"default"` | 8개 테마 중 선택 |
| `separator` | `"pipe"` | `"pipe"`, `"space"`, `"dot"`, `"arrow"` |
| `dailyBudget` | - | 일일 예산 (USD, 현재 미사용) |
| `disabledWidgets` | `[]` | 비활성화할 위젯 ID 목록 |
| `preset` | - | 위젯 단축 문자열. 한 글자 = 한 위젯, `\|`로 줄 구분 (예: `"PMC$R\|VaDBHF"`) |
| `lines` | - | 위젯 ID 배열의 배열로 직접 레이아웃 정의 (preset 대안) |

## Widgets

### Core

| ID | preset char | 설명 |
|----|:-:|------|
| `model` | `M` | 모델명 + 아이콘 (◆Opus/◇Sonnet/○Haiku) |
| `context` | `C` | 프로그레스바 + 사용률 + 토큰 수 |
| `cost` | `$` | 세션 비용 |
| `rateLimit5h` | `R` | 5시간 rate limit |
| `rateLimit7d` | `7` | 7일 rate limit |
| `rateLimit7dSonnet` | `S` | 7일 Sonnet rate limit |
| `projectInfo` | `P` | 디렉토리 + git branch (+ worktree, subpath) |

### Analytics (옵션 — preset/lines로 노출)

stdin payload에서 바로 계산되는 값들. compact 모드에는 포함되지 않으며, 위 `preset` char를 조합해서 노출시킵니다.

| ID | preset char | 설명 |
|----|:-:|------|
| `version` | `V` | Claude Code 앱 버전 (`v2.1.150`) |
| `apiDuration` | `a` | 누적 API 호출 시간 (`API: 14m`) |
| `sessionDuration` | `D` | 세션 누적 시간 (`Session: 45m`) |
| `burnRate` | `B` | 시간당 비용 (`Burn: $5.69/h`) |
| `cacheHit` | `H` | 마지막 턴 캐시 히트율 (`Cache: 93%`) |
| `performance` | `F` | API time / 전체 세션 time 비율 (`Perf: 32%`) |

> **preset 예시**: `"PMC$R\|VaDBHF"` → 1줄 `projectInfo │ model │ context │ cost │ rateLimit5h`, 2줄 `version │ apiDuration │ sessionDuration │ burnRate │ cacheHit │ performance`. `disabledWidgets`로 일부만 꺼서 라인을 단순화할 수 있습니다.

## Troubleshooting

### Idle 시 `projectInfo`가 가끔 사라지는 경우

Claude Code는 주기적으로 status line을 갱신하지 않고 이벤트 기반으로만 호출한다. Idle 상태에서 `workspace.current_dir`가 비어있는 stdin이 오면 `projectInfo` 위젯이 생략될 수 있다.

이를 완화하기 위해 직전 렌더의 workspace/worktree를 로컬 세션 캐시(`~/.cache/cc-usage/session-state-*.json`)에서 복원한다. 단, **30초 이내**의 캐시만 사용한다 — 사용자가 `cd`로 디렉터리를 옮긴 뒤 긴 idle이 발생했을 때 이전 경로가 고착되는 것을 막기 위함이다. 30초를 초과한 idle에서는 복원하지 않고 위젯을 생략한다.

### 플러그인 업데이트 시 SSH 인증 오류 (Windows)

```
git@github.com: Permission denied (publickey).
fatal: Could not read from remote repository.
```

플러그인 업데이트 과정에서 SSH URL로 clone을 시도하면서 발생할 수 있다. git 글로벌 설정으로 SSH를 HTTPS로 우회하면 해결된다.

```bash
git config --global url."https://github.com/".insteadOf "git@github.com:"
```

## Privacy

cc-usage는 외부 서버로 데이터를 전송하지 않는다.

- **입력**: Claude Code가 stdin으로 넘겨주는 세션 정보(model, context, cost, workspace path 등)만 읽는다.
- **네트워크**: OAuth 토큰(`~/.claude/.credentials.json`)으로 Anthropic 공식 API(`api.anthropic.com`)만 호출하여 rate limit을 조회한다. 제3자 서버나 애널리틱스는 사용하지 않는다.
- **저장**: `~/.cache/cc-usage/`에 rate limit 응답과 세션 스냅샷을 로컬 캐시한다. 사용자가 직접 삭제할 수 있다.
- **텔레메트리**: 없음.

## License

MIT
