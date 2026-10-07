# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 개요

Claude Code mod(TypeScript 함수 hook plugin)로 프롬프트 위 band에 세션 상태를 그린다.
데이터는 stdin이 아니라 mods API(`$.session.usage()`, `$.session.model()`, `$.session.cwd()`)에서 온다.

Go status line 버전은 `deprecated-go` 브랜치에 보존돼 있고 갱신하지 않는다.

## 구조

```
src/                          plugin 루트 (--plugin-dir ./src)
  .claude-plugin/plugin.json  manifest (name: cc-usage-mod)
  .claude-plugin/types/       Claude Code가 로드할 때 생성하는 버전별 타입 선언 (git ignored)
  hooks/hooks.json            modules: ["./register.ts"]
  hooks/register.ts           hooks module — 데이터 수집과 AbovePrompt band 렌더
  tsconfig.json               생성된 타입 선언을 extends
```

## hooks module 규칙

`claude plugin validate`가 소스를 정적으로 분석하므로 다음을 지킨다.

- mods API는 `$.namespace.method(...)`로 그대로 호출한다. `$`나 그 namespace를 변수에 담거나 구조분해하지 않는다.
- `$`를 넘길 수 있는 곳은 같은 파일 최상위에 선언한 함수뿐이다.
- `on(...)`의 이벤트 이름은 문자열 리터럴로 쓴다.
- import는 plugin 디렉토리 안의 상대 경로와 `claude-code`(타입·helper)만 쓴다.
- Node API·`setTimeout`이 없다. 타이머는 `$.clock.every`, 프로세스는 `$.process.run`을 쓴다.

타입 선언은 Claude Code 버전마다 바뀐다. 문서와 다르면 `src/.claude-plugin/types/claude-code/index.d.ts`를 따른다.

## 검증

```bash
claude plugin validate ./src
tsc -p src --noEmit
claude --plugin-dir ./src       # 실제 band 확인
```

## 배포

| 브랜치 | 용도 |
|--------|------|
| `main` | 개발 소스 |
| `release` | marketplace 배포 (GitHub default). `src/` 내용을 루트에 두고 `.claude-plugin/marketplace.json`, `LICENSE`, 배포용 `README.md`를 함께 둔다 (orphan 브랜치) |
| `deprecated-go` | Go 버전 보존 |

marketplace 엔트리 이름은 `cc-usage`이고 manifest 이름은 `cc-usage-mod`다.
사용자가 체감하는 변경은 `src/.claude-plugin/plugin.json`의 `version`을 올린다 — `/plugin` 업데이트 감지가 이 값을 쓴다.

release 반영은 orphan 브랜치라 머지하지 않고 파일을 복사해 새 commit을 쌓는다.

```bash
TMPWT=$(mktemp -d)
git worktree add "$TMPWT" release
git -C "$TMPWT" rm -r -q --ignore-unmatch .claude-plugin/plugin.json hooks
mkdir -p "$TMPWT/.claude-plugin"
cp src/.claude-plugin/plugin.json "$TMPWT/.claude-plugin/"
cp -R src/hooks LICENSE "$TMPWT/"
# marketplace.json과 release 전용 README.md는 release에만 있다
git -C "$TMPWT" add -A
git -C "$TMPWT" commit -m "release: sync from main @ <sha>"
git -C "$TMPWT" push origin release
git worktree remove "$TMPWT"
```
