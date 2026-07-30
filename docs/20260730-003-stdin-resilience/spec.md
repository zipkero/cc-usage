# spec: stdin-resilience

## 1. 범위

stdin 입력이 예상과 달라도 status line 전체가 사라지지 않게 한다. 두 갈래다.

- **파싱 격리**: 현재 `parseStdinReader`는 어떤 decode 오류에도 빈 `StdinInput`을 반환한다. 그러면 정체성
  신호가 전부 비어 무출력 조건을 통과해 status line이 통째로 사라진다. 최상위를 섹션 단위로 나눠 해석하고
  깨진 섹션만 버리도록 바꾼다.
- **부재 처리 정리**: rate limit의 `resets_at`이 없어 0으로 들어올 때 `time.Unix(0, 0)`이 Go zero time으로
  판정되지 않아, 잔여 시간 표기를 **표시 문자열 비교**로 걸러내고 있다. 표시용 출력을 제어 신호로 쓰는
  결합을 끊는다.

### 입력 맥락

`20260730-001-session-start-placeholder` 작업 중 확인한 사실이다.

- `parseStdinReader`(`stdin.go`)는 `json.NewDecoder(r).Decode(&input)`가 오류를 내면 `StdinInput{}`을
  돌려준다. 부분 실패 상태가 없다. 그 뒤 `shouldSuppressOutput`이 model·workspace·context가 모두 비었다고
  보고 출력을 생략하므로, **필드 하나의 타입 불일치가 전면 블랙아웃이 된다.**
- v0.5.4 fix(`45749a8`, "rate_limit·context 백분율 소수값 파싱 — 전체 출력 누락 수정")가 이미 이 부류였다.
  당시 `used_percentage`를 `int`로 모델링해 소수 입력이 전체 파싱을 실패시켰고 status line이 사라졌다.
  `stdin_test.go`의 `TestStdinFractionalPercentages` 주석에 그 경위가 남아 있다.
- 공식 status line 문서는 필드를 계속 늘리고 있다 — `fast_mode`, `effort.level`, `thinking.enabled`,
  `workspace.git_worktree`, `workspace.repo`, `pr`, `prompt_id`가 현재 스키마에 있고 cc-usage의
  `StdinInput`에는 없다. 타입이 바뀌거나 형태가 달라지는 변경이 또 오면 같은 실패가 반복된다.
- `20260727-001-statusline-schema-catchup`은 이 항목을 다루지 않는다 — 그 analysis 492줄을
  `RawMessage|전면|블랙아웃|파싱 실패`로 훑어도 언급이 0건이고, 그 spec §4 제외 범위에도 없다. 단순히 빠진
  항목이다.
- `renderRateLimit`(`widgets_core.go`)은 `ResetsAt`이 Go zero time이 아닐 때만 잔여 시간을 붙이는데,
  `resets_at: 0`은 `time.Unix(0, 0)` = 1970년이라 그 가드를 통과한다. 그래서 실제 차단은 그다음
  `formatTimeRemaining` 결과를 `"0" + t.Time.Minutes`와 **문자열로 비교**하는 데 의존한다.

  이 비교가 locale 문구에 따라 깨지지는 않는다 — 비교식의 좌우가 같은 `ctx.Translations`를 읽으므로
  문구를 어떻게 바꿔도 값이 함께 움직인다(실행으로 확인). 문제는 다른 둘이다. 첫째, **표시용 문자열을
  제어 신호로 쓰고 있어** `formatTimeRemaining`의 과거 분기 반환 형태를 누가 바꾸면(초 단위로, 또는
  `"expired"`처럼) 게이트가 조용히 열려 1970년 기준 잔여 시간이 표시된다. 둘째, 서로 다른 세 입력
  (키 부재, `0`이나 과거 시각, 60초 미만 미래)이 같은 표시 문자열로 뭉개져 구별할 수단이 없다.

같은 작업의 §4 제외 범위가 두 항목을 별도 feature로 넘겼고, `ROADMAP.md` M2가 그 자리다.

## 2. 목표

필드 하나가 예상과 달라도 해석 가능한 위젯은 계속 보이게 한다. 지금은 사용자가 "status line이 죽었다"를
보게 되는데, 실제로는 stdin의 한 구석만 달라진 것이다.

부재 처리 판정을 i18n 문구에서 떼어내, locale 파일을 고치다가 rate limit 표시가 조용히 깨지는 경로를
없앤다.

## 3. 제약

- Zero dependency 유지 — Go 표준 라이브러리만 쓰며 `go.mod`에 `require` 블록이 생기지 않는다.
- 단일 `main` 패키지 유지 — 서브 패키지를 만들지 않고 파일 단위로만 분리한다.
- stdout은 위젯 렌더 결과 + ANSI 코드만 담고, 진단은 stderr로만 보낸다.
- **관용 단위는 최상위 섹션이다.** 섹션별로 해석해 깨진 섹션만 버리고 나머지는 살린다. 섹션 안의 개별
  필드까지 살리는 관용은 하지 않는다 — 같은 섹션에서 일부만 살리면 "토큰 수는 있는데 컨텍스트 크기만
  깨진" 모순된 조합이 만들어져 잘못된 값을 표시할 위험이 생긴다.
- **부분 실패 알림은 stderr(`debugLog`)만이다.** status line에 실패 마커를 표시하지 않는다.
- 최상위 JSON 자체가 파싱 불가한 입력(구문 오류, 최상위가 객체가 아닌 경우)은 복구 대상이 아니다. 기존과
  같이 빈 입력으로 떨어져 무출력이 된다.
- 알 수 없는 필드를 무시하는 현재 동작을 유지한다(`DisallowUnknownFields`를 쓰지 않는다).
- 무출력 조건을 유지한다 — model·workspace·context가 모두 비면 출력을 전부 생략한다.
- 데이터 출처는 stdin과 git 명령으로 한정한다(`20260602-001-simplify-statusline` §5.1 유지).
- 사용자 체감 동작이 바뀌므로 CLAUDE.md §버전 정책에 따라 SemVer bump(`Makefile` VERSION과
  `.claude-plugin/plugin.json` version을 같은 값으로 동시 갱신)와 `bin/` 재빌드·release 브랜치 동기화를
  동반한다.
- 검증 명령은 `go test ./...`와 `go vet ./...`이며 실패 0으로 통과해야 한다. `20260730-002`가 CI를
  붙였으므로 세 OS 러너도 green이어야 한다.

## 4. 제외 범위

- **필드 단위 관용.** §3에서 섹션 단위로 확정했다.
- **status line에 실패 마커 표시.** 알림은 stderr로만 한다.
- **관용 수준을 조정하는 config 옵션.** 동작 하나로 고정한다.
- **새 stdin 필드의 수용·노출.** `fast_mode`·`effort.level`·`thinking.enabled`·`workspace.git_worktree`·
  `workspace.repo`·`pr`는 `20260727-001-statusline-schema-catchup`(ROADMAP M3) 소관이다. 이 feature는
  기존 필드의 해석 견고성만 다루고 스키마를 넓히지 않는다.
- **기존 위젯의 계산·표기 교정.** `used_percentage`의 input-only 의미 정합 같은 항목도 M3 소관이다.
- **`COLUMNS` 기반 줄 폭 축소, 새 위젯 추가, 테마·separator 변경.**
- **`20260528-001-refactor-structure`에서 살아남은 i18n 분리.** `ROADMAP.md` 보류 범위가 별도 feature로
  다시 정의하기로 한 항목이다.

## 5. 완료 조건

1. `rate_limits.five_hour.used_percentage`에 숫자가 아닌 값이 들어온 stdin에 대해, stdout에 model·
   context·cost 칸이 정상 출력된다 — 출력이 전부 사라지지 않는다.
2. `context_window` 섹션이 깨진 stdin에 대해, stdout에 model·cost 칸이 출력되고 context 칸만 생략된다.
3. 같은 방식으로 `workspace` 섹션이 깨진 stdin에 대해, model·context·cost 칸이 출력된다.
4. 최상위 JSON이 구문 오류이거나 객체가 아닌 stdin에 대해 stdout에 아무것도 출력되지 않는다.
5. 알 수 없는 필드가 섞인 정상 stdin에 대해 stdout이 이 변경 전과 동일하다.
6. 모든 섹션이 정상인 stdin에 대해 stdout이 이 변경 전과 동일하다.
7. 깨진 섹션이 있는 stdin에 대해 그 사실이 stderr에 남고, stdout에는 실패를 알리는 문자가 나타나지 않는다.
8. `rate_limits`에 `resets_at`이 없거나 `0`인 stdin에 대해 잔여 시간 표기가 stdout에 나타나지 않는다.
9. 8번 동작이 locale 문자열과 무관하게 성립한다 — `language`를 `en`과 `ko`로 각각 두어도 결과가 같고,
   locale 파일의 시간 단위 문구를 바꿔도 판정이 달라지지 않는다.
10. `resets_at`이 미래 시각인 stdin에 대해 잔여 시간 표기가 이 변경 전과 동일하게 나타난다.
11. model·workspace·context가 모두 비어 있는 stdin에 대해 stdout에 아무것도 출력되지 않는다.
12. `go test ./...`가 실패 없이 통과하고 `go vet ./...`가 통과한다.
13. `Makefile`의 VERSION과 `.claude-plugin/plugin.json`의 version이 같은 새 값으로 갱신되어 있다.
14. `main`에 반영된 뒤 CI 워크플로가 실행되어 세 러너 모두 성공한다.
15. `README.md`에 부분 입력 손상 시의 표시 동작이 반영되어 있다.
