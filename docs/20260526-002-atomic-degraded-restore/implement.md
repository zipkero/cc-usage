# atomic-degraded-restore — Implement

## 매핑 점검

- SPEC §5.1 → task-002, task-003
- SPEC §5.2 → task-002, task-003, task-005
- SPEC §5.3 → task-003, task-005
- SPEC §5.4 → task-001, task-003, task-005
- SPEC §5.5 → task-002, task-003, task-005
- SPEC §5.6 → task-004, task-005

미매핑 SPEC §5 기준 없음.

## Tasks

- [x] task-001: `cache.go` `workspaceRestoreTTL` 의미 재정의 및 미사용 헬퍼 정리 준비
  - 목적: atomic 복원 결정의 단일 TTL 상수가 코드 안에서 "atomic restore TTL"로 명확히 식별된다
  - 접근: `cache.go`의 `workspaceRestoreTTL = 60s` 주석을 ANALYSIS §5.C(C2) 의미로 갱신. 후속 Task에서 호출 형태가 바뀔 `shouldRestoreCost` / `restoreUsageFields`의 현재 시그니처·호출처를 식별만 해두고 본문은 task-002·003에서 통합. 본 Task 단독으로는 외부 동작 변경 없음.
  - 검증 조건:
    - 결과: `workspaceRestoreTTL` 상수의 의미가 주석상 "atomic restore TTL"로 명시되고, 기존 stdin 정상 입력에서 status line 출력이 변경 전과 동일하다.
    - 확인: `go vet ./...` 및 `go build ./...` 통과. 기존 `make test` 그린. README CLAUDE.md 본문 §degraded-input 복원 설명과 모순이 없는지 diff 확인.
  - 참조: SPEC §5.4, ANALYSIS §3, ANALYSIS §5.C

- [x] task-002: 단일 eligibility 결정 + 필드별 fill 헬퍼 도입
  - 목적: degraded stdin에 대한 캐시 보강 자격이 한 번의 yes/no로 결정되고, 그 결정이 true일 때만 Workspace·Worktree·Model·Cost·ContextWindow의 빈 자리가 캐시 값으로 채워진다
  - 접근: `cache.go`(또는 `main.go`) 에 `shouldRestoreFromSession(stdin, cached, now) bool`을 추가. 입력: 현재 stdin·cached SessionState·now. 검사 순서: cached 존재 → SavedAt 유효 + `workspaceRestoreTTL` 이내 → `shouldRestoreWorkspace(cached.Workspace.CurrentDir)` 통과 → 보강이 필요한 빈 필드가 하나라도 있음. 동시에 `fillFromSessionCache(stdin*, cached) (filled fieldMask)` 헬퍼를 도입해 Workspace/Worktree/Model/Cost/ContextWindow 각각을 "stdin 빈 자리 ↔ cached 채워짐" 조건으로 채우고, 실제로 채운 필드 집합을 반환. RateLimits는 절대 채우지 않음. fresh로 들어온 필드는 덮어쓰지 않음. 기존 `shouldRestoreCost` / `restoreUsageFields`의 역할을 이 두 함수에 흡수하고, 더 이상 호출되지 않는 헬퍼는 제거하거나 비공개 헬퍼로 축소.
  - 검증 조건:
    - 결과: 새 두 함수가 존재하고, eligibility=true일 때 stdin이 비어 있던 다섯 필드가 모두 cached 값으로 채워지며 fresh로 들어온 필드는 덮이지 않는다. RateLimits는 어느 경로로도 채워지지 않는다.
    - 확인: `cache_test.go`에 단위 테스트 추가 — (a) eligibility=true에서 모든 빈 필드가 채워지고 `RateLimits`는 유지된 채 nil, (b) stdin이 fresh로 들고 온 model/workspace/cost가 cached 값으로 덮이지 않음, (c) eligibility=false에서 stdin이 그대로 유지됨, (d) cwd mismatch / cached==nil / SavedAt 만료 각각에서 eligibility=false. `go test ./...` 통과.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.5, ANALYSIS §1, ANALYSIS §5.A, ANALYSIS §5.B, ANALYSIS §5.E

- [x] task-003: `main.go` restore 블록을 단일 결정 + 2차 orchestrate 흐름으로 교체
  - 목적: degraded stdin이 들어왔을 때 projectInfo·model·context·cost 위젯이 함께 살아남거나 함께 빠지고, "절반만 멀쩡한" status line이 발생하지 않는다
  - 접근: `main.go`의 기존 restore 분기(line 91–126 부근: `restoreWorkspace` / `usageDegraded` / `costRegressed` 세 boolean과 그에 따른 분기) 전체를 제거. 새 흐름: 1차 `orchestrate` → `shouldRestoreFromSession`로 단일 판정 → true면 `fillFromSessionCache`로 빈 필드 채우고 채운 필드 마스크 보관 → 2차 `orchestrate` → 이후 기존 `shouldSuppressOutput` 분기와 출력 경로는 그대로 유지. eligibility=false이거나 cached==nil이면 1차 result를 그대로 사용. RateLimits는 어느 경로에서도 SessionState로부터 주입되지 않음(기존 invariant 유지). 본 Task에서는 채운 필드 마스크를 task-004의 save 직전 stripping에서 쓸 수 있도록 함수 외부에 노출만 해두고 stripping 본체는 task-004에서 추가.
  - 검증 조건:
    - 결과: (a) 같은 cwd에서 직전 정상 stdin이 캐시에 있고 60초 이내인 상태에서 빈 `{}` stdin을 주면 projectInfo·model·context·cost 위젯이 함께 렌더된다. (b) cwd mismatch이거나 SavedAt가 TTL 초과인 상태에서 같은 빈 stdin을 주면 cost·context만 단독으로 채워지는 출력이 발생하지 않는다(빈 출력 또는 warmup 예외만 허용). (c) stdin이 일부 fresh를 들고 와도 그 fresh 값이 캐시 값으로 덮이지 않는다.
    - 확인: `main_test.go`에 (a)/(b)/(c) 케이스를 stdin·SessionState 픽스처로 추가하여 stdout이 위 결과를 만족하는지 검증. `make test` 통과. 수동 확인: CLAUDE.md §동작 확인 예시 stdin이 변경 전과 동일한 status line을 낸다.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.4, SPEC §5.5, ANALYSIS §1, ANALYSIS §2, ANALYSIS §5.A, ANALYSIS §5.C, ANALYSIS §5.E

- [x] task-004: save 직전 cache-복원 필드 stripping
  - 목적: degraded 호출이 반복되어도 SessionState 캐시 본문의 identity·usage 값이 자기참조로 영구히 살아남지 않는다
  - 접근: `main.go`의 `saveSessionState` 호출 직전, task-003에서 추적한 "이 호출에서 캐시로 채운 필드 마스크"를 사용해 저장용 스냅샷에서 해당 필드를 다시 빈 값으로 되돌린다(`RateLimits` stripping 패턴(`main.go:156` 부근)을 일반화). eligibility=false인 호출은 stdin 원본 그대로 저장하여 현재 동작 유지. `SavedAt`은 정상적으로 갱신.
  - 검증 조건:
    - 결과: 같은 cached SessionState 상태에서 빈 stdin으로 N회 연속 실행해도 매 호출 후 `session-state-<key>.json`의 cache-복원으로 채워진 필드(Workspace 외에 Model/Cost/ContextWindow 중 캐시에서 온 것)는 저장 본문에서 다시 비워져 있다. fresh로 들어온 필드는 정상 저장된다. `SavedAt`은 호출마다 갱신된다.
    - 확인: `main_test.go`(또는 `cache_test.go`)에 반복 호출 시나리오 추가 — 첫 호출 후 캐시 본문 vs 두 번째 호출 후 캐시 본문을 비교해 캐시-복원 필드가 누적되지 않음을 검증. `go test ./...` 통과.
  - 참조: SPEC §5.6, ANALYSIS §2, ANALYSIS §5.D

- [x] task-005: cross-workspace 노출 및 warmup·suppress 회귀 보호 테스트
  - 목적: 새 restore 흐름에서도 다른 워크스페이스 캐시 노출 금지, RateLimits 미복원, warmup 예외 및 무출력 조건이 변경 전과 동일하게 유지된다
  - 접근: `main_test.go` / `cache_test.go`에 e2e 성격의 회귀 케이스 묶음 추가. (a) 현재 cwd가 cached cwd와 normalized exact-match가 아닐 때 어떤 필드도 캐시에서 복원되지 않음, (b) stdin·캐시 모두에서 identity가 없고 RateLimits만 있을 때 5h/7d 위젯만 출력되는 warmup 예외 유지, (c) stdin·캐시·RateLimits 모두 없을 때 stdout이 빈 문자열, (d) `RateLimits`가 캐시 본문에서 stripping된 상태로 저장되고 어느 복원 경로에서도 ctx에 주입되지 않음.
  - 검증 조건:
    - 결과: 위 (a)~(d) 회귀가 모두 그린.
    - 확인: `make test` 통과. 특히 cross-workspace 케이스에서 stdout에 cached cwd 문자열·cached model id·cached cost 수치 어느 것도 등장하지 않음을 grep 형태의 어서션으로 확인.
  - 참조: SPEC §5.2, SPEC §5.3, SPEC §5.4, SPEC §5.5, SPEC §5.6, ANALYSIS §4

- [x] task-006: 버전 bump (Makefile, plugin.json, api.go userAgent)
  - 목적: `/plugin` UI의 update 감지를 위해 사용자 체감 가능한 fix가 새 SemVer 버전으로 노출된다
  - 접근: CLAUDE.md §버전 정책에 따라 `Makefile`의 `VERSION`, `.claude-plugin/plugin.json`의 `version`, `api.go`의 `userAgent` 세 곳을 v0.3.8 → v0.3.9로 동시 갱신. 본 Task는 코드 변경 동반 commit과 묶일 수 있도록 task-003·004 이후 위치에 둔다.
  - 검증 조건:
    - 결과: 세 위치의 버전 문자열이 모두 v0.3.9로 동일하다.
    - 확인: `make build-local` 후 `./dist/cc-usage --version` (또는 동등 경로)에서 v0.3.9가 보고됨. `grep -r "0.3.8"` 결과에 코드·메타데이터 잔여가 없음.
  - 참조: SPEC §5.1, SPEC §5.6

- [x] task-007: `make build`로 `bin/` 재생성 및 최종 검증
  - 목적: marketplace 배포용 pre-built 바이너리가 v0.3.9 동작과 일치한다
  - 접근: `make build`로 `bin/cc-usage-{darwin,linux,windows}-{arm64,amd64}` 재생성. 로컬에서 CLAUDE.md §동작 확인 예시 stdin과 빈 `{}` stdin을 각각 흘려 status line이 spec.md §5.1·§5.2·§5.3 동작과 일치하는지 수동 확인. release 브랜치 동기화는 본 Task 범위 외(사용자 별도 진행).
  - 검증 조건:
    - 결과: `bin/` 산출물이 v0.3.9로 빌드되어 있고, 정상 stdin으로 기대 status line이 나오며, 빈 stdin + cached SessionState 조건에서 projectInfo·model·context·cost가 함께 렌더된다. cached가 없거나 TTL 초과인 빈 stdin에서는 절반 출력이 발생하지 않는다.
    - 확인: `make build` exit 0, `make test` 그린, 위 두 수동 확인 모두 기대대로. `git status`에 `bin/` 갱신 반영.
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3
