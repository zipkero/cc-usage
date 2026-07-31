# statusline-schema-catchup — IMPLEMENT

## Section: 표시 의미 교정

- [x] task-001: 컨텍스트 퍼센트·토큰 표시를 input 계열로 정합
  - 목적: `used_percentage`가 빠진 입력에서도 컨텍스트 퍼센트가 그 값을 채워 넣은 입력과 같게
    나오고, 옆에 붙는 토큰 수가 그 퍼센트와 같은 기준으로 계산된다.
  - 접근: 실측 갈래의 분자를 `total_input_tokens` 우선, 그것이 0일 때만 `current_usage`의
    input·cache_creation·cache_read 합으로 바꾸고 표시 토큰 수도 같은 분자를 쓴다. placeholder
    갈래와 임계 상수 값은 그대로 두고, 임계 발화가 늦어진다는 사실만 상수 옆 주석으로 남긴다.
  - 검증 조건:
    - 결과: 공식 문서 예시 payload에서 출력이 `30% 60K`에서 `25% 50K`로 바뀌고, 같은 payload에
      `used_percentage: 25.0`을 주입한 출력과 퍼센트가 일치한다. 첫 응답 전 placeholder 칸의
      표시는 변하지 않는다.
    - 확인: payload를 `GetData`로 통과시키는 기존 케이스(`stdin_test.go`의
      `TestContextWidgetFractionalPercent`, `widgets_core_test.go`의 `TestContextWidgetGetData`)와
      orchestrate 줄 비교 케이스의 기대값을 input 기준으로 갱신하고, placeholder 케이스
      (`TestContextWidgetRender_Placeholder`, `TestOrchestrateSessionStartLine`)는 수정 없이
      통과한다. `used_percentage` 유무 두 payload를 직접 실행해 퍼센트가 같은지 대조한다.
      `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.1, §5.12 / ANALYSIS §2, §5 D1

- [x] task-002: 모델 기호 매핑을 표로 바꾸고 fable·mythos 계열 추가
  - 목적: fable·mythos 모델 ID로 실행하면 model 위젯이 기본 기호가 아니라 그 계열의 기호를
    내고, 기존 세 계열의 기호와 표시 이름은 달라지지 않는다.
  - 접근: 부분 문자열과 기호를 순서대로 담은 상수 표 + 기본 기호 하나로 판정을 바꾸고
    fable·mythos 항목을 더한다. 소비처가 없는 locale `model` 블록과 대응 번역 필드는 함께
    걷어낸다.
  - 검증 조건:
    - 결과: `claude-fable-5`·`claude-mythos-5`가 각각 고유 기호를 내고, opus·sonnet·haiku는
      기존 기호를 유지하며, 어느 항목에도 걸리지 않는 ID만 기본 기호로 떨어진다. locale 블록
      제거 후에도 stdout은 제거 전과 완전히 같다.
    - 확인: ID→기호 케이스 표 테스트를 다섯 계열 + 미지 ID로 추가하고, 표시 이름이 ID를
      우선하는 기존 동작을 같은 테스트에서 확인한다. locale 블록 제거 전후로 기본 preset 출력을
      직접 실행해 문자 단위로 대조한다. `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.6, §5.12 / ANALYSIS §5 D9

## Section: 새 stdin 필드 노출

- [x] task-003: fastMode 위젯 추가
  - 목적: 위젯을 켠 사용자가 fast mode가 켜져 있는 동안에만 그 사실을 알리는 칸을 보고, 꺼져
    있거나 해당 키가 없는 입력에서는 그 칸이 나타나지 않는다.
  - 접근: `fast_mode`를 부재와 `false`가 구분되는 포인터로 수용하고 새 최상위 키를 섹션 표에
    등재한 뒤, 참일 때만 locale 라벨 하나만 렌더하는 위젯을 등록하고 preset 문자 `f`를
    배정한다. rate limit cooldown 자동 강하를 알 수 없다는 한계는 주석으로 남긴다.
  - 검증 조건:
    - 결과: `"fast_mode": true`에서 라벨 칸이 출력되고, `false`와 키 부재 두 경우 모두 위젯이
      생략된다. 설정 파일이 없는 환경의 기본 위젯 구성은 v0.5.6과 같다.
    - 확인: `TestStdinSectionTableCompleteness`가 통과한다(새 최상위 키를 섹션 표에 등재하지
      않으면 이 테스트가 실패한다). true·false·부재 세 케이스 위젯 테스트를 추가한다.
      `TestRemovedPresetCharsAreUnmapped`와 compact 배열 비교 케이스가 수정 없이 통과한다.
      `f`를 preset에 넣은 실행과 넣지 않은 실행을 직접 대조한다. `make test`와 `go vet ./...`가
      실패 0으로 통과한다.
  - 참조: SPEC §5.2, §5.3, §5.12 / ANALYSIS §5 D2, D3

- [x] task-004: thinking·effort 위젯 추가
  - 목적: thinking 위젯을 켜면 확장 사고가 켜짐인지 꺼짐인지가 항상 보이고, effort 위젯은 모델이
    effort 수준을 넘길 때만 그 수준을 보여준다.
  - 접근: `thinking`·`effort`를 포인터로 수용하고 두 최상위 키를 섹션 표에 등재한 뒤,
    `<라벨>: on|off`와 `<라벨>: <level>` 형태의 위젯 두 개를 등록하고 preset 문자 `T`·`E`를
    배정한다. 라벨 색은 rate limit 실측 갈래와 같은 결로 두고 상태별 의미 색을 새로 만들지
    않는다.
  - 검증 조건:
    - 결과: `thinking.enabled`가 `true`든 `false`든 위젯이 렌더되어 두 상태가 구별되고,
      `thinking` 키가 없으면 생략된다. `effort.level`에 값이 있으면 그 값이 그대로 나오고, 키가
      없거나 값이 비면 생략된다. on/off와 level 값은 번역하지 않는다.
    - 확인: `TestStdinSectionTableCompleteness`가 통과한다. 두 위젯 각각에 (키 부재 / false 또는
      빈 값 / 값 있음) 케이스 테스트를 추가한다. 추가한 라벨 키가 en·ko 양쪽 locale에 모두
      있는지 확인한다. `T`·`E`를 preset에 넣은 실행에서 두 칸이 나오고 기본 preset 출력은
      v0.5.6과 같음을 직접 대조한다. `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.2, §5.3, §5.12 / ANALYSIS §5 D2, D3

- [x] task-005: project 계열 위젯에 worktree 토큰 반영
  - 목적: git worktree 정보를 담은 입력에서 project 계열 위젯 두 개가 지금 작업 중인 worktree를
    함께 보여주고, 그 정보가 없는 입력에서는 지금과 똑같이 출력된다.
  - 접근: `workspace.git_worktree`를 빈 문자열이 곧 부재인 값으로 수용하고(기존 `workspace`
    섹션 안이라 섹션 표 변경은 없다), 문자열의 마지막 경로 요소만 취해 토큰을 붙이는 렌더
    조립을 두 위젯이 공유하게 한다. 레거시 `worktree` 구조체와 branch 출처는 건드리지 않는다.
  - 검증 조건:
    - 결과: 해당 키가 있으면 두 위젯 모두 worktree 토큰을 포함하고, 경로가 와도 이름이 와도 같은
      표시를 낸다. 키가 없으면 두 위젯의 출력이 변경 전과 문자 단위로 같다.
    - 확인: 키 있음(경로 형태·이름 형태 각각) / 키 없음 케이스를 두 위젯에 대해 추가하고,
      `TestProjectInfoOmitsRemovedStatusTokens`·`TestProjectNameWidget`·
      `TestProjectNameGetDataSkipsWhenCwdUnknown`이 수정 없이 통과한다. 기본 preset 구성이
      바뀌지 않았음을 직접 실행해 확인한다. `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.4, §5.3, §5.12 / ANALYSIS §5 D4

- [x] task-006: repoInfo 위젯 추가
  - 목적: 저장소 좌표를 담은 입력에서 위젯을 켠 사용자가 `owner/name` 형태로 어떤 저장소인지
    보고, 그 정보가 없으면 칸이 나타나지 않는다.
  - 접근: `workspace.repo`를 부재와 값이 구분되는 포인터로 수용하고, owner와 name만 조합해
    렌더하는 위젯을 등록하고 preset 문자 `G`를 배정한다. host는 표시하지 않는다.
  - 검증 조건:
    - 결과: 키가 있으면 `owner/name`이 출력되고, 키가 없거나 owner·name이 비면 위젯이 생략된다.
      기본 위젯 구성은 v0.5.6과 같다.
    - 확인: (키 부재 / 값 완전 / owner 또는 name 결손) 케이스 테스트를 추가하고, 기존
      `workspace` 섹션 격리 테스트(`TestAssembleStdinSectionIsolation`)가 통과한다. `G`를 preset에
      넣은 실행으로 표시를 확인한다. `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.5, §5.3, §5.12 / ANALYSIS §5 D5

- [x] task-007: pullRequest 위젯 추가
  - 목적: 열린 PR 정보를 담은 입력에서 위젯을 켠 사용자가 PR 번호와 리뷰 상태를 짧은 형태로
    보고, URL이 있으면 그 텍스트를 터미널에서 클릭해 열 수 있으며, PR 정보가 없으면 칸이 없다.
  - 접근: `pr`을 포인터로 수용하고 새 최상위 키를 섹션 표에 등재한 뒤, 번호가 양수일 때
    `#<번호>`를 내고 URL이 있으면 OSC 8 링크로 감싸며 리뷰 상태를 기호로 접는 위젯을 등록하고
    preset 문자 `#`을 배정한다. 문서에 없는 리뷰 상태 값은 기호를 붙이지 않고 진단만 남긴다.
  - 검증 조건:
    - 결과: 번호·URL·리뷰 상태가 모두 있는 입력에서 링크가 걸린 `#<번호>`와 상태 기호가
      출력되고, URL이 없으면 링크 없이 번호만 나오며, `pr` 키가 없거나 번호가 0 이하면 위젯이
      생략된다. 미확인 리뷰 상태 값이 stdout에 원문으로 새지 않고 stderr 진단으로만 남는다.
      `pr`과 저장소 좌표가 모두 없는 입력에서는 두 위젯 모두 사라져 관련 출력이 전혀 없다.
    - 확인: `TestStdinSectionTableCompleteness`가 통과한다. (키 부재 / 번호만 / 번호+URL /
      리뷰 상태 열거값 각각 / 미확인 상태 값) 케이스 테스트를 추가하고, 미확인 값 케이스는
      stdout에 그 문자열이 없고 stderr에만 나타남을 확인한다. `#`을 preset에 넣은 실행으로
      링크 시퀀스를 확인한다. `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.5, §5.3, §5.12 / ANALYSIS §5 D5

## Section: 실행 환경 반영

- [x] task-008: branch 조회 결과 캐시 도입
  - 목적: 같은 세션에서 status line이 연달아 갱신될 때 두 번째 이후 실행은 git 하위 프로세스를
    띄우지 않고 같은 브랜치를 보여주며, 캐시가 없거나 깨져도 표시는 달라지지 않는다.
  - 접근: 원시 `gitBranch`는 그대로 두고 ANALYSIS §5 D6이 정한 캐시 계층(session_id+디렉토리 키,
    고정 TTL, rename 교체, lock 없음, 쓰기 시 나이 기준 청소)을 경유 래퍼로 두어 project 위젯
    둘이 그것을 부르게 한다. 캐시 루트는 테스트가 교체할 수 있는 값으로 둔다.
  - 검증 조건:
    - 결과: 같은 session_id로 연속 실행하면 브랜치 조회가 한 번만 일어나고 두 실행의 표시가
      같다. 캐시 산출물에 `cost`·`rate_limits` 유래 값, 위젯 렌더 결과, stdin 원문이 들어 있지
      않다. 캐시 파일을 지우거나 내용을 깨뜨리거나 TTL을 넘기면 다시 git을 실행해 같은 값을
      내고, 캐시 루트를 얻지 못하면 매번 git을 실행한다. session_id가 빈 입력에서는 캐시 파일이
      만들어지지 않는다. lock 파일이 남는 경로가 없다.
    - 확인: 원시 조회 함수를 패키지 레벨 변수 교체로 대신해 호출 횟수를 세는 테스트를 추가하고,
      (첫 실행 / 같은 키 재실행 / 다른 디렉토리 / TTL 초과 / 파일 손상 / session_id 없음) 케이스를
      덮는다. 캐시 루트를 `t.TempDir()`로 교체해 3-OS CI에서 같게 돌게 하고, 기록된 파일 내용을
      읽어 계정 유래 키가 없음을 확인하며, 정리 후 남은 파일 목록으로 나이 기준 청소를 확인한다.
      `t.Setenv("PATH", "")`로 git을 막는 기존 project 위젯 케이스가 수정 없이 통과한다.
      `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.8, §5.12 / ANALYSIS §1, §2, §5 D6

- [x] task-009: COLUMNS 기준 줄 맞춤과 표시폭 계산 도입
  - 목적: 좁은 터미널에서 status line이 터미널 폭을 넘지 않아 줄바꿈으로 화면이 밀리지 않고,
    폭 제약이 없는 환경에서는 출력이 지금과 완전히 같다.
  - 접근: `COLUMNS`를 정수로 읽어 양수일 때만 실행 컨텍스트에 싣고, ANALYSIS §5 D7·D8이 정한
    대로 조인된 줄을 오른쪽부터 덜어내고 마지막에 표시폭 기준으로 절단하는 줄 맞춤과, ANSI·OSC 8을
    0으로 걸러 rune별 표시폭을 재는 계층을 둔다. 위젯 계약·`barWidth`·경로 축약 예산은 불변이다.
  - 검증 조건:
    - 결과: `COLUMNS`가 설정된 환경에서 stdout 각 줄의 표시폭이 그 값을 넘지 않고, 절단된 줄은
      색이 열린 채 끝나지 않는다. `COLUMNS` 미설정·비수치·0 이하에서는 출력이 v0.5.6과 문자
      단위로 같다. 명시한 `barWidth` 설정이 폭 제약 때문에 덮이지 않는다.
    - 확인: 표시폭 계층에 (ANSI 색 코드 / OSC 8 링크 / 한글·CJK 2칸 / 현재 렌더에 쓰이는
      `◆ ◇ ○ ● █ ░ │ ↑ ↓ …` 1칸) 케이스 표 테스트를 추가하고, 줄 맞춤에 (여유 있음 / 오른쪽
      한 개 덜어냄 / 여러 개 덜어냄 / 하나만 남아 절단) 케이스를 추가해 결과 폭과 말미 리셋
      코드를 확인한다. 한글 경로·브랜치를 포함한 케이스를 넣어 3-OS CI에서 같은 결과를 확인한다.
      `COLUMNS`를 비운 실행과 v0.5.6 기준선 출력을 직접 대조한다. `make test`와 `go vet ./...`가
      실패 0으로 통과한다.
  - 참조: SPEC §5.7, §5.3, §5.12 / ANALYSIS §2, §5 D7, D8

- [ ] task-010: 설치 절차가 실행 환경의 설정 홈을 따르게 교정
  - 목적: 설정 디렉토리를 옮겨 쓰는 사용자가 설치 절차를 그대로 따라도 실제로 읽히는 settings
    파일이 수정되어 status line이 뜨고, 옮기지 않은 사용자는 기존과 같은 파일을 고르게 된다.
  - 접근: 설치 스킬 산문 앞에 "사용자 설정 홈" 정의(`CLAUDE_CONFIG_DIR`이 공백 아닌 값이면 그
    디렉토리, 아니면 `~/.claude`)를 두고 사용자 스코프 후보와 신규 생성 기본값이 그 정의를
    참조하게 한다. 프로젝트 스코프 후보와 영어 산문은 유지한다.
  - 검증 조건:
    - 결과: 환경변수가 설정된 환경에서 사용자 스코프 후보와 새로 만드는 파일이 모두 그 디렉토리
      아래를 가리키고, 미설정 환경에서는 `~/.claude` 아래를 가리킨다. 공백만 있는 값은 미설정과
      같게 해석된다. 스킬 출력이 고른 후보 경로와 설정 홈이 환경변수에서 왔는지를 밝힌다.
    - 확인: 교정된 산문의 해석 규칙을 `main.go`의 `configHomeDir`과 `main_test.go`의
      `TestConfigHomeDir` 케이스(환경변수 우선·공백 처리·폴백)와 한 줄씩 대조한다. 환경변수를
      설정한 상태와 비운 상태로 절차를 따라가 고르는 후보가 각각 달라지는지 직접 확인한다.
      `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.9, §5.12 / ANALYSIS §5 D10

## Section: 문서·버전 정합

- [ ] task-011: 개발자·사용자 문서를 실제 동작에 맞춤
  - 목적: 문서에 적힌 검증 명령과 동작 확인 예시를 그대로 실행하면 문서가 말한 결과가 나오고,
    사용자 문서의 위젯 목록·preset 문자·저장 동작 서술이 실제 프로그램과 일치한다.
  - 접근: `CLAUDE.md`의 없는 테스트 이름을 실재하는 이름으로 바꾸고 동작 확인 예시 기대 출력을
    실행 결과로 맞추며, 아키텍처 서술에서 ahead/behind를 걷고 `projectName`·캐시·`COLUMNS`·표시폭
    계층을 등재하고 stdin 메모의 총 input/output 의미를 고친다. `README.md`에는 신규 위젯 다섯
    개와 preset 문자를 넣고 fastMode에 "fast mode가 켜진 동안에만 표시"를 명기하며, Privacy의
    캐시 서술을 실제 동작으로 교정한다.
  - 검증 조건:
    - 결과: `CLAUDE.md`의 단일 테스트 예시가 저장소에 실재하는 테스트를 가리키고, 동작 확인 예시
      기대 출력이 그 명령의 실제 출력과 문자 단위로 같으며(task-001 반영 후 값), `projectName`이
      아키텍처 표에 있고 ahead/behind 서술이 남아 있지 않다. `README.md` 위젯 표의 항목 집합이
      registry에 등록된 위젯 ID 집합과 일치하고 preset 문자가 실제 매핑과 같으며, Privacy가
      "계정 유래 값은 저장하지 않고 git으로 다시 얻을 수 있는 값만 캐시하며 캐시가 없어도 표시가
      달라지지 않는다"는 실제 성질을 서술한다.
    - 확인: `CLAUDE.md`에 적힌 단일 테스트 명령과 동작 확인 예시를 그대로 실행해 문서 기대와
      대조하고, `TestSessionCacheKey`·ahead/behind 문구가 남아 있지 않은지 검색으로 확인한다.
      `README.md` 위젯 표와 preset 매핑을 코드와 항목별로 대조한다. `make test`와 `go vet ./...`가
      실패 0으로 통과한다.
  - 참조: SPEC §5.10, §5.8, §5.2, §5.12 / ANALYSIS §4, §5 D11

- [ ] task-012: 버전을 0.6.0으로 동시 갱신
  - 목적: 사용자가 `/plugin` UI에서 이 변경의 업데이트를 감지할 수 있고, 빌드된 바이너리가
    보고하는 버전이 플러그인 메타데이터와 같다.
  - 접근: `Makefile`의 VERSION과 `.claude-plugin/plugin.json`의 version을 같은 값 `0.6.0`으로
    함께 올린다. 새 위젯 추가와 표시 의미 변경이 포함되므로 minor를 올린다.
  - 검증 조건:
    - 결과: 두 파일의 값이 모두 `0.6.0`이고 서로 같다. 로컬 빌드 산출물의 `--version` 출력이
      `0.6.0`이다.
    - 확인: 두 파일에서 값을 각각 확인해 일치를 대조하고, `make build-local` 후 `--version`을
      실행해 출력을 확인한다. `make test`와 `go vet ./...`가 실패 0으로 통과한다.
  - 참조: SPEC §5.11, §5.12
