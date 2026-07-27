# statusline-schema-catchup

## 1. 범위

Claude Code status line 프로토콜의 현재 공식 스펙과 cc-usage의 실제 구현 사이에 생긴 차이를 좁힌다. 대상은 세 갈래다.

- stdin JSON 스키마에 추가된 필드의 수용과 노출
- 기존 위젯의 계산·표기가 공식 문서가 정의한 의미와 어긋난 지점의 교정
- 프로필별 설정 디렉토리 환경에서 설치 절차가 잘못된 파일을 고르는 문제의 교정

### 입력 맥락

조사 출발점은 공식 문서 `code.claude.com/docs/en/statusline.md`(2026-07-27 조회)와 현재 저장소 상태의 대조다. 확인된 차이는 다음과 같다.

- `used_percentage`는 input 계열 토큰(`input_tokens + cache_creation_input_tokens + cache_read_input_tokens`)만으로 계산되며
  `output_tokens`를 포함하지 않는다. 문서는 수동 계산 시 같은 input-only 공식을 쓰라고 명시한다. `widgets_core.go`의 fallback 경로는
  output을 더하고 있어, 같은 payload에서 `used_percentage`가 있을 때와 없을 때 퍼센트가 달라진다.
- v2.1.132부터 `total_input_tokens`·`total_output_tokens`는 세션 누적이 아니라 현재 컨텍스트 값이다. `CLAUDE.md`에는 output만 그렇게
  적혀 있다.
- stdin.go에 없는 필드: `fast_mode`, `effort.level`, `thinking.enabled`, `workspace.git_worktree`, `workspace.repo`, `pr`, `prompt_id`.
  이 중 `workspace.git_worktree`는 `--worktree` 세션에서만 채워지는 기존 `worktree.*`와 달리 일반 git worktree에서도 채워지므로,
  `CLAUDE.md`에 적힌 "worktree는 일반적으로 비어있음" 한계를 대체한다.
- 모델 기호 매핑에 `fable`·`mythos` 분기가 없어 해당 모델 ID가 기본 기호로 떨어진다. `Translations.Model`은 로드되지만 쓰이지 않는다.
- v2.1.153부터 `COLUMNS`·`LINES` 환경변수로 터미널 크기를 읽을 수 있다. 현재는 config 값과 위젯 내부 상수가 폭을 추정한다.
- 문서 Tips가 `session_id` 기반 캐시로 git 호출 비용을 줄이라고 권고한다. 현재는 매 실행마다 git 하위 프로세스를 띄운다.
- 설치 스킬이 대상 settings 파일 후보를 `~/.claude` 기준으로 하드코딩한다. Claude Code가 `CLAUDE_CONFIG_DIR`로 설정 디렉토리를
  옮긴 환경에서는 실제로 읽히지 않는 파일에 쓰게 된다.

프로필 도구(ccswitch)와의 상호영향도 함께 검토했다. 프로필 전환 시 `cc-usage.json`이 승계되지 않는 문제는 cc-usage에 상위 디렉토리
폴백을 넣는 방향을 검토했으나 접었다 — 프로필마다 테마를 달리 두는 용법을 무력화하고, 공유·분리 정책의 소유권이 프로필 도구 쪽에
있기 때문이다. 이 항목은 이번 범위에서 제외한다(§4). 설치 스킬 문제는 프로필 도구 유무와 무관하게 성립하므로 범위에 둔다.

## 2. 목표

statusline이 표시하는 값이 Claude Code가 실제로 넘긴 의미와 일치하게 만든다. 지금은 컨텍스트 퍼센트가 입력 payload의 형태에 따라
달라지고, 사용자가 세션에서 바꾼 상태(fast mode, effort 수준) 중 statusline에 반영되지 않는 것이 있다.

부수적으로, 설정 디렉토리를 옮겨 쓰는 사용자가 설치 절차를 따라도 statusline이 뜨지 않는 실패를 없앤다.

## 3. 제약

- **zero dependency 유지.** Go 표준 라이브러리만 쓴다. `go.mod`에 `require` 블록이 생기면 안 된다.
- **단일 `main` 패키지 유지.** 서브 패키지를 만들지 않고 파일로만 분리한다.
- **stdout 오염 금지.** stdout은 위젯 렌더 결과와 ANSI 코드 전용이다. 진단·경고는 stderr로만 낸다.
- **기본 출력 구성 불변.** 설정 파일이 없는 환경에서 출력되는 위젯 구성은 이번 변경 전과 같아야 한다. 새로 만드는 위젯은 기본 preset에
  넣지 않고 preset 문자 또는 `lines`로 켜는 opt-in으로 둔다. 이유: 현재 기본 6개에 3개가 더 붙으면 좁은 터미널에서 줄바꿈이 생기고,
  기존 사용자 화면이 업데이트만으로 바뀐다.
- **캐시 대상 제한.** git 결과 캐시를 도입하되 캐시에 담는 것은 git 유래 정보(브랜치, ahead/behind)로 한정한다. `cost`·`rate_limits`처럼
  계정에 딸린 값은 캐시하지 않는다. 이유: 한 머신에서 여러 계정을 프로필로 나눠 쓰는 환경에서 한 계정의 수치가 다른 계정 화면에 남는
  경로를 원천 차단한다. 현재 cc-usage가 상태를 저장하지 않아 성립하는 성질이므로, 캐시 도입이 이 성질을 깨는 유일한 항목이다.
- **누락 필드는 정상 경로.** 새로 수용하는 필드는 모두 조건부로 존재한다(`effort`는 모델이 지원할 때만, `pr`은 열린 PR이 있을 때만,
  `rate_limits`는 구독자에게만, `current_usage`는 첫 API 호출 전 `null`). 부재나 `null`에서 패닉하거나 오류를 출력하지 않는다.
- **버전 정책 준수.** 사용자가 체감하는 변경이 포함되므로 `Makefile` VERSION과 `.claude-plugin/plugin.json` version을 같은 값으로
  동시 갱신한다.

## 4. 제외 범위

- **subagentStatusLine 지원.** 서브에이전트 행을 커스터마이즈하는 별도 설정 키로, stdin 스키마(`tasks` 배열, `columns`)와 출력
  프로토콜(행별 JSON)이 기존 statusLine과 완전히 다르다. 같은 feature에 넣으면 완료 조건이 두 갈래로 갈린다. 별도 feature로 다룬다.
- **프로필 간 `cc-usage.json` 승계.** 프로필 도구(ccswitch)의 공유 자산 정책이 소유한다. cc-usage는 `CLAUDE_CONFIG_DIR`이 가리키는
  디렉토리 하나만 보는 현재 동작을 유지한다.
- **`prompt_id` 수용.** statusline 표시에 쓸 용처가 없다.
- **stdin.go의 `remote`·`agent_id`·`agent_type` 필드 정리.** 현재 공식 문서 스키마에 없으나, 있어서 해를 끼치지 않고 제거 근거도
  확인되지 않았다. 그대로 둔다.
- **컨텍스트 토큰 경고 임계값 재조정.** 현재 임계값이 200K 컨텍스트 모델에서는 발화하지 않는 성질은 확인했으나, 1M 모델 전용 임계로
  의도된 것인지 판단이 필요하다. 이번에는 근거만 문서에 남기고 값은 바꾸지 않는다.

## 5. 완료 조건

1. `used_percentage`가 없는 stdin payload에 대해, 같은 payload에 `used_percentage`를 채워 넣은 경우와 동일한 컨텍스트 퍼센트가
   stdout에 출력된다.
2. `fast_mode`, `effort.level`, `thinking.enabled` 각각에 대응하는 위젯을 preset으로 켰을 때 그 값이 stdout에 나타나고, 해당 키가 없는
   stdin에서는 그 위젯이 출력에서 생략된다.
3. 설정 파일이 없고 터미널 폭 제약이 걸리지 않는 환경에서 실행했을 때 stdout의 위젯 구성이 이번 변경 전과 동일하다.
   폭 제약이 실제로 걸리는 환경에서는 7번이 우선한다 — 기본 구성을 그대로 내면 폭 조건을 만족할 수 없기 때문이다.
4. `workspace.git_worktree`를 담은 stdin에서 project 계열 위젯이 worktree 정보를 반영해 출력하고, 그 키가 없으면 기존과 동일하게
   출력한다.
5. `workspace.repo`와 `pr`을 담은 stdin에서 해당 정보가 stdout에 나타나고, 두 키가 모두 없는 stdin에서 관련 출력이 생략된다.
6. `claude-fable-5`와 `claude-mythos-5`를 model ID로 담은 stdin에서 model 위젯이 기본 기호가 아닌 각 모델 계열의 기호를 출력한다.
7. `COLUMNS`가 설정된 환경에서 stdout 각 줄의 표시 폭이 그 값을 넘지 않는다.
8. 같은 `session_id`로 연속 실행할 때 두 번째 이후 실행에서 git 하위 프로세스가 실행되지 않고, 캐시 산출물에 `cost`·`rate_limits`
   유래 값이 들어 있지 않다.
9. `CLAUDE_CONFIG_DIR`이 설정된 환경에서 설치 절차가 그 디렉토리 아래의 settings 파일을 대상으로 고르고, 미설정 환경에서는 기존과
   동일하게 `~/.claude` 아래를 고른다.
10. `CLAUDE.md`에 적힌 검증 명령과 동작 확인 예시가 저장소의 실제 상태와 일치한다 — 존재하지 않는 테스트 이름을 예시로 들지 않고,
    stdin 스키마 관련 메모가 현재 공식 문서와 어긋나지 않는다.
11. `Makefile`의 VERSION과 `.claude-plugin/plugin.json`의 version이 같은 새 값으로 갱신되어 있다.
12. `make test`와 `go vet ./...`가 통과한다.
