# Context

## 현재 목표

M3 `20260727-001-statusline-schema-catchup`의 IMPLEMENT를 끝내 `ROADMAP.md` 서비스 완료 기준 5번(공식
status line 문서가 정의한 필드 의미와 cc-usage의 계산·표기 일치)을 충족한다.

## 현재 상태

IMPLEMENT 12개 Task 중 **9개 완료**(task-001~009), 전부 verify approved 후 Task별로 main에 commit·push됐다.
충족된 완료 조건은 spec.md §5.1·§5.3·§5.4·§5.5·§5.6·§5.7이다.

`/implement-loop`이 **task-010에서 정지 조건 3(자동 진행 제외)으로 멈췄다.** 이 Task가 고치는 것은
`skills/cc-usage-install/SKILL.md`의 영어 산문이고, 그 `확인` 필드의 `make test`·`go vet`은 SKILL.md 내용과
무관하게 통과하므로 Task의 실질을 검증하지 못한다. 나머지 확인 수단은 문서를 읽고 판단하는 수동 확인뿐이라,
루프가 자기 산문을 자기 눈으로 승인하는 구간이 된다.

버전은 아직 0.5.6이다 — 0.6.0 bump는 task-012가 소유하며, `bin/` 재빌드와 `release` 브랜치 동기화는 그 뒤의
배포 단계다.

## 현재 작업 문서

[docs/20260727-001-statusline-schema-catchup/implement.md](./docs/20260727-001-statusline-schema-catchup/implement.md)
— `task-010: 설치 절차가 실행 환경의 설정 홈을 따르게 교정`이 다음 대상이다. task-011·task-012는 미착수.

## 확정된 결정

- **spec.md §5.2의 bool 표시 정책은 필드별로 갈린다**(analysis §5 D3). `fast_mode`는 참일 때만 라벨만
  렌더하고, `thinking`은 키가 있으면 항상 `<라벨>: on|off`로 렌더한다. 근거는 공식 문서 재조회 결과 —
  두 필드는 조건부 부재 목록에 없어 항상 존재하고 기본 상태가 서로 반대다(fast mode는 `/fast` opt-in,
  extended thinking은 기본 켜짐이며 Fable 5는 끌 수도 없다).
- **context 퍼센트·토큰 표시를 모두 input 계열로 맞춘다**(§5 D1). 이전 턴 output은 이미
  `total_input_tokens`에 cache_read로 편입돼 있어 현재 턴 output을 더하면 이중 계산이 된다.
- **`COLUMNS` 축소는 줄 오른쪽부터**이고, 하나만 남아도 넘치면 표시폭 기준 절단 + RESET이다(§5 D7).
- **`ROADMAP.md` 서비스 완료 기준 6번을 좁혀 git 유래 캐시를 허용했다**(§5 D6). 이전 문구는 "캐시 파일을
  읽거나 쓰지 않으며"였고 M3의 git 캐시와 정면 충돌했다. 그 기준이 실제로 막으려던 것(계정 유래 값이 프로필
  간에 남는 경로, 렌더 중 네트워크 접속)은 좁힌 문구에 그대로 남아 있다.
- **커밋 단위는 Task별**이다. 버전 bump 없이 코드 이력을 Task 단위로 main에 쌓고, 사용자에게 도달하는
  버전 갱신은 task-012에서 한 번에 한다.
- 보류·제외 범위(stdin 밖 데이터 출처, 프로필 간 설정 승계, `prompt_id`, 컨텍스트 토큰 임계값 재조정)는
  `ROADMAP.md` §보류·제외 범위가 소유한다.

## 미확정 판단

- **task-010을 어떻게 진행할지.** 두 선택지를 올렸고 답을 받지 않았다 — ① SKILL.md를 고치고 바뀐 산문을
  사용자가 직접 확인해 승인, ② `확인` 필드를 실행 가능하게 바꾼다(implement.md 판정 기준 변경이라
  `/implement-loop` 금지 사항이고 사용자 결정이 필요하다). 권고는 ①이다. 대상 Task 원문은
  [implement.md](./docs/20260727-001-statusline-schema-catchup/implement.md)의 task-010에 있다.
- **verify가 approve하면서 남긴 잔여 위험 3건을 별도 Task나 후속 feature로 다룰지.** 어느 완료 조건도
  다루지 않는 항목이며 판단을 올렸으나 답을 받지 않았다.
  - task-008: 쓰기 경로의 `sweepBranchCache` 호출이 테스트로 고정되지 않아 그 배선 한 줄을 지워도 CI가
    통과한다. 과거 "파일 무한 누적" 결함의 재발 방지 장치로는 한 칸 약하다.
  - task-008: `gitBranch` 실패 시의 빈 문자열도 캐시되어, 일시적 실패 직후 최대 TTL(5초)까지 브랜치 칸이
    비어 보인다. 캐시 이전에는 매 실행이 재시도했다.
  - task-009: 좁은 터미널에서 pullRequest 위젯만 남아 잘리면 OSC 8 하이퍼링크 종료자가 사라진다.
    `RESET`은 SGR만 닫으므로 터미널 상태가 status line 밖으로 샌다. D7·§5.7이 "색"에만 한정해 명시
    위반은 아니다.
- **M4 subagent status line을 실제로 진행할지.** `ROADMAP.md` M4에 마일스톤으로 적혀 있으나 진행을 확정한
  적은 없다. M3가 닫히면 바로 갈리는 지점이다.

## 다음 작업

- 작업: task-010을 진행한다. `skills/cc-usage-install/SKILL.md`에 "사용자 설정 홈" 정의
  (`CLAUDE_CONFIG_DIR`이 공백 아닌 값이면 그 디렉토리, 아니면 `~/.claude`)를 두고 사용자 스코프 후보와
  신규 생성 기본값이 그 정의를 참조하게 고친 뒤, 바뀐 산문을 사용자 확인에 올린다. 프로젝트 스코프 후보와
  영어 산문은 유지한다. 진행 방식은 위 미확정 판단의 답을 먼저 받는다.
- 완료 기준: 교정된 산문의 해석 규칙이 `main.go`의 `configHomeDir`과 `main_test.go`의 `TestConfigHomeDir`
  케이스(환경변수 우선·공백 처리·폴백)와 어긋나지 않고, 환경변수 설정/미설정 두 환경에서 고르는 후보가
  각각 달라지며, 사용자 승인 후 implement.md의 task-010이 `[x]`가 된다.

## 먼저 읽을 문서

- [docs/20260727-001-statusline-schema-catchup/implement.md](./docs/20260727-001-statusline-schema-catchup/implement.md)
  — task-010 원문과 남은 task-011·task-012
- [docs/20260727-001-statusline-schema-catchup/analysis.md](./docs/20260727-001-statusline-schema-catchup/analysis.md)
  — §5 D10(설치 스킬의 설정 홈 인식 표현), §5 D11(문서 갱신 범위와 버전 bump)
- [docs/20260727-001-statusline-schema-catchup/spec.md](./docs/20260727-001-statusline-schema-catchup/spec.md)
  — §5.9~§5.12
- [ROADMAP.md](./ROADMAP.md) — 서비스 완료 기준 5번·6번, M3 전환 기준, M4

## 문서 반영 필요

- `README.md`의 배너 예시(6행)에 `30% 60K`가 남아 있다. task-001의 input 계열 교정으로 실제 출력은
  `25% 50K`이므로 어긋난 상태다. analysis.md §4·§5 D11의 `README.md` 갱신 목록은 위젯 표·`fastMode` 표기·
  Privacy 세 항목만 들고 있어 이 줄이 빠져 있다 — task-011에서 함께 고치기로 했으나 문서에는 반영되지
  않았다.
