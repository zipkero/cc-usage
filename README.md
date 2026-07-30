# cc-usage

Claude Code status line plugin. 모델, 컨텍스트 사용량, 비용, rate limit 등을 status line에 표시한다.

```
my-project (main) │ ◆ Opus │ ██░░░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%
```

> **브랜치 안내**: 이 `main` 브랜치는 소스 코드와 개발 환경입니다. 마켓플레이스 배포는 [`release` 브랜치](https://github.com/zipkero/cc-usage/tree/release)(GitHub default)에서 이뤄지며 미리 빌드된 바이너리만 포함합니다. 단순 설치만 원한다면 아래 *Plugin Marketplace* 또는 release 브랜치의 README를 참고하세요.

## Features

- Zero dependency (Go 표준 라이브러리만 사용)
- 크로스 플랫폼 (macOS, Linux, Windows)
- 모듈러 위젯 시스템
- 8개 컬러 테마 (default, minimal, catppuccin, dracula, gruvbox, nord, tokyoNight, solarized)
- 다국어 지원 (English, 한국어)
- Display mode: compact, custom

## Installation

### Plugin Marketplace (권장)

```bash
# 1. marketplace 등록
/plugin marketplace add zipkero/cc-usage

# 2. 플러그인 설치
/plugin install cc-usage

# 3. status line 설정 적용 — 반드시 이 단계를 거치세요
/cc-usage:cc-usage-install

# 4. 적용
/reload-plugins
```

> **업데이트 시 경로가 깨지지 않게 하려면 위 3단계를 반드시 실행하세요.**
> Claude Code의 `/plugin install`은 settings.json에 버전 포함 경로
> (`~/.claude/plugins/cache/zipkero-cc-usage/cc-usage/<VERSION>/bin/run.sh`)를
> 써넣는 경우가 있는데, 이러면 다음 `/plugin update`마다 settings.json을 손으로
> 고쳐야 합니다. `/cc-usage:cc-usage-install`은 버전 segment가 없는 안정 경로
> (`~/.claude/plugins/marketplaces/zipkero-cc-usage/bin/...`)로 자동 교체합니다.
> 이 디렉터리는 `/plugin update` 시 in-place로 git pull되어 settings.json은 손댈 필요 없습니다.

### Manual (소스 빌드)

이 `main` 브랜치를 클론하고 직접 빌드한 뒤 settings에 등록한다. Go 1.20+ 필요.

```bash
git clone --branch main https://github.com/zipkero/cc-usage.git
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

설정 파일: `~/.claude/cc-usage.json` (또는 `--config`로 지정).
`CLAUDE_CONFIG_DIR`로 config 홈을 옮긴 환경에서는 그 아래의 `cc-usage.json`을 따른다
(env 미설정 시 `~/.claude`).

```json
{
  "language": "auto",
  "displayMode": "compact",
  "theme": "default",
  "separator": "pipe",
  "disabledWidgets": [],
  "widgets": { "context": { "barWidth": 8 } }
}
```

| 필드 | 기본값 | 설명 |
|------|--------|------|
| `language` | `"auto"` | `"auto"`, `"en"`, `"ko"` |
| `displayMode` | `"compact"` | `"compact"` 또는 `"custom"` (custom은 `preset`/`lines` 정의 시 자동 적용) |
| `theme` | `"default"` | 8개 테마 중 선택 |
| `separator` | `"pipe"` | `"pipe"`, `"space"`, `"dot"`, `"arrow"` |
| `disabledWidgets` | `[]` | 비활성화할 위젯 ID 목록 |
| `preset` | - | 위젯 단축 문자열. 한 글자 = 한 위젯, `\|`로 줄 구분 (예: `"PMC$R"`, `"NMC$R"`) |
| `lines` | - | 위젯 ID 배열의 배열로 직접 레이아웃 정의 (preset 대안) |
| `widgets` | - | 위젯별 옵션 (네임스페이스). 아래 *위젯 옵션* 참고 |

### 위젯 옵션 (`widgets`)

| 경로 | 기본값 | 설명 |
|------|--------|------|
| `widgets.context.barWidth` | `8` | context 프로그레스바 폭(칸 수). 허용 범위 `1`~`40`, 벗어나면 경고 후 기본값 사용 |

## Widgets

### Core

| ID | preset char | 설명 |
|----|:-:|------|
| `model` | `M` | 모델명 + 아이콘 (◆Opus/◇Sonnet/○Haiku) |
| `context` | `C` | 프로그레스바 + 사용률 + 토큰 수 (첫 응답 전에는 빈 bar + `-`로 표시) |
| `cost` | `$` | 세션 비용 |
| `rateLimit5h` | `R` | 5시간 rate limit (첫 응답 전에는 `5h: -`로 자리를 지키며, rate limit 데이터가 오지 않는 계정에서는 첫 응답 이후 칸이 생략됨) |
| `rateLimit7d` | `7` | 7일 rate limit (첫 응답 전에는 `7d: -`로 자리를 지키며, rate limit 데이터가 오지 않는 계정에서는 첫 응답 이후 칸이 생략됨) |
| `projectInfo` | `P` | 디렉토리 + git branch |
| `projectName` | `N` | 현재 디렉토리 base name + git branch |

> **preset 예시**: `"PMC$R"` → `projectInfo │ model │ context │ cost │ rateLimit5h`.
> `"NMC$R"` → `projectName │ model │ context │ cost │ rateLimit5h`.
> `disabledWidgets`로 일부만 꺼서 라인을 단순화할 수 있습니다.

## Troubleshooting

### Idle 시 `projectInfo`가 가끔 사라지는 경우

Claude Code는 주기적으로 status line을 갱신하지 않고 이벤트 기반으로만 호출한다.
`workspace.current_dir`가 비어있는 stdin이 오면 cc-usage는 `CLAUDE_PROJECT_DIR` 또는 현재 작업
디렉터리를 사용해 `projectInfo` 또는 `projectName`을 표시한다. 둘 다 확인할 수 없으면 이전
값을 복원하지 않고 위젯을 생략한다.

### 플러그인 업데이트 시 SSH 인증 오류 (Windows)

```
git@github.com: Permission denied (publickey).
fatal: Could not read from remote repository.
```

플러그인 업데이트 과정에서 SSH URL로 clone을 시도하면서 발생할 수 있다. git 글로벌 설정으로 SSH를 HTTPS로 우회하면 해결된다.

```bash
git config --global url."https://github.com/".insteadOf "git@github.com:"
```

### stdin 섹션 하나가 예상과 다른 값으로 올 때의 표시 동작

사라지지 않는다. 관용 단위는 **최상위 섹션**이다(`model`, `workspace`, `context_window`, `cost`,
`rate_limits` 등). stdin의 섹션 하나가 예상과 다른 형태로 와도 그 섹션에서 나오는 칸만 빠지고
나머지 섹션의 칸은 그대로 표시된다 — 예를 들어 `context_window`가 깨지면 context 칸만 생략되고
model·cost 칸은 그대로 나온다. 섹션 안의 중첩 필드 하나만 깨진 경우에도 그 섹션 전체가 버려진다
— `rate_limits.five_hour.used_percentage`에 숫자가 아닌 값이 오면 5h·7d 두 칸이 함께 빠지며,
살아남은 절반으로 `5h: 0%` 같은 값을 만들어 보여주지 않는다. status line에는 실패를 알리는 문자나 마커가 전혀 나타나지 않는다.

위젯마다 "빠진 자리"의 형태가 다를 수 있다는 점은 참고할 것. `cost` 섹션이 깨지면 칸이 생략되지
않고 `$0.00`으로 표시되며, `context_window`가 깨지면 context 칸은 생략되지만 rate limit 데이터가
오지 않는 계정에서는 5h·7d 칸이 `5h: -`처럼 placeholder로 나타난다(§Widgets 참고).

어느 섹션이 버려졌는지는 `DEBUG=cc-usage`로 실행하면 stderr에서 확인할 수 있다. 알려진 섹션 목록에
없는 최상위 키가 섞여 있어도 함께 기록된다.

```bash
DEBUG=cc-usage /path/to/dist/cc-usage < payload.json
```

단, **최상위 JSON 자체**가 구문 오류이거나 객체가 아닌 경우는 예외다 — 이때는 섹션 단위 관용의
대상이 아니라 기존과 같이 아무것도 출력되지 않는다.

## Privacy

cc-usage는 외부 서버로 데이터를 전송하지 않는다.

- **입력**: Claude Code가 stdin으로 넘겨주는 세션 정보(model, context, cost, workspace path 등)만 읽는다.
- **네트워크**: cc-usage는 네트워크 엔드포인트에 접속하지 않는다.
- **저장**: status line 렌더 중 별도 캐시 파일을 읽거나 쓰지 않는다. 예전 버전이 만든
  `~/.cache/cc-usage/` 파일은 더 이상 사용하지 않으며 사용자가 직접 삭제할 수 있다.
- **git**: project 위젯 표시를 위해 현재 작업 디렉터리에서 `git status --porcelain=v2 --branch`를 실행할 수 있다.
- **텔레메트리**: 없음.

## 문서

- [ROADMAP.md](./ROADMAP.md) — 최종 결과물, 서비스 완료 기준, 마일스톤, 보류 범위
- [CLAUDE.md](./CLAUDE.md) — 아키텍처, 위젯 추가 절차, 빌드·배포 절차

## License

MIT
