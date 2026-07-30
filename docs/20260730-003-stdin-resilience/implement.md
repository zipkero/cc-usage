# stdin-resilience — IMPLEMENT

- [x] task-001: 최상위 섹션 격리 조립
  - 목적: stdin의 최상위 섹션 하나가 예상과 다른 타입으로 와도 나머지 섹션에서 나오는 칸이
    그대로 출력되고, 최상위 JSON 자체가 파싱 불가한 입력에서는 지금처럼 아무것도 출력되지
    않는다.
  - 접근: 최상위를 `map[string]json.RawMessage`로 decode한 뒤 `StdinInput` 최상위 json 태그
    18개를 고정 순서로 나열한 섹션 테이블을 순회해 섹션별 `Unmarshal`하고, 실패한 섹션은
    zero value로 남기고 손상 목록만 모아 돌려준다. `StdinInput`의 필드·태그·포인터 여부와
    `parseStdinReader` 시그니처는 그대로 두고 손상 섹션이 zero value로 남는 불변식을 doc
    주석에 적는다.
  - 검증 조건:
    - 결과: `rate_limits.five_hour.used_percentage`가 `"high"`인 stdin에서 model·context·cost
      칸이 정상 출력되고 5h·7d 칸만 빠진다 — `5h: 0%`처럼 섹션 일부만 살아난 값은 나타나지
      않는다. `context_window`가 `[1,2,3]`인 stdin에서는 model·cost 칸이 나오고 context 칸만
      생략되며, `workspace`가 `"nope"`인 stdin에서는 model·context·cost 칸이 나온다.
      `version:7`처럼 스칼라 섹션 하나가 깨져도 그 필드만 비고 다른 칸은 전부 그대로다.
      최상위가 구문 오류·비객체·`null`·빈 입력인 네 형태에서는 `parseStdinReader`가
      `StdinInput{}`을 돌려주고 그 값이 무출력 판정을 통과한다. model·workspace·context가
      모두 빈 stdin도 그대로 무출력이다. 알 수 없는 최상위 키가 섞인 정상 payload와 모든
      섹션이 정상인 payload의 stdout은 이 변경 전과 같다. 섹션 테이블의 키 집합은
      `StdinInput` 최상위 json 태그 18개와 정확히 일치한다.
    - 확인: `stdin_test.go`에 섹션별 손상 table-driven 케이스를 추가해 조립 함수가 돌려주는
      손상 목록과 살아남은 섹션의 필드 값을 함께 단정하고, 최상위 실패 네 형태·알 수 없는
      키·중복 키(마지막 승)·후행 쓰레기 케이스도 같은 파일에 둔다. 같은 파일에 `reflect`로
      `StdinInput` 최상위 필드의 json 태그를 훑어 섹션 테이블 키 집합과 양방향 비교하는
      완전성 테스트를 둔다. `widget_test.go`에 손상 섹션별 `orchestrate` stdout 한 줄을
      `stripANSI`로 고정하는 케이스를 추가하되 `TestOrchestrateSessionStartLine`의 `PATH=""` +
      `Separator` 명시 + 명시 `Lines` 패턴을 따른다. 기존 `TestShouldSuppressOutput`,
      `TestStdinFractionalPercentages`, `TestContextWidgetFractionalPercent`,
      `TestOrchestrateSessionStartLine`은 수정 없이 통과해야 한다. `go test ./...` 실패 0,
      `go vet ./...` 통과. 로컬 빌드
      (`go build -ldflags="-s -w -X main.version=0.5.5" -o dist/cc-usage.exe .`) 후 수동 확인 —
      `CLAUDE.md` §동작 확인 payload의 기대 출력
      `tmp │ ◆ claude-opus-4-6 │ ██░░░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%`가 그대로 나오고,
      같은 payload에서 `used_percentage`만 `"high"`로 바꾼 입력에서 5h·7d를 뺀 나머지 칸이
      남는다.
  - 참조: SPEC §5.1, §5.2, §5.3, §5.4, §5.5, §5.6, §5.11, §5.12 /
    ANALYSIS §5 D1, D2, D3, D4, D5, D9, D10

- [x] task-002: 부분 실패 진단을 stderr로만 결정적으로 기록
  - 목적: 어느 섹션이 버려졌고 테이블에 없는 최상위 키가 무엇이었는지가 디버그 출력에만
    실행마다 같은 순서로 남고, 위젯 출력에는 실패를 알리는 문자가 전혀 섞이지 않는다.
  - 접근: 조립이 돌려준 손상 목록을 섹션 테이블 순서대로 `debugLog`로 남기고, 테이블에 없는
    최상위 키는 정렬해 한 줄로 남긴다. 손상 상태는 `Context`나 위젯에 전달하지 않는다.
  - 검증 조건:
    - 결과: `DEBUG=cc-usage`에서 섹션 둘이 깨지고 테이블에 없는 키가 섞인 stdin을 반복 실행해도
      stderr 줄 내용과 줄 순서가 매번 같고, 알 수 없는 키는 payload에 적힌 순서와 무관하게
      사전순 한 줄로 나온다. 같은 입력의 stdout에는 손상이나 알 수 없는 키를 알리는 문자가
      전혀 없고 살아있는 칸만 나온다. `DEBUG` 미설정에서는 stderr가 비어 있다. 알 수 없는
      필드가 섞인 정상 stdin의 stdout은 이 변경 전과 동일하다.
    - 확인: `stdin_test.go`에 기존 `captureStderr` helper를 쓰는 케이스 하나를 두고, 섹션 둘이
      깨지고 테이블에 없는 키가 사전순과 어긋난 순서로 셋 들어 있는 payload로 줄 내용·줄
      순서·키 정렬을 함께 단정한다 — `captureStderr`를 쓰는 케이스는 이 하나로 제한한다.
      `widget_test.go`에서 같은 payload의 `orchestrate` stdout을 `stripANSI`로 확인해 진단
      문자가 없음을, 그리고 알 수 없는 키만 섞인 정상 payload의 stdout이 정상 payload와 같음을
      단정한다. 로컬 빌드 바이너리에 `DEBUG` 미설정으로 손상 payload를 넣어 stderr가 비어 있고
      stdout에 살아있는 칸만 나오는지 수동 확인. `go test ./...` 실패 0, `go vet ./...` 통과.
  - 참조: SPEC §5.5, §5.7, §5.12 / ANALYSIS §5 D8, D11

- [x] task-003: rate limit 잔여 시간 판정을 표시 문자열에서 시각 계산으로 이전
  - 목적: rate limit 초기화 시각이 오지 않았거나 `0`이거나 이미 지난 stdin에서 잔여 시간 괄호가
    붙지 않고, 1분 이상 남은 시각에서는 이 변경 전과 똑같은 잔여 시간 표기가 붙으며, 그 판정이
    locale 문구를 어떻게 바꿔도 달라지지 않는다.
  - 접근: 두 rate limit 위젯의 `GetData`가 `resets_at` 0 이하를 Go zero time으로 매핑하고,
    `formatTimeRemaining`을 `(string, bool)`로 바꿔 `int(diff.Minutes()) > 0`일 때만 `true`를
    준다. 렌더의 `"0" + Minutes` 문자열 비교를 그 불리언으로 대체한다. `ResetsAt`은 `int64`로
    유지한다.
  - 검증 조건:
    - 결과: `resets_at` 키가 없는 입력과 `resets_at: 0` 입력 모두에서 퍼센트만 나오고 `(…)`
      접미사가 없다. 이미 지난 시각과 60초 미만 미래에서도 접미사가 없어 변경 전 출력과 같다.
      2시간 뒤·3일 뒤 시각에서는 변경 전과 같은 시간·분 접미사 문자열이 붙는다.
      `Translations.Time.Minutes`를 `"m"`·`"분"`·`" minutes"`·`""`로 바꿔도 위 억제·표기 여부가
      넷 다 같고, `language`를 `en`과 `ko`로 두어도 결과가 같다. 렌더·포맷 경로에 표시 문자열을
      제어 신호로 쓰는 비교가 남아 있지 않다.
    - 확인: `widgets_core_test.go`에 R0(부재 / `0`)·R1(과거 / 59초 미래)·R2(2시간 뒤 / 3일 뒤)
      table-driven 케이스를 두고 `stripANSI` 결과의 접미사 유무를, R2에서는 접미사 문자열
      자체를 단정한다. 같은 파일에 `Time.Minutes`를 위 네 값으로 채운 `&Translations{}`로 판정
      불변성을 단정하는 케이스를 추가한다. `"0" +` 형태의 문자열 비교가 남아 있지 않은지
      `grep`으로 확인. 기존 `TestRateLimitWidgetsGetData`와 `TestRenderRateLimit_Placeholder`는
      수정 없이 통과해야 한다. `go test ./...` 실패 0, `go vet ./...` 통과. 로컬 빌드 후 수동
      확인 — `CLAUDE.md` §동작 확인 payload(`resets_at: 0`)의 기대 출력 `5h: 42% │ 7d: 69%`가
      그대로이고, `resets_at`을 2시간 뒤 epoch로 바꾸면 두 칸에 잔여 시간 괄호가 붙는다.
  - 참조: SPEC §5.8, §5.9, §5.10, §5.12 / ANALYSIS §5 D6, D7, D11

- [x] task-004: README에 부분 입력 손상 시의 표시 동작 반영
  - 목적: 사용자가 README만 보고, stdin 한 섹션이 예상과 달라도 status line이 통째로 사라지지
    않고 그 섹션 칸만 빠진다는 것과 어느 섹션이 버려졌는지 어떻게 확인하는지 알 수 있다.
  - 접근: §Troubleshooting에 항목 하나를 더해 관용 단위가 최상위 섹션이라는 것, status line에
    실패 마커를 표시하지 않는다는 것, `DEBUG=cc-usage`로 버려진 섹션을 stderr에서 볼 수 있다는
    것, 최상위 JSON 자체가 깨진 입력은 예외로 무출력이라는 것을 적는다.
  - 검증 조건:
    - 결과: README.md §Troubleshooting에 해당 항목이 있고 서술이 task-001~task-003의 실제 동작과
      일치한다. 손상 섹션 칸만 빠지는 것과 최상위 실패 시 무출력이 구분되어 적혀 있다.
    - 확인: README.md diff 검토 + 적어 넣은 동작을 로컬 빌드 바이너리에 같은 payload를 넣어
      수동으로 재현. 문서 한 줄 표시폭이 기존 README 관례를 넘지 않는지 확인.
  - 참조: SPEC §5.15

- [x] task-005: 버전 값을 두 메타데이터 파일에 같은 새 값으로 갱신
  - 목적: 사용자의 플러그인 업데이트 감지가 이번 변경을 새 버전으로 알아보고, 빌드된 바이너리가
    그 값을 보고한다.
  - 접근: `Makefile`의 VERSION과 `.claude-plugin/plugin.json`의 version을 `0.5.5`에서 `0.5.6`
    으로 동시에 올린다 — 사용자 체감 fix이므로 CLAUDE.md §버전 정책의 patch bump다.
  - 검증 조건:
    - 결과: 두 파일의 값이 모두 `0.5.6`이고 서로 같다. 그 값으로 빌드한 바이너리가 `--version`에
      `0.5.6`을 출력한다.
    - 확인: 두 파일 diff 확인 +
      `go build -ldflags="-s -w -X main.version=0.5.6" -o dist/cc-usage.exe .` 성공 후
      `./dist/cc-usage.exe --version` 출력 확인. `go test ./...` 실패 0, `go vet ./...` 통과.
  - 참조: SPEC §5.12, §5.13

- [ ] task-006: main 반영 후 CI 3 러너 통과 확인과 release 브랜치 동기화
  - 목적: 이번 변경이 세 OS 러너에서 모두 성공한 상태로 기본 브랜치에 올라가고, marketplace로
    설치하는 사용자가 새 버전 바이너리를 받는다.
  - 접근: `Makefile`의 `build` 타깃과 같은 GOOS/GOARCH 조합·ldflags로 `go build`를 직접 돌려
    `bin/` 바이너리를 새 버전으로 다시 만들고, commit·push 후 CI 결과를 확인한 뒤 CLAUDE.md
    §릴리스 절차대로 `release` 브랜치에 파일을 복사해 새 commit을 올린다. `bin/` 재빌드 결과
    commit, `main` push, `release` push는 원격·배포에 영향을 주므로 각 단계를 실행 전 사용자에게
    확인받고 진행한다.
  - 검증 조건:
    - 결과: `main`의 최신 commit에 대한 CI 워크플로 실행에서 ubuntu·macos·windows 세 job이 모두
      success다. `bin/` 바이너리 5개가 새 버전으로 갱신되어 있고, `release` 브랜치 최신 commit의
      `bin/`·`.claude-plugin/`이 `main`과 같은 버전을 담고 있다.
    - 확인: push 전 `go test ./...` 실패 0과 `go vet ./...` 통과를 로컬에서 먼저 확인.
      `gh run list`·`gh run view`로 세 job의 결론을 각각 확인하고, 재빌드한 바이너리 중 현재
      플랫폼용을 `--version`으로 확인. `git log`·`git show`로 `release` 브랜치 동기화 결과 확인.
  - 참조: SPEC §5.13, §5.14
