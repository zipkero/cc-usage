# session-start-placeholder — IMPLEMENT

- [x] task-001: context 칸의 첫 응답 전 placeholder 표기
  - 목적: 새 세션 첫 렌더에서 context 칸이 `0% 0` 대신 빈 progress bar와 흐린 `-` 하나로
    나타나고, 토큰이 보고되기 시작하면 기존과 같은 퍼센트·토큰 수 표기로 돌아온다.
  - 접근: `Context`에 `total_input_tokens > 0` 하나만 보는 읽기 전용 파생 판정을 두고, `-`
    상수와 "문자열을 `Secondary` + faint로 감싸는" 조립을 렌더 계층에 추가한다. `contextData`에
    zero value가 실측을 뜻하는 상태 필드를 붙여 `GetData`가 채우고 `Render`가 두 갈래로 갈린다.
  - 검증 조건:
    - 결과: `context_window_size`가 양수이고 `total_input_tokens`가 0인 stdin에서 context 칸이
      `<빈 bar> -`로 나타난다 — bar는 `widgets.context.barWidth` 해석 결과 폭 그대로 출력되고
      dim이 붙지 않으며, `-`에만 dim이 붙는다. `%` 기호와 토큰 수 조각은 나타나지 않는다.
      `total_input_tokens`가 0보다 크면 기존과 동일한 `<bar> <퍼센트>% <토큰>` 표기가 나온다.
      `context_window_size`가 0 이하이면 `total_input_tokens` 값과 무관하게 칸이 사라진다.
      placeholder 조각(bar 제외)은 ASCII 문자로만 이루어진다.
    - 확인: `widgets_core_test.go`에 `context_window_size`(0 이하 / 양수) ×
      `total_input_tokens`(0 / 양수) table-driven `GetData` 케이스와, `stripANSI`로 조립 형태·
      ASCII 구성을 보고 `stripANSI` 전 문자열에서 `theme.Dim` 포함 여부를 보는 placeholder 렌더
      케이스를 추가한다(`splitContextRender`는 placeholder 케이스에 쓰지 않는다). 기존
      `TestContextWidgetRender_TokenColor` 네 케이스와 `stdin_test.go`의
      `TestContextWidgetFractionalPercent`는 수정 없이 통과해야 한다. `go test ./...`가 착수 시점
      대비 새 실패 없이 돌고 `go vet ./...` 통과. 로컬 빌드
      (`go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/cc-usage .`) 후 수동 확인 —
      `echo '{"model":{"id":"claude-opus-4-6","display_name":"Opus"},"workspace":{"current_dir":"/tmp"},"context_window":{"total_input_tokens":0,"total_output_tokens":0,"context_window_size":200000,"current_usage":{"input_tokens":0,"output_tokens":0}},"cost":{"total_cost_usd":0}}' | ./dist/cc-usage`
      의 context 칸이 `░░░░░░░░ -`이고, `CLAUDE.md` §동작 확인 payload의 기대 출력
      `██░░░░░░ 30% 60K`가 그대로 유지된다.
  - 참조: SPEC §5.2, §5.6, §5.7, §5.8 / ANALYSIS §5 D1, D2, D3, D4, D5, D6

- [x] task-002: rateLimit5h·rateLimit7d 칸의 첫 응답 전 placeholder 표기
  - 목적: 새 세션 첫 렌더에서 사라져 있던 5h·7d 칸이 `5h: -`·`7d: -`로 자리를 지키고, rate limit
    데이터가 도착하면 실측 퍼센트로 바뀌며, 데이터가 끝까지 오지 않는 계정에서는 첫 응답 이후
    지금처럼 다시 칸이 사라진다.
  - 접근: `rateLimitData`에 zero value가 실측을 뜻하는 상태 필드를 붙이고, 두 `GetData`의 nil
    반환 조건을 "대응 rate limit 포인터가 nil **이고** 첫 응답이 이미 도착함"으로 좁힌다. 공유
    `renderRateLimit`에 라벨 + `: ` + placeholder 전체를 흐리게 내는 갈래를 둔다.
  - 검증 조건:
    - 결과: `rate_limits`가 없고 `total_input_tokens`가 0인 stdin에서 두 칸이 `5h: -`, `7d: -`로
      나타나고 라벨까지 dim이 적용되며 잔여 시간 `(…)` 접미사가 붙지 않는다. `rate_limits`가
      없고 `total_input_tokens`가 0보다 크면 두 `GetData`가 nil을 돌려 칸이 나타나지 않는다.
      `rate_limits`가 있으면 `total_input_tokens` 값과 무관하게 실측 퍼센트가 나타난다.
      placeholder 조각은 ASCII 문자로만 이루어진다.
    - 확인: `widgets_core_test.go`에 `total_input_tokens`(0 / 양수) × `rate_limits`(부재 / 존재)
      네 조합을 table-driven으로 두고 각 칸의 기대를 (nil 반환 / placeholder 상태 / 실측)으로
      고정한다. 렌더는 `stripANSI`로 `5h: -` 형태와 ASCII 구성을, `stripANSI` 전 문자열로
      `theme.Dim` 적용을 확인한다. `go test ./...`가 착수 시점 대비 새 실패 없이 돌고
      `go vet ./...` 통과. 로컬 빌드 후 task-001의 첫 렌더 payload를 실행해 줄 끝이 `│ 5h: - │ 7d: -`로 끝나고, 같은 payload에
      `"rate_limits":{"five_hour":{"used_percentage":42,"resets_at":0},"seven_day":{"used_percentage":69,"resets_at":0}}`
      만 더하면 `│ 5h: 42% │ 7d: 69%`가 나오는 것을 확인한다.
  - 참조: SPEC §5.1, §5.4, §5.5, §5.7 / ANALYSIS §5 D1, D2, D3, D4

- [x] task-003: 첫 렌더·첫 응답 이후 stdout 한 줄 전체 고정
  - 목적: 첫 렌더 stdin과 첫 응답 이후 stdin에 대해 실제로 출력되는 줄 전체가 기대 형태와
    일치하고, 정체성 신호가 모두 빈 stdin에서는 여전히 아무것도 출력되지 않는다.
  - 접근: `widget_test.go` 관례대로 `Separator: "space"` + 명시 `Lines`로 `context`·`cost`·
    `rateLimit5h`·`rateLimit7d`만 담은 줄을 만들고 `t.Setenv("PATH", "")`로 git을 차단한 뒤,
    두 payload의 `orchestrate` 결과 줄 전체를 비교한다.
  - 검증 조건:
    - 결과: `rate_limits` 없음 + `total_input_tokens: 0` + `context_window_size` 양수 stdin에서
      줄이 `<빈 bar> -  $0.00  5h: -  7d: -`(stripANSI 기준, `space` separator이므로 칸 사이는
      공백 두 개)가 되어 5h·7d 칸이 줄에 실제로 존재하고 cost 칸은 placeholder가 아닌 `$0.00`이다.
      `total_input_tokens` 양수 + `rate_limits` 없음 stdin에서는 같은 줄에 5h·7d 조각이 전혀
      나타나지 않는다. model·workspace·context가 모두 빈 stdin에서는 출력이 전부 생략된다.
    - 확인: 위 두 payload 케이스를 담은 orchestrate 수준 테스트가 통과하고, 무출력 조건은
      `main_test.go`의 기존 `TestShouldSuppressOutput` 다섯 케이스가 수정 없이 통과하는 것으로
      확인한다(`shouldSuppressOutput`의 판정 입력을 바꾸지 않았음을 `git diff main.go`로 대조).
      `go test ./...`가 착수 시점 대비 새 실패 없이 돌고 `go vet ./...` 통과. 로컬 빌드 후
      task-001·task-002의 두 payload를 실행해 줄 전체를 눈으로 확인하고,
      `echo '{}' | ./dist/cc-usage`가 아무것도 출력하지 않는 것을 확인한다.
  - 참조: SPEC §5.1, §5.2, §5.3, §5.4, §5.9, §5.11 / ANALYSIS §5 D7

- [x] task-004: 배포 버전 0.5.5 갱신
  - 목적: `/plugin` UI가 이번 표기 변경을 업데이트로 감지할 수 있도록 배포 버전이 0.5.5로
    올라가 있다.
  - 접근: `Makefile`의 `VERSION`과 `.claude-plugin/plugin.json`의 `version`을 같은 값 `0.5.5`로
    동시에 갱신한다.
  - 검증 조건:
    - 결과: 두 파일의 버전 문자열이 모두 `0.5.5`이고 서로 어긋나지 않는다.
    - 확인: `git diff Makefile .claude-plugin/plugin.json`으로 두 값이 같은지 대조하고,
      로컬 빌드 후 `./dist/cc-usage --version`이 `0.5.5`를 출력하는 것을 확인한다.
      `go test ./...`가 착수 시점 대비 새 실패 없이 돌고 `go vet ./...` 통과.
  - 참조: SPEC §5.10 / ANALYSIS §5 D8

- [x] task-005: README 위젯 표에 첫 응답 전 표기 반영
  - 목적: README 위젯 표를 읽는 사용자가 첫 응답 전 `-` 표기와 rate limit 칸 생략이 정상
    동작임을 문서에서 확인할 수 있다.
  - 접근: 위젯 표의 `context`·`rateLimit5h`·`rateLimit7d` 설명 행에 첫 응답 전 표기를 덧붙인다
    — context는 빈 bar + `-`, rate limit은 `5h: -`와 데이터가 오지 않는 계정에서의 생략.
    Troubleshooting에 항목을 새로 만들지 않는다.
  - 검증 조건:
    - 결과: 위젯 표 세 행에 첫 응답 전 표기 서술이 들어가고, 그 문구가 task-003 수동 실행에서
      실제로 나온 출력과 일치한다. 다른 행과 config 표는 바뀌지 않는다.
    - 확인: `git diff README.md`로 세 행만 바뀐 것을 확인하고, 서술한 표기를 task-003의 두
      payload 수동 실행 출력과 대조한다.
  - 참조: SPEC §5.12 / ANALYSIS §5 D8
