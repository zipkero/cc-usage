# cc-usage — 세션 상태 캐싱 + 누적 표시

각 Task는 "무엇이 동작하면 완료인가"를 정의한다. 구현 방법/순서를 나열하지 않는다.

> **전역 규칙:** Decision Point가 남은 Phase는 사용자 승인 없이 구현 Task로 넘어가지 않는다.

---

## Phase 1: 열화 감지 및 캐시 기반 복원

이전 Phase Exit Criteria: 없음 (첫 Phase).

Claude Code가 간헐적으로 빈/리셋 JSON을 보내면 status line이 `$0.00`만 표시되거나 완전히 사라진다.
이 Phase가 끝나면, 열화된 JSON이 들어와도 마지막 유효 출력이 유지된다.

---

### ─── 한 호흡: 세션 상태 영속화 ───

- [x] **T1.1** 정상 실행 후 `~/.cache/cc-usage/session-state.json`에 현재 세션 값이 저장된다
  - 목적: 프로세스 간 상태 공유의 기반. 이 파일이 없으면 이후 모든 캐시 기반 동작이 불가능
  - 입력: 유효한 stdin JSON (model.id 존재, 위젯 2개 이상 렌더)
  - 산출물: `session-state.json` 파일, 세션 상태 읽기/쓰기 함수
  - Exit Criteria: 정상 JSON 입력 후 `~/.cache/cc-usage/session-state.json`이 존재하고, cost/token 값이 stdin 입력과 일치

- [x] **T1.2** 캐시 파일이 없거나 손상되었을 때 기존 동작과 동일하게 출력된다
  - 목적: 캐시 도입이 기존 기능을 깨뜨리지 않음을 보장
  - 입력: 캐시 파일 없음 + 유효한 stdin JSON / 캐시 파일 손상 + 유효한 stdin JSON
  - 산출물: 기존과 동일한 stdout 출력
  - Exit Criteria: 캐시 파일 삭제 후 실행해도 에러/패닉 없이 정상 출력. 캐시 파일에 `{invalid`를 써놓고 실행해도 동일

---

### ─── 한 호흡: 열화 감지 및 복원 ───

- [x] **T1.3** model.id가 비어있고 context_window_size가 0인 stdin이 들어오면 이전 유효 출력이 반환된다
  - 목적: Claude Code가 빈 JSON을 보내는 순간에도 사용자에게 의미 있는 status line이 보임
  - 입력: 캐시에 유효한 last_output 존재 + 열화된 stdin JSON (`{"cost":{"total_cost_usd":0}}` 등)
  - 산출물: stdout에 캐시된 이전 출력
  - Exit Criteria: 정상 JSON → 열화 JSON 순서로 두 번 실행 시, 두 번째 실행의 stdout이 첫 번째와 동일

- [x] **T1.4** 열화 감지 시 캐시 파일이 갱신되지 않는다
  - 목적: 열화된 값으로 캐시를 오염시키면 복원 자체가 불가능해짐
  - 입력: 캐시에 유효 상태 존재 + 열화 stdin
  - 산출물: session-state.json의 내용이 실행 전후로 동일
  - Exit Criteria: 열화 JSON 실행 전후 `session-state.json`의 md5/내용 비교 시 변경 없음

**Phase 1 Exit Criteria:** 정상 → 열화 → 정상 순서로 3회 실행 시, 열화 시점에 이전 출력이 유지되고, 이후 정상 입력이 다시 정상 반영된다.

---

## Phase 2: 세션 간 누적 표시

이전 Phase Exit Criteria 회귀 체크: Phase 1 Exit Criteria 재검증.

세션을 재시작("다시 시작하기")하면 Claude Code가 cost/token을 0부터 보낸다.
이 Phase가 끝나면, 이전 세션들의 누적 cost/token이 현재 세션 값에 합산되어 표시된다.

---

### Decision Point: 누적 대상 필드 — **결정 완료**

| 필드 | 누적? | 이유 |
|------|-------|------|
| `total_cost_usd` | **O** | 세션 간 총 비용 추적이 핵심 용도 |
| `total_input_tokens` + `total_output_tokens` | **X** | context bar는 현재 세션의 윈도우 사용률을 보여주는 것. 열화 시 이전 유효 값 유지(Phase 1)로 충분 |
| `context_window.used_percentage` | **X** | 동일 |
| `rate_limits` | **X** | API 실시간 조회 값, 누적 의미 없음 |

---

### ─── 한 호흡: 세션 리셋 감지 ───

- [x] **T2.1** 현재 cost가 캐시의 last_cost보다 작고 model.id가 존재하면 세션 리셋으로 판정된다
  - 목적: "세션 재시작"과 "열화 JSON"을 구분하는 기준. 열화는 model.id가 비고, 리셋은 model.id가 있지만 값이 줄어듦
  - 입력: 캐시에 last_cost=1.25 + stdin cost=0.00, model.id="claude-opus-4-6"
  - 산출물: 세션 리셋 감지 로직, accumulated 값 갱신
  - Exit Criteria: 위 입력 시 accumulated_cost가 이전 last_cost만큼 증가하고, last_cost가 0.00으로 갱신됨

- [x] **T2.2** 세션 리셋 후 출력되는 cost가 이전 세션 누적분을 포함한다
  - 목적: 사용자가 세션 재시작 후에도 총 비용을 확인 가능
  - 입력: 캐시에 accumulated_cost=1.25, last_cost=0.42 + stdin cost=0.10
  - 산출물: stdout의 cost가 $1.77 (리셋 시 1.25+0.42=1.67 누적 후 +0.10)
  - Exit Criteria: 위 시나리오에서 cost 위젯이 `$1.77`을 표시

---

- [x] **T2.3** 연속 세션 리셋이 발생해도 누적이 정확하다
  - 목적: 3회 이상 재시작 시에도 누적 로직이 깨지지 않음을 보장
  - 입력: 세션A(cost 1.00 도달) → 리셋 → 세션B(cost 0.50 도달) → 리셋 → 세션C(cost 0.20)
  - 산출물: 세션C에서 표시되는 cost가 $1.70
  - Exit Criteria: 3회 순차 실행 시 최종 cost가 이전 세션들의 합 + 현재 값

- [x] **T2.4** 누적 리셋 수단이 존재한다
  - 목적: 하루가 지나거나 사용자가 원할 때 누적을 초기화할 수 있어야 함
  - 입력: 사용자가 캐시 파일을 삭제하거나 특정 동작 수행
  - 산출물: 누적값이 0으로 초기화된 상태
  - Exit Criteria: `session-state.json` 삭제 후 실행 시 누적 없이 현재 세션 값만 표시

**Phase 2 Exit Criteria:** 세션A(cost $1.00) → 리셋 → 세션B(cost $0.50) → 열화 JSON → 세션B 정상(cost $0.60) 시나리오에서: 세션B 정상 시점에 cost가 $1.60으로 표시되고, 열화 시점에는 직전 유효 출력이 유지된다. context/tokens는 현재 세션 값 그대로 표시.
