<!-- prowl-workflow: v1 -->
# mod-feature-parity 명세

## 1. 범위
TS mod(v0.6.2, `src/`)를 유지한 채, Go status line 판(`deprecated-go` 브랜치)에 있던 표시·설정 기능 중 mods API로 재현할 수 있는 것을 옮긴다.
대상은 설정, 테마, 구분자, 다국어, band 폭 맞춤과 표시폭 계산, 브랜치 조회 빈도 제한, `projectInfo`·`projectName`·`repoInfo` 위젯, 위젯 배치와 끄기다.

- 입력 맥락:
  - 현재 동작은 `src/hooks/register.ts`가 소유한다. 고정 순서 `프로젝트(브랜치) │ 모델 │ 컨텍스트 │ 비용 │ 5h │ 7d │ spend`를 프롬프트 위 band(`AbovePrompt`)에 그리며, 설정·테마·다국어가 없다. 프로젝트 이름과 브랜치는 `$.session.root()` 기준이다.
  - 이식 기준은 `deprecated-go` 브랜치(`1005ee8`)의 `config.go`(설정 스키마·검증), `render.go`(테마 8종·구분자·진행 막대·퍼센트 색 기준), `widget.go`(`displayPresets`, `presetCharToWidget`, 줄 맞춤), `widgets_core.go`, `widgets_project.go`(`projectInfo`·`projectName`·`repoInfo`), `display_width.go`, `branch_cache.go`, `locales/{en,ko}.json`이다.
  - mods API에서 얻을 수 있는 값은 `$.session.usage()`(context·rateLimits·cost), `$.session.model()`, `$.session.root()`, `$.session.repo()`, `$.process.run`(git)이며, 근거는 `src/.claude-plugin/types/claude-code/index.d.ts`(Claude Code 2.1.292가 생성)다.
  - 사전 논의에서 확정한 방향: 설정은 플러그인 `userConfig`로 받고 기존 `cc-usage.json` 파일은 읽지 않는다. mods API로 값을 얻을 수 없는 위젯은 버리고, Go 바이너리를 함께 실행하는 혼합안은 쓰지 않는다.

## 2. 목표
Go 판에서 v0.6.x로 넘어온 사용자가 잃은 화면 구성 능력(무엇을 어떤 순서·색·언어로 보일지 고르는 것)을 되찾게 한다.
설정은 `settings.json`을 손으로 고치지 않고 플러그인 설정 화면(`/config`)에서 바꿀 수 있게 한다.

## 3. 제약
- 단일 hooks module 규칙을 지킨다 — mods API는 `$.namespace.method(...)` 형태로만 부르고, `$`는 같은 파일 최상위 함수에만 넘기며, `on(...)`의 이벤트 이름은 문자열 리터럴이다. `claude plugin validate ./src`가 통과해야 한다.
- 외부 npm 의존을 두지 않는다. import는 plugin 디렉토리 안의 상대 경로와 `claude-code`뿐이다.
- 렌더 중 네트워크에 접속하지 않는다. 외부 프로세스는 `git`만 실행한다.
- 설정은 플러그인 `userConfig`로만 받는다. 기존 `cc-usage.json` 파일을 읽지 않는다.
- 설정값이 허용 범위를 벗어나거나 형식이 틀려도 band 전체가 사라지지 않는다. 그 항목만 기본값으로 돌아간다.
  예외: `settings.json`의 `pluginConfigs`를 손으로 고쳐 선언 타입과 다른 JSON 값을 넣으면 Claude Code가 mod 로드를 거부하며, mod는 이 경로에 개입할 수 없다.
- 사용자가 체감하는 변경은 `src/.claude-plugin/plugin.json`의 `version`을 올리고 `release` 브랜치에 동기화한다.
- 요구 Claude Code 버전은 mods 최소 버전(v2.1.287) 이상에서 올리지 않는다.

## 4. 제외 범위
- `fastMode`·`thinking`·`effort`·`pullRequest` 위젯과 worktree 표시 — mods API에서 값을 얻을 수 없어 버린다.
- Go 바이너리를 mod가 실행하는 혼합안.
- 기존 `cc-usage.json` 설정 파일의 읽기·마이그레이션.
- Go 판의 stdin 파싱·손상 격리, 첫 응답 전 rate limit placeholder — mod는 stdin을 받지 않고 엔진이 값을 직접 준다.
- 디스크 기반 브랜치 캐시 파일 — mod는 세션 동안 살아 있는 한 프로세스라 디스크 캐시의 존재 이유(실행마다 새 프로세스)가 없다.
- 프로필 간 설정 승계, subagent 행 커스터마이즈(`subagentStatusLine`).

## 5. 완료 조건
1. 설정을 하나도 바꾸지 않은 사용자가 보는 band의 항목과 순서는 v0.6.2와 같다 — `projectName`(디렉토리 이름과 브랜치) │ 모델 │ 컨텍스트 │ 비용 │ 5h │ 7d │ spend.
2. 사용자가 `/config`의 플러그인 설정에서 줄별 위젯 구성을 바꾸면 band가 그 구성대로 여러 줄·순서로 그려진다. 구성은 Go 판의 preset 문자 표기(`M C $ R 7 P N G`)와 위젯 ID 중 하나로 지정할 수 있다.
3. 사용자가 끈 위젯은 구성에 들어 있어도 band에 나타나지 않는다.
4. 테마를 `default`·`minimal`·`catppuccin`·`dracula`·`gruvbox`·`nord`·`tokyoNight`·`solarized` 중 하나로 고르면 band의 위젯별 색이 그 테마로 바뀐다.
5. 구분자를 `pipe`(`│`)·`dot`(`·`)·`arrow`(`›`)·`space` 중 하나로 고르면 위젯 사이 구분자가 그것으로 바뀐다.
6. 언어를 `ko`로 고르면 rate limit 라벨과 리셋까지 남은 시간이 한국어(`5시간`, `7일`, `일`·`시간`·`분`)로, `en`이면 영어(`5h`, `7d`, `d`·`h`·`m`)로 표시된다. `auto`면 실행 환경의 로캘 환경변수(`LC_ALL`·`LC_MESSAGES`·`LANG`)가 한국어일 때 한국어로 표시된다.
7. 컨텍스트 막대 폭을 1–40 사이로 지정하면 막대 칸 수가 그 값이 되고, 범위 밖 값이면 기본 폭 8로 그려진다.
8. 컨텍스트와 rate limit의 퍼센트 색은 Go 판과 같은 기준을 따른다 — 50 이하 안전, 80 이하 경고, 그 초과 위험.
9. band 폭보다 한 줄이 길면 오른쪽 위젯부터 빠지고, 하나만 남아도 넘치면 표시폭 기준으로 잘린다. 한글·CJK 문자는 2칸으로 계산한다.
10. `projectInfo`는 홈을 `~`로 줄인 현재 프로젝트 경로와 브랜치를, `projectName`은 프로젝트 디렉토리 이름과 브랜치를, `repoInfo`는 `origin` remote의 `owner/name`을 표시한다. remote가 없으면 `repoInfo`는 생략된다.
11. Bash에서 `cd`로 하위 디렉토리에 들어가도 프로젝트 칸의 이름·경로·브랜치가 바뀌지 않는다.
12. 세션이 쉬는 동안에도 같은 프로젝트에서 `git` 실행은 일정 간격 안에 한 번을 넘지 않는다.
13. `claude plugin validate ./src`와 `tsc -p src --noEmit`이 오류 없이 통과한다.
14. 배포된 `release` 브랜치에서 `/plugin update` 후 `/reload-plugins`를 실행하면 위 동작이 반영된 새 버전이 로드된다.
15. `claude plugin test ./src`가 실패 없이 통과하며, 그 테스트가 §5.2–§5.9의 각 동작을 세션 없이 확인한다.
