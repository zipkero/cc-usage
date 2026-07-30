# session-start-placeholder — ANALYSIS

## 근거

읽은 spec 범위: `spec.md` §1–§5 전체. 범위는 세션 첫 API 응답 전 구간에서 `context`·`rateLimit5h`·
`rateLimit7d` 세 위젯의 **표기**만 바꾸는 것이고(§1), 제약은 zero dependency·단일 `main` 패키지·
stdout 오염 금지·데이터 출처를 stdin과 git으로 한정·대상 위젯 3개 한정·해제 신호를
`context_window.total_input_tokens > 0` 하나로 통일·ASCII 1칸 문자·기존 theme dim 재사용·기존 생략
규약 유지·버전 동시 갱신(§3), 제외는 cost placeholder·stdin 밖 출처·placeholder config 옵션·stdin 파싱
전면 실패 완화·`COLUMNS` 폭 축소·`resets_at` 표기 정리·schema-catchup 문서 갱신(§4)이다.
spec.md에 미답(승인 전 확인) 항목은 없다.

표기 값의 출처를 밝혀 둔다. placeholder 문자 `-`, `5h: -`·`░░░░░░░░ -` 조립 형태, rate limit 라벨까지
흐리게 하는 범위는 `/spec-init` 단계에서 사용자가 선택한 화면 예시가 확정한 값이며, 본 analysis는 그
선택을 §5 D3·D4에서 근거와 함께 설계로 옮겼다. 한 곳만 갈라진다 — 그 예시는 context 칸 전체(progress
bar 포함)를 흐리게 표시했으나 D4는 bar를 dim 대상에서 뺀다. 근거는 D4에 적었다.

코드에서 직접 확인한 사실:

- 진입점(`main.go`)은 `flag 파싱 → loadConfig → parseStdin → loadTranslations → Context 조립 →
  shouldSuppressOutput → orchestrate → stdout` 한 경로다. `shouldSuppressOutput`의 판정 입력은
  `Workspace.CurrentDir`·`Model.ID`·`Model.DisplayName`·`ContextWindow.ContextWindowSize`뿐이다.
- `StdinInput`(`stdin.go`)에는 이번에 필요한 필드가 이미 다 있다 — `ContextWindow.TotalInputTokens`는
  값 `int`, `UsedPercentage`/`RemainingPercentage`는 `*float64`, `RateLimits`와 그 하위
  `FiveHour`/`SevenDay`는 각각 포인터다. 즉 rate limit은 **키 부재를 nil로 구별할 수 있고**,
  토큰 수는 0과 부재를 구별할 수 없다. 스키마 추가는 필요 없다.
- `parseStdinReader`는 어떤 decode 오류에서도 빈 `StdinInput`을 돌려준다 — 부분 실패 상태가 없다.
- `orchestrate`(`widget.go:141`)는 라인별로 `disabledWidgets` → registry 조회 →
  `GetData`(nil 또는 error면 skip) → `Render`(빈 문자열이면 skip) → separator 조인만 한다.
  위젯이 화면에 나타날지를 결정하는 유일한 장치는 이 두 skip이다.
- `contextWidget.GetData`(`widgets_core.go:68`)는 `ContextWindowSize <= 0`이면 nil을 돌려 칸이
  사라지고, 그 외에는 `UsedPercentage`가 있으면 `clampPercent`, 없으면 input+output 합으로
  `calculatePercent`를 쓴다. 표시 토큰 수도 같은 합이다. `Render`는 `"%s %s%d%%%s %s%s%s"` 형식으로
  bar·퍼센트·토큰을 공백 두 개로 이어 붙인다.
- `rateLimit5hWidget.GetData`/`rateLimit7dWidget.GetData`는 대응 포인터가 nil이면 `nil, nil`을 돌린다 —
  이것이 현재 첫 렌더에서 두 칸이 사라지는 유일한 원인이다. placeholder를 내려면 이 반환을 바꿔야 한다.
- `renderRateLimit`(`widgets_core.go:179`)은 `theme.Secondary`로 라벨을, `getColorForPercent` 색으로
  퍼센트를 내고 RESET으로 닫은 뒤, `ResetsAt`이 Go zero time이 아니고 잔여 시간이 `"0"+분` 문자열과
  다를 때만 `theme.Dim`으로 `(잔여)`를 덧붙인다.
- `render.go`의 모든 테마가 `Dim: "\x1b[2m"`으로 동일하다. 이는 색이 아니라 SGR 2(faint) 속성이고
  `RESET`은 SGR 0이므로, `theme.Secondary` 뒤에 RESET 없이 `theme.Dim`을 이어 붙이면 색은 유지된 채
  faint만 더해진다. `Secondary`는 테마별로 다른 전경색이다.
- `getColorForPercent(0, theme)`는 `Safe`를 돌려준다. 따라서 `renderProgressBar(0, ...)`는
  `Safe` 코드를 낸 뒤 채운 칸이 0개이고 곧바로 `BarEmpty`로 덮이는 구조다 — 실제 출력 바이트로 확인했다
  (`^[[38;5;151m^[[38;5;240m░░░░░░░░^[[0m`). 색이 칠하는 대상이 없는 상태가 이미 현행 동작이다.
- 현행 동작을 실행으로 재현했다. `rate_limits` 없음 + `total_input_tokens: 0` + `context_window_size:
  200000` payload는 `<경로> (main) │ ◆ <모델> │ ░░░░░░░░ 0% 0 │ $0.00`을 내고, 5h·7d 칸은 없다.
  같은 payload에 토큰과 `rate_limits`를 채우면 `██░░░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%`가 나온다 —
  후자는 `CLAUDE.md` §동작 확인의 기대 출력과 일치하므로 이번 변경이 그 예시를 무효화하지 않는다.
  `resets_at: 0`에서 잔여 시간 표기가 붙지 않는 것도 이 실행으로 확인했다(SPEC §4의 관찰과 일치).
- 위젯 데이터 타입은 테스트가 리터럴로 구성한다 — `widgets_core_test.go:74,86,98,110`이
  `&contextData{Percent: …, TotalTokens: …}`를, `stdin_test.go:58`이 `data.(*contextData).Percent`를
  쓴다. `rateLimitData`와 `renderRateLimit`을 참조하는 테스트는 없다(grep 무결과).
- `newContextRenderCtx()`(`widgets_core_test.go:10`)는 `Config.Theme`만 채운 `Context`를 만든다 —
  `Stdin`은 zero value이므로 `TotalInputTokens`가 0이다. `splitContextRender`는 렌더 결과를 공백으로
  3분할하고 세 조각이 아니면 `t.Fatalf`한다.
- `Translations`에는 `Labels.FiveH`/`SevenD`와 `Time.*`, 그리고 소비처가 없는 `Model` 블록이 있다
  (`grep Translations.Model` / `Model.(Opus|Sonnet|Haiku)` 무결과). `locales/en.json`과 `ko.json`은
  `model` 블록의 값이 서로 완전히 동일하다 — 양쪽에 같은 값을 넣는 키가 번역 자산이 되지 못한 선례다.
- `config.go`의 위젯 옵션 규약은 `WidgetConfig` 아래 위젯별 struct(`Context ContextWidgetConfig`)이고
  zero value를 "미설정"으로 보고 `ContextBarWidth()`가 `defaultContextBarWidth = 8`로 폴백한다.
  범위 밖 값은 `validateConfigWidgets`가 stderr 경고 후 0으로 리셋한다.
- 테스트 관례: 파일당 `Test<대상>` + `t.Run` 서브테스트, 순수 로직은 케이스 슬라이스(table),
  ANSI 제거는 `stripANSI`(`widgets_project_test.go:9`), git 차단은 `t.Setenv("PATH", "")`,
  orchestrate 수준 검증은 `Separator: "space"` + 명시 `Lines`/`Preset`으로 대상을 좁힌다
  (`widget_test.go:31,55`).
- 버전 현황: `Makefile` `VERSION := 0.5.4`, `.claude-plugin/plugin.json` `"version": "0.5.4"`.
- `README.md` 위젯 표는 `context`를 "프로그레스바 + 사용률 + 토큰 수", `rateLimit5h`/`7d`를
  "5시간/7일 rate limit"으로만 적고 있어 첫 응답 전 표기를 담고 있지 않다(SPEC §5.12 대상).

SPEC 문구 해석 하나를 밝혀 둔다. §5.7("placeholder 표기가 ASCII 범위 문자로만 이루어진다")은 context
칸 전체가 아니라 **placeholder 표기 자체**를 가리킨다 — §5.2가 같은 stdin에서 progress bar(`░`,
비-ASCII)를 함께 출력하도록 요구하므로 다른 읽기는 두 조건을 동시에 만족시킬 수 없다. §3의
"ASCII 범위의 1칸 문자만"도 글자 수를 1개로 제한하는 것이 아니라 쓰이는 각 문자의 표시폭을 1칸으로
제한하는 것으로 읽는다(제약의 근거가 표시폭 갈림이므로).

추정으로 남기는 것: `/clear`·`/compact` 직후 렌더에서 `total_input_tokens`가 다시 0으로 보고되는지는
확인하지 못했다. §5 D2의 우선순위 결정은 이 추정이 맞든 틀리든 실측값을 숨기지 않는 쪽으로 택했다.

## 1. 구조

새 모듈·레이어·서브 패키지·파일을 만들지 않는다. 이 변경은 "이미 stdin에 있는 값의 표기"만 바꾸므로
기존 경계 셋 안에서 끝난다.

- **오케스트레이션 경계**(`widget.go`): `Context`에 "첫 API 응답이 도착했는가"를 답하는 파생 판정
  하나를 둔다. `Stdin`에서만 계산되는 읽기 전용 파생값이며, `orchestrate`의 skip 규약·registry·
  `displayPresets`·`presetCharToWidget`은 손대지 않는다.
- **core 위젯 경계**(`widgets_core.go`): 세 위젯이 그 판정을 소비한다. context 위젯은 데이터에 상태를
  하나 더 실어 `Render`가 두 갈래로 갈리고, rate limit 두 위젯은 `GetData`가 nil을 돌리는 조건이
  좁아지며 공유 렌더가 placeholder 갈래를 갖는다. `cost`·`model` 위젯은 건드리지 않는다(SPEC §3, §4).
- **렌더 경계**(`render.go`): placeholder 문자 상수와 "주어진 문자열을 흐리게 감싸는" 조립 하나를
  둔다. ANSI 코드와 테마 색 합성은 `CLAUDE.md` 아키텍처 표가 이 파일에 배정한 역할이고, placeholder
  문자는 `█`·`░`와 같은 표시 글리프 계열이다.

경계가 바뀌지 않는 곳을 명시한다 — `stdin.go`(필요한 필드가 이미 있다), `config.go`(placeholder
옵션은 SPEC §4 제외), `locales/*.json`과 `Translations`(§5 D5), `main.go`의 `shouldSuppressOutput`
(SPEC §3, §5.9), `format.go`. 사용자 진실 layer인 `README.md`만 표기 동작을 반영한다(SPEC §5.12).

## 2. 데이터 흐름

진입부터 stdout까지의 순서. 굵은 항목이 이번에 끼어드는 단계다.

1. `loadConfig(path)` — 새 config 키가 없으므로 불변. `widgets.context.barWidth` 해석도 그대로다.
2. `parseStdin()` — 스키마 변경이 없으므로 불변. decode 실패는 기존대로 빈 `StdinInput`으로 떨어진다.
3. `loadTranslations()` — locale 키 변경이 없으므로 불변.
4. `Context{Stdin, Config, Translations}` 조립 — 필드 추가 없음. **첫 응답 판정은 이 시점에 계산해
   저장하지 않고, `Stdin`에서 그때그때 파생하는 판정으로 노출한다**(§5 D1).
5. `shouldSuppressOutput` — 판정 입력이 그대로이므로 불변. placeholder는 이 관문을 통과한 뒤에만
   의미를 가진다(SPEC §5.9).
6. `orchestrate` — 라인별로 위젯을 돌린다. **위젯 세 개의 데이터 경로에 placeholder 분기가 들어간다.**
   - context: `ContextWindowSize <= 0`이면 지금처럼 nil → 칸 생략(SPEC §5.8). 그 외에는 퍼센트·토큰
     계산을 지금과 똑같이 수행하고, 여기에 **첫 응답 판정 결과를 상태로 함께 실어** 돌려준다.
     `Render`는 그 상태가 "미측정"이면 bar + placeholder를, 아니면 기존 bar + 퍼센트 + 토큰을 낸다.
   - rateLimit5h / rateLimit7d: 대응 포인터가 있으면 **판정과 무관하게** 지금처럼 실측 데이터를
     돌린다. 포인터가 없을 때만 판정을 보고, 첫 응답 전이면 placeholder 상태를 실은 데이터를,
     첫 응답 이후면 지금처럼 nil을 돌려 칸을 생략한다(§5 D2).
   - cost·model·projectInfo·projectName: 경로 불변(SPEC §3, §4).
7. stdout 출력 — `orchestrate` 결과 줄을 그대로 이어 붙인다. 진단은 전부 `debugLog`로 stderr(SPEC §3).

상태 전이는 세션당 한 번, 한 방향이 기본이다.

```
첫 렌더 (rate_limits 키 없음, total_input_tokens == 0)
  context     : ░░░░░░░░ -          (bar 빈 상태 + dim placeholder)
  rateLimit5h : 5h: -               (라벨 유지 + dim placeholder)
  rateLimit7d : 7d: -
  cost        : $0.00               (참값, 변경 없음 — SPEC §5.3)

첫 API 응답 이후 (total_input_tokens > 0)
  context     : ██░░░░░░ 30% 60K    (기존과 동일 — SPEC §5.6)
  rateLimit5h : 5h: 42%             (구독 계정: 실측 — SPEC §5.5)
              : (칸 없음)            (비구독 계정: rate_limits 영구 부재 — SPEC §5.4)
```

부재·실패 경로:

- `context_window_size <= 0`: placeholder 판정에 닿기 전에 `GetData`가 nil을 돌려 context 칸이
  사라진다. 토큰 값과 무관하다(SPEC §5.8).
- 정체성 신호 전부 부재: `shouldSuppressOutput`이 `orchestrate` 앞에서 끊으므로 placeholder만 남은
  줄이 나오는 경로가 없다(SPEC §5.9).
- stdin decode 실패: 빈 `StdinInput` → 정체성 신호도 전부 비어 있으므로 위 경로로 수렴해 무출력이다.
  파싱 실패 완화는 범위 밖이며(SPEC §4) 이번 변경이 그 성질을 건드리지 않는다.
- `model`만 있고 `context_window`가 없는 payload: context 칸은 생략되지만 5h·7d는 placeholder로
  나타난다. rate limit placeholder는 `context_window_size`를 조건으로 두지 않기 때문이며, 이는
  "신호 하나로 통일"(SPEC §3)의 직접적 결과다. 실사용에서 `context_window_size`는 첫 렌더에도 채워지므로
  (SPEC §1) 도달하지 않는 조합이고, 추가 관문을 두는 것보다 신호를 하나로 유지하는 편을 택한다.
- placeholder 상태의 rate limit 데이터는 reset 시각을 담지 않는다. 따라서 `formatTimeRemaining`과
  `(잔여)` 접미사 경로에 들어가지 않고, SPEC §4가 범위 밖으로 둔 `resets_at`/`time.Unix(0, 0)` 문제를
  건드리지 않는다.
- `/clear`·`/compact`로 `total_input_tokens`가 0으로 되돌아가는 경우(추정): context는 다시 placeholder가
  되고 — 실제로 다시 미측정이므로 맞는 표기다 — rate limit은 키가 이미 있으므로 실측값을 계속 낸다.

## 3. 인터페이스

경계를 가로지르는 계약만 적는다.

- **stdin 스키마**(Claude Code → cc-usage): 변경 없음. 수용 필드를 늘리지 않고 기존 필드의 의미도
  재해석하지 않는다.
- **`Widget` 인터페이스**: `ID()`/`GetData(*Context) (any, error)`/`Render(any, *Context) string` 계약,
  nil·error·빈 문자열 skip, 패닉 금지 모두 불변이다.
- **`rateLimit5h`/`rateLimit7d`의 `GetData` nil 조건**: 이 계약만 좁아진다. 지금은 "대응 rate limit
  포인터가 nil이면 nil"이고, 앞으로는 "포인터가 nil **이고** 첫 응답이 이미 도착했으면 nil"이다.
  호출자는 `orchestrate` 하나이며 skip 규약 자체는 그대로다.
- **`Context`**: 첫 응답 도착 여부를 답하는 파생 판정이 추가된다. 값은 `Stdin` 하나에서 계산되며
  별도 초기화·주입이 필요 없다 — `Context`를 직접 만드는 코드(테스트 10곳)가 새 필드를 채울 의무를
  지지 않는다는 것이 이 형태의 계약이다(§5 D1).
- **위젯 데이터 타입의 zero value**: `contextData`에 상태 필드가 하나 붙는다. 이 타입은 패키지 내부
  구조지만 테스트가 리터럴로 구성하므로 **zero value가 "실측"을 뜻해야 한다** — 극성을 뒤집으면
  기존 리터럴 4곳이 조용히 placeholder 경로로 넘어간다(§5 D2, §4).
- **위젯 ID·preset 문자·`displayPresets`**: 불변. 새 위젯이 없고 기본 레이아웃도 그대로다.
- **config 스키마**: 불변. placeholder를 켜고 끄는 키를 만들지 않는다(SPEC §4).
- **locale 키**: 불변. placeholder 문자는 번역 대상이 아니다(§5 D5).
- **사용자 대상 표시 계약**: 첫 응답 전 context 칸은 `<빈 bar> -`, 5h·7d 칸은 `<라벨>: -`로 나타나고
  모두 dim으로 표시된다. dim이 무시되는 단색 터미널에서도 `-`만으로 미측정임이 읽힌다(SPEC §3).
  이 계약을 `README.md` 위젯 표에 명기한다(SPEC §5.12).
- **버전 계약**: `Makefile` VERSION과 `.claude-plugin/plugin.json` version이 같은 새 값을 갖는다 —
  `/plugin` UI의 업데이트 감지가 후자에 의존한다(SPEC §5.10, CLAUDE.md §버전 정책).

## 4. 영향 범위

수정 파일·함수(탐색으로 확인한 직접 대상):

- `widget.go` — `Context`에 첫 응답 판정 추가. `orchestrate`·`registry`·`displayPresets`·
  `presetCharToWidget`·`resolvePreset`·`Translations`는 불변.
- `widgets_core.go` — `contextData`에 상태 필드 추가, `contextWidget.GetData`에 상태 설정,
  `contextWidget.Render`에 placeholder 갈래, `rateLimit5hWidget.GetData`·`rateLimit7dWidget.GetData`의
  nil 조건 축소, `rateLimitData`에 상태 필드 추가, `renderRateLimit`에 placeholder 갈래.
  `modelWidget`·`costWidget`·`init()` 등록은 불변.
- `render.go` — placeholder 문자 상수와 흐리게 감싸는 조립 추가. `themes`·`getTheme`·
  `getColorForPercent`·`renderSeparator`·`renderProgressBar`와 bar 폭 상수는 시그니처·동작 모두 불변.
- `README.md` — 위젯 표의 `context`·`rateLimit5h`·`rateLimit7d` 설명에 첫 응답 전 표기 추가(SPEC §5.12).
- `Makefile` VERSION과 `.claude-plugin/plugin.json` version — 같은 새 값으로 동시 갱신(SPEC §5.10).

수정하지 않는 파일과 그 근거:

- `stdin.go`·`config.go`·`format.go`·`locales/*.json`·`main.go`·`widgets_project.go` — 각각 §1에서
  경계가 바뀌지 않는 이유를 적었다. 특히 `clampPercent`/`calculatePercent`는 context와 rate limit이
  공유하지만 두 함수의 계약은 그대로다.
- `CLAUDE.md` — 아키텍처 표·위젯 추가 절차·출력 채널 규칙·`GetData` nil skip 서술이 모두 여전히
  사실이고, §동작 확인 예시 payload는 `total_input_tokens: 50000`이라 첫 응답 이후 구간이어서 기대
  출력이 바뀌지 않는다(실행으로 확인). 갱신 대상 없음.
- `docs/20260727-001-statusline-schema-catchup/` — SPEC §4가 명시적으로 이번 범위에서 뺐다.

직접·간접 의존(grep으로 확인):

- `renderRateLimit`의 호출자는 rate limit 위젯 두 곳뿐이고, 테스트에서 직접 부르는 곳은 없다.
  placeholder 갈래를 이 공유 함수에 두면 두 위젯이 자동으로 같은 표기를 낸다.
- `renderProgressBar`의 호출자는 `contextWidget.Render` 한 곳뿐이다. placeholder 갈래도 같은 함수를
  쓰므로 호출자 수는 그대로다.
- `rateLimitData`를 참조하는 테스트는 없다 — 이 타입에 필드를 더해도 기존 테스트가 깨지지 않는다.

테스트 영향:

- `widgets_core_test.go` — `TestContextWidgetRender_TokenColor`의 네 서브테스트가
  `&contextData{Percent: …, TotalTokens: …}` 리터럴 + `newContextRenderCtx()`(zero `Stdin`)를 쓴다.
  채택안(상태를 데이터에 싣고 zero value를 "실측"으로 두는 방식)에서는 네 케이스가 수정 없이 통과한다.
  반대로 `Render`가 `ctx.Stdin`을 직접 보는 방식을 택하면 zero `Stdin`이 곧 미측정이 되어 네 케이스가
  전부 placeholder 경로로 넘어가 깨진다 — 이 파일이 그 대안을 배제하는 근거다(§5 D2).
- `widgets_core_test.go`의 `splitContextRender`는 공백 3분할을 강제하고 조각 수가 다르면 `t.Fatalf`한다.
  placeholder 렌더는 조각이 둘이므로 새 placeholder 케이스는 이 헬퍼를 쓸 수 없다. 기존 네 케이스는
  실측 경로만 통과하므로 헬퍼 자체는 그대로 두고, placeholder 검증은 `stripANSI` 기반으로 문자열
  전체를 본다(§5 D7).
- `stdin_test.go`의 `TestContextWidgetFractionalPercent`는 `total_input_tokens`가 없는 payload로
  `contextWidget.GetData`를 부른 뒤 `Percent == 8`을 확인한다. 즉 **placeholder 상태에서도 `GetData`가
  퍼센트 계산을 계속 수행해야** 이 테스트가 유효하다 — 채택안은 계산을 그대로 두고 상태만 덧붙인다.
  계산을 건너뛰는 변형을 택하면 이 케이스가 깨진다.
- `main_test.go`의 `TestShouldSuppressOutput` — 판정 입력을 바꾸지 않으므로 다섯 케이스 모두 유효하다.
  "rate limits alone do not bypass suppression" 케이스는 억제가 `orchestrate` 앞에서 일어나므로
  rate limit `GetData`의 변경과 무관하다.
- `widget_test.go` — `displayPresets["compact"]` 6개 배열 비교, `TestRemovedPresetCharsAreUnmapped`,
  preset 파싱 케이스 모두 불변 대상이다. `preset N` 서브테스트는 `projectName` 하나만 렌더한다.
- `widgets_project_test.go`·`config_test.go` — 영향 없음.

하위 호환: 기존 `cc-usage.json`(`preset`·`lines`·`disabledWidgets`·`widgets.context.barWidth`)은 그대로
해석된다. `disabledWidgets`로 세 위젯을 끈 사용자는 placeholder도 보지 않는다 — `orchestrate`가 위젯을
아예 돌리지 않기 때문이며 의도한 동작이다.

## 5. Decision Points

### D1. 첫 응답 판정 신호의 계산 위치

- 옵션 A: 각 위젯이 `ctx.Stdin.ContextWindow.TotalInputTokens > 0`을 직접 본다.
- 옵션 B: `Context`에 파생 판정(입력이 `Stdin`뿐인 읽기 전용 메서드)을 두고 세 위젯이 공유한다.
- 옵션 C: `main.go`가 판정 결과를 계산해 `Context`의 새 필드에 싣는다.
- 채택: **B**. 근거 — SPEC §3이 "신호 하나로 통일"을 요구하는데, A는 같은 비교식을 세 위젯에
  복제해 한쪽만 고치는 회귀 경로를 만든다. C는 필드를 채우는 책임이 생성자에게 생기고, `Context`를
  직접 구성하는 코드가 저장소에 10곳(전부 테스트)이라 채우지 않은 곳이 조용히 "첫 응답 전"으로
  판정된다. 이번 판정은 `Stdin`에서 완전히 파생되므로 메서드가 값과 원본의 불일치를 원천적으로
  없앤다. 대가 — 위젯이 호출할 때마다 다시 계산한다. 정수 비교 하나이고 한 프로세스에서 최대 3회이므로
  무시할 수 있다.
- 판정식은 SPEC §3이 고정한 `total_input_tokens > 0` 그대로다. `used_percentage`의 nil 여부나
  `current_usage`를 보조 신호로 섞지 않는다 — 위젯별 독립 판정이 비구독 계정에서 placeholder를 세션
  내내 고착시킨다는 §3의 근거가 보조 신호에도 같이 적용된다.
- 이름·주석: 판정이 "무엇을 뜻하는가"(첫 API 응답 도착)와 "왜 이 필드인가"(rate limit은 비구독 계정에서
  영구 부재라 자체 신호로 쓸 수 없다)를 판정 옆 주석에 남긴다. 다음 사람이 `rate_limits != nil`로
  바꾸려는 것이 이 설계에서 가장 그럴듯한 오수정이다.

### D2. placeholder 상태의 표현 위치와 실측값 우선순위

- 옵션 A: `Render`가 `ctx.Stdin`을 직접 보고 분기한다.
- 옵션 B: `GetData`가 placeholder 여부를 데이터에 상태로 실어 돌리고 `Render`가 그 상태로 분기한다.
- 옵션 C: placeholder 전용 타입을 따로 만들고 `Render`가 타입 스위치한다.
- 채택: **B**. 근거 — rate limit은 placeholder를 내려면 `GetData`의 nil 반환을 바꿔야 하므로
  `GetData`가 이미 판정에 관여할 수밖에 없다. 여기서 `Render`가 다시 독립적으로 판정하면 두 곳이
  어긋날 수 있다. B는 "`GetData`가 상태를 정하고 `Render`가 글리프를 정한다"로 책임이 한 번씩만
  나타난다. A는 여기에 더해 `widgets_core_test.go`의 네 `Render` 케이스를 전부 깨뜨린다(§4).
  C는 `renderRateLimit`이 받는 `any`를 두 타입으로 늘려 타입 스위치를 추가하는데, 상태가 bool 하나인
  변화에 타입을 하나 더 만들 만한 이득이 없다. 대가 — 데이터 타입이 표시 상태를 알게 된다. 이미
  `contextData`가 표시용 정수(절삭된 퍼센트)를 담고 있어 성질이 같은 자리다.
- 상태 필드의 극성: **zero value가 "실측"**이어야 한다. 근거 — 기존 테스트가 `&contextData{Percent:
  30, TotalTokens: 60_000}` 리터럴로 실측 케이스를 만든다. 극성을 뒤집으면 이 리터럴이 조용히
  placeholder를 뜻하게 되어 네 케이스가 표기 검증이 아닌 것을 검증하게 된다.
- `contextData`는 placeholder 상태에서도 퍼센트·토큰 계산 결과를 그대로 담는다. 근거 —
  `stdin_test.go`의 `TestContextWidgetFractionalPercent`가 그 계산을 확인하고 있고(§4), 계산을 건너뛰면
  "표기만 바꾼다"는 SPEC §1 범위를 넘어 데이터 산출까지 바꾸는 일이 된다. 표시하지 않는 값을 계산하는
  비용은 정수 나눗셈 하나다.
- **실측값 우선순위(rate limit)**: 대응 rate limit 포인터가 있으면 판정과 무관하게 실측값을 낸다.
  판정을 먼저 보는 순서는 택하지 않는다. 근거 — `rate_limits`가 이미 도착한 세션에서 어떤 이유로든
  `total_input_tokens`가 0으로 보고되면(§근거의 `/clear` 추정) 판정 우선 순서는 실측 데이터를 덮어
  `5h: -`를 보여준다. 이는 SPEC §2가 막으려는 오해를 새로 만드는 방향이다. SPEC §5.1·§5.4·§5.5는
  "rate_limits 있음 + 토큰 0" 조합을 규정하지 않으므로 어느 순서도 완료 조건을 위반하지 않고,
  데이터가 있으면 데이터를 쓰는 쪽이 어느 추정에서도 안전하다.
- **context는 판정만으로 결정한다**(실측 우선 예외를 두지 않는다). 근거 — rate limit은 포인터로
  부재를 구별할 수 있지만 context의 토큰 수는 0과 부재가 같은 값이라 "실측이 있다"를 표현할 수단이
  없다. `used_percentage`가 nil이 아닌 것을 실측 근거로 쓰는 변형은 SPEC §5.2가 그 조합에서도
  placeholder를 요구하는 문구와 부딪히고, 퍼센트만 실측이고 토큰 수는 0인 `8% 0` 같은 자기모순
  표기를 만든다. 요약하면 placeholder는 **보고되지 않은 값**을 대신하며, 부재를 표현할 수 있는
  자리에서는 존재가 이긴다.

### D3. placeholder 문자와 칸 조립 형태

- 문자 옵션: `-`, `?`, `.`, `~`, `_`. 채택 **`-`**. 근거 — SPEC §3이 ASCII 1칸 문자로 제약하므로
  다섯 후보 모두 제약은 만족한다. `-`는 CLI 표에서 "값 없음"을 뜻하는 가장 흔한 관례라 dim이 무시되는
  단색 터미널에서도 추가 설명 없이 읽힌다(SPEC §3 후단). `?`는 오류·경고로 읽혀 SPEC §2가 막으려는
  "고장났다"는 인상을 오히려 강화한다. `.`·`_`는 separator·bar와 시각적으로 섞이고, `~`는 "대략"으로
  읽혀 값이 있는 것처럼 보인다.
- rate limit 조립: **라벨 유지** — `5h: -`, `7d: -`. 근거 — SPEC §5.1은 "5h·7d **칸**이 각각
  placeholder 표기로 나타난다"를 요구하고, 어느 칸인지 알려주는 것은 라벨뿐이다. 라벨을 빼면 인접한
  두 칸이 `- │ -`가 되어 구별이 불가능하다. 라벨과 `: ` 구분자를 첫 응답 이후와 똑같이 두면 전환
  시점에 칸의 위치와 모양이 그대로 유지되고 값 한 조각만 바뀐다. 잔여 시간 `(…)` 접미사는 붙이지
  않는다 — placeholder 상태에는 reset 시각이 없다(§2).
- context 조립: **`<빈 bar> -`**. 퍼센트 자리와 토큰 자리를 `-` 하나로 합치고 `%` 기호도 빼며,
  bar와 `-` 사이 공백 하나만 남긴다. 근거 — 두 값은 같은 하나의 미측정에서 나온 것이라 표시를 둘로
  나누면 없는 정보를 두 번 보여준다. `-%`는 깨진 숫자로 읽히고, `- -`는 (SPEC §4가 범위 밖으로 둔
  `COLUMNS` 축소가 나중에 들어올 때) 아무 정보 없이 두 칸을 더 먹는다. bar는 SPEC §5.2가 명시적으로
  함께 출력하도록 요구하므로 유지한다.
- 대가 — 첫 렌더의 context 칸이 `░░░░░░░░ 0% 0`에서 `░░░░░░░░ -`로 짧아지고, 반대로 5h·7d 칸 두 개가
  새로 생겨 첫 렌더 줄 전체는 길어진다. 두 변화 모두 사용자가 화면 예시로 확인한 형태다(§근거).

### D4. dim 적용 범위와 ANSI 합성 방식

- 옵션 A: placeholder 문자만 dim, rate limit 라벨은 지금과 같은 `theme.Secondary` 그대로.
- 옵션 B: 칸 전체(rate limit은 라벨+`: `+placeholder, context는 placeholder)를 dim.
- 채택: **B**. 근거 — SPEC §3의 "흐린 표시"가 노리는 것은 "이 칸이 아직 준비되지 않았다"를 한눈에
  주는 것이고, 라벨이 첫 응답 이후와 같은 강조로 남으면 값 한 글자만 다른 화면이 되어 미측정 신호가
  약해진다. A는 색 대비 차이가 작은 테마에서 특히 구별이 어렵다. 대가 — 첫 응답 전에는 `5h`·`7d`
  라벨도 평소보다 흐리게 보인다. 사용자가 화면 예시로 확인한 형태다(§근거).
- **progress bar는 dim 대상에서 뺀다.** 근거 — 빈 bar는 이미 모든 테마에서 어두운 `BarEmpty` 색을
  쓰고 있어 muted 역할을 수행한다. 그 위에 SGR 2를 겹치면 대비가 더 낮은 테마에서 bar가 사실상
  보이지 않게 되는데, SPEC §5.2는 bar가 출력되기를 요구한다. 또 첫 응답 전후로 bar의 표시 방식이
  같아야 전환이 값 교체로만 보인다. 이 한 가지가 사용자가 확인한 화면 예시와 갈리는 지점이다 —
  그 예시는 context 칸 전체를 흐리게 표시했다.
- 합성 방식: `theme.Secondary` 다음에 RESET 없이 `theme.Dim`을 이어 붙이고 끝에서 한 번 RESET한다.
  근거 — 모든 테마의 `Dim`은 `\x1b[2m` 하나로 동일하며 색이 아니라 SGR 2(faint) 속성이므로 앞선
  전경색과 겹쳐 적용되고, RESET(SGR 0)이 둘을 함께 닫는다(§근거). 기존 `renderRateLimit`이
  잔여 시간에 쓰는 `theme.Dim`은 RESET **뒤에** 오므로 기본 전경색 + faint이고, 이번 조립과 자리가
  달라 상쇄·중복이 생기지 않는다. 새 색을 테마에 추가하지 않는다(SPEC §3).
- 두 위젯이 같은 조립을 쓰도록 "문자열을 `Secondary` + faint로 감싸는" 조립 하나를 공유한다. context
  placeholder도 같은 조립을 쓰므로 세 칸의 흐림 정도가 테마별로 어긋나지 않는다.

### D5. placeholder 상수·조립의 배치와 i18n 처리

- i18n 옵션: (a) `Translations`에 새 키를 넣고 `locales/{en,ko}.json` 양쪽에 값을 둔다,
  (b) 코드 상수로 둔다. 채택 **(b)**. 근거 — SPEC §3이 ASCII 1칸 문자를 제약으로 걸었는데 locale
  파일은 그 제약을 강제할 수단이 없다. 번역자가 `—`를 넣으면 표시폭 가정이 조용히 깨지고, 그 실수를
  잡는 장치는 코드 어디에도 없다. 게다가 en·ko 값이 필연적으로 동일하다 — `Translations.Model`이
  양쪽에 같은 값을 두고 소비처도 없이 남아 있는 것이 이 저장소에 이미 있는 같은 패턴의 결과다
  (§근거). 대가 — 나중에 locale별로 다른 표기를 쓰고 싶어지면 상수를 locale 키로 옮겨야 한다.
  ASCII 1칸 제약이 유지되는 동안에는 그럴 여지 자체가 거의 없다.
- 배치 옵션: (a) `render.go`, (b) `widgets_core.go`. 채택 **(a)**. 근거 — `CLAUDE.md` 아키텍처 표가
  `render.go`에 "theme, separator, progress bar, ANSI 코드"를 배정하고, 흐리게 감싸는 조립은 테마 색과
  ANSI 속성 합성 그 자체다. placeholder 문자도 `█`·`░`와 같은 표시 글리프 계열이고, 위젯 전용
  상수인 `defaultContextBarWidth`가 이미 `render.go`에 있는 선례와 맞는다. (b)는 두 소비처가 그
  파일에 있다는 점에서 자연스럽지만, 나중에 project 계열 위젯이 같은 흐림 표기를 쓰려 하면 core 위젯
  파일에서 조립을 꺼내 쓰는 모양이 된다. `contextTokenWarn`/`Danger`처럼 위젯 고유 임계값은 계속
  `widgets_core.go`에 둔다 — 배치 기준은 "표시 조립인가 위젯 정책인가"다.
- config 옵션은 만들지 않는다(SPEC §4). placeholder 문자·dim 여부 모두 상수로 고정하며,
  `WidgetConfig`·`validateConfigWidgets`를 건드리지 않는다.

### D6. placeholder 상태의 progress bar 색과 토큰 경고 임계값

- bar 옵션: (a) 기존 `renderProgressBar(0, ContextBarWidth(), theme)`를 그대로 쓴다,
  (b) `BarEmpty`만으로 빈 bar를 따로 조립해 쓰이지 않는 `Safe` 코드를 빼낸다.
- 채택: **(a)**. 근거 — `getColorForPercent(0)`이 `Safe`를 돌려주고 채운 칸이 0개라 그 색이 칠하는
  대상 없이 곧바로 `BarEmpty`로 덮이는데, 이는 이번 변경이 만드는 성질이 아니라 현행 첫 렌더 출력에
  이미 있는 성질이다(실제 바이트로 확인, §근거). (b)는 bar 조립 경로를 둘로 늘려 `barWidth` 해석·
  테마 색 사용이 두 곳에서 갈리게 만드는데, 얻는 것은 화면에 보이지 않는 이스케이프 시퀀스 하나
  제거뿐이다. `widgets_core_test.go`의 `TestRenderProgressBarWidth`가 `renderProgressBar`를 직접
  검증하고 있어 같은 함수를 쓰는 편이 placeholder bar의 폭도 함께 보장된다.
- `getColorForPercent`는 손대지 않는다. placeholder에서는 퍼센트를 표시하지 않으므로 퍼센트 색 결정
  자체가 호출되지 않는다.
- 토큰 경고 임계값(`contextTokenWarn` 256K / `contextTokenDanger` 512K): placeholder 갈래는 토큰 수를
  표시하지 않으므로 임계값 분기를 타지 않는다. 잃는 동작은 없다 — SPEC §1이 첫 응답 전
  `total_input_tokens`·`total_output_tokens`가 모두 0임을 확인했으므로 그 구간에서 임계값이 발화할
  수 있는 입력이 없다. 임계값 자체와 실측 갈래의 색 분기는 그대로 둔다(SPEC §1의 표기 한정 범위).

### D7. 테스트 계층

- 위젯 단위(`widgets_core_test.go`): `GetData`의 상태 판정을 table-driven으로 덮는다. 축은
  `total_input_tokens`(0 / 양수) × `rate_limits`(부재 / 존재)이고, 각 칸의 기대는 (nil 반환인가,
  placeholder 상태인가, 실측인가)다. 네 조합이 SPEC §5.1·§5.4·§5.5와 D2의 실측 우선순위를 한 표에서
  고정한다. context는 `context_window_size`(0 이하 / 양수) × `total_input_tokens`(0 / 양수)로 같은
  형태를 만들어 SPEC §5.2·§5.6·§5.8을 덮는다. 이 저장소가 순수 판정 로직에 케이스 슬라이스를 쓰는
  관례(`TestRenderProgressBarWidth`, project 위젯의 경로 축약 케이스)에 맞는 자리다.
- 렌더 단위(같은 파일): placeholder 렌더 문자열을 `stripANSI`로 훑어 조립 형태와 ASCII 제약을 본다
  (SPEC §5.7). `splitContextRender`는 공백 3분할을 강제하므로 placeholder 케이스에 쓰지 않고 기존
  실측 케이스 전용으로 남긴다(§4). dim 적용은 `stripANSI` 전 문자열에 `theme.Dim`이 있는지로 확인한다 —
  기존 `TestContextWidgetRender_TokenColor`가 테마 색 상수 포함 여부로 검사하는 방식과 같다.
- orchestrate 단위: **필요하다.** 근거 — SPEC §5.1·§5.4는 "stdout에 칸이 나타난다 / 나타나지 않는다"를
  요구하고, 그 판정은 `GetData`의 nil과 `Render`의 빈 문자열을 `orchestrate`가 해석한 결과다. 위젯
  단위 테스트만으로는 "위젯이 nil을 돌린다"까지만 보장되고 줄에서 사라지는 것까지는 보장되지 않는다.
  `widget_test.go`가 이미 쓰는 방식 — `Separator: "space"` + 명시 `Lines`, `t.Setenv("PATH", "")`로 git
  차단 — 을 그대로 따라 `context`·`cost`·`rateLimit5h`·`rateLimit7d`만 담은 줄을 만들고 첫 렌더 payload와
  첫 응답 이후 payload 두 개로 전체 줄을 비교한다. 같은 케이스가 cost가 `$0.00`으로 남는 것도 함께
  고정한다(SPEC §5.3).
- 무출력 조건은 새 테스트를 만들지 않고 `main_test.go`의 기존 `TestShouldSuppressOutput`에 의존한다
  (SPEC §5.9) — 판정 입력을 바꾸지 않았으므로 그 테스트가 이미 조건을 덮는다.
- 검증 명령은 SPEC §3·§5.11대로 `go test ./...`와 `go vet ./...`다. 판정 기준은 절대 통과가 아니라
  착수 시점(`ea393fb`) 대비 실패 집합 불변이다 — 그 시점에 이미 `config.go`·`widgets_project.go`의
  POSIX 경로 fixture가 Windows에서 4개 실패하고 있고, SPEC §4가 그 수정을 범위 밖으로 뺐다. 이번에
  추가하는 테스트는 모두 통과해야 한다.

### D8. 버전 bump 폭과 문서 갱신 범위

- 버전 옵션: (a) patch(0.5.5), (b) minor(0.6.0). 채택 **(a)**. 근거 — 이번 변경은 위젯·config 키·
  preset 문자를 늘리지 않고 기존 세 위젯의 표기만 고치는 수정이며, 그 성격은 SPEC §1이 규정한
  "오해를 만드는 표시의 교정"이다. `Makefile` VERSION과 `.claude-plugin/plugin.json` version에 같은
  값을 넣어야 `/plugin` UI가 업데이트를 감지한다(SPEC §5.10, CLAUDE.md §버전 정책). 대가 — 첫 렌더에
  칸 두 개가 새로 생기는 것은 사용자 체감 변화이므로 minor로 볼 여지도 있다. 그러나 그 칸들은 원래
  기본 preset에 있던 위젯이 데이터 부재로 사라졌던 자리이고 새 기능이 아니다.
- `README.md`(SPEC §5.12): 위젯 표의 세 항목 설명에 첫 응답 전 표기를 넣는다 — `context`는
  "첫 응답 전에는 빈 bar + `-`", `rateLimit5h`/`7d`는 "첫 응답 전에는 `5h: -`, 데이터가 오지 않는
  계정에서는 생략". Troubleshooting에 항목을 새로 만들지 않는다 — 위젯 표가 표기 계약의 자리이고,
  이번 변경의 목적이 "설명을 읽어야 이해되는 화면"을 없애는 것이므로 표기 자체가 설명을 대신한다.
- `CLAUDE.md`는 갱신하지 않는다 — 근거는 §4에 적었다.
- `bin/` 재빌드와 release 브랜치 동기화는 SPEC §5에 조건이 없는 배포 단계이며 CLAUDE.md §배포 절차가
  소유한다. 이번 변경이 사용자에게 도달하려면 필요하다는 사실만 남긴다.
