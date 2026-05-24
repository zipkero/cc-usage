# IMPLEMENT — safe-empty-stdin-fallback

실행 순서는 위에서 아래로. Task ID는 영구 식별자이며 재번호하지 않는다.

## Section: cwd 식별·정규화 기반

- [ ] task-001: cwd 정규화 함수 도입
  - 목적: 동일한 워크스페이스를 가리키는 서로 다른 경로 표기(`/private` 접두, trailing slash, `.` 포함)가 매칭 시 동일 값으로 비교된다
  - 접근: `cache.go`에 `normalizeCwd(raw string) string` 추가 — `filepath.EvalSymlinks` 1회 시도 후 실패하면 `filepath.Clean` 결과로 폴백. 빈 입력은 빈 문자열 그대로 반환
  - 검증 조건:
    - 결과: 절대 경로 입력에 대해 EvalSymlinks 가능 경로는 평가된 결과, 실패 경로는 Clean된 결과를 반환. 빈 입력은 빈 문자열
    - 확인: `cache_test.go`에 정규화 단위 테스트(symlink, trailing slash, `.` 세그먼트, 빈 입력)를 추가하고 `go test ./...` 통과
  - 참조: SPEC §5.2, §5.9, ANALYSIS §4.2, §12 D3, D4

- [ ] task-002: 현재 워크스페이스 식별 신호 감지 함수 도입
  - 목적: 빈 stdin이 들어와도 cc-usage가 현재 워크스페이스의 cwd 후보를 환경에서 얻을 수 있다
  - 접근: `cache.go`에 `detectCurrentCwd() string` 추가 — `CLAUDE_PROJECT_DIR` env 우선, 비어있으면 `os.Getwd()` 폴백. 둘 다 부재·실패면 빈 문자열. 결과는 `normalizeCwd`로 정규화. PWD env는 사용하지 않음. 테스트성을 위해 env getter·getwd를 함수 변수로 분리
  - 검증 조건:
    - 결과: env가 설정되면 env 값을 정규화해 반환, env가 비고 Getwd 성공이면 그 값을 정규화해 반환, 둘 다 실패면 빈 문자열
    - 확인: `cache_test.go`에 세 분기 단위 테스트(env hit, env miss + getwd hit, 모두 실패)를 추가하고 `go test ./...` 통과
  - 참조: SPEC §5.2, §5.3, ANALYSIS §2, §12 D1

## Section: cwd 기반 fallback 매칭

- [ ] task-003: 캐시 저장 시점 cwd 정규화 적용
  - 목적: 저장된 캐시의 `Workspace.CurrentDir`이 fallback 매칭에서 일치 비교되도록 정규화된 형태로 디스크에 들어간다
  - 접근: `saveSessionState`(또는 동등 경로) 직전에 `snapshot.Workspace.CurrentDir`을 `normalizeCwd`로 한 번 정규화해서 저장. 다른 필드는 건드리지 않음
  - 검증 조건:
    - 결과: 정상 stdin 저장 후 디스크 파일을 다시 읽었을 때 `Workspace.CurrentDir`이 정규화된 절대 경로
    - 확인: `cache_test.go`에 저장→로드 라운드트립 테스트로 정규화 적용 확인, `go test ./...` 통과
  - 참조: SPEC §5.1, §5.9, ANALYSIS §4.2, §12 D4

- [ ] task-004: `loadByWorkspaceCwd` fallback 매칭 함수 도입
  - 목적: 빈 stdin이 도착해 cacheKey가 비어도 현재 cwd와 정확 일치하는 비-만료 세션 캐시가 있으면 그 캐시를 찾아 반환한다
  - 접근: `cache.go`에 `loadByWorkspaceCwd(dir, cwd string, now time.Time) *SessionState` 추가. `session-state-*.json` glob → 각 파일 unmarshal → `normalizeCwd(Workspace.CurrentDir)`이 `cwd`와 **정확 일치**하고 `sessionStateTTL` 이내인 후보 추출 → 0개면 nil, 1개 이상이면 mtime newest 선택. subpath/substring/case-insensitive 매칭 금지. 빈 cwd면 즉시 nil 반환
  - 검증 조건:
    - 결과: 동일 cwd 캐시 존재 시 그 SessionState 반환, 부재·만료 시 nil 반환, 빈 cwd 시 nil 반환
    - 확인: `cache_test.go`에서 임시 디렉터리에 cwd 다른 캐시 2개를 만들고 cwd=X로 호출 시 X 캐시만 반환, cwd=Z(미존재) 호출 시 nil, mtime 6분 전 파일은 nil을 어서션. `go test ./...` 통과
  - 참조: SPEC §5.1, §5.2, §5.7, ANALYSIS §3.2, §4.2, §12 D2, D3

- [ ] task-005: main.go에서 빈 cacheKey 경로에 fallback 호출 연결
  - 목적: 빈 stdin이 도착했을 때 사용자가 보던 status line이 직전 정상 호출과 동등한 full 출력으로 유지된다(동일 워크스페이스 + 비-만료 캐시 보유 시)
  - 접근: `main.go`의 `loadSessionState` 호출 직후 `cacheKey == ""`인 경우에 한해 `detectCurrentCwd` → `loadByWorkspaceCwd(cacheDir, cwd, now)`를 호출해 `cached`에 대입. 그 외 정상 경로는 무변경. fallback으로 가져온 `cached.RateLimits`는 그대로 nil이어야 하며 별도 처리 없이 기존 restore 흐름을 그대로 사용
  - 검증 조건:
    - 결과: 빈 stdin + cwd 매칭 캐시 존재 시 restore 블록이 cached로 진입해 다시 orchestrate가 호출되고 full 위젯이 출력됨. 정상 stdin 입력은 fallback 호출을 건드리지 않음
    - 확인: `main_test.go`(또는 동등)에서 (1) 빈 stdin + cwd 매칭 캐시 → full 복원, (2) 정상 stdin → fallback 미호출 두 케이스를 어서션하고 `go test ./...` 통과
  - 참조: SPEC §5.1, §5.4, ANALYSIS §3.2, §12 D2

## Section: stale cwd 방어

- [ ] task-006: workspace 복원 시 cwd 일치 가드 추가
  - 목적: 같은 session 안에서 `cd`로 다른 디렉토리로 이동한 직후 빈 workspace stdin이 와도 직전 디렉토리의 cwd/projectInfo가 화면에 노출되지 않는다
  - 접근: `main.go`의 workspace 복원 분기에서 cached `Workspace.CurrentDir`을 `normalizeCwd`로 정규화한 값이 `detectCurrentCwd()` 결과와 **정확 일치**할 때만 복원. 일치하지 않으면 workspace 필드는 복원하지 않음(cost/context 등 다른 복원은 영향받지 않음). `detectCurrentCwd`가 빈 값을 반환하면(식별 불가) workspace 복원도 skip
  - 검증 조건:
    - 결과: cached cwd != current cwd 인 시퀀스에서 workspace 필드 미복원, cached cwd == current cwd 인 시퀀스에서는 기존대로 복원
    - 확인: `main_test.go`에 cd 시뮬레이션 테스트(detectCurrentCwd fake로 cwd 전환) 추가, `go test ./...` 통과
  - 참조: SPEC §5.11, ANALYSIS §5.2, §12 D5

- [ ] task-007: `workspaceRestoreTTL`을 60s로 단축
  - 목적: 가드를 통과한 경우라도 stale cwd가 화면에 노출될 수 있는 시간 창이 v0.3.4의 300s에서 60s로 단축된다
  - 접근: `cache.go`의 `workspaceRestoreTTL` 상수를 `60 * time.Second`로 변경. 다른 TTL(`sessionStateTTL` 등)은 변경하지 않음
  - 검증 조건:
    - 결과: 상수 값이 60s. 기존 workspace 복원 TTL 기반 단위 테스트가 새 값에 맞게 갱신되고 통과
    - 확인: `grep workspaceRestoreTTL` 결과가 단일 값, `go test ./...` 통과
  - 참조: SPEC §5.11, ANALYSIS §5.2, §12 D5

## Section: 관찰성

- [ ] task-008: fallback 결정 debugLog 추가
  - 목적: 빈 stdin에서 fallback이 발동했는지, 어떤 cwd 신호로 매칭됐는지, 또는 왜 미발동했는지를 `DEBUG=cc-usage` 환경에서 stderr로 확인할 수 있다
  - 접근: 적중·미적중·신호 부재 세 분기에서 `debugLog` 한 줄씩 출력. 형식 예: `empty stdin -> matched cache via cwd=<cwd> source=<env|getwd> path=<filename>`, `empty stdin -> no cache for cwd=<cwd> source=<...>`, `empty stdin -> no cwd signal (env miss, getwd=<err|val>) -> suppress/partial`. cwd 일치 가드 미충족 분기도 한 줄 추가. stdout 오염 금지
  - 검증 조건:
    - 결과: `DEBUG=cc-usage` 환경에서 빈 stdin smoke 실행 시 세 분기 중 해당하는 한 줄이 stderr에 기록. stdout에는 영향 없음
    - 확인: 수동 smoke(`DEBUG=cc-usage echo '{}' | ./dist/cc-usage 2>&1 >/dev/null`) 3종으로 메시지 형태 확인
  - 참조: SPEC §5.6, ANALYSIS §7

## Section: 회귀 보호 테스트

- [ ] task-009: SPEC §5.7 네 경로 자동 테스트
  - 목적: fallback 도입 후에도 네 경로(식별+적중 → 복원, 식별+부재 → 미복원, 식별 실패 → 미복원, TTL 초과 → 미복원)가 일관되게 동작한다
  - 접근: `cache_test.go` 또는 `main_test.go`에 네 케이스를 명시적 테스트 함수로 추가. 임시 cacheDir + `detectCurrentCwd` fake로 cwd 신호 주입. 케이스 (b)에서는 다른 워크스페이스 캐시를 사전 배치해 cross-workspace 미노출까지 어서션
  - 검증 조건:
    - 결과: 네 케이스 모두 PASS, cross-workspace 노출 0회
    - 확인: `go test ./...` 통과
  - 참조: SPEC §5.7, SPEC §5.2, SPEC §5.3, ANALYSIS §8.1

- [ ] task-010: 멀티 워크스페이스 시퀀스 통합 테스트
  - 목적: A→B→A→C 시퀀스(각 워크스페이스에서 빈 stdin 다수 발생)에서 어느 시점에도 cross-workspace 데이터가 노출되지 않는다
  - 접근: `main_test.go`에 ANALYSIS §4.3 t0~t6 시퀀스를 그대로 재현하는 통합 테스트 추가. detectCurrentCwd fake로 워크스페이스 전환 표현. 각 step 후 출력의 cwd/projectInfo가 의도한 워크스페이스인지 어서션
  - 검증 조건:
    - 결과: t3·t5·t6 어서션 모두 PASS, cross-workspace 노출 0회
    - 확인: `go test ./...` 통과
  - 참조: SPEC §5.9, ANALYSIS §4.3

- [ ] task-011: RateLimits 격리 단위 테스트
  - 목적: fallback이 발동해도 `ctx.RateLimits`가 session-state 캐시로부터 채워지지 않고 API 캐시(`cache-<tokenHash>.json`)에서만 채워진다
  - 접근: `main_test.go`(또는 `cache_test.go`)에 빈 stdin + cwd 매칭 캐시 (RateLimits nil 저장됨) 시나리오에서 restore 후 `ctx.RateLimits`가 cached 값을 반영하지 않음을 어서션. 저장 측은 이미 nil이므로 reload 결과도 nil임을 명시적으로 확인
  - 검증 조건:
    - 결과: fallback restore 경로의 `cached.RateLimits == nil`, `ctx.RateLimits`는 별도 API 캐시 경로에서만 세팅
    - 확인: `go test ./...` 통과
  - 참조: SPEC §5.5, ANALYSIS §6.1

- [ ] task-012: SPEC §5.11 cd 시나리오 회귀 테스트
  - 목적: 같은 session 안 cd 직후 stale cwd가 노출되지 않는다는 가드 동작이 회귀 보호된다
  - 접근: `main_test.go`에 시나리오 추가 — t0: cwd=A에서 정상 stdin 저장, t1: detectCurrentCwd fake가 cwd=B를 반환하는 상태에서 빈 workspace stdin 도착. workspace 필드가 A로 복원되지 않음을 어서션
  - 검증 조건:
    - 결과: cd 후 빈 workspace stdin 케이스에서 출력의 cwd/projectInfo가 A를 포함하지 않음
    - 확인: `go test ./...` 통과
  - 참조: SPEC §5.11, ANALYSIS §5.2

- [ ] task-013: v0.3.4 baseline 보존 회귀 확인
  - 목적: fallback 도입이 v0.3.4 baseline 동작(`shouldSuppressOutput`의 noIdentity + rate-limit OR, `restoreUsageFields`의 Cost/Context 복원, `cleanOldSessionStates` glob, `cleanOldCaches` 무조건 호출)을 변경하지 않는다
  - 접근: 기존 v0.3.4 회귀 테스트가 변경 없이 통과하는지 확인. 누락된 baseline 어서션이 있으면 보완(예: `cleanOldCaches` 무조건 호출 어서션이 약하면 강화)
  - 검증 조건:
    - 결과: 기존 v0.3.4 baseline 테스트 4종이 코드 수정 없이 PASS 유지
    - 확인: `go test ./...` 통과, 변경된 baseline 테스트가 없음을 `git diff` 확인
  - 참조: SPEC §5.4, SPEC §5.8, ANALYSIS §9

## Section: 배포

- [ ] task-014: 버전 patch bump (v0.3.6 → v0.3.7)
  - 목적: `/plugin` UI가 본 fallback 변경을 새 버전으로 인식해 사용자 머신에 update가 전파된다
  - 접근: `Makefile`의 `VERSION`, `.claude-plugin/plugin.json`의 `version`, `api.go`의 `userAgent`를 모두 `v0.3.7`(또는 동등 표기)로 동일하게 갱신. version-only commit이 아니라 본 feature commit에 묶임
  - 검증 조건:
    - 결과: 세 파일의 grep 결과가 동일한 새 버전, `make build-local` 후 `./dist/cc-usage --version`(또는 동등)이 새 버전 출력
    - 확인: `grep -n "0\.3\.7" Makefile .claude-plugin/plugin.json api.go`로 세 hit 확인, 로컬 빌드 산출물 버전 확인
  - 참조: SPEC §5.10, ANALYSIS §11
