# context-token-color — ANALYSIS

## 근거

읽은 spec.md 범위: §1 ~ §5 전체.

코드베이스에서 확인한 사실:
- `widgets_core.go:54-91` — `contextWidget` 정의. `contextData{Percent int, TotalTokens int}`만 보유. `GetData`(63-83)에서 `cw.TotalInputTokens + cw.TotalOutputTokens`를 `TotalTokens`로 그대로 저장하며, `Render`(85-91)는 progress bar → percent(색 적용) → `formatTokens(d.TotalTokens)`를 공백으로 연결한다. 토큰 표시 부분에는 현재 어떠한 ANSI 색 wrap도 없다 (`%s` 포맷에 plain text로 들어감).
- `render.go:10-39` — `ThemeColors` 구조에 `Warning`, `Danger` 필드가 이미 존재. 8개 테마 모두 두 키를 채우고 있어 추가 ANSI 코드 도입 없이 재사용 가능 (예: `default` Warning `\x1b[38;5;222m` 노란빛, Danger `\x1b[38;5;210m` 붉은빛).
- `render.go:123-131` — `getColorForPercent(percent int, theme)`는 percent 값만 받아 `Safe/Warning/Danger`를 반환. 토큰 절대값과 무관하므로 토큰 색 분기와 독립적이다. percent 색 분기에 손대지 않아도 토큰 색 분기를 percent 색 다음에 별도 wrap으로 끼워 넣을 수 있다.
- `format.go:9-28` — `formatTokens(int) string`. 색 wrap 책임을 가지지 않는 순수 포맷터. 호출자 측에서 결과를 ANSI로 감싸는 책임을 진다 (현재 호출자는 `contextWidget.Render` 한 곳뿐 — `grep`으로 확인).
- `widget_test.go:17` — 기존 displayPreset 테스트가 `{"model", "cost", "context"}` 형태의 string slice만 비교. context 위젯의 토큰 색 분기에 대한 회귀 테스트는 부재.

추정과 분리한 사실: spec.md §3은 "기존 theme 체계 안에서 선택"을 요구하며 새 ANSI 코드 금지를 명시. 위 ThemeColors 조사로 `Warning`/`Danger`가 정확히 그 역할이며 모든 테마에 정의되어 있음을 확인했으므로, 임계값에 매핑할 ANSI 코드는 **신규 정의 없이 두 필드를 재사용**한다.

## 1. 구조

변경은 `contextWidget` 한 위젯의 Render 경로 내부에서 종결한다. 새 파일·새 helper 모듈·새 패키지 분할 없음.

- **토큰 임계값 상수**: `widgets_core.go` 안에 context 위젯 전용 패키지 레벨 상수 `contextTokenWarn = 256_000`, `contextTokenDanger = 512_000`을 둔다. 다른 위젯이 참조하지 않음을 보장하기 위해 명시적으로 context 위젯 영역에 묶고 export 하지 않는다 (`main` 패키지 내부 소문자). [SPEC §3]
- **색 선택 책임**: `contextWidget.Render` 내부에서 `d.TotalTokens`를 두 임계값과 비교해 ANSI 코드를 선택한다. 별도 helper 함수는 도입하지 않는다 — 호출 지점이 하나뿐이고, 분기가 3단(미만/Warn/Danger)으로 단순하며, helper 분리는 spec.md §3의 "context 위젯 전용 신호로 재사용 가능한 형태로 빼지 않는다" 제약과 정합적이다.

## 2. 데이터 흐름

stdin payload → `contextWidget.GetData` → `contextData{Percent, TotalTokens}` → `contextWidget.Render` → status line 출력.

토큰 색 분기는 Render 단계에서만 작동한다.

1. `GetData`는 변경 없음 — `TotalTokens = total_input_tokens + total_output_tokens`를 그대로 보존 (절대값이 임계값 비교에 필요하므로 추가 가공 불필요).
2. `Render`에서 현재 다음 순서로 출력 조각이 만들어진다:
   - `bar` (progress bar, percent 기반 색)
   - `color`+`percent`+`RESET` (percent 색 wrap)
   - `formatTokens(d.TotalTokens)` (**여기에 색 wrap 추가**)
3. 토큰 색 선택: `TotalTokens >= contextTokenDanger`이면 `theme.Danger`, `>= contextTokenWarn`이면 `theme.Warning`, 그 외에는 빈 문자열. 빈 문자열이면 `formatTokens` 결과를 그대로 출력하여 변경 전과 동일한 표시를 유지한다 [SPEC §5.3].
4. 출력 형식은 `"%s %s%d%%%s %s%s%s"`로 확장된다 — percent wrap과 토큰 wrap이 각각 자기 RESET을 가진다. RESET 누락 시 separator(`│`)까지 색이 새는 회귀를 막는다.

200K context model 경로: `TotalTokens`는 사실상 200K를 넘지 못하므로 `< 256_000` 분기로 떨어져 빈 색 코드가 선택된다 → 출력은 변경 전과 동일 [SPEC §5.5]. percent 색은 `getColorForPercent`가 그대로 산출하므로 영향 없음 [SPEC §5.4].

경계 케이스:
- `TotalTokens == 256_000` → Warning (spec §5.1의 `>=` 조건).
- `TotalTokens == 512_000` → Danger (spec §5.2의 `>=` 조건). 256K/512K 사이 회색지대 없음.
- `TotalTokens < 0` 케이스는 발생하지 않지만 만일 음수가 들어와도 `< 256_000` 분기로 떨어져 안전.
- degraded-input 복원(main.go:83-111) 이후 재호출되는 orchestrate에서도 동일 데이터 흐름을 타므로 별도 분기 불필요.

## 3. 인터페이스

외부 경계 인터페이스 변경 없음.

- Widget 인터페이스 (`ID/GetData/Render`) 시그니처 불변.
- `contextData` 구조체 필드 불변.
- `formatTokens` 시그니처 불변 — 호출자에서만 결과를 wrap.
- Config 스키마, locale JSON 키, stdin 파싱 스키마, CLI 플래그 모두 불변 [SPEC §3].

## 4. 영향 범위

직접 수정 대상:
- `widgets_core.go` — `contextWidget.Render` 본문, 같은 파일에 임계 상수 2개 추가.
- `Makefile` (VERSION), `.claude-plugin/plugin.json` (version), `api.go` (userAgent) — `0.3.2 → 0.3.3` 세 곳 동기화 [SPEC §5.7, CLAUDE.md §버전 정책].

간접 영향:
- `render.go`의 `ThemeColors`·`getColorForPercent`·테마 정의는 **읽기만** 하고 수정하지 않는다. percent 색 분기는 spec §3 제약에 의해 명시적으로 보호된다.
- `format.go`의 `formatTokens` 호출자는 `contextWidget.Render` 단 한 곳 (조사로 확인) — 새 호출자가 없어 다른 위젯의 토큰 표시에 부수 영향 없음.
- 테스트: `widget_test.go`에 context 위젯 Render 회귀 테스트(미만/256K/512K 경계 세 케이스)를 추가하는 것이 IMPLEMENT 단계의 검증 손잡이가 된다 — analysis가 단정하지 않고 implement.md가 commit한다.

하위 호환·마이그레이션: 해당 없음. stdin 스키마·캐시 포맷·Config 키 모두 불변이라 v0.3.2 → v0.3.3 업그레이드는 무중단.

## 5. Decision Points

### 5.1 색 매핑 — `ThemeColors.Warning` / `Danger` 재사용

- 옵션 A: 기존 `theme.Warning`(노랑 계열) / `theme.Danger`(빨강 계열) 재사용.
- 옵션 B: 토큰 색 전용 새 theme 필드(`TokenWarn`, `TokenDanger`) 도입.
- 옵션 C: 즉시 ANSI 리터럴(`\x1b[33m` 등) 사용.

채택: A. `Warning`/`Danger` 필드가 8개 테마 모두에 정의되어 있고 의미상 "경고/위험" 신호와 정확히 일치한다. B는 spec §3의 "새 ANSI 색 코드를 도입하지 않는다" 제약과 ThemeColors 인터페이스 확장 비용을 짊어지지만 새 시각 정보를 제공하지 않는다. C는 테마 시스템을 우회해 catppuccin/dracula 등 컬러풀 테마에서 컬러 불일치를 만든다.

### 5.2 임계값 비교 위치 — Render

- 옵션 A: `Render` 안에서 `d.TotalTokens`를 비교해 색 선택.
- 옵션 B: `GetData`가 `TotalTokens` 외에 `TokenSeverity` 같은 enum 필드를 미리 계산해 `contextData`에 실어 전달.

채택: A. `contextData`는 현재 `Percent`와 `TotalTokens` 두 필드만 가지며, `getColorForPercent`도 같은 패턴(Render에서 percent로부터 색 선택)을 따른다. B는 데이터 모델을 표시 결정으로 오염시키고, `GetData`에 `ctx.Config.Theme` 접근이 필요 없는 이점도 잃는다. Render 측 분기가 코드 흐름과 일관된다.

### 5.3 경계 비교 — `>=` 채택

- 옵션 A: `>= 256_000`, `>= 512_000`.
- 옵션 B: `> 256_000`, `> 512_000`.

채택: A. spec.md §5.1/§5.2가 명시적으로 `>=`. 정확히 256K/512K에 닿는 stdin이 들어왔을 때(테스트 fixture에서 자주 발생) 색 신호가 일관되게 켜진다.

### 5.4 256K/512K 경계 충돌 시 우선순위

- 옵션 A: 512K 임계가 우선 (더 강한 신호로 덮어쓴다).
- 옵션 B: 두 색을 함께 표시 (불가능 — ANSI 코드는 마지막 값이 우선).

채택: A. spec §5.2의 "256K 색보다 시각적으로 강한 신호다"가 직접 명시. 분기 순서를 `>= Danger` 먼저, 그다음 `>= Warning`으로 두면 자연스럽게 강한 신호 우선이 보장된다.

### 5.5 색 적용 방식 — 인라인 wrap

- 옵션 A: `Render` 내부에 인라인으로 `color + formatTokens(...) + RESET`을 fmt.Sprintf로 조합.
- 옵션 B: `wrapTokenColor(tokens int, theme ThemeColors) string` 같은 새 helper 도입.

채택: A. 호출 지점이 1개이고 분기 로직이 3줄로 정착한다. helper는 추상화 비용만큼의 재사용이 없으며, spec §3의 "context 위젯 전용 신호로 재사용 가능한 형태로 빼지 않는다"와도 정합. 빈 색 코드일 때는 `formatTokens` 결과만 그대로 두어 변경 전 출력과 byte-identical하게 유지한다 [SPEC §5.3, §5.5].
