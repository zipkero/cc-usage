# cc-usage

Claude Code 프롬프트 위 band에 프로젝트·모델·컨텍스트·비용·rate limit을 표시하는 Claude Code [mod](https://code.claude.com/docs/en/plugins/mods/overview)다.

```
cc-usage (main) │ ◆ claude-opus-5-5 │ ██░░░░░░ 30% 60K │ $1.25 │ 5h: 7% (2h25m) │ 7d: 23% (4d 5h)
```

> Go로 만든 이전 status line 버전(v0.5.x)은 [`deprecated-go`](https://github.com/zipkero/cc-usage/tree/deprecated-go) 브랜치에 보존돼 있으며 더 이상 갱신하지 않는다.

## 요구 사항

- Claude Code v2.1.287 이상 (터미널, Claude 데스크톱 앱 Code 탭). mod가 그리는 band는 VS Code 확장 채팅 패널과 `claude -p`에는 나타나지 않는다.
- 조직 설정의 `allowManagedModsOnly`나 `disableAllHooks`가 켜져 있으면 로드되지 않는다.

## 설치

```bash
/plugin marketplace add zipkero/cc-usage
/plugin install cc-usage
/reload-plugins
```

### Go 버전(v0.5.x)에서 넘어오는 경우

Go 버전은 `settings.json`의 `statusLine`에 바이너리 경로를 기록했다.
v0.6.0부터 그 바이너리가 배포되지 않으므로, `settings.json`에서 `cc-usage`를 가리키는 `statusLine` 항목을 지운다.
지우지 않으면 status line 칸이 비어 보인다.

## 표시 항목

| 위젯 ID | 문자 | 표시 | 출처 | 비고 |
|---------|------|------|------|------|
| `projectInfo` | `P` | 프로젝트 경로 (브랜치) | `$.session.root()`, `git branch --show-current` | 홈은 `~`로 줄이고, 50자를 넘으면 `~/…/cc-usage`처럼 줄인다 |
| `projectName` | `N` | 프로젝트 디렉토리 이름 (브랜치) | `$.session.root()`, `git branch --show-current` | 셸에서 `cd`해도 세션 root 기준이다 |
| `repoInfo` | `G` | 저장소 `owner/name` | `$.session.repo()` | origin remote가 없거나 읽을 수 없으면 생략 |
| `model` | `M` | 모델 id | `$.session.model()` | 앞 기호: opus `◆`, sonnet `◇`, haiku `○`, fable `◈`, mythos `◎`, 그 밖 `●` |
| `context` | `C` | 컨텍스트 막대·퍼센트·토큰 수 | `$.session.usage().context` | 첫 응답 전에는 빈 막대와 `-` |
| `cost` | `$` | 세션 누적 비용 | `$.session.usage().cost` | |
| `rateLimit5h` | `R` | 5시간 rate limit | `$.session.usage().rateLimits` | 구독·gateway 사용자만. 리셋까지 남은 시간 포함 |
| `rateLimit7d` | `7` | 7일 rate limit | 위와 같음 | 위와 같음 |
| `spendLimit` | `L` | spend limit | 위와 같음 | 위와 같음 |

데이터가 없는 위젯은 생략한다.
퍼센트는 50% 이하 safe, 80% 이하 warning, 그 초과 danger 색이다.
줄이 band 폭보다 길면 오른쪽 위젯부터 빼고, 위젯 하나만 남아도 넘치면 끝을 `…`로 자른다.

턴이 끝날 때와 rate limit이 움직일 때 갱신하고, 세션이 쉬는 동안에는 30초마다 갱신한다.

## 설정

`settings.json`의 `pluginConfigs`에 적는다.

```json
"pluginConfigs": {
  "cc-usage@zipkero-cc-usage": {
    "options": {
      "layout": "NMC$|R7L",
      "theme": "default"
    }
  }
}
```

| 키 | 기본값 | 설명 |
|----|--------|------|
| `layout` | `NMC$R7L` | 줄마다 그릴 위젯. 줄은 `\|`로, 줄 안의 토큰은 쉼표나 공백으로 나눈다. 토큰이 위젯 ID면 그 위젯이고, 아니면 글자마다 위 표의 문자로 읽는다. 모르는 글자는 무시하고, 결과가 비면 기본값을 쓴다. 예: `PMC$\|R7L`, `projectInfo, model \| rateLimit5h rateLimit7d` |
| `disabledWidgets` | (없음) | `layout`에 있어도 숨길 위젯. 문자나 위젯 ID를 쉼표·공백·`\|`로 나눠 적는다. 예: `$, rateLimit7d` |
| `theme` | `default` | `default`, `minimal`, `catppuccin`, `dracula`, `gruvbox`, `nord`, `tokyoNight`, `solarized` |
| `separator` | `pipe` | 위젯 사이 구분자. `pipe`(`│`), `dot`(`·`), `arrow`(`›`), `space`(공백 두 칸) |
| `language` | `auto` | rate limit 라벨과 남은 시간의 언어. `en`(5h, 7d, 1d 2h), `ko`(5시간, 7일, 1일 2시간). `auto`는 `LC_ALL`·`LC_MESSAGES`·`LANG`이 `ko`로 시작하면 한국어다 |
| `contextBarWidth` | `8` | 컨텍스트 막대 칸 수. 1–40 정수이고, 범위 밖이나 소수면 8 |

테마 색은 어두운 배경 기준으로 골랐다.
밝은 배경에서는 `default` 같은 파스텔 색이 흐리게 보일 수 있다.

## 개발

```bash
claude --plugin-dir ./src       # 저장하면 hot reload
claude plugin validate ./src    # 정적 검증
tsc -p src --noEmit             # 타입 검사 (처음 로드할 때 생성되는 타입 선언 필요)
claude plugin test ./src        # src/tests/*.test.ts 실행
```

## License

MIT
