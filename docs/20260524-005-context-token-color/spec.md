# context-token-color — SPEC

## 1. 범위

context 위젯 출력의 토큰 수 표시 부분(`formatTokens(d.TotalTokens)` 결과)에 절대값 기반 색 분기를 추가한다. 1M context model에서 사용자가 작업을 어디까지 채웠는지를 한 눈에 식별하고 clear(대화 정리) 시점을 시각적으로 권유하는 cue로 활용한다.

- 임계값은 `total_input_tokens + total_output_tokens` 누적 기준 절대값 — `256K`에서 경고색(노랑), `512K`에서 강조색(빨강).
- 임계값 미만 토큰 수는 기존 출력(색 없음 또는 dim 톤) 그대로 유지한다.
- 이 변경은 1M context model 전용 신호다. 200K context model은 토큰이 256K에 도달하지 않으므로 토큰 색 분기가 트리거되지 않고, 기존 percent 색 분기(`getColorForPercent`)가 그 역할을 그대로 수행한다.

v0.3.2(`projectinfo-display` + `lock-leak-cleanup`) 다음 release인 v0.3.3 patch로 단독 처리한다.

## 2. 목표

- **clear 시점 시각 cue**: 1M context로 작업 중인 사용자가 토큰 누적이 256K / 512K 임계를 넘었음을 status line 한 줄에서 즉시 식별할 수 있다.
- **percent 색과의 분리**: 토큰 색은 절대값 기반의 독립 신호로 작동하며, percent 색은 기존 동작 그대로 percent 임계 기반 분기를 유지한다. 두 신호가 서로 간섭하지 않는다.
- **200K model 회귀 없음**: 200K context model에서 출력 형태와 색 분기 동작이 변경 전과 동일하다 (256K 임계가 트리거되지 않으므로 토큰 부분 색은 추가되지 않음).

## 3. 제약

- Zero dependency 유지, 단일 `main` 패키지 유지.
- 출력 형식·CLI 플래그·Config 스키마·locale JSON 키 변경 없음. 색 임계값 토글이나 사용자 정의 임계값 옵션을 도입하지 않는다.
- 임계값(`256_000`, `512_000`)은 context 위젯 내부 상수로 둔다. 다른 곳에서 재사용 가능한 형태로 빼지 않는다 (현재로선 context 위젯 전용 신호).
- 기존 `getColorForPercent` 함수와 percent 색 분기 흐름은 손대지 않는다.
- 색 코드는 기존 theme 체계(`theme.Warning`, `theme.Danger` 등 — 정확한 키는 ANALYSIS에서 commit) 안에서 선택한다. 새 ANSI 색 코드를 도입하지 않는다.
- v0.3.2 → v0.3.3 SemVer patch bump를 동반한다 (CLAUDE.md §버전 정책: 사용자 체감 동작 변경은 항상 bump).

## 4. 제외 범위

- percent 색 임계값(`getColorForPercent`) 변경 — 본 spec 밖.
- 200K context model 또는 그 외 context_window_size별 토큰 임계값 변경 — 1M 전용 절대값으로 commit.
- 진행 바(`renderProgressBar`) 색 분기 변경 — 본 spec 밖.
- 토큰 단위 표기 변경 (`256K` → `0.25M` 같은 형식 전환) — 본 spec 밖.
- 사용자 정의 임계값을 위한 Config 옵션 추가 — 본 spec 밖.
- bin/ 재빌드와 release 브랜치 sync는 본 spec 작업의 소스 변경 결과로 별도 commit/push 단계에서 처리한다.

## 5. 완료 조건

1. `total_input_tokens + total_output_tokens >= 256_000`이고 `< 512_000`인 stdin에서 출력의 토큰 수 부분이 경고색(노랑 계열) ANSI 코드로 감싸여 나타난다.

2. `total_input_tokens + total_output_tokens >= 512_000`인 stdin에서 출력의 토큰 수 부분이 강조색(빨강 계열) ANSI 코드로 감싸여 나타나며, 256K 색보다 시각적으로 강한 신호다.

3. `total_input_tokens + total_output_tokens < 256_000`인 stdin에서 출력의 토큰 수 부분에 새 색 코드가 적용되지 않는다 (변경 전과 동일한 표시).

4. percent 색 분기는 변경 전후 동일하다 — `getColorForPercent` 결과가 percent 부분에 그대로 적용되며 토큰 색 분기 추가에 의해 percent 색이 영향받지 않는다.

5. 200K context model의 일반 사용 범위(0K ~ 200K)에서 토큰 부분 출력은 변경 전과 동일하다 — 256K 임계가 트리거되지 않아 토큰 색이 추가되지 않는다.

6. `make build`, `make build-local`, `go test ./...` 세 명령이 변경 전과 동일하게 exit 0으로 종료한다.

7. `Makefile` VERSION, `.claude-plugin/plugin.json` version, `api.go` userAgent 세 곳이 모두 `0.3.3`으로 일치한다. `dist/cc-usage --version`이 `0.3.3`을 출력한다.
