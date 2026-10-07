# cc-usage

Claude Code 프롬프트 위 band에 프로젝트·모델·컨텍스트·비용·rate limit을 표시하는 Claude Code [mod](https://code.claude.com/docs/en/plugins/mods/overview)다.

```
cc-usage (main) │ ◆ claude-opus-5-5 │ ██░░░░░░ 30% 60K │ $1.25 │ 5h: 7% (2h25m) │ 7d: 23% (4d 5h)
```

> Go로 만든 이전 status line 버전(v0.5.x)은 [`deprecated-go`](https://github.com/zipkero/cc-usage/tree/deprecated-go) 브랜치에 보존돼 있으며 더 이상 갱신하지 않는다.

## 요구 사항

- Claude Code v2.1.287 이상 (터미널). mod가 그리는 band는 VS Code 확장 채팅 패널과 `claude -p`에는 나타나지 않는다.
- 조직 설정의 `allowManagedModsOnly`나 `disableAllHooks`가 켜져 있으면 로드되지 않는다.

## 설치

```bash
/plugin marketplace add zipkero/cc-usage
/plugin install cc-usage
/reload-plugins
```

### Go 버전(v0.5.x)에서 넘어오는 경우

Go 버전은 `settings.json`의 `statusLine`에 바이너리 경로를 기록했다.
이 버전부터 그 바이너리가 배포되지 않으므로, `settings.json`에서 `cc-usage`를 가리키는 `statusLine` 항목을 지운다.
지우지 않으면 status line 칸이 비어 보인다.

## 표시 항목

| 항목 | 출처 | 비고 |
|------|------|------|
| 프로젝트 (브랜치) | `$.session.cwd()`, `git branch --show-current` | 브랜치는 git 실행 결과 |
| 모델 | `$.session.model()` | 모델 id |
| 컨텍스트 | `$.session.usage().context` | 첫 응답 전에는 `ctx: -` |
| 비용 | `$.session.usage().cost` | |
| 5h / 7d / spend | `$.session.usage().rateLimits` | 구독·gateway 사용자만. 리셋까지 남은 시간 포함 |

턴이 끝날 때와 rate limit이 움직일 때 갱신하고, 세션이 쉬는 동안에는 30초마다 갱신한다.

## 개발

```bash
claude --plugin-dir ./src       # 저장하면 hot reload
claude plugin validate ./src    # 정적 검증
tsc -p src --noEmit             # 타입 검사 (처음 로드할 때 생성되는 타입 선언 필요)
```

## License

MIT
