# context-token-color — IMPLEMENT

## 컨텍스트

- SPEC: [spec.md](./spec.md) §1~§5 (완료 조건 7개)
- ANALYSIS: [analysis.md](./analysis.md) §1~§5 (Decision Points 5개 모두 commit, 사용자 위임 없음)
- 변경 표면: `widgets_core.go` `contextWidget.Render` 한 곳 + 회귀 테스트 1개 + version bump 3 파일
- 임계 상수: `contextTokenWarn = 256_000`, `contextTokenDanger = 512_000` (`widgets_core.go` 내부, 소문자)
- 색 매핑: `theme.Warning` / `theme.Danger` 재사용 (analysis §5.1)
- 출력 포맷 변경: `"%s %s%d%%%s %s"` → `"%s %s%d%%%s %s%s%s"` (토큰 wrap RESET 추가)
- 버전: v0.3.2 → v0.3.3 patch bump (Makefile / plugin.json / api.go userAgent 동시 갱신)

## SPEC §5 매핑 커버리지

- §5.1 (256K Warning) → task-001, task-002
- §5.2 (512K Danger) → task-001, task-002
- §5.3 (256K 미만 변경 없음) → task-001, task-002
- §5.4 (percent 색 분기 불변) → task-001, task-002
- §5.5 (200K model 회귀 없음) → task-001, task-002
- §5.6 (build/test exit 0) → task-001, task-002, task-003의 `확인` 라인에 횡단 포함
- §5.7 (v0.3.3 세 곳 일치) → task-003

미매핑 §5 기준 없음.

## Task 목록

### task-001 — `contextWidget.Render`에 토큰 색 분기 + 임계 상수 도입

- [x] 완료

**목적**
1M context model에서 토큰 누적이 256K / 512K 임계를 넘었을 때 status line의 토큰 수 부분을 경고색/강조색으로 표시해 clear 시점 시각 cue를 제공한다. percent 색 분기는 변경 없이 유지한다.

**접근**
- `widgets_core.go`의 context 위젯 영역에 패키지 레벨 상수 추가:
  - `const contextTokenWarn = 256_000`
  - `const contextTokenDanger = 512_000`
  - 소문자 / 미export. 다른 위젯이 참조하지 않도록 context 위젯 정의 인근에 배치.
- `contextWidget.Render` 본문 수정:
  - 기존 percent 색 선택 로직(`getColorForPercent`)은 손대지 않는다.
  - `d.TotalTokens` 값으로 토큰 색 코드 `tokenColor`를 선택한다. 분기 순서는 Danger → Warning → 빈 문자열(analysis §5.4 강한 신호 우선):
    - `d.TotalTokens >= contextTokenDanger` → `theme.Danger`
    - `d.TotalTokens >= contextTokenWarn` → `theme.Warning`
    - 그 외 → `""`
  - 토큰 색 RESET 변수 `tokenReset`은 `tokenColor != ""`일 때만 `theme.Reset` (또는 기존 RESET 상수와 동일한 값), 빈 문자열일 때는 `""`.
  - 출력 포맷을 `"%s %s%d%%%s %s%s%s"`로 확장하고 인자 순서는 `bar, percentColor, percent, percentReset, tokenColor, formatTokens(d.TotalTokens), tokenReset`.
- helper 함수·새 ANSI 리터럴·`contextData` 필드 추가는 금지 (analysis §5.5 / SPEC §3).
- `formatTokens` 시그니처와 호출자 위치는 변경하지 않는다 — 호출자에서만 결과를 wrap.

**검증 조건**
- 결과:
  - `widgets_core.go`에 `contextTokenWarn`, `contextTokenDanger` 상수가 추가되었다.
  - `contextWidget.Render`가 `d.TotalTokens >= 512_000`에서 `theme.Danger`, `>= 256_000 && < 512_000`에서 `theme.Warning`, `< 256_000`에서 색 코드 없이 토큰을 출력한다.
  - percent 색 wrap(`%s%d%%%s`)은 기존 형태 그대로다.
  - 토큰 색이 빈 문자열인 경우 출력은 변경 전과 byte-identical (separator/공백 추가 없음).
- 확인:
  - `go vet ./...` exit 0.
  - `go build ./...` exit 0.
  - `make build-local` exit 0이며 `dist/cc-usage` 생성.
  - 수동 stdin smoke (CLAUDE.md §동작 확인 패턴):
    - `total_input_tokens=50000, total_output_tokens=10000` → 토큰 부분에 ANSI 색 wrap 없음 (`grep -c $'\x1b'`로 토큰 직전/직후 색 코드 부재 확인).
    - `total_input_tokens=200000, total_output_tokens=60000` (=260K) → 토큰 부분에 `theme.Warning` 코드 wrap.
    - `total_input_tokens=400000, total_output_tokens=120000` (=520K) → 토큰 부분에 `theme.Danger` 코드 wrap.
  - 200K context model fixture(`context_window_size=200000, total_*=180K 등 < 256K`)에서 출력 byte sequence가 변경 전과 동일.

**참조**
- SPEC §5.1, §5.2, §5.3, §5.4, §5.5, §5.6
- ANALYSIS §1, §2, §5.1, §5.2, §5.3, §5.4, §5.5

---

### task-002 — context 위젯 토큰 색 회귀 테스트 추가

- [x] 완료

**목적**
토큰 색 분기 3단(미만 / 256K / 512K)과 경계 비교(`>=`), Danger 우선 규칙, percent 색 분기 비간섭, 200K model 회귀 없음을 회귀 테스트로 고정한다.

**접근**
- 신규 파일 `widgets_core_test.go`(없는 경우) 또는 기존 `widget_test.go`에 `TestContextWidgetRender_TokenColor`(또는 동등 이름)를 추가한다.
- 테스트는 `contextWidget{}.Render(contextData{...}, ctx)` 직접 호출 형태로 작성한다. `ctx`는 테스트용 최소 RenderContext (`Theme = themes["default"]` 또는 동등) 구성.
- 케이스(최소):
  - `TotalTokens = 60_000` → 출력에 `theme.Warning`/`theme.Danger` ANSI 코드 substring이 토큰 부분에 등장하지 않는다. percent 부분의 색은 기존대로 등장 가능.
  - `TotalTokens = 256_000` (경계) → 토큰 부분 직전에 `theme.Warning` substring, 토큰 부분 직후에 RESET substring 등장. Danger substring은 등장하지 않는다.
  - `TotalTokens = 400_000` (Warning 구간 내부) → Warning substring, Danger 없음.
  - `TotalTokens = 512_000` (경계) → 토큰 부분 직전에 `theme.Danger` substring, Warning은 토큰 부분에 등장하지 않는다 (percent 부분은 무관).
  - `TotalTokens = 800_000` (Danger 구간 내부) → Danger substring, Warning 없음.
- percent 색 비간섭 확인: `Percent=30, TotalTokens=60_000`과 `Percent=30, TotalTokens=400_000` 두 케이스에서 percent 부분의 색 코드 substring이 동일하다 (토큰 색 분기 추가로 percent 색이 영향받지 않는다).
- 200K model 회귀: `Percent=90, TotalTokens=180_000`(< 256K) 케이스에서 토큰 부분에 Warning/Danger substring이 없다.
- 테스트는 정확한 출력 문자열 동등 대신 substring 포함 여부로 단언해 ANSI 코드의 정확한 escape sequence 비교에 묶이지 않게 한다 (단, 빈 색 코드 케이스는 byte-identical 비교가 필요하면 변경 전 기대 문자열을 inline로 둘 것).

**검증 조건**
- 결과:
  - 새 테스트가 모든 케이스에서 통과한다.
  - 256K/512K 경계 케이스가 `>=` 의도대로 동작함을 단언한다.
  - percent 색 substring 동일성 단언이 존재한다.
- 확인:
  - `go test ./...` exit 0.
  - `go test -run TestContextWidgetRender ./...` exit 0이며 추가한 케이스가 실제로 실행됨을 출력에서 확인.

**참조**
- SPEC §5.1, §5.2, §5.3, §5.4, §5.5, §5.6
- ANALYSIS §4, §5.3, §5.4

---

### task-003 — v0.3.2 → v0.3.3 SemVer patch bump 동기화

- [x] 완료

**목적**
사용자 체감 동작 변경(토큰 색 cue)에 대응해 CLAUDE.md §버전 정책에 따라 세 곳의 version 식별자를 `0.3.3`으로 동시 갱신한다. `/plugin` UI가 update를 감지해 marketplace 사본의 stale 고착을 방지한다.

**접근**
- `Makefile`의 `VERSION` 변수 값을 `0.3.2` → `0.3.3`으로 갱신.
- `.claude-plugin/plugin.json`의 `version` 필드를 `0.3.2` → `0.3.3`으로 갱신.
- `api.go`의 `userAgent`에 박힌 버전 문자열을 `0.3.2` → `0.3.3`으로 갱신 (정확한 변수/리터럴 위치는 구현 시 검색해 확인 — userAgent는 단일 정의 지점이어야 함).
- 세 위치 외 다른 곳에서 `0.3.2` 리터럴이 검색되면 일관성을 위해 함께 갱신할지 별도 항목인지 확인 후 처리 (예: 문서/README의 버전 표기는 본 task 범위 밖).
- `bin/` 재빌드와 release 브랜치 sync는 본 task 범위 밖 (SPEC §4 제외 범위).

**검증 조건**
- 결과:
  - `Makefile` VERSION = `0.3.3`.
  - `.claude-plugin/plugin.json` `"version"` = `"0.3.3"`.
  - `api.go` userAgent 문자열에 `0.3.3` 포함, `0.3.2` 부재.
- 확인:
  - `grep -n "0.3.2" Makefile .claude-plugin/plugin.json api.go` 결과 0건.
  - `grep -n "0.3.3" Makefile .claude-plugin/plugin.json api.go` 결과 각 파일 1건 이상.
  - `make build-local` exit 0이며 `./dist/cc-usage --version` 출력이 `0.3.3`.
  - `make build` exit 0이며 빌드 자체가 ldflags 주입으로 `0.3.3`을 굳힌다 (bin/ 갱신 commit은 본 task 밖).

**참조**
- SPEC §5.6, §5.7
- ANALYSIS §4
- CLAUDE.md §버전 정책
