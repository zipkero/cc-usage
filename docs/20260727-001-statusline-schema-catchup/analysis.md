# statusline-schema-catchup — ANALYSIS

## 근거

읽은 spec 범위: `spec.md` §1–§5 전체. 범위는 ① 새 stdin 필드의 수용·노출, ② 기존 위젯 계산·표기의
의미 교정, ③ 설정 디렉토리를 옮긴 환경의 설치 절차 교정 세 갈래이며(§1), 제약은 zero dependency·
단일 `main` 패키지·stdout 오염 금지·기본 출력 구성 불변·캐시 대상 제한·누락 필드 정상 처리·버전
동시 갱신(§3), 제외는 subagentStatusLine·프로필 간 설정 승계·`prompt_id`·`remote`/`agent_*` 정리·
컨텍스트 토큰 임계값 재조정(§4)이다. spec.md에 미답(승인 전 확인) 항목은 없다.

main이 전달한 공식 문서 조회 결과(`code.claude.com/docs/en/statusline.md`, 2026-07-27)를 외부 계약의
사실로 사용했다 — `used_percentage`의 input-only 공식, v2.1.132의 total_* 의미 변경, 새 필드 목록과
각 필드의 조건부 부재, `current_usage`의 세션 초반 `null`, v2.1.153의 `COLUMNS`/`LINES`, `session_id`
기반 캐시 권고, `claude-fable-5`/`claude-mythos-5`의 실재. 이 항목들은 저장소 코드로 검증할 수 없으므로
문서 조회 결과로만 인용한다.

2026-07-30 재조회로 세션 상태 세 필드의 존재 조건과 기본값을 확정했다. 같은 문서의
"Fields that may be absent" 목록에 `effort`·`workspace.repo`·`workspace.git_worktree`·`pr`·
`rate_limits`·`worktree`·`vim`·`agent`·`session_name`·`prompt_id`가 들어 있고, **`fast_mode`와
`thinking`은 그 목록에 없다** — 예시 payload에도 `"fast_mode": false`, `"thinking": {"enabled": true}`로
나타난다. 즉 두 필드는 현행 Claude Code에서 항상 존재하고, `effort`만 "현재 모델이 effort 파라미터를
지원할 때만" 조건부다. 기본값은 `code.claude.com/docs/en/costs.md`가 "Extended thinking is enabled by
default"로, `code.claude.com/docs/en/fast-mode.md`가 `/fast`(또는 `"fastMode": true`)로 켜는 opt-in에
Opus 5·4.8 한정·usage credits 필요·$10/$50 가격으로 규정한다. 두 필드의 기본 상태가 서로 반대라는
사실이 §5 D3의 근거다. fast mode는 rate limit cooldown에서 표준 속도로 자동 강하하지만 그 상태를
알리는 stdin 필드가 없다는 점도 같은 문서에서 확인했다.

사용자 확인 결과(2026-07-30): §5 D1(context 토큰 표시를 input-only로 정합), §5 D7(COLUMNS 축소는 줄
오른쪽부터)은 채택안 그대로 확정됐고, §5 D3은 "두 bool을 한 규칙으로 묶지 않는다"로 방향이 바뀌어
아래 본문에 반영했다.

코드에서 직접 확인한 사실:

- 진입점(`main.go`)은 `flag 파싱 → loadConfig → parseStdin → loadTranslations → Context 조립 →
  shouldSuppressOutput → orchestrate → stdout` 한 경로다. 환경변수는 `DEBUG`와
  `CLAUDE_CONFIG_DIR`만 읽는다(`grep os.Getenv` → `main.go:16,27`, `widget.go:60`(locale 감지),
  `widgets_project.go:22`(cwd 탐지 hook)). `COLUMNS`/`LINES`를 읽는 코드는 없다(`grep COLUMNS` 무결과).
- `configHomeDir(home)`은 `CLAUDE_CONFIG_DIR`을 `strings.TrimSpace` 후 비어 있지 않으면 그대로 쓰고,
  아니면 `<home>/.claude`로 떨어진다. `defaultConfigPath()`가 그 아래 `cc-usage.json`을 가리킨다.
  `main_test.go`의 `TestConfigHomeDir`가 env 우선·공백 처리·폴백 세 경우를 이미 고정한다.
- `StdinInput`(`stdin.go`)에는 `fast_mode`·`effort`·`thinking`·`workspace.git_worktree`·
  `workspace.repo`·`pr`가 없다. `context_window.current_usage`는 포인터가 아닌 값 struct이고
  `used_percentage`/`remaining_percentage`는 `*float64`다. `null` 입력을 별도 프로그램으로 재현해
  값 struct와 포인터 모두 디코드 오류 없이 zero/nil로 남는 것을 확인했다 — 새 필드도 같은 성질을
  쓸 수 있다.
- `parseStdinReader`는 더 이상 어떤 오류에서든 빈 입력을 돌리지 않는다(`20260730-003-stdin-resilience`).
  최상위를 `map[string]json.RawMessage`로 받은 뒤 `stdinSectionTable`이 열거한 섹션마다 따로
  `json.Unmarshal`하고, 실패한 섹션만 `reflect.Zero`로 되돌린 채 나머지를 살린다
  (`stdin.go:114,149,207`). 빈 입력으로 떨어지는 경로는 최상위 JSON 자체가 깨진 경우 하나뿐이다.
  새 필드를 추가할 때는 **`stdinSectionTable`에도 같은 최상위 키를 등재해야 한다** —
  `TestStdinSectionTableCompleteness`가 struct 태그와 표를 양방향으로 대조하므로 빠뜨리면 테스트가
  잡는다. `fast_mode`·`effort`·`thinking`·`pr`이 새 최상위 키이고, `workspace.*`는 기존 `workspace`
  섹션 안이라 표 변경이 없다.
- context 위젯(`widgets_core.go:73`)은 `used_percentage`가 있으면 `clampPercent`로 절삭하고, 없으면
  `TotalInputTokens + TotalOutputTokens`를 분자로 `calculatePercent`를 쓴다. 표시 토큰 수도 같은 합이다.
  실행으로 차이를 재현했다 — 같은 payload에서 `used_percentage` 부재 시 30%, `used_percentage: 25.0`
  주입 시 25%가 나온다(문서 예시 payload, `--config` 미존재 경로로 기본 설정 고정).
- 같은 위젯에 placeholder 갈래가 생겼다(`20260730-001-session-start-placeholder`). `contextData`와
  `rateLimitData`에 `Placeholder bool`이 붙어 있고 zero value가 "실측"을 뜻하며, `GetData`가 상태를
  정하고 `Render`가 글리프를 정한다. rate limit 두 위젯은 대응 포인터가 nil이어도
  `ctx.FirstResponseReceived()`가 false면 `&rateLimitData{Placeholder: true}`를 돌려 `5h: -` 칸을
  유지한다(`widgets_core.go:179,206`). 즉 **"데이터가 없으면 위젯이 사라진다"는 더 이상 이 저장소의
  단일 규약이 아니다** — 첫 응답 전에는 흐린 자리를 지키고, 첫 응답 뒤에만 사라진다.
- 첫 응답 판정은 `Context.FirstResponseReceived()`이고 판정식은 `total_input_tokens > 0`이다.
  D1이 context 퍼센트·토큰의 분자를 input 계열로 바꾸면 **표시 값의 분자와 placeholder 해제 신호가
  같은 필드가 된다** — 실측 갈래가 나타나는 구간과 분자가 0이 아닌 구간이 정확히 일치해 `0%`를
  표시하는 실측 화면이 생기지 않는다.
- model 위젯 기호는 `strings.Contains(idLower, ...)` if/else 체인으로 opus `◆`/sonnet `◇`/haiku `○`,
  그 외 `●`다. `claude-fable-5`를 넣어 `●`로 떨어지는 것을 실행으로 확인했다. 표시 이름은 `d.ID`가
  우선이고 `DisplayName`은 ID가 빈 경우에만 쓴다.
- `Translations.Model`(opus/sonnet/haiku)은 로드만 되고 소비처가 없다(`grep Translations.Model` /
  `\.Model\.(Opus|Sonnet|Haiku)` 무결과). `locales/en.json`·`ko.json` 양쪽 `model` 블록의 값이
  `Opus`/`Sonnet`/`Haiku`로 동일해 번역 가치도 없다.
- `presetCharToWidget`은 `map[byte]string`이고 `resolvePreset`이 preset 문자열을 바이트 단위로
  훑는다(`widget.go:99,119`) — preset 문자는 ASCII 1바이트여야 한다. 현재 점유: `M C $ R 7 P N`.
  `widget_test.go`의 `TestRemovedPresetCharsAreUnmapped`가 `S V a D B H F`를 "매핑되지 않아야 함"으로
  못 박고 있고, 같은 파일의 다른 케이스가 `displayPresets["compact"]`를 6개 ID 배열로 정확히 비교한다.
- `orchestrate`는 라인별로 `disabledWidgets` → registry 조회 → `GetData`(nil/error면 skip) →
  `Render`(빈 문자열이면 skip) → separator 조인만 하고, 폭을 보지 않는다. 결과 문자열은 `main.go`가
  `"\n"`으로 이어 그대로 출력한다.
- project 위젯 두 개(`projectInfo`/`projectName`)는 각각 `gitBranch(currentDir)`를 호출하고,
  `gitBranch`는 `exec.LookPath("git")` 확인 후 500ms 타임아웃으로 `git status --porcelain=v2 --branch`를
  실행해 `# branch.head`만 파싱한다. `branch.ab`(ahead/behind)는 더 이상 파싱하지 않는다.
  `exec.` 호출은 저장소 전체에서 이 두 줄뿐이다(`grep exec.` → `widgets_project.go:54,61`).
- `Stdin.Worktree`(`--worktree` 전용 구조체)는 스키마에만 남아 있고 소비처가 없다
  (`grep Worktree` → `stdin.go` 정의 + `widgets_project_test.go`의 무시 검증 케이스).
- `shrinkPath(s, max)`는 max를 인자로 받지만 호출자는 위젯 전용 상수 `pathDisplayMaxRunes = 50`
  하나만 넘긴다. `format.go`의 `truncate`와 `osc8Link`는 비-테스트 호출자가 없다(`grep` 확인).
  폭 계산은 전부 `utf8.RuneCountInString` 기준이므로 한글·CJK의 실제 표시폭 2칸은 반영되지 않는다.
- config는 `WidgetConfig` 아래 위젯별 struct 네임스페이스 패턴이고(`Context ContextWidgetConfig`),
  `barWidth` 0을 "미설정"으로 보고 `ContextBarWidth()`가 `defaultContextBarWidth = 8`로 폴백한다.
  범위 밖 값은 `validateConfigWidgets`가 stderr 경고 후 0으로 리셋한다(`render.go`의 1~40 상수).
- 설치 스킬(`skills/cc-usage-install/SKILL.md`)은 산문 절차서이며 settings 후보 4개를
  `.claude/settings.local.json` → `.claude/settings.json` → `~/.claude/settings.local.json` →
  `~/.claude/settings.json` 순으로 하드코딩하고, 아무 후보에도 `statusLine`이 없으면
  `~/.claude/settings.local.json`을 새로 만든다. `CLAUDE_CONFIG_DIR` 언급이 없다.
- 테스트 관례: 파일당 `Test<대상>` + `t.Run` 서브테스트, 순수 로직은 케이스 슬라이스(table) 방식
  (`widgets_project_test.go`의 compressHome/shrinkPath, `widgets_core_test.go`의 bar 폭).
  ANSI 제거 헬퍼 `stripANSI`와 context 렌더 분해 헬퍼 `splitContextRender`가 이미 있고, git을 막는
  방식은 `t.Setenv("PATH", "")`다. 외부 상태 주입은 `detectCwdEnv`/`detectCwdGetwd` 같은 패키지 레벨
  변수 교체로 한다.
- 과거 캐시 제거 경위: `simplify-statusline` spec §5.1이 "렌더 중 별도 파일 read/write 금지"를 완료
  조건으로 뒀고, 그 앞의 세 feature가 캐시 자체의 결함을 다뤘다 —
  `session-state-fixes`는 ① cost가 0이어도 항상 렌더되어 degraded 판정을 우회하고 `cost=0`이 캐시에
  영구 오염되는 문제, ② `session-state-*.json`이 어떤 경로에서도 청소되지 않아 무한 누적되는 문제,
  `lock-leak-cleanup`은 advisory lock의 `.lock` 파일이 회수되지 않고 누적되는 문제를 각각 spec으로
  남겼다. 삭제 시점의 `cache.go`(commit 73d9348의 부모)를 확인한 결과 경로는 하드코딩된
  `$HOME/.cache/cc-usage/session-state-<key>.json`이었고, TTL 300초 + 시간당 throttle되는
  fire-and-forget goroutine 청소 + 파일 lock 구조였다. 즉 제거된 것은 "stdin 필드를 캐시로 복원하는
  계정 데이터 캐시"이며, 결함의 원천은 (a) 계정 유래 값 저장, (b) 청소 부재, (c) lock 파일,
  (d) session_id 없을 때 cwd만으로 키를 만든 교차 오염이었다.
- `CLAUDE.md`가 실제 상태와 어긋난 지점(SPEC §5.10 대상): 단일 테스트 예시로 든
  `TestSessionCacheKey`는 저장소에 없다(`grep` 무결과). 아키텍처 표는 `widgets_project.go`를
  "branch + ahead/behind"로 적고 있으나 ahead/behind 파싱은 제거됐고 `projectName` 위젯은 표에 없다.
  동작 확인 예시의 기대 출력은 `tmp │ …`인데 기본 설정 실제 출력은 `/tmp │ ◆ claude-opus-4-6 │
  ██░░░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%`다(실행 확인). `total_output_tokens`에 대한 메모는
  현재 컨텍스트 값이라는 점을 output에만 적어 두어 input 쪽 의미 변경이 빠져 있다.
- `README.md`의 Privacy는 "status line 렌더 중 별도 캐시 파일을 읽거나 쓰지 않는다"고 단정한다 —
  git 캐시 도입으로 거짓이 되는 문장이다. 위젯 표는 현재 7개 위젯을 등재한다.
  같은 문장이 `ROADMAP.md` §서비스 완료 기준 6번에도 있었다 — 그 기준은 §5 D6의 결정에 따라
  2026-07-30에 좁혔고, `README.md` 쪽 문장은 이 feature의 수정 대상으로 남아 있다.
- 버전 현황: `Makefile` `VERSION := 0.5.6`, `.claude-plugin/plugin.json` `"version": "0.5.6"`
  (M1·M2 배포 반영, 2026-07-30 확인).

추정과 사실의 분리: 위젯 기호·preset 문자·캐시 TTL·표시폭 테이블 범위 같은 값은 spec이 요구를
남기고 값을 열어둔 부분이라 본 analysis가 확정하는 설계값이며 근거가 아니다. `workspace.git_worktree`
문자열이 경로인지 이름인지는 문서 조회 결과에 명시되지 않아 확정하지 않고, 어느 쪽이든 같은 결과를
내는 표시 방식을 §5 D4에서 고른다.

## 1. 구조

새 모듈·레이어·서브 패키지를 만들지 않는다. 단일 `main` 패키지 안에서 기존 경계 셋을 확장하고
역할이 다른 경계 둘을 파일로 새로 분리한다.

기존 경계 확장:

- **입력 수용 경계**(`stdin.go`): 조건부로 존재하는 새 필드 다섯 묶음을 스키마에 더한다. 존재 여부가
  표시 여부를 가르는 필드는 포인터로 두어 "키 없음"과 "false/빈 값"을 구분한다(SPEC §5.2, §5.5).
- **core 위젯 경계**(`widgets_core.go`): 세션 상태 위젯 세 개(fast mode, effort, thinking)를 추가하고,
  context 위젯의 퍼센트·토큰 산출을 input 계열로 교정하며(SPEC §5.1), model 위젯의 기호 결정을 순서가
  드러나는 표로 바꿔 계열을 늘린다(SPEC §5.6).
- **project 위젯 경계**(`widgets_project.go`): 두 project 위젯이 worktree 토큰을 공유 방식으로 덧붙이고
  (SPEC §5.4), branch 조회를 캐시 경유 호출로 바꾼다(SPEC §5.8). 저장소·PR 정보는 이 경계에 넣지 않고
  별도 opt-in 위젯으로 둔다(SPEC §5.5, §3의 기본 구성 불변).
- **오케스트레이션 경계**(`widget.go`): 새 위젯의 registry 등록과 preset 문자 매핑, 폭 제약 적용 지점.
  `displayPresets["compact"]`는 손대지 않는다(SPEC §5.3).
- **설치 절차 경계**(`skills/cc-usage-install/SKILL.md`): 사용자 스코프 후보 경로를 실행 시 해석되는
  설정 홈 기준으로 바꾼다(SPEC §5.9).

새 파일로 분리하는 경계:

- **git 결과 캐시**: session_id + 대상 디렉토리로 키를 만들고 branch 문자열만 저장·조회하는 얇은 계층.
  위젯도 렌더도 아니고 유일한 외부 상태이므로 별도 파일이 경계로 맞다. 이 계층의 값 범위는 git 유래
  정보로 닫혀 있다(SPEC §3).
- **표시폭 계산**: ANSI 이스케이프를 폭 0으로 걸러내고 rune별 표시폭을 재는 계층 + 그 위에 얹는 줄
  맞춤. rune 범위 표를 들고 있어 포맷터(`format.go`)나 테마(`render.go`)와 관심사가 다르다(SPEC §5.7).

문서 경계: `CLAUDE.md`는 개발자 진실 layer로서 아키텍처·검증 명령·stdin 메모를 실제 상태에 맞춘다
(SPEC §5.10). `README.md`는 사용자 진실 layer로서 새 위젯·preset 문자와 저장 동작 변화를 반영한다.

## 2. 데이터 흐름

진입부터 출력까지의 순서는 다음과 같다. 굵은 항목이 이번에 새로 끼어드는 단계다.

1. `loadConfig(path)` — 경로는 `--config` 또는 `{CLAUDE_CONFIG_DIR or ~/.claude}/cc-usage.json`.
   새 config 키는 추가하지 않으므로 이 단계 동작은 불변이다.
2. `parseStdin()` — 새 필드를 포함해 디코드한다. 새 최상위 키(`fast_mode`·`effort`·`thinking`·`pr`)는
   `stdinSectionTable`에 함께 등재해 섹션 단위 격리의 대상이 된다. 깨진 섹션은 그 섹션만 zero value로
   버려지고 나머지 위젯은 계속 표시되며, 개별 키 부재·`null`은 포인터 nil 또는 zero 값으로 남아 오류가
   되지 않는다(SPEC §3의 누락 필드 정상 경로).
3. `loadTranslations()` — locale에서 `model` 블록이 사라지고 세션 상태 위젯 라벨이 들어온다.
4. **터미널 폭 해석** — `COLUMNS`를 정수로 읽어 양수일 때만 `Context`에 싣는다. 없음·비수치·0 이하는
   전부 "제약 없음"으로 같게 취급한다.
5. `Context{Stdin, Config, Translations, Columns}` 조립.
6. `shouldSuppressOutput` — 판정 입력(model·current_dir·context_window_size)이 그대로이므로 불변이다.
   새 필드는 정체성 신호로 쓰지 않는다.
7. `orchestrate` — 라인별로 위젯을 돌린다. 위젯별 데이터 경로:
   - context: `used_percentage`가 있으면 그 값, 없으면 **input 계열 합**을 분자로 계산한다. 분자는
     `total_input_tokens`를 우선하고 그것이 0이면 `current_usage`의 input·cache_creation·cache_read
     합으로 보완한다. 표시 토큰 수도 같은 분자를 쓴다(SPEC §5.1). placeholder 갈래는 이 값을 표시하지
     않으므로 분자 교정의 영향을 받지 않는다.
   - model: 소문자화한 ID를 기호 표와 순서대로 대조하고, 어디에도 걸리지 않으면 기본 기호(SPEC §5.6).
   - fastMode/effort/thinking: 대응 키가 nil이면 `GetData`가 nil을 돌려 orchestrator가 건너뛴다.
     키가 있을 때는 `fastMode`만 참일 때 렌더하고, `thinking`·`effort`는 값을 그대로 렌더한다
     (SPEC §5.2, §5 D3).
   - repoInfo/pullRequest: `workspace.repo`·`pr`가 nil이면 각각 skip. 둘 다 없으면 관련 출력이 전부
     사라진다(SPEC §5.5).
   - project 위젯: cwd 결정(stdin → `detectCurrentCwd()`) → **branch 조회(캐시 경유)** → worktree 키가
     있으면 토큰 추가(SPEC §5.4).
8. **줄 맞춤** — `Context.Columns`가 있을 때만, 조인된 줄의 표시폭이 그 값을 넘으면 오른쪽 위젯부터
   덜어내고, 위젯 하나만 남았는데도 넘치면 표시폭 기준으로 잘라 생략 기호와 리셋 코드를 붙인다.
   Columns가 없으면 이 단계를 건너뛰어 기존 출력과 완전히 같다(SPEC §5.3, §5.7).
9. stdout 출력. 진단은 전부 `debugLog`로 stderr(SPEC §3).

branch 조회의 상태 전이(유일한 외부 상태):

```
GetData(dir)
  └─ session_id 없음 ──────────────────────────────► git 실행 (캐시 미사용)
  └─ session_id 있음
       ├─ 프로세스 내 memo 적중 ─────────────────► 그 값 반환 (I/O 0)
       ├─ 캐시 파일 적중 && 기록 dir 일치 && TTL 내 ► 그 값 반환 (git 실행 없음)
       └─ 그 외(파일 없음·읽기 실패·JSON 깨짐·dir 불일치·TTL 초과)
            └─ git 실행 → 결과를 memo + 캐시에 기록(원자적 교체) → 반환
```

도달 가능한 상태는 "캐시 없음", "신선한 캐시", "만료·불일치 캐시" 세 가지뿐이고 어느 쪽에서도 표시
결과는 git을 그대로 실행한 경우와 같거나(신선), 다시 git을 실행해 갱신된다(그 외). 실패 경로는 모두
"캐시 없음"으로 수렴한다 — 캐시 디렉토리를 만들 수 없거나 쓰기가 실패하면 `debugLog`만 남기고 표시에는
영향이 없다. 캐시는 stdin에 없는 값을 채우는 데 절대 쓰이지 않는다. 브랜치는 언제든 git으로 다시 얻을
수 있는 국소 파생값이고, 캐시에 담기는 값의 집합이 그 하나로 닫혀 있기 때문이다(SPEC §3).

동시성: goroutine을 새로 만들지 않는다. 여러 세션이 동시에 실행되면 각자 다른 파일을 쓰고, 같은 세션의
동시 실행은 임시 파일 생성 후 rename으로 마지막 쓰기가 이긴다. lock을 쓰지 않으므로 lock 파일이 남는
경로가 없다. 오래된 파일 청소는 쓰기 경로에서 동기적으로 한 번, 나이 기준으로만 수행한다.

## 3. 인터페이스

경계를 가로지르는 계약만 적는다.

- **stdin 스키마**(Claude Code → cc-usage): 추가 수용 필드는 `fast_mode`(bool),
  `effort.level`(string), `thinking.enabled`(bool), `workspace.git_worktree`(string),
  `workspace.repo.{host,owner,name}`, `pr.{number,url,review_state}`다. `fast_mode`·`effort`·
  `thinking`·`workspace.repo`·`pr`는 부재와 값을 구분해야 하므로 포인터로 받고, `workspace.git_worktree`는
  빈 문자열이 곧 부재이므로 값으로 받는다. `prompt_id`는 받지 않는다(SPEC §4). 기존 필드
  (`remote`·`agent_id`·`agent_type`·`worktree`)는 그대로 둔다(SPEC §4).
- **`Widget` 인터페이스**: `ID()`/`GetData(*Context) (any, error)`/`Render(any, *Context) string`
  계약과 nil·빈 문자열 skip, 패닉 금지 규칙 모두 불변이다. 새 위젯 다섯 개가 같은 계약을 구현한다.
- **`Context`**: `Columns int` 필드가 추가된다. 값 0은 "제약 없음"이며 위젯은 이 필드를 읽지 않는다 —
  소비자는 줄 맞춤 단계 하나다.
- **위젯 ID 계약**: `fastMode`, `effort`, `thinking`, `repoInfo`, `pullRequest`가 신규 ID로 추가된다.
  `lines`·`disabledWidgets`가 이 문자열로 위젯을 지목한다. 기존 7개 ID는 불변이다.
- **preset 문자 계약**: 문자는 ASCII 1바이트여야 한다(`map[byte]` + 바이트 순회). 신규 배정은
  `f`=fastMode, `E`=effort, `T`=thinking, `G`=repoInfo, `#`=pullRequest. `S V a D B H F`는 계속
  미매핑으로 남긴다(§5 D2).
- **config 스키마**: 새 키를 추가하지 않는다. `widgets.context.barWidth`의 의미와 범위(1~40, 기본 8)도
  불변이며 COLUMNS가 이 값을 덮어쓰지 않는다(§5 D7).
- **locale 키**: `model` 블록(opus/sonnet/haiku)을 제거하고 `Translations`의 대응 필드도 없앤다.
  세션 상태 위젯 라벨을 `labels`에 추가한다. `effort.level` 값과 `pr.review_state` 값은 시스템
  식별자이므로 번역하지 않는다.
- **git 캐시 파일 형식**: 캐시는 cc-usage 내부 산출물이며 외부 계약이 아니다. 형식이 바뀌거나 깨져
  읽히면 미적중으로 처리해 스스로 복구한다. 저장 항목은 대상 디렉토리, branch 문자열, 기록 시각뿐이며
  계정 유래 값은 담지 않는다(SPEC §3, §5.8).
- **설치 스킬의 후보 경로 계약**: 사용자 스코프 후보와 신규 생성 기본값은 "실행 환경의 설정 홈"
  기준으로 해석된다 — `CLAUDE_CONFIG_DIR`이 공백이 아닌 값으로 설정되어 있으면 그 디렉토리, 아니면
  `~/.claude`. 이 해석 규칙은 `main.go`의 `configHomeDir`와 같아야 한다. 프로젝트 스코프 후보
  (`.claude/settings.*.json`)는 저장소 상대 경로이므로 영향받지 않는다(SPEC §5.9).

## 4. 영향 범위

수정 파일(탐색으로 확인한 직접 대상):

- `stdin.go` — 새 필드 수용과 `stdinSectionTable`에 새 최상위 키 4개(`fast_mode`·`effort`·`thinking`·`pr`)
  등재. 기존 필드 제거·개명 없음.
- `widgets_core.go` — context 퍼센트·토큰 산출 교정(실측 갈래만, placeholder 갈래는 불변),
  model 기호 표, 세션 상태 위젯 3개 신설과 등록.
- `widgets_project.go` — worktree 토큰 공유 렌더, branch 조회를 캐시 경유로 전환, repoInfo·
  pullRequest 위젯 신설과 등록(project 계열 파일에 둔다).
- `widget.go` — `Context.Columns` 추가, `presetCharToWidget` 5개 추가, `Translations.Model` 제거,
  `orchestrate`에 줄 맞춤 적용. `displayPresets`는 불변.
- `main.go` — `COLUMNS` 해석 후 `Context`에 전달.
- 신규 파일 2개 — git 캐시 계층, 표시폭 계산·줄 맞춤 계층.
- `locales/en.json`·`locales/ko.json` — `model` 블록 제거, 세션 상태 라벨 추가.
- `render.go`/`format.go`/`config.go` — 신규 상수를 어디에 둘지 외에는 계약 변경 없음.
  `renderProgressBar` 시그니처와 `ContextBarWidth()` 동작은 그대로다.
- `skills/cc-usage-install/SKILL.md` — 후보 경로 산문 교정(SPEC §5.9). 이 디렉토리는 release 브랜치로
  복사 전파되는 배포 산출물에 포함된다.
- `CLAUDE.md` — 아키텍처 표(projectName 누락, ahead/behind 오기), 위젯 목록, 검증 명령의 없는 테스트
  이름, 동작 확인 예시 기대 출력, stdin 메모(총 input/output 의미), 캐시·COLUMNS 서술 추가(SPEC §5.10).
- `README.md` — 위젯 표에 신규 5개와 preset 문자 추가, `fastMode`의 "켜진 동안에만 표시" 명기.
  Privacy의 "캐시 파일을 읽거나 쓰지 않는다" 문장 교정(§5 D11) — 이번 변경이 이 문장을 거짓으로 만든다.
- `ROADMAP.md` — §서비스 완료 기준 6번과 M3 항목은 2026-07-30에 이미 갱신했다(D6 결정 반영).
  이 feature의 코드 변경 대상은 아니다.
- `Makefile` VERSION과 `.claude-plugin/plugin.json` version — 같은 새 값으로 동시 갱신(SPEC §5.11).

직접·간접 의존(grep·실행으로 확인):

- `gitBranch`의 호출자는 project 위젯 두 곳뿐이다. 캐시 래퍼를 새로 두고 두 호출자만 바꾸면 되며,
  `gitBranch` 자체의 시그니처·동작은 유지되어 기존 degrade 테스트가 그대로 유효하다.
- `exec.` 사용처는 `widgets_project.go`의 두 줄뿐이다 — 캐시가 억제해야 하는 하위 프로세스 표면이
  거기로 닫혀 있음을 확인했다(SPEC §5.8).
- `renderProgressBar`의 호출자는 context 위젯 한 곳, `shrinkPath`/`compressHome`의 호출자는 project
  위젯뿐이다. 폭 제약을 줄 단위로 처리하는 채택(§5 D7) 아래에서는 이 셋 다 수정 대상이 아니다.
- `clampPercent`/`calculatePercent`는 context·rateLimit 위젯이 공유한다. context 쪽 분자만 바뀌므로
  두 함수의 계약은 건드리지 않는다.
- `truncate`·`osc8Link`는 비-테스트 호출자가 없다. `osc8Link`는 pullRequest 위젯의 URL 링크로 소비처가
  생기고, `truncate`는 rune 수 기준이라 표시폭 기준 절단에는 쓰이지 않는다(§5 D8).
- `Translations.Model` 제거는 소비처가 없어 컴파일·출력 영향이 없다(grep 무결과).

테스트 영향:

- `widget_test.go` — `TestRemovedPresetCharsAreUnmapped`(`S V a D B H F`)와 compact 6개 배열 비교는
  신규 문자 배정이 그 집합을 피하고 기본 preset을 건드리지 않으므로 그대로 통과해야 한다. 두 케이스가
  이번 변경의 회귀 감지 장치 역할을 한다.
- `widgets_core_test.go` — `splitContextRender`는 렌더 문자열을 공백 3분할로 가정한다. context 렌더
  포맷을 유지하므로 유효하다. bar 폭 케이스도 폭 결정 방식을 바꾸지 않으므로 유효하다.
  `20260730-001-session-start-placeholder`가 추가한 placeholder 케이스는 토큰 수를 표시하지 않는 갈래이므로
  D1의 분자 교정에 영향받지 않는다. 반대로 **실측 갈래를 검증하는 케이스는 기대값이 달라진다** — 리터럴
  `&contextData{...}`로 데이터를 직접 넣는 렌더 케이스는 그대로지만, payload에서 `GetData`를 거치는
  케이스(`stdin_test.go`의 `TestContextWidgetFractionalPercent` 계열)와 orchestrate 수준 줄 비교 케이스는
  input-only 값으로 갱신해야 한다.
- `stdin_test.go` — `TestStdinSectionTableCompleteness`가 `StdinInput`의 최상위 태그와
  `stdinSectionTable`을 양방향으로 대조한다. 새 최상위 필드를 표에 등재하지 않으면 이 테스트가 실패한다 —
  누락을 잡아주는 장치이므로 새 케이스를 만들 필요는 없다.
- `widgets_project_test.go` — `t.Setenv("PATH", "")`로 git을 막는 케이스들이 캐시 파일에 오염되면
  안 된다. 이 테스트들은 `StdinInput{}`을 쓰므로 session_id가 비어 있고, session_id 없으면 캐시를
  아예 타지 않는 설계(§5 D6)에서 기존 케이스가 영향받지 않는다.
- `main_test.go`의 `TestConfigHomeDir` — `CLAUDE_CONFIG_DIR` 해석은 바꾸지 않으므로 유효하며, 설치
  스킬 교정이 이 함수와 같은 규칙을 따르는지의 기준점이 된다.

하위 호환: 기존 `cc-usage.json`(preset·lines·disabledWidgets·widgets.context.barWidth)은 그대로
해석된다. 신규 preset 문자는 이전에 미매핑이라 무시되던 문자이므로 기존 설정의 의미가 바뀌는 경우는
없다. 다만 `f`/`E`/`T`/`G`/`#`를 이미 preset에 적어둔 사용자는 지금까지 무시되던 문자가 위젯으로
살아난다 — 문서화 대상이며 코드 대응은 필요 없다. 디스크에 남은 구버전 캐시 파일
(`~/.cache/cc-usage/session-state-*.json` 등)은 새 캐시가 다른 파일명·다른 디렉토리 규약을 쓰므로
읽히지 않는다.

## 5. Decision Points

### D1. context 퍼센트 fallback 공식과 토큰 표시 정합

- 옵션 A: 퍼센트만 input 계열로 고치고 토큰 표시는 input+output 유지.
- 옵션 B: 퍼센트와 토큰 표시를 모두 input 계열로 맞춤.
- 채택: **B**. 근거 — 문서 조회 결과에 따르면 `total_output_tokens`는 현재 턴 output이라 "컨텍스트에
  얼마나 찼는가"를 나타내는 합에 들어갈 값이 아니고, A를 고르면 `25% 60K`처럼 사용자가 눈으로 검산할 수
  있는 두 수치가 서로 어긋난다. B는 `used_percentage`가 있는 payload와 없는 payload에서 퍼센트가 같아지고
  (SPEC §5.1) 토큰 수도 그 퍼센트와 같은 분자를 쓴다. 대가 — 같은 세션에서 기존보다 작은 토큰 수가
  표시된다(문서 예시 payload 기준 `30% 60K` → `25% 50K`). 이 사용자 체감 변화는 2026-07-30에 사용자
  확인을 받아 채택으로 확정했다.
- 이전 턴의 모델 출력은 이미 분자에 들어 있다 — 다음 요청에서 input(대개 `cache_read_input_tokens`)으로
  되돌아오고 `total_input_tokens`가 그 세 값의 합이기 때문이다. 즉 input-only는 output을 버리는 것이
  아니라 **현재 턴 output을 미리 더하지 않는 것**이고, 그 값은 다음 턴에 input으로 편입된다. 지금
  방식은 같은 토큰을 두 번 세는 구간을 만든다.
- 분자 결정: `total_input_tokens`를 우선하고 0일 때만 `current_usage`의 input·cache_creation·cache_read
  합으로 보완한다. 근거 — 문서 조회 결과에서 `total_input_tokens`가 이미 그 세 값의 합이므로 통상 경로는
  덧셈 없이 끝나고, `current_usage`가 `null`인 세션 초반에도 zero 값으로 안전하게 0%가 된다(디코드
  동작 확인).
- 임계값(`contextTokenWarn` 256K / `contextTokenDanger` 512K)은 바꾸지 않는다(SPEC §4). 다만 input-only로
  바뀌면 발화 시점이 더 늦어진다는 사실은 상수 옆 주석으로 남긴다 — spec이 근거만 문서에 남기라고 했고,
  다음 사람이 "왜 200K 모델에서 안 뜨는가"를 다시 조사하지 않게 하는 최소 기록이다.

### D2. 세션 상태 필드의 위젯 분해 단위와 preset 문자

- 옵션 A: fast mode·effort·thinking을 묶은 위젯 1개.
- 옵션 B: 위젯 3개.
- 채택: **B**. 근거 — SPEC §5.2는 세 필드 각각에 대응하는 위젯과, 키가 없을 때 그 위젯이 생략되는 동작을
  요구한다. A에서는 키 하나만 없을 때 위젯이 생략되지 않아 조건을 만족할 수 없다. 대가 — 셋을 모두 켜면
  separator가 두 개 더 붙어 줄이 길어진다. 이는 기본 preset에 넣지 않는 opt-in 정책(SPEC §3)과 줄 맞춤
  (D7)으로 흡수한다.
- 배치: 세션·모델 상태이므로 `widgets_core.go`에 둔다(CLAUDE.md의 위젯 종류별 파일 규칙).
- preset 문자: `f`=fastMode, `E`=effort, `T`=thinking. 근거 — `F`는 제거된 performance 위젯의 문자이고
  `TestRemovedPresetCharsAreUnmapped`가 미매핑을 못 박고 있다. `F`를 재사용하면 옛 preset을 그대로 둔
  사용자에게 다른 위젯이 되살아나 보이고 그 회귀 가드도 무력해진다. 그래서 fastMode만 소문자 `f`로 두고,
  이 예외 이유를 매핑 표 주석에 남긴다. `E`·`T`는 기존 점유(`M C $ R 7 P N`)와 금지 집합
  (`S V a D B H F`) 모두와 충돌하지 않는다.

### D3. bool 상태가 false일 때의 표시 정책과 라벨 방식

- 옵션 A: 키가 있으면 항상 렌더하고 false는 꺼짐 표기로 보여준다.
- 옵션 B: 참일 때만 렌더하고 false는 위젯 생략.
- 옵션 C: 필드마다 기본 상태를 보고 규칙을 따로 정한다.
- 채택: **C** (2026-07-30 사용자 확인). `fast_mode`는 **참일 때만 렌더**하고, `thinking`은 **키가 있으면
  항상 렌더**하며 on/off를 구별해 표시한다. `effort`는 꺼짐 개념이 없어 값이 있으면 항상 렌더한다.
- 근거 — 두 bool의 **기본 상태가 서로 반대다**(§근거의 2026-07-30 재조회). `fast_mode`는 `/fast`로 켜는
  opt-in에 Opus 5·4.8 한정·usage credits 필요·$10/$50 가격이라 켜진 것 자체가 알릴 값이고, 꺼짐이
  압도적 다수인 상태다. `thinking`은 반대로 기본이 켜짐이고 Fable 5에서는 끌 수도 없다. 따라서 한 규칙
  ("참일 때만 렌더")을 둘에 함께 적용하면 `thinking` 위젯은 거의 항상 "켜짐"만 보여주면서 정작 알릴 값이
  있는 `false`를 숨기는 정반대 동작이 된다.
- 원래 채택안(B)이 들었던 "데이터가 의미를 갖지 않으면 위젯이 사라진다"는 근거는 **더 이상 성립하지
  않는다**. `20260730-001-session-start-placeholder` 이후 rate limit 위젯은 첫 응답 전에 `5h: -`로 흐린
  자리를 지키고, 첫 응답 뒤에만 사라진다(§근거). 이 저장소의 현재 규약은 "사라진다"가 아니라 "부재를
  구별해 표기한다"에 가깝고, 그 선례는 오히려 C를 지지한다.
- 두 필드가 statusline.md의 "Fields that may be absent" 목록에 없다는 사실이 C의 무게를 더한다 —
  SPEC §5.2 후단이 요구하는 "키가 없으면 위젯 생략"은 현행 Claude Code에서 도달하지 않는 경로이므로,
  `false`를 어떻게 다루는지가 곧 실제 화면이다. B를 택하면 `fast_mode`·`thinking` 위젯을 켠 사용자가
  대부분의 시간(fastMode)과 알릴 값이 있는 시점(thinking)에 아무것도 보지 못한다.
- 대가 — `thinking` 위젯을 켜면 상태와 무관하게 칸 하나를 상시 점유한다. 좁은 터미널에서는 D7의 줄
  맞춤이 흡수한다. `fastMode`는 반대로 꺼져 있는 동안 보이지 않으므로, 설정이 먹지 않은 것으로
  오해하지 않도록 README 위젯 표에 "fast mode가 켜진 동안에만 표시"를 명기한다.
- 라벨 방식 옵션: (a) 기호만, (b) locale 라벨 + 값. 채택 **(b)**. 근거 — 기호만으로는 세 상태를 구분해
  기억하기 어렵고, 폭 계산(D8)에서 East Asian Ambiguous·이모지 폭이 터미널마다 갈리는 기호를 늘리면
  SPEC §5.7 보장이 흔들린다. `effort.level` 값(low/medium/high/xhigh/max)과 `pr.review_state` 값은 시스템
  식별자이므로 번역하지 않고 그대로 쓴다.
- 위젯별 조립: `fastMode`는 **라벨만** 낸다 — 존재 자체가 값을 나르므로 `fast: on`처럼 항상 같은 값을
  덧붙이면 폭만 먹는다. `thinking`은 `<라벨>: on` / `<라벨>: off`로 값을 붙이고 on/off는 시스템 식별자로
  번역하지 않는다. `effort`는 `<라벨>: <level>`이다. 색은 rate limit 실측 갈래와 같은 결
  (`theme.Secondary` 라벨 + 기본색 값)로 두고 상태별 의미 색을 새로 배정하지 않는다 — `off`는 고장이
  아니라 사용자가 고른 상태이며, `theme.Dim`은 placeholder가 "미측정"으로 이미 쓰고 있어 뜻이 겹친다.
- 알 수 없는 상태 하나를 기록해 둔다: fast mode는 rate limit cooldown에서 표준 속도로 자동 강하하고
  CLI는 `↯` 아이콘을 회색으로 바꿔 알리지만, 그 상태를 담은 stdin 필드가 없다. 따라서 cooldown 중에도
  위젯은 켜짐으로 표시된다. stdin 밖 출처를 쓰지 않는다는 원칙(SPEC §3) 아래에서는 고칠 수 없는
  한계이며, 라벨 옆 주석으로 남긴다.

### D4. worktree 정보의 배치와 git 호출 대체 여부

- 옵션 A: 신규 worktree 위젯.
- 옵션 B: project 위젯 두 개를 확장해 worktree 토큰을 덧붙임.
- 채택: **B**. 근거 — SPEC §5.4가 project 계열 위젯이 worktree 정보를 반영하도록 요구하고, worktree는
  경로·브랜치와 같은 "지금 어디서 작업 중인가" 축이라 같은 토큰 묶음에 속한다. 위젯을 새로 만들면
  기본 preset에 넣지 못해(SPEC §3) 기본 사용자는 볼 수 없고, §5.4가 요구하는 "project 계열 위젯의
  반영"과도 어긋난다. 키가 없으면 토큰을 붙이지 않아 기존 출력과 동일하다(SPEC §5.4 후단).
  대가 — `project-widget-split`에서 걷어낸 `[worktree]` 토큰이 다시 생긴다. 그때 제거한 것은
  `--worktree` 세션에서만 채워져 대개 비어 있던 필드였고, 지금 들어오는 것은 일반 git worktree에서도
  채워지는 다른 필드다(문서 조회 결과). 표시 근거가 바뀌었으므로 되돌림이 아니다.
- 표시 형태: 문자열을 그대로 쓰지 않고 마지막 경로 요소만 취해 표시한다. 근거 — 문서 조회 결과가 이
  문자열이 경로인지 이름인지 확정하지 않는데, 경로면 base name이 읽기 좋고 이름이면 base name 연산이
  값을 바꾸지 않아 어느 쪽이든 같은 결과가 된다. 두 project 위젯이 같은 형태를 내도록 렌더 조립을
  공유한다.
- git 호출 대체: **하지 않는다**. 근거 — 이 필드는 worktree 위치만 알려주고 branch를 담지 않으므로
  `gitBranch`를 대신할 수 없다. 레거시 `worktree.branch`를 branch 출처로 쓰는 대안도 접는다 —
  `--worktree` 세션에서만 채워져 branch 출처가 세션 종류에 따라 갈리고, 이는 이 저장소가 이미 한 번
  공유 헬퍼로 통일한 지점을 다시 쪼개는 일이다. 하위 프로세스 절감은 캐시(D6)가 담당한다.

### D5. repo·pr의 배치와 review_state 표기

- 옵션 A: project 위젯 확장.
- 옵션 B: repo와 pr을 합친 신규 위젯 1개.
- 옵션 C: `repoInfo`·`pullRequest` 신규 위젯 2개.
- 채택: **C**. 근거 — A는 origin이 있는 저장소라면 거의 항상 값이 있어 기본 출력에 토큰이 새로 붙고
  SPEC §3의 기본 구성 불변과 좁은 터미널 우려에 정면으로 걸린다. B와 C 모두 SPEC §5.5(두 키가 다 없으면
  관련 출력 생략)를 만족하지만, C는 저장소 좌표와 PR 상태를 따로 켤 수 있어 이 저장소가 config 키 대신
  위젯 단위 선택으로 분기해 온 관례에 맞는다. 대가 — 위젯 두 개를 켜면 separator가 하나 더 붙는다.
- 표시: repoInfo는 `owner/name`을 쓰고 host는 표시하지 않는다(폭 대비 정보량이 낮고, 두 값만으로 저장소가
  식별된다). pullRequest는 번호가 양수일 때 `#<번호>`를, `pr.url`이 있으면 그 텍스트를 OSC 8 링크로
  감싼다 — 이미 있으나 호출자가 없던 `osc8Link`의 자연스러운 소비처다.
- `review_state` 표기: 원문 값을 그대로 쓰지 않고 짧은 기호로 접는다(`changes_requested`는 17칸으로
  status line에서 과하다). approved/pending/changes_requested/draft에 각각 기호를 배정하고, 문서에 없는
  값이 오면 기호를 붙이지 않고 `debugLog`만 남긴다. 근거 — 열거값이 늘어날 때 미확인 문자열이 그대로
  출력되는 것을 막는다(SPEC §3의 누락·미확인 필드 정상 처리 취지).

### D6. git 결과 캐시의 위치·형식·무효화와 과거 제거 경위

- 저장 위치 옵션: (a) 프로세스 메모리만, (b) `os.UserCacheDir()` 아래 파일, (c) `os.TempDir()` 아래 파일,
  (d) 설정 디렉토리 아래 파일. 채택 **(b)**. 근거 — cc-usage는 status line 갱신마다 새 프로세스로 실행되어
  (a)로는 SPEC §5.8을 만족할 수 없다. (d)는 설정 디렉토리를 프로필 단위로 공유·분리하는 사용자 정책에
  cc-usage가 상태를 끼워 넣는 일이라 §4의 프로필 정책 불개입과 어긋난다. (c)는 OS별 정리 정책이 제각각이라
  수명 예측이 어렵다. (b)는 플랫폼별 캐시 규약을 따르고, 삭제된 구버전이 하드코딩했던
  `$HOME/.cache/cc-usage/session-state-*.json`과 파일명 규약이 겹치지 않아 잔존 파일과 혼동되지 않는다
  (삭제 시점 코드로 경로 확인). `os.UserCacheDir()` 실패 시에는 캐시를 쓰지 않고 매번 git을 실행한다.
- 키 옵션: (a) session_id만, (b) session_id + 대상 디렉토리, (c) session_id 없으면 디렉토리만으로 대체.
  채택 **(b)**, 그리고 **session_id가 비어 있으면 캐시를 아예 타지 않는다**. 근거 — 한 세션이 디렉토리를
  바꿀 수 있어 (a)는 다른 디렉토리의 브랜치를 보여줄 수 있다. (c)는 삭제된 구버전에서 session_id 없는
  cwd-only 키가 세션 간 교차 오염을 만든 경로 그 자체이므로 되풀이하지 않는다. session_id는 stdin에서
  오는 신뢰할 수 없는 문자열이므로 파일명에 그대로 쓰지 않고 해시(표준 라이브러리)로 접어 경로 조작
  여지를 없앤다.
- 저장 내용: 대상 디렉토리, branch 문자열, 기록 시각. `cost`·`rate_limits` 유래 값은 담지 않는다
  (SPEC §3, §5.8 후단). 위젯 렌더 결과나 stdin 원문도 담지 않는다.
- 무효화 옵션: (a) 짧은 고정 TTL, (b) `.git/HEAD` 스탬프 기반 정확 무효화, (c) 무효화 없음.
  채택 **(a)**. 근거 — (c)는 세션 중 브랜치를 바꾸면 세션이 끝날 때까지 틀린 브랜치를 보여준다.
  (b)는 이론적으로 정확하지만 git 디렉토리 탐색(상위 디렉토리 순회, worktree·submodule의 `.git` 파일
  안 `gitdir:` 해석, `GIT_DIR` 환경변수)을 직접 구현해야 하고, 이는 zero dependency 제약 아래에서 git
  내부 규약을 재구현하는 일이라 캐시가 노리는 절감보다 위험이 크다. (a)는 몇 초 단위 고정 TTL로
  staleness를 그 시간 안에 가두고, status line이 짧은 간격으로 연달아 호출되는 구간에서 하위 프로세스를
  걷어낸다. TTL은 코드 내부 상수로 두고 config 키는 만들지 않는다 — 설정 표면을 늘리지 않는 이 저장소
  관례를 따르고, 필요해지면 `widgets` 네임스페이스 패턴으로 나중에 노출할 수 있다.
- 쓰기·청소 방식: 임시 파일 생성 후 rename으로 교체하고, advisory lock은 쓰지 않는다. 오래된 파일은
  쓰기 경로에서 나이 기준으로 동기 정리하며 goroutine을 만들지 않는다. 읽기 실패·JSON 파손·형식 불일치는
  모두 미적중으로 처리한다.
- 과거 제거와의 관계: 삭제된 캐시가 남긴 결함은 네 갈래였다 — 계정 유래 값(cost) 오염, 청소 부재로 인한
  파일 무한 누적, lock 파일 누적, session_id 부재 시 cwd 키 교차 오염(각각 `session-state-fixes`·
  `lock-leak-cleanup` spec과 삭제 시점 `cache.go`에서 확인). 이번 캐시는 저장 값이 git 유래 branch 하나로
  닫혀 있어 첫째가 성립하지 않고(계정 값이 아예 들어가지 않는다), 쓰기마다 나이 기준 청소를 하고,
  lock을 쓰지 않으며, session_id가 없으면 캐시를 타지 않는다. 더 근본적인 차이는 역할이다 — 옛 캐시는
  stdin에 없는 필드를 캐시로 복원하는 계층이라 캐시가 곧 표시의 진실 출처였고, 이번 캐시는 언제든 git으로
  다시 얻을 수 있는 국소 파생값의 메모라서 무효화되면 표시가 원래 경로로 되돌아간다.
- 진입점: `gitBranch`는 하위 프로세스 원시 호출로 그대로 두고 캐시 경유 래퍼를 새로 두어 두 project
  위젯이 그것을 부른다. 근거 — `gitBranch`의 타임아웃·`(detached)`·실패 degrade 동작을 건드리지 않아
  기존 테스트가 그대로 유효하고, 캐시 유무가 branch 의미를 바꾸지 않는다. 한 프로세스에서 project 위젯을
  둘 다 켠 경우 두 번째 조회는 방금 쓴 값을 재사용해 파일 I/O도 한 번으로 끝난다.
- `ROADMAP.md` §서비스 완료 기준 6번과의 충돌은 **기준을 좁히는 쪽으로 닫혔다**(2026-07-30 사용자 확인).
  이전 문구는 "네트워크 엔드포인트에 접속하지 않고 **캐시 파일을 읽거나 쓰지 않으며**"였고, 이 D6과
  SPEC §3·§5.8이 요구하는 git 캐시가 그 문장을 정면으로 어긋나게 만들었다. 대안은 캐시 도입을 이
  feature에서 빼는 것이었으나, 그 기준이 실제로 막으려던 것은 계정 유래 값이 프로필 간에 남는 경로와
  렌더 중 네트워크 접속이고 둘 다 이 캐시에서 성립하지 않는다 — 담기는 값이 git 유래 branch 하나로
  닫혀 있고 네트워크를 타지 않는다. 좁힌 문구는 "계정 유래 값을 디스크에 저장하지 않으며, 디스크에 쓸 수
  있는 것은 git으로 다시 얻을 수 있는 국소 파생값뿐"이다. **이 문구가 캐시 설계에 거는 조건은 하나** —
  캐시가 유실·손상되면 표시가 git을 그대로 실행하는 경로로 되돌아가야 한다. 위의 무효화·미적중 처리가
  이미 그 성질을 갖는다.

### D7. COLUMNS 반영 방식 — 측정 위치, 축소 순서, barWidth 우선순위

- 측정·조정 위치 옵션: (a) 위젯 렌더 후 줄 단위로 조정, (b) 폭 예산을 `Context`에 실어 각 위젯이 스스로
  줄임, (c) 두 번 렌더(측정 후 축소 힌트로 재렌더). 채택 **(a)**. 근거 — (b)는 어떤 위젯도 다른 위젯의
  폭을 모르므로 자기 몫을 정할 수 없고, (c)는 위젯 계약에 축소 모드를 추가해 모든 위젯이 두 가지 렌더를
  갖게 만든다. (a)는 위젯 계약을 건드리지 않고 조인된 줄 하나만 다루므로 SPEC §5.7의 "각 줄" 단위와
  정확히 맞고, 축소 결과가 결정적이다.
- 축소 순서: 오른쪽 위젯부터 하나씩 덜어내고, 위젯 하나만 남았는데도 넘치면 표시폭 기준으로 잘라
  생략 기호와 리셋 코드를 붙인다. 근거 — 사용자가 `lines`·`preset`에 적은 순서가 곧 우선순위이므로
  왼쪽(경로·모델·컨텍스트)을 지키는 것이 의도에 가깝고, 마지막 절단 단계가 있어 어떤 입력에서도
  "폭을 넘지 않는다"가 무조건 성립한다. 잘라낸 자리에 색 코드가 열린 채 남지 않도록 리셋을 붙인다.
- `widgets.context.barWidth` 우선순위: **명시 설정이 항상 이긴다**. 근거 — 축소를 줄 단위로 하는
  채택 아래에서 bar 폭은 조정 대상이 아니고, 사용자가 명시한 값을 프로그램이 조용히 덮어쓰면 설정이
  먹지 않는 것으로 보인다. 폭이 부족한 상황은 위젯 생략과 절단으로 해결하므로 SPEC §5.7 보장은 유지된다.
  경로 축약 예산(`pathDisplayMaxRunes`)도 같은 이유로 손대지 않는다.
- COLUMNS 해석: 정수 파싱에 실패하거나 0 이하면 미설정과 같게 취급하고, 여유 칸을 따로 남기지 않는다
  (조건이 "넘지 않는다"이므로 값과 같은 폭은 허용).
- SPEC §5.3(설정 없는 환경의 구성 불변)과의 관계: 줄 맞춤은 COLUMNS가 실제로 제약이 될 때만 개입하므로,
  COLUMNS가 없거나 줄이 들어가는 환경에서는 출력이 이번 변경 전과 완전히 같다. 폭 제약이 실제로 걸리는
  환경에서는 §5.3이 §5.7에 양보한다는 우선순위가 spec §5.3 본문에 명시되어 있으므로, 좁은 터미널에서
  뒤쪽 위젯이 사라지는 것은 두 조건을 함께 만족시키는 유일한 동작이다. 이 체감 변화는 2026-07-30에
  사용자 확인을 받아 채택으로 확정했다 — 좁은 터미널에서 기본 6개 중 뒤쪽(7d → 5h → cost)이 빠진
  화면이 나오는 것을 포함한다.
- `LINES`는 소비하지 않는다. 근거 — SPEC §5.7은 가로 폭만 요구하고, 세로 제한은 줄 수를 줄이는
  별개 정책(어떤 줄을 버릴지)을 필요로 해 범위를 넘는다.

### D8. 표시폭 계산의 구현 범위

- 옵션 A: 기존 `truncate`처럼 rune 수로 계산.
- 옵션 B: ANSI 이스케이프를 제외하고 rune별 표시폭(wide=2, 결합 문자=0)을 재는 계산기를 새로 둔다.
- 채택: **B**. 근거 — 출력에는 색 코드가 rune의 절반 이상 섞여 있어 A로는 폭이 크게 과대평가되고,
  한글 경로·브랜치명은 실제로 두 칸을 먹어 과소평가된다. 어느 쪽이든 SPEC §5.7의 보장이 깨진다.
  zero dependency 제약(SPEC §3) 아래에서는 외부 폭 라이브러리를 쓸 수 없으므로 직접 둔다.
- 범위: CSI 색 코드와 OSC 8 링크 시퀀스를 폭 0으로 건너뛰고, 폭 2 rune은 한글·CJK·가나·전각 폼·
  주요 이모지 구간을 담은 고정 범위 표로 판정한다. East Asian Ambiguous(현재 출력에 쓰이는 `◆ ◇ ○ ●
  █ ░ │ ↑ ↓ …` 포함)는 1칸으로 본다 — 대다수 터미널 기본 동작이며, 이 가정 덕분에 기존 렌더 문자의
  폭이 변하지 않는다. 표에 없는 희귀 문자는 1칸으로 떨어지는 근사이며, 그 한계를 함수 주석에 남긴다.
- 기존 `truncate`는 손대지 않는다. 근거 — 호출자가 없지만 rune 수 기준 절단이라 표시폭 절단으로 대체할
  대상이 아니고, spec §1이 이 정리를 범위로 들지 않았다. 새 절단 함수와 역할이 다르다는 점만 주석으로
  구분한다.

### D9. 모델 기호 매핑 확장과 `Translations.Model` 처리

- 매핑 구조 옵션: (a) 기존 if/else 체인에 두 분기 추가, (b) 순서가 드러나는 (부분 문자열, 기호) 표 +
  기본값. 채택 **(b)**. 근거 — 계열이 다섯으로 늘고 앞으로도 늘어날 축인데, 체인은 판정 순서가 코드
  흐름에만 있어 새 계열을 끼울 때 순서 실수가 드러나지 않는다. 표는 항목 추가가 한 줄이고 기본 기호가
  한 곳에서만 정의된다. 새 추상화가 아니라 상수 슬라이스 하나다.
- 기호: fable·mythos에 기존 `◆ ◇ ○ ●`과 같은 Geometric Shapes 계열의 다른 글리프를 배정한다
  (예: fable `◈`, mythos `◎`). 근거 — SPEC §5.6은 기본 기호가 아닌 계열별 기호만 요구한다. 같은 블록에서
  고르면 폭 가정(D8의 Ambiguous=1)과 터미널 지원 범위가 기존 기호와 동일해 새 위험이 없다. 이모지는
  폭이 갈려 배제한다.
- `Translations.Model` 옵션: (a) 구조체 필드와 locale `model` 블록 제거, (b) 살려서 모델 표시 이름으로 사용.
  채택 **(a)**. 근거 — (b)는 표시 이름을 현재의 모델 ID에서 계열명으로 바꾸는 일이라 기본 출력 텍스트를
  건드리고 spec 범위 밖이다. 게다가 en/ko의 값이 동일해 번역 자산으로서의 가치가 없고, 이름 출처가
  둘(모델 ID / locale)로 갈리면 다음 사람이 어느 쪽을 고쳐야 하는지 알 수 없다. 죽은 키를 제거하는 것은
  `simplify-statusline`에서 죽은 locale 키를 `Translations` 필드와 함께 지운 선례와 같다. 계열명 표시가
  필요해지면 (b)가 아니라 기호 표를 확장하는 것이 자연스러운 자리다.

### D10. 설치 스킬의 설정 홈 인식 표현

- 옵션 A: 후보 목록에 `CLAUDE_CONFIG_DIR` 경로 두 줄을 추가해 6개 후보로 만든다.
- 옵션 B: 절차 앞에 "사용자 설정 홈" 정의(설정되어 있으면 `CLAUDE_CONFIG_DIR`, 아니면 `~/.claude`)를 두고
  사용자 스코프 후보와 신규 생성 기본값이 그 정의를 참조하게 한다.
- 채택: **B**. 근거 — A는 두 환경의 후보를 한 목록에 섞어 우선순위가 6갈래로 늘고, 잘못 고르는 조합
  (env가 설정된 환경에서 `~/.claude`를 먼저 고르는 경로)이 목록 안에 그대로 남는다. B는 치환 지점이
  하나여서 SPEC §5.9의 두 요구(env 설정 시 그 디렉토리, 미설정 시 기존과 동일)가 정의 한 줄에서 함께
  성립한다. 공백만 있는 값은 미설정으로 본다 — `main.go`의 `configHomeDir`와 같은 규칙이라 설치가 고른
  파일과 cc-usage가 읽는 설정 위치가 갈리지 않는다.
- 프로젝트 스코프(`.claude/settings.local.json`, `.claude/settings.json`)는 저장소 상대 경로이므로
  바꾸지 않는다. 근거 — `CLAUDE_CONFIG_DIR`은 사용자 설정 홈을 옮기는 값이고, 프로젝트 스코프를 함께
  옮기면 저장소별 설정이 엉뚱한 곳을 가리킨다. 스킬 출력에는 어느 후보를 골랐는지와 함께 설정 홈이
  env에서 왔는지도 한 줄로 밝혀, 사용자가 statusLine이 안 뜰 때 원인을 바로 볼 수 있게 한다.
- 스킬 산문은 영어를 유지한다. 근거 — 이 파일은 배포 산출물로 release 브랜치에 복사되는 사용자 대상
  문서이며 현재 전문이 영어다. 이번 변경은 후보 경로 교정에 한정한다.

### D11. 문서 갱신 범위와 버전 bump 폭

- `CLAUDE.md`(SPEC §5.10): 없는 테스트 이름을 든 단일 테스트 예시를 실제 존재하는 테스트로 바꾸고,
  동작 확인 예시의 기대 출력을 실행 결과와 일치시키며(현재 첫 토큰과 퍼센트·토큰 수가 어긋나고, D1
  적용 후 값도 달라진다), 아키텍처 표에서 ahead/behind 서술을 걷고 `projectName`을 등재하고, stdin 메모의
  총 input/output 의미를 문서 조회 결과에 맞춘다. 이번에 추가되는 캐시 파일과 COLUMNS 소비도 아키텍처·
  경로 서술에 넣는다 — 개발자 진실 layer가 외부 상태와 환경 입력을 빠뜨리면 다음 작업이 "상태 없음"을
  전제로 잘못된 판단을 한다.
- `README.md`: 위젯 표에 신규 5개와 preset 문자를 넣고, `fastMode`에는 "fast mode가 켜진 동안에만 표시"를
  명기한다(D3). Privacy의 "캐시 파일을 읽거나 쓰지 않는다"는 실제 동작(git 유래 정보만 담고 계정 관련
  값은 저장하지 않으며, 캐시가 없어도 표시가 달라지지 않음)으로 교정한다 — `ROADMAP.md` §서비스 완료
  기준 6번이 같은 방향으로 좁혀졌으므로(D6) 두 문서의 문구가 같은 성질을 가리켜야 한다.
- 버전 옵션: (a) patch, (b) minor(0.6.0). 현재 값은 0.5.6이다. 채택 **(b)**. 근거 — 새 위젯 다섯 개와
  COLUMNS 반영은 버그 수정이 아닌 기능 추가이고 context 퍼센트·토큰 표기 의미도 바뀐다. `Makefile` VERSION과
  `.claude-plugin/plugin.json` version에 같은 값을 넣어야 `/plugin` UI가 업데이트를 감지한다(SPEC §5.11).
  `bin/` 재빌드와 release 브랜치 동기화는 SPEC §5에 조건이 없는 배포 단계이며 CLAUDE.md §배포 절차가
  소유한다.
