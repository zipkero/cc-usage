# spec: session-start-placeholder

## 1. 범위

세션 초반 — Claude Code가 첫 API 응답을 아직 돌려주지 않은 구간 — 에 statusline이 **아직 측정되지 않은 값**을
숫자 `0`으로 표시하는 문제를 고친다. 대상은 `context` 위젯과 `rateLimit5h`·`rateLimit7d` 위젯의 표시 표기이며,
데이터 출처는 지금과 같이 stdin 하나로 유지한다.

첫 응답 이후 구간의 동작은 바꾸지 않는다. 특히 `rate_limits`가 끝까지 오지 않는 계정에서 두 위젯이 출력에서
생략되는 현행 동작은 그대로 둔다.

### 입력 맥락

증상: 새 세션 첫 렌더에서 아래 한 줄만 나온다.

```
~\GolandProjects\cc-usage (main) │ ◆ claude-opus-5[1m] │ ░░░░░░░ 0% 0 │ $0.00
```

공식 문서(`code.claude.com/docs/en/statusline.md`, 2026-07-30 조회) 대조 결과 이 출력은 stdin에 충실하며
버그가 아니다.

- `rate_limits`는 "Claude.ai 구독자(Pro/Max)에게 세션의 첫 API 응답 이후"에만 나타난다. 첫 렌더에는 키가 없어
  두 위젯이 `GetData`에서 nil을 반환하고 orchestrator가 생략한다. 구독 계정이 아니면 세션 내내 오지 않는다.
- `total_input_tokens`·`total_output_tokens`는 첫 API 응답 전에는 둘 다 0이다.
- `context_window.used_percentage`는 세션 초반에 `null`일 수 있다.
- `context_window_size`는 첫 렌더에도 채워지므로 context 위젯은 생략되지 않고 `0%`로 남는다.

따라서 `context 0%`는 실제로 0이 아니라 **미측정**이다 — 첫 응답 전이라 보고되지 않을 뿐, 실제 컨텍스트에는
시스템 프롬프트·CLAUDE.md·도구 정의가 이미 들어차 있다. 반면 `cost $0.00`은 API 호출이 없었으므로 **진짜 0**이다.
이 구분이 §3의 적용 범위 제약과 §4의 cost 제외 근거다.

`/spec-init` 전 논의에서 검토하고 접은 접근 둘.

- **transcript(`{CLAUDE_CONFIG_DIR|~/.claude}/projects/**/*.jsonl`) 기반 보강**: 첫 렌더의 참값이 0인 구간에
  직전 세션 값을 채우면 오해를 만들고, `20260526-001-transcript-truth-source` §3의 정확성 우선 원칙과 부딪힌다.
  또 실측으로 transcript에 rate limit 기록이 없음을 확인해(현 저장소 전체 grep 0건) 5h/7d는 애초에 채울 수 없다.
- **OAuth usage API(`api.anthropic.com/api/oauth/usage`) 재도입**: 첫 렌더부터 5h/7d를 채우는 유일한 경로지만
  `20260602-001-simplify-statusline` §5.1(네트워크 접속 없음)의 정면 폐기이고, 렌더 경로에 자격증명 읽기와
  macOS keychain 프롬프트가 들어온다. 이번 범위에서 제외한다.

## 2. 목표

세션을 새로 열었을 때 사용자가 "statusline이 고장났다 / 위젯 설정이 안 먹었다"로 읽지 않게 한다. 지금 화면은
"아직 측정되지 않음"을 `0`으로 표시해 두 상태를 구별할 수 없고, rate limit 칸은 통째로 사라져 설정 오류처럼 보인다.

첫 응답 전에는 미측정임이 화면에서 구별되고, 첫 응답 이후에는 기존과 똑같이 실측치가 표시되거나 (데이터가 없는
계정에서는) 칸이 생략되게 한다.

## 3. 제약

- Zero dependency 유지 — Go 표준 라이브러리만 사용하며 `go.mod`에 `require` 블록이 생기지 않는다.
- 단일 `main` 패키지 유지 — 서브 패키지를 만들지 않고 파일 단위로만 분리한다.
- stdout은 위젯 렌더 결과 + ANSI 코드만 출력하고, debug/error는 stderr로만 보낸다.
- 데이터 출처는 stdin과 git 명령으로 한정한다 — 별도 파일 읽기·쓰기나 네트워크 접속을 추가하지 않는다
  (`20260602-001-simplify-statusline` §5.1 유지).
- placeholder 적용 대상은 `context`, `rateLimit5h`, `rateLimit7d` 셋으로 한정한다. `cost`는 첫 응답 전 값이
  참값이므로 대상이 아니다.
- placeholder 해제 신호는 `context_window.total_input_tokens > 0`(첫 API 응답 도착) 하나로 통일한다. 위젯별로
  다른 신호를 쓰지 않는다 — `rate_limits`는 비구독 계정에서 영구 부재이므로 위젯별 독립 판정은 placeholder를
  세션 내내 고착시킨다.
- placeholder 표기 문자는 **ASCII 범위의 1칸 문자만** 쓴다. East Asian Ambiguous 문자(`—`, `–` 등)는 터미널·폰트에
  따라 표시폭이 1칸/2칸으로 갈려 후속 작업의 줄 폭 보장을 흔든다.
- 흐린 표시는 기존 theme의 dim 색을 재사용한다. 새 색을 도입하지 않으며, dim이 무시되는 단색 터미널에서도
  placeholder 문자만으로 미측정임이 읽혀야 한다.
- 기존 생략 규약을 유지한다 — `context_window_size`가 0 이하이면 context 위젯은 placeholder 없이 생략되고,
  model·dir·context가 모두 비면 출력 전체를 생략한다.
- 사용자 체감 동작이 바뀌므로 CLAUDE.md §버전 정책에 따라 SemVer bump(`Makefile` VERSION,
  `.claude-plugin/plugin.json` version을 같은 값으로 동시 갱신)와 release 브랜치 동기화를 동반한다.
- 검증 명령은 `go test ./...`와 `go vet ./...`이다. 전자는 `Makefile`의 `test` 타깃과 같은 명령이며,
  `make`가 없는 환경도 있으므로 직접 부른다. 착수 시점(`ea393fb`)에 이미 실패하는 테스트 4개가 있고
  (`config.go`·`widgets_project.go`의 POSIX 경로 fixture가 Windows에서 깨지는 건) 이 feature가 건드리는
  코드와 겹치지 않는다 — 그 실패를 고치는 일은 §4 제외 범위다.

## 4. 제외 범위

- **`cost` 위젯의 placeholder 적용.** 첫 응답 전 `$0.00`은 참값이다.
- **stdin 밖 데이터 출처 도입.** transcript 파싱, OAuth usage API, 세션 상태 캐시 — §1에서 접은 접근들이며
  `20260602-001-simplify-statusline`의 결정을 유지한다.
- **placeholder를 켜고 끄는 config 옵션.** 기본 동작 하나로 확정하고, 옵션화는 요구가 확인된 뒤 별도로 다룬다.
- **stdin 파싱이 한 필드에서 실패하면 출력 전체가 사라지는 문제.** `parseStdinReader`가 어떤 decode 오류에도
  빈 `StdinInput`을 반환해 무출력 조건을 통과하는 성질을 확인했으나, 이번 표기 변경과 실패 의미가 다르다.
  별도 feature로 다룬다.
- **`COLUMNS` 기반 줄 폭 축소.** `20260727-001-statusline-schema-catchup` 소관이며, 본 feature는 그 축소 정책이
  전제하는 표시폭 계산을 흔들지 않는 문자만 쓰는 것으로 대응한다(§3).
- **rate limit의 `resets_at` 표기 정리.** `resets_at`이 없어 0으로 들어올 때 `time.Unix(0, 0)`이 zero time으로
  판정되지 않아 잔여 시간 표기를 i18n 문자열 비교로 걸러내고 있는 성질을 확인했으나, 이는 첫 응답 이후 구간의
  문제이므로 범위 밖이다.
- **착수 시점에 이미 실패하는 테스트 4개의 수정.** `TestConfigHomeDir/defaultConfigPath_falls_back_to_home`,
  `TestProjectPathCompressHome/current_under_home_gets_tilde_prefix`,
  `TestProjectPathShrink/{tilde,absolute}_path_over_budget_*` — 테스트가 POSIX 경로 fixture와 `HOME`
  override를 쓰는데 Windows는 `\` 구분자와 `USERPROFILE`을 쓰기 때문이며, 실제 Windows 입력에서는
  구현이 맞게 동작하므로 배포 동작의 결함이 아니다. `config.go`·`widgets_project.go` 소관이고 이
  feature가 건드리는 파일과 겹치지 않는다. 별도 feature로 다룬다.
- **`20260727-001-statusline-schema-catchup` 문서의 갱신.** 본 feature가 기본 위젯 구성을 바꾸므로 그 feature의
  §5.3 기준선("이번 변경 전과 동일")과 D3 근거(rate limit 위젯을 "데이터가 없으면 사라진다"의 예시로 든 문장)가
  갱신 대상이 된다. 갱신은 본 feature 완료 후 그 feature 문서에서 수행하며, 여기서 미리 고치지 않는다.

## 5. 완료 조건

1. `rate_limits`가 없고 `context_window.total_input_tokens`가 0인 stdin에 대해, stdout의 5h·7d 칸이 각각
   placeholder 표기로 나타난다.
2. 같은 stdin에 대해 stdout의 context 칸이 퍼센트·토큰 수 대신 placeholder 표기로 나타나며, progress bar는
   빈 상태로 함께 출력된다.
3. 같은 stdin에 대해 stdout의 cost 칸이 `$0.00`으로 나타난다 — placeholder 표기가 아니다.
4. `context_window.total_input_tokens`가 0보다 크고 `rate_limits`가 없는 stdin에 대해, stdout에 5h·7d 칸이
   나타나지 않는다.
5. `context_window.total_input_tokens`가 0보다 크고 `rate_limits`가 있는 stdin에 대해, stdout의 5h·7d 칸이
   실측 퍼센트로 나타난다.
6. `context_window.total_input_tokens`가 0보다 큰 stdin에 대해, stdout의 context 칸이 실측 퍼센트와 토큰 수로
   나타난다.
7. stdout에 나타나는 placeholder 표기가 ASCII 범위 문자로만 이루어진다.
8. `context_window_size`가 0 이하인 stdin에 대해, `total_input_tokens` 값과 무관하게 stdout에 context 칸이
   나타나지 않는다.
9. model·workspace·context가 모두 비어 있는 stdin에 대해 stdout에 아무것도 출력되지 않는다.
10. `Makefile`의 VERSION과 `.claude-plugin/plugin.json`의 version이 같은 새 값으로 갱신되어 있다.
11. `go vet ./...`가 통과하고, `go test ./...`의 실패 집합이 이 feature 착수 시점(`ea393fb`)과
    같다 — 새로 실패하는 테스트가 없고, 이번에 추가한 테스트는 모두 통과한다.
12. `README.md`의 위젯 설명에 첫 응답 전 표기 동작이 반영되어 있다.
