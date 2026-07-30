# stdin-resilience — ANALYSIS

## 근거

읽은 범위는 spec.md 전체(§1~§5)와 `stdin.go`, `main.go`, `widget.go`, `widgets_core.go`,
`widgets_project.go`, `format.go`, `config.go`, `locales/{en,ko}.json`, `stdin_test.go`,
`main_test.go`, `widget_test.go`, `widgets_core_test.go`, `config_test.go`, `ROADMAP.md` §M2·§M3,
`README.md` §Widgets·§Troubleshooting, `CLAUDE.md`다. spec.md에 미답 항목은 없다 — §1~§4가 관용
단위·알림 채널·제외 범위를 모두 확정했고, §5의 15개 조건에 빈 자리가 없다.

코드로 확인한 사실이다.

- `StdinInput`의 최상위 필드는 18개다 — `model`, `workspace`, `worktree`, `context_window`, `cost`,
  `rate_limits`, `output_style`, `exceeds_200k_tokens`, `transcript_path`, `version`, `session_id`,
  `session_name`, `permission_mode`, `vim`, `agent`, `remote`, `agent_id`, `agent_type`. 이 가운데
  포인터인 것은 `worktree`, `rate_limits`, `output_style`, `vim`, `agent`, `remote` 6개이고 나머지는
  값 타입이다. 섹션 안에서 포인터로 모델링된 필드는 `context_window`의 두 백분율과 `cost`의 네 집계,
  `rate_limits`의 `five_hour`·`seven_day`다.
- `parseStdinReader`는 `Decode` 오류에 `StdinInput{}`을 반환한다. 부분 결과를 버린다.
- `shouldSuppressOutput`은 `Workspace.CurrentDir`, `Model.ID`, `Model.DisplayName`,
  `ContextWindow.ContextWindowSize` 네 값만 본다. 즉 세 섹션이 동시에 사라져야 무출력이 되는데, 현재는
  어느 한 필드의 타입 불일치가 그 세 섹션을 동시에 지운다.
- `renderRateLimit`의 잔여 시간 게이트는 두 겹이다. `!d.ResetsAt.IsZero()`가 앞에 있지만 stdin에서 온
  값에는 사실상 통과 게이트다 — 실제 차단은 `formatTimeRemaining` 결과를
  `"0"+ctx.Translations.Time.Minutes`와 문자열 비교하는 뒤쪽이 한다.
- `formatTimeRemaining`은 `diff <= 0`에 `"0"+t.Time.Minutes`를 반환하고, 미래이지만 총 분이 0이면
  마지막 분기에서 같은 문자열을 만든다. 호출자는 `renderRateLimit` 한 곳뿐이고 전용 테스트가 없다.

실행으로 확인한 사실이다(스크래치패드 probe + `0.5.5` 빌드 바이너리).

- 타입 불일치는 예외 없이 전면 블랙아웃이 된다. `rate_limits.five_hour.used_percentage:"high"`,
  `context_window:[1,2,3]`, `workspace:"nope"`, 그리고 스칼라 하나인 `version:7`까지 네 payload 모두
  stdout 0바이트 + stderr `parse error` + `suppressing output`이었다.
- `encoding/json`은 struct 대상 decode에서 타입 오류를 만나도 그 값만 건너뛰고 나머지 필드를 계속 채운다.
  오류는 마지막에 하나 반환된다. 즉 오류를 무시하고 부분 결과를 쓰면 **필드 단위** 관용이 되고,
  `rate_limits`가 깨진 위 payload에서는 `five_hour` 포인터가 할당된 채 `used_percentage`만 0으로 남아
  `5h: 0%`라는 없는 값이 표시된다. spec §3이 금지한 조합이다.
- **최상위를 `map[string]json.RawMessage`로 받으면 갈림이 정확히 원하는 자리에 생긴다.** 값이 구문상
  유효하고 타입만 틀린 경우(`context_window:[1,2,3]`, `version:7`, `used_percentage:"high"`) `Decode`는
  오류 없이 성공하고 모든 키가 raw 바이트로 남는다. 반면 값 안에 구문 오류가 있으면(`{...200000,}`)
  최상위 `Decode`가 실패한다 — decoder가 값 전체를 훑어야 하므로 섹션 내부 구문 오류도 최상위 실패다.
  최상위가 객체가 아닌 경우(`[1,2,3]`, `"hello"`)도 실패한다. `null`은 성공하지만 map이 nil이어서 섹션
  0개다. 빈 입력은 `EOF`다.
- map decode는 현재 struct decode와 다음 동작이 같다 — 알 수 없는 키를 그냥 담고, 중복 키는 마지막이
  이기고, 값 하나만 읽어 뒤에 붙은 쓰레기를 오류로 보지 않는다.
- `time.Unix(0,0).IsZero()`는 false다(1970-01-01). Go zero time은 `time.Time{}`(1년)이다.
- `resets_at` 부재와 `0`은 현재 `int64` 값 필드로 구별되지 않는다(둘 다 0). `*int64`로 바꾸면 구별된다.
- **잔여 시간 게이트는 locale 문구와 무관하게 성립한다.** `Time.Minutes`를 `"m"`, `"분"`, `" minutes"`,
  `""`로 두고 각각 `resets_at=0`·1시간 과거·30초 미래·2시간 미래를 넣었을 때 억제 여부가 넷 다 동일했다
  (앞 셋 억제, 마지막만 표기). 바이너리에서도 `language`를 `en`/`ko`로 두고 같은 결과를 얻었다. 비교식의
  양쪽이 같은 `Translations`를 읽기 때문이다. spec §1 입력 맥락이 이 사실을 반영해 갱신됐고, 실제 결함
  두 가지도 거기 적혀 있다 — 표시 문자열을 제어 신호로 쓰는 결합, 그리고 서로 다른 세 입력(부재,
  `0`/과거, 60초 미만 미래)이 같은 표시 문자열로 뭉개지는 것.
- 이 사실의 결과로 SPEC §5.9는 새로 고치는 동작이 아니라 **이미 성립하는 성질을 고정하는 회귀 pin**이다.
  이 설계는 판정 근거를 문자열에서 시각 계산으로 옮기므로 그 성질이 구조적으로 보장되게 된다.
- 테스트 기반은 이미 있다. `orchestrate`는 `Context`만 있으면 직접 호출 가능하고
  (`widget_test.go` `TestOrchestrateSessionStartLine`이 `PATH=""` + `Separator:"space"` + 명시 `Lines`로
  stdout 한 줄을 고정하는 패턴), `stripANSI`(widgets_project_test.go)와
  `captureStderr`(config_test.go)가 패키지 전역 helper로 존재한다. 병렬 테스트(`t.Parallel`)는 한 곳도
  없어 `os.Stderr` 교체가 안전하다.
- 버전 기준선은 `Makefile` VERSION `0.5.5`, `.claude-plugin/plugin.json` `0.5.5`다.

추정으로 남는 것은 하나다. Claude Code가 `resets_at`을 실제로 어떤 형태로 비우는지(키 생략인지 `0`인지)는
확인하지 못했다. 설계는 두 형태를 같게 처리해 이 추정에 의존하지 않는다.

## 1. 구조

경계는 파싱 층 안에 둔다. 새 중간 층을 만들지 않고, 위젯 층에도 손상 여부를 알리지 않는다.

`stdin.go`가 지금 하는 일은 "바이트 → `StdinInput`" 하나다. 이것을 두 단계로 쪼갠다.

- **최상위 분해**: 바이트를 `map[string]json.RawMessage`로 읽는다. 이 단계는 전부 성공하거나 전부
  실패한다. 실패하면 기존과 동일하게 빈 `StdinInput`으로 떨어진다(spec §3의 복구 대상 밖, SPEC §5.4).
- **섹션 조립**: 고정 순서 테이블을 따라 섹션별로 해당 필드에 `Unmarshal`한다. 성공한 섹션만 `StdinInput`에
  들어가고, 실패한 섹션은 버려져 zero value로 남는다.

이 두 단계 모두 `stdin.go` 안에 둔다. 파일 단위 분리만 허용하는 spec §3 아래에서 새 파일을 만들 이유가
없다 — 조립은 `StdinInput` 정의와 같은 자리에 있어야 태그와 테이블의 어긋남을 알아채기 쉽다.

`StdinInput`의 형태는 바꾸지 않는다. 필드도 태그도 포인터 여부도 그대로다(§5 D4). 이 타입은
`shouldSuppressOutput`, 7개 위젯의 `GetData`, `Context.FirstResponseReceived`가 이미 소비하고 있고,
SPEC §5.5·§5.6이 정상 입력에서의 stdout 무변화를 요구한다. 조립 방식만 바뀌고 결과 타입은 불변이라는
것이 이 feature의 격리선이다.

두 번째 갈래(부재 처리)의 경계는 다른 자리다. 판정을 렌더 층의 문자열 비교에서 포맷 층의 계약으로
옮긴다 — `formatTimeRemaining`이 "표기할 값이 없다"를 반환값으로 알리고, `renderRateLimit`은 그
불리언만 본다. 판정 근거가 표시 문자열에서 시각·시간 계산으로 바뀌므로 표시 형식과의 결합이 끊긴다.

## 2. 데이터 흐름

```
os.Stdin
  → json.NewDecoder(r).Decode(&map[string]json.RawMessage)
      ├─ err ≠ nil  → debugLog("stdin", "parse error: …")  → StdinInput{}  ─┐
      └─ err = nil  → 섹션 테이블 순회                                       │
                        ├─ 키 부재            → 건너뜀 (zero value)          │
                        ├─ Unmarshal 성공     → 필드에 대입                  │
                        └─ Unmarshal 실패     → debugLog(섹션명, err)        │
                                                필드는 zero value 유지       │
                      → 테이블에 없는 키 정렬 후 debugLog 한 줄              │
                      → StdinInput                                          │
  → shouldSuppressOutput(StdinInput) ←──────────────────────────────────────┘
      ├─ true  → debugLog → 아무것도 출력하지 않고 종료
      └─ false → orchestrate(ctx) → 위젯별 GetData/Render → stdout
```

핵심 분기는 최상위 decode 성공 여부 하나뿐이다. 성공하면 섹션 손상은 흐름을 끊지 않고 그 섹션의 값만
비운다. stderr 경로는 세 곳이며 전부 `debugLog`이므로 `DEBUG` 미설정 상태에서는 아무것도 나가지 않는다.
stdout에는 어떤 경우에도 진단 문자가 섞이지 않는다(SPEC §5.7).

섹션을 테이블 순서로 순회하는 것이 중요하다. map을 직접 순회하면 Go의 무작위 순서 때문에 손상 섹션이
둘 이상일 때 stderr 줄 순서가 실행마다 달라진다. 테이블 순회는 그 비결정성을 없앤다.

파싱 층에서 도달 가능한 상태 집합이다.

- **S0 최상위 실패** — 구문 오류, 최상위 비객체, 빈 입력. 결과는 `StdinInput{}`이고 무출력으로 이어진다.
- **S1 손상 0개** — 모든 존재 섹션이 해석됐다. 현재 동작과 동일한 stdout.
- **S2 손상 1개 이상** — 손상 섹션만 zero value. 나머지 섹션에서 나오는 위젯은 그대로 렌더된다.

섹션 하나의 상태는 셋이지만 `StdinInput`에 남는 결과는 둘이다 — "존재·해석 성공"과 "그 밖"(부재이거나
손상)이 zero value로 합쳐진다. 이 합침이 §5 D5의 결론이고, 세 가지 관찰 가능한 결과를 낳는다.

- `cost` 섹션이 깨지면 cost 위젯은 생략되지 않고 `$0.00`을 표시한다. `costWidget.GetData`가 nil을
  반환하는 경로가 없기 때문이다. SPEC §5에 이 조건이 없고 spec §4가 기존 위젯 표기 교정을 제외했으므로
  그대로 둔다.
- `context_window`가 깨지면 context 칸이 생략되는 동시에 `FirstResponseReceived()`가 false가 되어,
  `rate_limits`가 부재인 계정에서는 rate limit 칸이 placeholder(`5h: -`)로 나타난다.
  `20260730-001`이 정의한 기존 동작이 그대로 적용된 결과다.
- `workspace`가 깨지면 `CurrentDir`가 비어 `projectInfo`가 기존 degraded cwd 경로
  (`CLAUDE_PROJECT_DIR` → `os.Getwd`)로 폴백한다. 이것도 기존 설계된 동작이다.

잔여 시간 쪽 흐름은 rate limit 위젯의 `GetData`에서 시작한다. `resets_at`이 0 이하이면 `time.Time{}`을
싣고, 양수이면 `time.Unix(v, 0)`을 싣는다. 렌더는 `formatTimeRemaining`에 그 값을 넘기고, 함수가
"표기할 것 없음"을 알리면 접미사를 붙이지 않는다. 여기서 도달 가능한 상태는 셋이다.

- **R0 시각 미상** — `resets_at` 부재 또는 0 이하. 접미사 없음(SPEC §5.8).
- **R1 잔여 1분 미만** — 과거이거나 60초 미만 미래. 접미사 없음. 현재 출력과 동일하다.
- **R2 잔여 1분 이상** — 접미사 표기. 현재 출력과 동일하다(SPEC §5.10).

## 3. 인터페이스

경계를 가로지르는 계약은 셋이다.

**`StdinInput` (파싱 층 → 무출력 판정·위젯 층).** 변경 없음. 필드 목록·타입·포인터 여부·json 태그가
그대로다. 조립 실패한 섹션이 zero value로 남는다는 것이 새로 생기는 불변식이며, 소비자 입장에서는
"그 섹션이 오지 않았다"와 구별되지 않는다. 이 불변식은 doc 주석으로 `StdinInput` 위에 남긴다.

**`parseStdinReader(r io.Reader) StdinInput`.** 시그니처와 반환 계약을 유지한다. 오류를 반환하지
않으며, 어떤 입력에도 사용 가능한 `StdinInput`을 준다. 최상위 실패 시 `StdinInput{}`을 준다는 기존
계약도 유지된다. 손상 섹션 목록은 이 함수의 반환값에 노출하지 않고, 내부의
`assembleStdin(map[string]json.RawMessage) (StdinInput, []string)`이 돌려준다 — 두 번째 값은 테이블
순서로 정렬된 손상 섹션 키 목록이다(§5 D10). 기존 호출부 세 곳(`main.go`, `stdin_test.go`,
`widgets_core_test.go`)은 수정 없이 동작한다.

**`formatTimeRemaining(resetAt, now time.Time, t *Translations) (string, bool)`.** 두 번째 반환값이
"표기할 잔여 시간이 있다"다. `resetAt`이 Go zero time이거나 계산된 총 분이 0 이하이면 `("", false)`다.
그 밖에는 기존과 같은 문자열과 `true`다. 호출자는 문자열 내용을 판정에 쓰지 않는다. 유일한 호출자가
`renderRateLimit`이고 이 함수 전용 테스트가 없으므로 별도 helper를 새로 만들지 않고 계약 자체를
바꾼다(§5 D7).

`rateLimitData.ResetsAt`의 의미도 계약이다. Go zero time이면 "reset 시각 미상"이다. 이 값은
`GetData`가 세우고 `renderRateLimit`이 읽으며, 1970년이 미상을 뜻하는 일은 더 이상 없다.
`Placeholder`의 극성(zero value = 실측)은 그대로다.

## 4. 영향 범위

- `stdin.go` — `parseStdinReader` 재작성(최상위 map decode + 섹션 조립 위임), 섹션 테이블과
  `assembleStdin` 추가, `StdinInput` doc 주석에 손상 섹션 zero value 불변식 추가. 타입 정의 자체는
  손대지 않는다.
- `format.go` — `formatTimeRemaining`의 시그니처와 과거·부재 분기 변경. 다른 포맷터는 그대로다.
- `widgets_core.go` — `rateLimit5hWidget.GetData`·`rateLimit7dWidget.GetData`에서 `resets_at` 0 이하를
  `time.Time{}`으로 매핑, `renderRateLimit`에서 `"0"+Minutes` 문자열 비교 제거하고
  `formatTimeRemaining`의 불리언으로 대체. `rateLimitData` 필드 구성은 그대로다.
- `stdin_test.go` — 섹션별 손상 케이스 table-driven 테스트, 섹션 테이블이 `StdinInput` 최상위 태그를
  빠짐없이 덮는지 검사하는 reflection 테스트, 최상위 실패·알 수 없는 키·중복 키·후행 쓰레기 케이스 추가.
  기존 `TestStdinFractionalPercentages`와 `TestContextWidgetFractionalPercent`는 정상 payload를 쓰므로
  그대로 통과한다.
- `widget_test.go` — 손상 섹션별 stdout 한 줄을 고정하는 `orchestrate` 수준 테스트 추가
  (`TestOrchestrateSessionStartLine`의 `PATH=""` + 명시 `Lines` 패턴 재사용). 기존 테스트는 영향 없다.
- `widgets_core_test.go` — 잔여 시간 표기 케이스(R0/R1/R2)와 locale 무관성 케이스 추가. 기존
  `TestRateLimitWidgetsGetData`는 `resets_at: 0`을 쓰지만 `Percent`와 `Placeholder`만 단정하므로
  `ResetsAt` 매핑 변경에 깨지지 않는다. `TestRenderRateLimit_Placeholder`도 영향 없다.
- `main.go`, `widget.go`, `widgets_project.go`, `config.go`, `locales/*.json` — 변경 없음.
  `shouldSuppressOutput`은 그대로 유지된다(SPEC §5.11).
- `Makefile` VERSION과 `.claude-plugin/plugin.json` version — `0.5.5`에서 같은 새 값으로 동시
  갱신(SPEC §5.13). `bin/` 재빌드와 release 브랜치 동기화가 따른다.
- `README.md` — §Troubleshooting에 부분 입력 손상 시의 표시 동작 항목 추가(SPEC §5.15).

깨질 것으로 확인된 기존 테스트는 없다. `formatTimeRemaining`은 전용 테스트가 없고 호출자가 한 곳이므로
시그니처 변경이 테스트를 건드리지 않는다.

## 5. Decision Points

**D1. 섹션 분해는 `map[string]json.RawMessage` 최상위 decode + 섹션별 `Unmarshal`로 한다.**
근거는 §근거의 실행 확인이다. 이 방식은 "값이 구문상 유효하지만 타입이 틀림"과 "구문 자체가 깨짐"을
정확히 원하는 자리에서 가른다 — 앞쪽은 최상위 decode가 성공해 섹션 격리가 가능하고, 뒤쪽은 최상위
decode가 실패해 spec §3이 복구 대상에서 뺀 그 경로로 떨어진다. 섹션 내부 구문 오류도 최상위 실패에
포함되는데, 그 입력은 최상위 JSON 자체가 파싱 불가하므로 §3과 어긋나지 않는다.
반려한 대안 둘이다. **오류를 무시하고 부분 struct 결과를 쓰는 방법**은 한 줄 변경이라 가장 유혹적이지만
필드 단위 관용이 되어 spec §3·§4가 금지한다. `rate_limits.five_hour.used_percentage:"high"`에서
`5h: 0%`라는 없는 값이 표시되는 것을 실행으로 확인했다. **최상위 struct decode를 먼저 시도하고 실패 시
섹션 조립으로 폴백하는 2단계**는 정상 입력에서 경로가 하나 줄지도 않으면서 동작 경로를 둘로 늘린다.

**D2. 섹션은 `StdinInput`의 최상위 json 태그 18개 전부이며, 객체와 스칼라를 구분하지 않는다.**
스칼라도 최상위 단위이므로 spec §3의 "관용 단위는 최상위 섹션"에 그대로 들어맞고, 스칼라에는 §3이
경계한 "같은 섹션 일부만 살아 모순된 조합"이 성립하지 않는다. `version:7` 하나가 전면 블랙아웃을 만드는
것을 실행으로 확인했으므로 스칼라를 빼면 실제 실패 사례를 못 막는다. 스칼라 하나가 깨지면 그 필드만
zero value가 된다.

**D3. 섹션은 고정 테이블 순서로 순회한다.** map을 직접 순회하면 손상 섹션이 둘 이상일 때 stderr 줄
순서가 실행마다 달라져 SPEC §5.7을 확인하는 테스트가 흔들린다. 테이블 순회는 존재하는 키만 골라 처리하고
순서를 결정적으로 만든다.

**D4. `StdinInput`의 형태를 유지하고 조립만 바꾼다.** 소비자가 이미 7개 위젯 +
`shouldSuppressOutput` + `FirstResponseReceived`이고, SPEC §5.5·§5.6이 정상 입력의 stdout 무변화를
요구한다. 필드를 포인터화하면 "부재"를 표현할 수 있게 되지만 소비 지점 전부를 nil 검사로 고쳐야 하고,
D5에 따라 그 구별을 쓸 곳이 없다. 대가는 섹션 테이블이 json 태그를 한 번 더 적는 것 — D9의
reflection 테스트로 어긋남을 막는다.

**D5. 깨진 섹션은 zero value로 남기고 손상 상태를 위젯 층에 전파하지 않는다.**
spec §3이 status line 실패 마커를 금지하고 §4가 그것을 제외 범위로 확정했으므로 위젯이 손상 여부로
표시를 바꿀 근거가 없다. SPEC §5의 어떤 조건도 위젯이 이 상태를 읽어야 성립하지 않는다 — §5.1~§5.3은
"살아있는 칸이 나온다"이고 §5.7은 "stdout에 실패 문자가 없다"다. 상태를 `Context`에 실으면 쓰이지 않는
필드가 남고 위젯이 그것을 보기 시작할 여지만 생긴다. 대가는 §2에 적은 세 가지 — 깨진 `cost`가 `$0.00`으로
보이고, 깨진 `context_window`가 rate limit placeholder를 부르고, 깨진 `workspace`가 cwd 폴백을 탄다.
모두 기존 위젯 동작이 그대로 적용된 결과이며 spec §4가 그 교정을 제외했다.

**D6. `resets_at` 부재와 `0`을 구별하지 않고 둘 다 "시각 미상"으로 본다.**
`StdinInput`의 `ResetsAt int64`를 그대로 둔다(D4와 같은 방향). 포인터화하면 두 경우를 구별할 수 있지만
(§근거에서 확인) 구별해서 다르게 표시할 동작이 없다 — SPEC §5.8이 부재와 `0`을 한 조건으로 묶어 같은
결과를 요구한다. 대신 구별이 필요 없다는 사실을 코드에서 드러나게 하려고, 위젯 `GetData`가 0 이하를
Go zero time으로 매핑해 `rateLimitData.ResetsAt`의 zero time이 곧 "미상"이 되게 한다. `time.Unix(0,0)`이
zero time이 아니라는 것이 원래 문제의 뿌리이므로, 그 값이 `rateLimitData`까지 들어오지 못하게 조립
지점에서 막는 것이 맞다. 이렇게 하면 `!d.ResetsAt.IsZero()` 가드가 처음으로 실제 게이트가 된다.

**D7. `formatTimeRemaining`의 계약을 `(string, bool)`로 바꾸고, 판정 기준은 총 분이 0 이하인지로 한다.**
호출자가 한 곳이고 전용 테스트가 없어 계약 변경 비용이 가장 낮다. 표시 문자열을 제어 신호로 쓰는 결합을
없애는 것이 spec §2의 요구이며, 판정이 시각·시간 계산에서 나오므로 표시 형식과 무관해진다.
반려한 대안 둘이다. **문자열 반환을 유지하고 호출 측에서 시각을 직접 비교하는 방법**은 잔여 시간 판정
기준이 두 함수에 나뉘어 다시 어긋날 수 있다. **`remainingTimeLabel` helper를 새로 얹는 방법**은
`formatTimeRemaining`의 죽은 과거 분기를 그대로 남긴다.
판정 기준을 `diff > 0`이 아니라 `int(diff.Minutes()) > 0`으로 두는 것은 60초 미만 미래 구간에서 현재
stdout을 그대로 보존하기 위한 선택이다(사용자가 확정). 현재는 그 구간도 `"0"+Minutes`를 만들어 억제되므로
(실행 확인), `diff > 0`으로 바꾸면 rate limit 초기화 직전 1분 동안 `(0분)`이 새로 나타나 SPEC §5.10을
문자 그대로는 어긴다.
SPEC §5.9(locale 무관성)는 §근거에 적은 대로 이미 성립하는 성질이다. 이 계약 변경은 그 성질을 문자열
동일성이 아니라 시각 계산으로 보장하게 만들므로, §5.9는 새 동작이 아니라 구조가 바뀐 뒤에도 성질이
유지되는지 확인하는 회귀 pin으로 기능한다.

**D8. 테이블에 없는 최상위 키는 무시하되 정렬해 stderr에 한 줄로 남긴다.**
spec §3이 알 수 없는 필드 무시를 요구하므로 동작에는 영향이 없고, 기록은 stderr 전용이라 SPEC §5.5의
stdout 동일성을 건드리지 않는다. 남기는 이유는 M3(스키마 정합)이 "어떤 필드가 실제로 오는가"를 필요로
하는데 지금은 그 정보가 어디에도 남지 않기 때문이다. 정렬하는 이유는 D3과 같다 — map 순회 순서가
무작위여서 정렬 없이는 출력이 실행마다 달라진다. 되돌리기 쉬운 세부이고 §4 제외 범위(새 필드의
수용·노출)를 넘지 않는다 — 기록만 하고 값을 쓰지 않는다.

**D9. 섹션 테이블의 완전성은 reflection 테스트로 고정한다.**
D4가 태그를 두 곳(struct 태그, 섹션 테이블)에 적는 대가를 받아들였으므로, `StdinInput`에 필드를 더하고
테이블에 빠뜨리는 드리프트를 막는 장치가 필요하다. `reflect`로 최상위 필드의 json 태그를 훑어 테이블
키 집합과 비교한다. M3이 새 필드를 추가할 예정이라 이 장치는 곧 실제로 쓰인다.

**D10. `parseStdinReader`의 시그니처를 유지하고 손상 목록은 내부 `assembleStdin`이 돌려준다.**
`parseStdinReader`에 손상 목록을 더하면 호출부 세 곳을 다 고쳐야 하고, 그 값을 쓰는 곳은 stderr 기록
하나뿐이다. 조립을 별도 함수로 떼면 손상 목록을 반환값으로 단정할 수 있어 테스트가
`os.Stderr`를 만지지 않고도 "어느 섹션이 버려졌는가"를 확인한다.

**D11. 테스트는 세 계층으로 닫고 바이너리 실행 테스트는 쓰지 않는다.**
SPEC §5의 여러 조건이 "stdout에 무엇이 나온다"로 서술돼 있으므로 stdout 문자열을 고정하는 층이 반드시
필요하다. 그 층은 `orchestrate` + `stripANSI`로 충분하다 — `main`이 `orchestrate` 결과를 그대로
`fmt.Print`하므로 둘 사이에 변환이 없다. 계층 배분은 이렇게 둔다.

- 파싱 층 table-driven(`stdin_test.go`) — 섹션별 손상, 최상위 실패 형태 4종(구문 오류, 비객체, `null`,
  빈 입력), 알 수 없는 키, 중복 키, 후행 쓰레기. `assembleStdin`의 손상 목록과 `StdinInput` 필드 값을
  단정한다. SPEC §5.4·§5.7의 stderr 절반을 여기서 닫는다.
- orchestrate 층(`widget_test.go`) — `parseStdinReader`로 만든 `StdinInput`에서 stdout 한 줄을 고정한다.
  SPEC §5.1~§5.3, §5.5~§5.7의 stdout 절반, §5.11을 닫는다. git 하위 프로세스가 결과에 끼어들지 않게
  `PATH=""`와 명시 `Lines`를 쓰는 기존 패턴을 따른다.
- 렌더 층(`widgets_core_test.go`) — R0/R1/R2 세 상태와, `Time.Minutes`를 임의 값으로 채운
  `&Translations{}`로 판정의 locale 무관성을 단정한다. SPEC §5.8~§5.10을 닫는다.

바이너리를 exec하는 통합 테스트는 쓰지 않는다. `20260730-002`가 붙인 3-OS CI에서 빌드 산출물 경로와
ldflags에 의존하는 테스트는 이식성 위험이 크고, `orchestrate` 층이 stdout을 이미 같은 정확도로 고정한다.
`debugLog`가 실제로 stderr에 쓰는지는 이미 있는 `captureStderr` helper로 한 케이스만 확인한다.
