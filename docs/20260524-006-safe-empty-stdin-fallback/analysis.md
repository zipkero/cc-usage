# ANALYSIS — safe-empty-stdin-fallback

기준 코드: v0.3.6 hotfix HEAD (`1e6e0c1`). spec.md §5 완료 조건 매핑.

## 1. 현재 동작 정리

### 1.1 빈 stdin 흐름 (`main.go:33–161`)

```
parseStdin                 -> StdinInput{} (전부 zero value)
sessionCacheKey(input)     -> ""  (session_id/remote/agent/transcript/cwd 전부 비어 있음)
loadSessionState("")       -> nil (sessionStatePath("")가 "" 반환으로 단락)
orchestrate(ctx)           -> cost(=0)와 rateLimit만, result.WidgetCount = 1~2
cached == nil              -> degraded restore 블록 전체 skip
shouldSuppressOutput       -> noIdentity=true. rateLimits 있으면 통과(partial), 없으면 묵음
saveSessionState           -> cacheKey="" -> 단락 (저장 안 됨)
```

핵심: **빈 stdin 호출에서 키 매칭·저장이 모두 단락**. v0.3.4부터 일관. 직전 정상 캐시가 디스크에 있어도 키가 비어 매칭 불가.

### 1.2 정상 stdin baseline

`sessionCacheKey`는 `session_id > remote.session_id > agent_id > transcript_path > cwd` 순(`cache.go:39-58`). 일반 호출은 거의 항상 `session_id`로 떨어져 cwd 기반 키는 거의 만들어지지 않음. 즉 **정상 흐름의 키 namespace와 빈 stdin fallback 후보 키 namespace가 다르다**.

### 1.3 v0.3.5 회귀의 본질

`loadMostRecentSessionState`가 `~/.cache/cc-usage/session-state-*.json` 전체에서 newest mtime만 골라 매칭. 워크스페이스 A 캐시 저장 직후 B에서 빈 stdin 진입 시 A 캐시가 B 화면에 노출. **현재 워크스페이스 식별 신호가 매칭 입력으로 사용되지 않은 것**이 근본 원인.

### 1.4 빈 stdin에서 stdin 자체로 워크스페이스 식별 불가

`Workspace.CurrentDir`도 빈 stdin에서는 비어있음. fallback이 유효하려면 stdin 외부 신호(env / 프로세스 CWD)에 의존해야 한다.

## 2. 식별 신호 가용성·신뢰도

| 신호 | 가용성 | 매핑 신뢰도 | 비고 |
|---|---|---|---|
| stdin `session_id` 등 | 빈 stdin에서 0% | (가용 시) 100% | 빈 stdin 케이스엔 의미 없음 |
| `CLAUDE_PROJECT_DIR` env | 미확정 (status line 호출 시점 실측 필요) | (가용 시) 100% | Claude Code가 의도해 주입 |
| `os.Getwd()` | 100% (syscall) | 불확정 | Claude Code가 워크스페이스로 chdir 했는지 의존 |
| `PWD` env | 미확정 | `Getwd()`보다 낮음 | stale 가능 |

본 환경에서의 부분 실측: Bash 도구 환경에서 `CLAUDE_PROJECT_DIR`은 비어있고 `PWD=/Users/zipkero/GolandProjects/cc-usage`. 단, 이는 status line 호출 컨텍스트와 다른 컨텍스트의 측정이라 단정 불가. → §결정 D1.

## 3. 매칭 전략 옵션

### 3.1 옵션 A — `sessionCacheKey` 보강 (cwd 추정 키 단일화)

빈 stdin에서도 §2 신호로부터 비-빈 키(예: `cwd-<hash>`)를 만들도록 보강. 정상 stdin과 빈 stdin이 같은 워크스페이스 → 같은 키.

단점: 정상 stdin의 우선 키(`session_id`)와 fallback 키(`cwd-<hash>`)가 서로 다른 파일에 저장 → 정상 호출 직후 빈 stdin이 와도 매칭 미스. 해결하려면 **정상 호출에서 cwd 사본도 추가 저장** 필요 → 파일 수 워크스페이스당 2배.

### 3.2 옵션 B — 별도 fallback 함수 (cwd 일치 후보 검색)

`sessionCacheKey` 무변경. `cacheKey == ""`인 경우에만 별도 `loadByWorkspaceCwd(cwd)`:

1. §2 신호로 cwd 결정. 빈 값이면 fallback 미발동.
2. `session-state-*.json` glob → 각 파일 unmarshal → `CachedStdin.Workspace.CurrentDir`이 cwd와 일치(정규화 후)하는 비-만료 후보 추출.
3. 0개 → 미복원. 1개 이상 → newest mtime 선택.

단점: 디스크 scan + read+unmarshal. 일반적으로 한 자릿수 파일이라 비용 미미.
v0.3.5와의 차이: **cwd 일치 조건 강제**. mtime은 동률 결정용으로만.

### 3.3 옵션 C — A + B 혼합

정상 호출 저장 시 추가로 cwd 키 사본 저장 + 빈 stdin은 cwd 키로 직행. lookup O(1)이지만 저장 부담 2배 고정.

### 3.4 비교

| 항목 | A | B | C |
|---|---|---|---|
| 정상 흐름 출력 변경 | 없음 | 없음 | 없음 |
| 정상 흐름 저장 부담 | 2배 | 변경 없음 | 2배 고정 |
| 빈 stdin lookup 비용 | O(1) | O(파일 N개 read) | O(1) |
| cross-workspace 노출 | cwd 키 단일화로 0 | cwd 일치 조건으로 0 | 0 |
| 회귀 표면 | 한 곳 | 한 곳 (격리) | 두 곳 |
| 정확성 | 동등 | 동등 | 동등 |

→ §결정 D2. 분석자 권장: **B** — 정상 흐름 저장 부담 0, fallback 함수 하나로 회귀 표면 격리, SPEC §5.4·§5.8 baseline 보존 검증이 가장 단순.

## 4. 다중 워크스페이스 시퀀스 안전성 (SPEC §5.9)

### 4.1 cross-workspace 노출의 일반화된 경로

- 매칭 입력에 현재 워크스페이스 식별 신호 부재 (v0.3.5).
- cwd 비교가 prefix·substring 형태일 때 monorepo 서브패키지 오매칭.
- cwd 정규화 부족으로 `/private` 접두·trailing slash 표기 차이가 false negative → 부정확 후보로 떨어짐.

### 4.2 안전 매칭 조건 (옵션 공통)

- **cwd 정확 일치만 매칭**. prefix/substring/case-insensitive 금지.
- 정규화 절차:
  - `filepath.Clean`으로 trailing slash·`./` 제거.
  - macOS `/private` 동등성 → `filepath.EvalSymlinks` 1회 + 실패 시 raw로 폴백 (→ D4).
  - `~`는 사용 안 함(stdin·env 둘 다 절대 경로 가정).
- 캐시 저장 시점에도 동일 정규화 적용해야 매칭 성립.

### 4.3 시나리오 검증

| t | 입력 | 기대 |
|---|---|---|
| t0 | A 정상 | A 캐시 저장 |
| t1 | A 빈 | A 복원 |
| t2 | B 정상 | B 캐시 저장 |
| t3 | B 빈 | B 복원 (A 절대 노출 금지) |
| t4 | A 빈 | A 복원 |
| t5 | C 빈 (캐시 없음) | 미복원, 보수적 출력 |
| t6 | cwd 신호 부재 빈 | 미복원, 보수적 출력 |

t3·t5·t6이 SPEC §5.9의 핵심 검증 지점.

## 5. TTL 정책

두 개의 분리된 TTL.

### 5.1 `sessionStateTTL` = 300s (변경 없음)

session-state 캐시 파일 자체의 만료 기준. fallback도 이 값을 따른다.
- 더 짧으면 SPEC §5.1 위반(TTL 안인데 깜빡임).
- 더 길면 stale 데이터가 cwd 매칭으로 매칭되어도 정확성 문제는 없으나 cost·context 등이 너무 오래된 값일 수 있음.

### 5.2 `workspaceRestoreTTL` (SPEC §5.11)

v0.3.4에서 30s → `sessionStateTTL`(300s)로 늘렸으나 **같은 session 안에서 사용자가 `cd`로 다른 워크스페이스로 이동한 후 workspace만 빈 stdin이 오면 stale cwd가 최대 5분간 노출**되는 부작용이 보고됨. 두 가지 보완:

- (a) **값 축소**: v0.3.3 이전의 30s로 되돌리거나, 절충안인 60s. 짧을수록 stale 노출 시간 창이 짧음.
- (b) **cwd 일치 가드 추가**: §3.2의 cwd 추정 신호로 현재 cwd를 잡고, cached `Workspace.CurrentDir`이 일치하지 않으면 workspaceRestoreTTL 이내라도 복원하지 않음. 이 가드만 있으면 TTL 값과 무관하게 stale 노출 0이 가능.

분석자 권장: **(a) + (b) 둘 다**. 가드만 두면 정확성은 확보되지만 신호 부재 시 보수적 단락 비율이 늘어남. 값을 단축(60s)해 fallback 발동 빈도를 약간만 낮추고 가드로 정확성 보장. → §결정 D5.

`workspaceRestoreTTL`은 cache.go:22의 상수.

## 6. 저장 측 변경

### 6.1 RateLimits 제외 (SPEC §5.5)

`main.go:155`의 `snapshot.RateLimits = nil`이 이미 적용. fallback 발동해도 `CachedStdin.RateLimits`는 항상 nil. ctx.RateLimits는 API 캐시에서만 채워짐. **추가 변경 불필요**. 단위 테스트 1건 추가는 IMPLEMENT.

### 6.2 빈 stdin 저장 정책

옵션 B(권장)는 `sessionCacheKey` 무변경 → 빈 stdin은 cacheKey="" → 기존 가드(빈 키 단락)로 저장 안 됨. **변경 불필요**.
옵션 A 채택 시에는 빈 stdin이 정상 호출 캐시를 덮어쓰지 않도록 별도 가드 필요(stdin 비식별 시 저장 skip).

## 7. debugLog 책임 (SPEC §5.6)

- 빈 stdin + 매칭 적중: `empty stdin -> matched cache via cwd=<cwd> source=<env|getwd|pwd> path=<filename>`
- 빈 stdin + 매칭 없음: `empty stdin -> no cache for cwd=<cwd> source=<...>`
- 빈 stdin + 식별 실패: `empty stdin -> no cwd signal (env miss, getwd=<val>) -> suppress/partial`
- TTL 초과: 기존 `session state expired` 로그 cover.

stderr only. cwd 값 노출은 DEBUG 한정이므로 허용.

## 8. 회귀 방지 테스트 골격

### 8.1 SPEC §5.7 네 경로

| 경로 | 입력 | 기대 |
|---|---|---|
| (a) 식별 + 적중 | 빈 stdin, cwd=X, X 캐시 비-만료 존재 | full 복원 |
| (b) 식별 + 부재 | 빈 stdin, cwd=X, X 캐시 없음 (다른 워크스페이스 캐시는 있음) | 미복원, cross-workspace 0 |
| (c) 식별 실패 | 빈 stdin, cwd 신호 부재 | 미복원, 보수적 출력 |
| (d) TTL 초과 | 빈 stdin, cwd=X, X 캐시 mtime 6분 전 | 미복원 |

### 8.2 SPEC §5.8 v0.3.4 baseline 보존

- `shouldSuppressOutput` 4 케이스 유지.
- `restoreUsageFields` 서브테스트 유지.
- `cleanOldSessionStates` glob 3패턴 + legacy 유지.
- `cleanOldCaches` 무조건 호출 유지.

### 8.3 SPEC §5.9 멀티-워크스페이스 시퀀스

임시 캐시 디렉터리에서 A→B→A 시퀀스 시뮬레이션. 각 단계 후 cross-workspace 노출 0회 어서션.

### 8.4 함수 분리 (테스트성)

옵션 B 채택 가정:
- `detectCurrentCwd(env, getwd) string` — 신호 우선순위 적용. 함수 변수로 fake injection 가능.
- `loadByWorkspaceCwd(dir, cwd, now) *SessionState` — fallback 함수 본체.
- `normalizeCwd(raw) string` — 정규화 공통.

## 9. v0.3.4 baseline 보존 평가 (SPEC §5.4)

옵션 A·B·C 모두에서 4개 baseline 동작이 보존됨. 정상 stdin은 어떤 옵션에서도 fallback 경로를 건드리지 않음.

## 10. 비-기능 요구 정합

- zero dep: 표준 라이브러리만 (`os`, `path/filepath`, `time`, `crypto/sha256`).
- 단일 main 패키지 유지.
- stdout 위젯 전용, stderr debugLog.
- 호출당 오버헤드: 옵션 B의 scan도 일반 한 자릿수 파일. `cacheLockTimeout = 200ms`와 동일 차수.
- 동시성: `withCacheFileLock` 그대로.

## 11. 버전 정책 (SPEC §5.10)

`Makefile` VERSION + `.claude-plugin/plugin.json` version + `api.go` userAgent 세 곳을 같은 새 SemVer patch로 갱신. v0.3.6 → v0.3.7 권장. 본 phase에서 값 확정 안 함.

## 12. 결정 사항 (IMPLEMENT 입력)

| ID | 결정 |
|---|---|
| D1 | 워크스페이스 식별 신호: **`CLAUDE_PROJECT_DIR` env 우선, 부재 시 `os.Getwd()` 폴백**. 둘 다 부재하거나 빈 값이면 fallback 미발동(보수적 단락). `PWD` env는 사용하지 않는다. |
| D2 | 매칭 전략: **옵션 B**. `sessionCacheKey` 무변경. `cacheKey == ""`일 때만 별도 `loadByWorkspaceCwd(cwd)` 호출. |
| D3 | cwd 비교: **정규화된 정확 일치만**. subpath/substring/case-insensitive 금지. |
| D4 | cwd 정규화: **`filepath.EvalSymlinks` 1회 + 실패 시 `filepath.Clean` 결과로 폴백**. 캐시 저장 시점에도 동일 정규화 적용. |
| D5 | `workspaceRestoreTTL` = **60s**. cwd 일치 가드(SPEC §5.11)도 함께 도입. 가드가 정확성을 보장하고 TTL 단축이 stale 노출 시간 창을 추가로 줄임. |

## 13. IMPLEMENT 진입 시 예상 변경 범위

implement.md가 결정할 task 분할의 입력. 본 phase에서는 task 형식을 정하지 않는다.

대상 파일 (예측):
- `cache.go` — `loadByWorkspaceCwd`, `normalizeCwd`, `detectCurrentCwd` 신규 + `workspaceRestoreTTL` 값 변경 + 저장 시점 cwd 정규화.
- `main.go` — `cacheKey == ""` 경로에서 fallback 호출 + workspace 복원 시 cwd 일치 가드.
- `cache_test.go` / `main_test.go` — SPEC §5.7 네 경로, §5.9 multi-workspace 시퀀스, §5.11 cd 시나리오, §5.5 RateLimits 격리.
- `Makefile`, `.claude-plugin/plugin.json`, `api.go` — version bump (v0.3.6 → v0.3.7).
