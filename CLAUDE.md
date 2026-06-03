# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 개요

Go 기반 Claude Code status line plugin. **단일 바이너리 / single `main` 패키지 / zero dependency**.

```
stdin (Claude Code JSON) → cc-usage → stdout (ANSI 컬러 텍스트)
```

## 아키텍처

```
main.go            : 진입점 — config 로드 → stdin 파싱 → 무출력 판정 → orchestrate → 출력
stdin.go           : Claude Code status line 프로토콜 JSON → StdinInput
config.go          : {CLAUDE_CONFIG_DIR or ~/.claude}/cc-usage.json → Config (기본값 머지)
widget.go          : Widget 인터페이스 + registry + displayPresets + presetCharToWidget
                     + orchestrate(): 라인별로 GetData/Render 후 separator 조인
widgets_core.go    : model, context, cost, rateLimit{5h,7d} 위젯
widgets_project.go : projectInfo 위젯 (git porcelain=v2로 branch + ahead/behind)
render.go          : theme, separator, progress bar, ANSI 코드
format.go          : 토큰/비용/시간/퍼센트 포맷터
locales/{en,ko}.json : i18n 문자열 (go:embed로 임베드)
```

### 무출력 조건

`workspace.current_dir`, `model.id/display_name`, `context_window_size`가 모두 비어있으면 출력을
**완전히 생략**. 이전 실행 값이나 rate limit API 값으로 보강하지 않는다.

## 위젯 추가 절차

1. `widget.Widget` 인터페이스 구현 — `ID()`, `GetData(ctx) (any, error)`, `Render(data, ctx) string`.
2. 같은 파일의 `init()`에서 `registerWidget(yourWidget{})`.
3. preset 문자(한 글자) 매핑이 필요하면 `widget.go`의 `presetCharToWidget`에 추가.
4. 기본 레이아웃에 노출하려면 `displayPresets`(현재 `compact`만 정의) 수정.

### 위젯 구현 규칙

- `GetData`가 `nil`이나 error를 반환하면 orchestrator가 자동 skip. **패닉 금지**.
- Render 결과가 빈 문자열이면 자동 skip.
- 새 위젯은 widget 종류에 따라:
  - **Core** (모델/컨텍스트/비용/rate limit): `widgets_core.go`
  - **Project**: `widgets_project.go`
- stdin payload에 데이터가 실제로 들어오는지 먼저 확인할 것.
  struct 필드가 있다고 항상 채워지는 건 아니다 — 예: `worktree`, `vim`, `agent`, `remote`는 일반적으로 비어있음.
- 또한 `context_window.total_output_tokens`는 **세션 누적이 아니라 현재 턴 output**이다
  (`current_usage.output_tokens`와 항상 일치). 누적 output token 기반 계산(예: 평균 tok/s)은 불가능.

## 빌드 / 테스트

```bash
make build-local  # dist/cc-usage — 로컬 개발용 (git ignored)
make build        # bin/cc-usage-{darwin,linux,windows}-{arm64,amd64} — marketplace 배포용 (git tracked)
make test         # = go test ./...
go vet ./...
go build ./...
```

- 버전은 ldflags로 주입: `-X main.version=$(VERSION)` (Makefile의 `VERSION`).
- 단일 테스트: `go test -run TestSessionCacheKey ./...`

### 동작 확인 (수동)

```bash
echo '{"model":{"id":"claude-opus-4-6","display_name":"Opus"},"workspace":{"current_dir":"/tmp"},"context_window":{"total_input_tokens":50000,"total_output_tokens":10000,"context_window_size":200000,"current_usage":{"input_tokens":50000,"output_tokens":10000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"cost":{"total_cost_usd":1.25},"rate_limits":{"five_hour":{"used_percentage":42,"resets_at":0},"seven_day":{"used_percentage":69,"resets_at":0}}}' | ./dist/cc-usage
# 기대: tmp │ ◆ claude-opus-4-6 │ ███░░░░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%
```

### 디버그 로그

```bash
DEBUG=cc-usage ./dist/cc-usage <<< '{...}'   # 또는 DEBUG=1
```
`debugLog`는 stderr로만 출력. **stdout은 절대로 오염시키지 않는다** (위젯 렌더 결과 전용).

## 프로젝트 규칙

### 의존성

- **Zero dependency.** Go 표준 라이브러리만 사용. `go.mod`에 `require` 블록이 생기면 안 됨.
- 외부 모듈 추가 금지 — 필요해 보이면 표준 라이브러리로 풀거나 사용자에게 먼저 확인.

### 패키지

- 단일 `main` 패키지. **서브 패키지 생성 금지** — 파일로만 분리.

### 출력

- **stdout**: 위젯 렌더링 결과 + ANSI 코드만.
- **stderr**: 모든 debug/error 출력 (debugLog 사용).

### 경로

| 용도 | 경로 | 비고 |
|------|------|------|
| 설정 | `--config <path>` 또는 `{CLAUDE_CONFIG_DIR or ~/.claude}/cc-usage.json` | env 미설정 시 `~/.claude` |

## 설계 문서

진실 layer는 세 곳이다.

- **`README.md`**: 사용자 대상 — install / config / 노출 위젯 / troubleshooting.
- **`CLAUDE.md`**: 개발자·Claude 대상 — 아키텍처, 위젯 추가 절차, 빌드/테스트, 배포 절차.
- **소스 코드**: 런타임 진실 — `stdin.go` (입력 스키마), `widget.go`+`widgets_*.go` (위젯 명세), `config.go` (설정 스키마), `locales/*.json` (i18n).

진행 상태 트래커는 `docs/<feature-dir>/` 아래의 spec/analysis/implement.md가 담당한다. 새 위젯·새 캐시·새 외부 통합 같은 의미 있는 설계는 그 feature spec에서 commit한다.

## 배포

### 브랜치 구성

| 브랜치 | 용도 | 내용 |
|--------|------|------|
| `main` | 개발 + 소스 | 전체 소스, 설계 문서, Makefile, 테스트 |
| `release` | marketplace 배포 (**GitHub default**) | `bin/`, `.claude-plugin/`, `skills/`, `README.md`, `LICENSE` — 소스 없음 (orphan 브랜치) |

`/plugin install cc-usage`는 GitHub default branch를 clone하므로 사용자는 `release`만 받는다.
브라우저에서 https://github.com/zipkero/cc-usage 도 default가 `release`로 열린다.

### 배포 산출물

- `bin/`: pre-built 바이너리 (git tracked, release 브랜치로 전파)
- `dist/`: 로컬 빌드 산출물 (git ignored)
- `bin/run.sh`: OS/ARCH 감지 후 알맞은 바이너리 실행하는 wrapper
- `.claude-plugin/{plugin,marketplace}.json`: Claude Code plugin 메타데이터

### 버전 정책

사용자가 체감 가능한 fix·feature 변경은 항상 SemVer patch(또는 minor) bump를 동반한다
(`Makefile` VERSION, `.claude-plugin/plugin.json` version을 같은 값으로 동시 갱신).

- 이유: `/plugin` UI의 update 감지는 `plugin.json`의 version 필드 변화에 의존한다. 동등 hash 재빌드만 push하면 사용자 머신의 marketplaces 사본이 stale로 고착되어 fix가 적용되지 않는다.
- version-only commit은 권장하지 않으며 해당 fix·feature commit에 묶어 한 번에 올린다.
- 순수 docs·테스트·내부 리팩터(사용자에게 노출 동작 변화 없음)는 bump 대상이 아니다.

### 릴리스 절차 (bin/ 갱신 시)

`make build`로 `bin/`을 다시 만든 뒤, main에 commit하고 release 브랜치에도 반영해야 한다.
orphan 브랜치라서 main과 머지하지 않고 **파일을 복사해 새 commit**으로 쌓는다.

```bash
# 1) main에서 bin/ 갱신
make build
git add bin/ && git commit -m "build: rebuild binaries vX.Y.Z"

# 2) release 브랜치를 worktree로 꺼내서 파일 동기화
TMPWT=$(mktemp -d)
git worktree add "$TMPWT" release
cp -R bin .claude-plugin skills LICENSE "$TMPWT/"
# README.md는 release 전용 사본을 유지 — 필요 시 별도 편집

# 3) commit + push
git -C "$TMPWT" add -A
git -C "$TMPWT" commit -m "release: sync from main @ <sha>"
git -C "$TMPWT" push origin release

# 4) cleanup
git worktree remove "$TMPWT"
```

자동화하고 싶다면 `make release` 타깃을 추가하는 게 자연스럽다 (현재 미구현).
