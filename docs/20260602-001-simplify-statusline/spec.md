# spec: simplify-statusline

## 1. 범위

cc-usage의 데이터 출처와 표시 항목을 Claude Code 공식 status line 문서가 보여주는 수준으로
단순화한다. 구체적으로 두 축이다.

- **백엔드 단순화**: 지금 cc-usage가 stdin 외에 별도로 읽고 돌리는 무거운 로직 —
  OAuth credential 로딩, `/api/oauth/usage` 호출과 3-tier(memory·file·API) 캐시,
  세션 상태 캐시 기반 degraded-input 복원, transcript 파일 기반 cost 추정 — 을 걷어내고
  공식 문서처럼 **stdin과 git 명령만으로** 동작하게 한다.
- **표시 항목 정리**: 공식 문서에 없는 자체 확장인 Sonnet 전용 7일 rate limit(`7d-S`)과
  analytics 표시 항목(version·apiDuration·sessionDuration·burnRate·cacheHit·performance)을
  제거한다.

위젯 아키텍처(`Widget` 인터페이스 + registry + `orchestrate()` + 멀티라인 레이아웃)와
config/preset/i18n 체계, 현재 렌더 비주얼(테마·progress bar·separator)은 유지한다.

## 2. 목표

- cc-usage를 공식 문서가 안내하는 "stdin을 읽어 그린다"는 단순 모델에 맞춰, 외부 파일·네트워크
  의존과 그에 딸린 복잡한 복원·캐시 로직을 없앤다.
- 유지보수 표면을 줄여 동작을 예측 가능하게 만들고, 공식 문서와 동일한 rate limit 노출 동작을
  따른다.
- 위젯을 추가·구성하는 방식은 그대로 두어 기존 사용자 설정과 확장 경로를 보존한다.

## 3. 제약

- Zero dependency 유지 — Go 표준 라이브러리만 사용하며 `go.mod`에 `require` 블록이 생기지 않는다.
- 단일 `main` 패키지 유지 — 서브 패키지를 만들지 않고 파일 단위로만 분리한다.
- stdout은 위젯 렌더 결과 + ANSI 코드만 출력하고, debug/error는 stderr로만 보낸다.
- 위젯 추가 절차와 `Widget` 인터페이스 계약(`ID`/`GetData`/`Render`, nil·빈 문자열 skip, 패닉 금지)을
  유지한다.
- 사용자 체감 동작이 바뀌므로 CLAUDE.md §버전 정책에 따라 SemVer bump(Makefile VERSION,
  `.claude-plugin/plugin.json`, `api.go`가 남는다면 그 userAgent 포함 동일 값)와 release 브랜치
  동기화를 동반한다.

## 4. 제외 범위

- 공식 문서에 없는 새 위젯·새 표시 항목 추가는 하지 않는다.
- config/preset/i18n 시스템 자체의 재설계나 멀티라인 레이아웃 표현 방식 변경은 하지 않는다(유지).
- 렌더 비주얼(테마 색상 체계, progress bar 문자, separator, 이모지 스타일)의 전면 개편은 하지
  않는다 — 본 작업의 "단순화"는 데이터 출처·표시 항목에 한정한다.
- git 정보 취득 방식(`porcelain=v2` 기반 branch + ahead/behind)은 변경하지 않는다.
- 5h/7d를 stdin에서만 받게 되어 세션 초반·무료 구간에 잠시 비거나 미표시되는 동작은 공식 문서와
  동일한 결과로 수용하며, 이를 보완하는 대체 데이터 출처는 도입하지 않는다.

## 5. 완료 조건

1. status line을 렌더하는 동안 cc-usage는 OAuth credential, 세션 상태 캐시, API 응답 캐시 같은
   별도 파일을 읽거나 쓰지 않고 어떤 네트워크 엔드포인트에도 접속하지 않는다. stdin과 git 명령
   외의 외부 I/O가 관찰되지 않는다.
2. 5h/7d rate limit 표시는 stdin `rate_limits.five_hour` / `rate_limits.seven_day` 값만으로
   결정된다. 해당 필드가 stdin에 없으면 공식 문서와 동일하게 그 항목이 출력되지 않는다.
3. Sonnet 전용 7일 rate limit(`7d-S`)은 어떤 입력·설정에도 출력되지 않는다.
4. analytics 표시 항목(version, apiDuration, sessionDuration, burnRate, cacheHit, performance)은
   어떤 입력·설정에도 출력되지 않는다.
5. cost 표시는 stdin `cost.total_cost_usd` 값만으로 결정되며, transcript 파일을 읽어 비용을
   추정하지 않는다. 추정값을 뜻하던 `~` 접두 표기는 어떤 입력에도 출력되지 않는다.
6. 부분적·비어 있는 stdin이 들어와도 직전 실행 캐시로 필드를 복원하지 않고 입력에 담긴 값만
   렌더한다. model·dir·context가 모두 비어 있을 때 출력을 생략하는 동작은 유지된다.
7. 정상 stdin 입력에서 model·project(dir)·git branch·context·cost·5h·7d 표시가 정상 출력되고,
   멀티라인 레이아웃 설정 시 각 줄이 별도 행으로 출력된다.
8. 위 변경이 `README.md`와 `CLAUDE.md`에 반영되어 credential·OAuth API·3-tier 캐시·
   degraded-input 복원·세션 캐시·`7d-S`·analytics·transcript 기반 cost 추정에 대한 서술이 남지
   않는다.
