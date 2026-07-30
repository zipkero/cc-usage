# cc-usage 로드맵

## 최종 결과물

- **대상 사용자**: Claude Code를 쓰면서 세션의 모델·컨텍스트 사용량·비용·rate limit을 항상 눈에 두고
  작업하려는 개발자. macOS·Linux·Windows를 쓰며, 별도 런타임을 설치하지 않고 플러그인 하나로 끝내려는 쪽.
- **핵심 사용자 가치**: 세션 상태를 status line 한 줄로 읽는다. 값이 무엇을 뜻하는지 문서를 뒤지지 않아도
  화면만으로 알 수 있고, 데이터가 아직 없는 구간과 실제로 0인 구간이 구별된다.
- **목표 제공 수준**: Claude Code 마켓플레이스에서 설치·설정·업데이트가 끊김 없이 되는 배포 플러그인.
  단일 바이너리, zero dependency, 단일 `main` 패키지를 유지하며 네트워크에 접속하지 않는다.
- **완성 상태**: 공식 status line 프로토콜이 넘기는 값을 그 의미대로 표시하고, 부분적·비정상 입력에도
  화면이 통째로 사라지지 않으며, 지원 5개 플랫폼에서 같은 동작을 내고, 저장소의 검증 명령이 통과한다.

## 서비스 완료 기준

1. `/plugin marketplace add zipkero/cc-usage` → `/plugin install cc-usage` →
   `/cc-usage:cc-usage-install` → `/reload-plugins` 만으로 status line이 표시된다. 이후 `/plugin update`가
   settings.json을 손대지 않고 in-place로 반영된다.
2. `bin/`의 5개 바이너리(darwin arm64·amd64, linux amd64·arm64, windows amd64)가 같은 버전을 보고하고
   같은 표시 결과를 낸다.
3. stdin에 알 수 없는 필드나 예상과 다른 타입이 섞여 들어와도 status line 전체가 사라지지 않고, 해석
   가능한 위젯은 계속 표시된다.
4. `go test ./...`와 `go vet ./...`가 실패 없이 통과한다.
5. 공식 status line 문서가 정의한 필드 의미와 cc-usage의 계산·표기가 일치한다 — 같은 payload에서 값이
   입력 형태에 따라 달라지지 않는다.
6. status line 렌더 중 네트워크 엔드포인트에 접속하지 않고 계정 유래 값(`cost`·`rate_limits` 등)을
   디스크에 저장하지 않으며, 텔레메트리가 없다. stdout은 위젯 렌더 결과와 ANSI 코드만 담는다.
   디스크에 쓸 수 있는 것은 git 명령으로 언제든 다시 얻을 수 있는 국소 파생값(브랜치)뿐이고, 그 값이
   유실·손상되면 표시는 git을 그대로 실행하는 원래 경로로 되돌아가야 한다.

현재 미달인 것은 3번(§M2), 4번(§M1), 5번(§M3)이다. 1·2·6번은 v0.5.6 기준 충족한다.

6번의 문구는 2026-07-30에 좁혔다. 이전 문구는 "캐시 파일을 읽거나 쓰지 않으며"였고, M3이 도입하려는
git 결과 캐시가 그 문장과 정면으로 충돌했다. 이 기준이 실제로 지키려던 것은 한 머신에서 프로필로 여러
계정을 쓸 때 한 계정의 수치가 다른 계정 화면에 남는 경로와 렌더 중 네트워크 접속이며, 둘 다 좁힌 문구에
그대로 남아 있다. 근거는 `docs/20260727-001-statusline-schema-catchup/analysis.md` §5 D6이 소유한다.

## 포함 범위

- 위젯 시스템 — core(`model`, `context`, `cost`, `rateLimit5h`, `rateLimit7d`)와
  project(`projectInfo`, `projectName`), `Widget` 인터페이스 + registry + `orchestrate()` 기반 확장.
- 설정 — 테마 8종, separator, `preset`/`lines` 레이아웃, `disabledWidgets`, 위젯별 네임스페이스 옵션
  (`widgets.<widget>.<option>`), `CLAUDE_CONFIG_DIR` 반영.
- i18n — en·ko locale을 `go:embed`로 임베드.
- 배포 — `main`(소스)과 `release`(마켓플레이스, GitHub default) 두 브랜치 구성, pre-built 5플랫폼 바이너리,
  OS/ARCH 감지 wrapper.
- 아키텍처 제약 유지 — zero dependency, 단일 `main` 패키지, stdout/stderr 채널 분리.

## 마일스톤

### M1. 검증 기반 정비

저장소의 검증 명령이 개발 머신에서 실제로 통과하고, 그 상태가 자동으로 지켜진다. 지금은 Windows에서
테스트 4개가 실패한 채로 남아 있어 이후 모든 feature가 완료 조건을 "착수 시점 대비 실패 집합 불변"으로
우회해야 하고, 그 사이 진짜 회귀가 baseline에 섞여도 드러나지 않는다. 실패 원인은 fixture가 POSIX 경로를
쓰고 테스트가 `HOME`을 덮는 것이며, 실제 Windows 입력에서는 구현이 맞게 동작하므로 배포 동작의 결함은
아니다. 저장소에 CI가 없어 이 상태가 오래 방치됐다.

- 의존 관계: 없음
- 전환 기준: 서비스 완료 기준 4번이 충족되고, 그 상태를 확인하는 자동 게이트가 존재한다
- Feature 문서 후보: `test-portability-and-ci`

### M2. stdin 견고성

입력이 예상과 달라도 화면이 통째로 사라지지 않는다. 현재 `parseStdin`은 어떤 decode 오류에도 빈 입력을
반환하고, 그러면 무출력 조건을 통과해 status line이 전부 사라진다 — 필드 하나의 타입 흔들림이 전면
블랙아웃이다. v0.5.4의 백분율 소수값 수정이 이미 이 부류였고, 공식 문서는 필드를 계속 늘리고 있다.
rate limit의 `resets_at` 부재를 i18n 문자열 비교로 걸러내는 우회도 같은 층에 있다.

- 의존 관계: M1 (판정 기준이 되는 테스트 상태가 먼저 정상이어야 한다)
- 전환 기준: 서비스 완료 기준 3번이 충족된다
- Feature 문서 후보: `stdin-resilience`

### M3. status line 스키마 정합

공식 프로토콜에 추가된 필드를 수용·노출하고, 기존 위젯의 계산이 문서가 정의한 의미와 어긋난 지점을
교정한다. `20260727-001-statusline-schema-catchup`으로 SPEC·ANALYSIS가 작성돼 있고, analysis의 승인 대기
3건(context 토큰 표시 기준, bool 상태 false의 표시 정책, `COLUMNS` 축소 순서)은 2026-07-30에 확정돼
문서에 반영됐다. M0·M1·M2가 바꾼 전제(기본 위젯 구성, 섹션 단위 stdin 파싱, 버전)와 git 캐시를 허용하도록
좁힌 서비스 완료 기준 6번도 함께 갱신했다. 선행 판단이 남아 있지 않으므로 IMPLEMENT 작성으로 넘어갈 수 있다.

- 의존 관계: M2 (둘 다 rate limit 렌더 경로를 건드려 순서를 지키지 않으면 충돌한다)
- 전환 기준: 서비스 완료 기준 5번이 충족된다
- Feature 문서 후보: `statusline-schema-catchup` (기존 문서 재사용)

### M4. subagent status line 지원

서브에이전트 행을 커스터마이즈하는 `subagentStatusLine`을 지원한다. stdin 스키마(`tasks` 배열, `columns`)와
출력 프로토콜(행별 JSON)이 기존 statusLine과 완전히 달라 기존 위젯 계약을 그대로 재사용할 수 없다.

- 의존 관계: M3 (기존 프로토콜 정합이 끝난 뒤에 두 번째 프로토콜을 얹는다)
- 전환 기준: 서브에이전트 행이 설정대로 표시되고, 기존 statusLine 동작에 회귀가 없다
- Feature 문서 후보: `subagent-statusline`

## 최종 관문

여러 마일스톤에 공통으로 적용된다.

- 사용자가 체감하는 fix·feature 변경은 SemVer bump을 동반하며 `Makefile`의 VERSION과
  `.claude-plugin/plugin.json`의 version을 같은 값으로 동시에 갱신한다. `/plugin` UI의 업데이트 감지가
  후자에 의존하므로 이를 빠뜨리면 사용자 머신의 사본이 stale로 고착된다.
- `bin/` 재빌드 후 `release` 브랜치에 반영한다. orphan 브랜치이므로 머지하지 않고 파일을 복사해 새 commit을
  쌓으며, 바이너리의 실행 비트를 유지한다. release 브랜치 `README.md`는 별도 사본이므로 사용자에게 보이는
  동작이 바뀌면 그 사본도 함께 갱신한다.
- stdout은 위젯 렌더 결과와 ANSI 코드만 담고 진단은 stderr로만 보낸다.

## 보류·제외 범위

- **stdin 밖 데이터 출처 도입** — OAuth usage 엔드포인트 호출로 5h/7d를 세션 첫 렌더부터 채우는 것,
  그리고 transcript(JSONL) 전수 스캔으로 누적 사용량 지표를 더하는 것. 제외 이유는
  `20260602-001-simplify-statusline`이 확정한 "stdin과 git 명령만 사용" 원칙(서비스 완료 기준 6번)을
  정면으로 되돌리는 일이고, 렌더 경로에 네트워크·자격증명 읽기가 다시 들어오기 때문이다.
  M3의 git 캐시가 6번의 문구를 좁혔지만 이 항목은 그 예외에 해당하지 않는다 — 저장하려는 값이 계정 유래
  수치이고 출처가 git 명령이 아니라 네트워크와 transcript다.
  관련 위험은 문서화되지 않은 beta 엔드포인트에 의존한다는 점과 macOS keychain 접근이다.
  `20260730-001-session-start-placeholder`의 placeholder 표기로 체감 문제가 해소되어 시급성도 낮다.
  다시 검토할 조건 — 첫 렌더부터 rate limit 실수치가 필요하다는 요구가 확인되고, 갱신 주체와 소비 주체를
  분리해 렌더 경로가 파일 읽기 한 번으로 끝나는 설계가 성립할 때.
- **프로필 간 `cc-usage.json` 승계** — 프로필 도구의 공유 자산 정책이 소유한다. cc-usage는
  `CLAUDE_CONFIG_DIR`이 가리키는 디렉토리 하나만 본다. 다시 검토할 조건 — 프로필 도구 쪽에서 공유·분리
  정책이 정해지고 cc-usage에 요구되는 동작이 명시될 때.
- **`prompt_id` 수용** — status line 표시에 쓸 용처가 확인되지 않았다. 다시 검토할 조건 — 이 값으로
  구별해야 하는 표시 요구가 생길 때.
- **컨텍스트 토큰 경고 임계값 재조정** — 현재 임계값(256K/512K)이 200K 컨텍스트 모델에서는 발화하지 않는
  성질을 확인했으나 1M 모델 전용 의도인지 판단이 필요해 값을 바꾸지 않았다. 다시 검토할 조건 — 임계값의
  의도가 정해질 때.
- **`20260528-001-refactor-structure`** — 상위 변경으로 무효화됐다(superseded). implement.md의 Task
  다수가 `api.go`·`cache.go`·`cache_api.go`를 대상으로 하는데 `20260602-001-simplify-statusline`이 그
  파일들을 없앴다. 다시 검토할 조건 — 살아남은 부분(i18n을 `widget.go`에서 분리)을 따로 열 값이 있다고
  판단될 때. 그때는 기존 문서를 되살리지 않고 새 feature로 다시 정의한다.

## 문서

- [README.md](./README.md) — 사용자 대상 설치·설정·위젯·troubleshooting
- [CLAUDE.md](./CLAUDE.md) — 개발자 대상 아키텍처·위젯 추가 절차·빌드·배포 절차
- `docs/<feature-dir>/` — feature별 spec·analysis·implement
