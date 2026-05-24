# lock-leak-cleanup — ANALYSIS

## 근거

### 읽은 자료
- `docs/20260524-004-lock-leak-cleanup/spec.md` 전체 (§5.1–§5.6).
- `file_lock_unix.go` 전체 (`acquireCacheFileLock`, release closure).
- `file_lock_windows.go` 전체 (동일 추상화, `LockFileEx`/`UnlockFileEx`).
- `api.go` `cleanOldCaches`(310–335) — glob `cache-*.json`만 처리.
- `cache.go` `cleanOldSessionStates`(v0.3.1에 추가) — glob `session-state-*.json`만 처리.
- `withCacheFileLock`의 호출자: `api.go`의 `readFileCache`/`writeFileCache`, `cache.go`의 `loadSessionState`/`saveSessionState`.
- 실측: `~/.cache/cc-usage/*.lock` 75개 누적, 가장 오래된 09:19, 가장 최근 18:28.

### 코드베이스에서 확인한 사실
- 두 플랫폼 모두 release closure가 `Flock(LOCK_UN)`(또는 `UnlockFileEx`) + `file.Close()`만 호출하고 `os.Remove(lockPath)`는 없다 (`file_lock_unix.go:24-30`, `file_lock_windows.go:39-46`).
- `flock` / `LockFileEx`는 fd 단위 advisory lock이라 디스크에 남은 `.lock` 파일이 새 acquire를 막지 않는다 — 기능 회귀는 없다.
- `cleanOldCaches`의 glob `cache-*.json`은 `.lock` suffix와 매치되지 않는다 (`filepath.Glob`은 단순 패턴, suffix가 다르면 false). `cleanOldSessionStates`도 동일.
- `withCacheFileLock`의 시그니처는 (path, fn) → error. release closure 안의 동작 변경은 호출자 인터페이스에 영향이 없다.
- 호출자 4곳 모두 release closure를 `defer`로 호출 — closure가 panic-safe하면 비정상 종료 시에도 unlock은 시도된다. 단 SIGKILL이나 OS-level abort 시에는 closure가 실행되지 않아 `.lock` 파일이 디스크에 남는다.

### 추정과 미확인
- 사용자별 실 누적 속도는 측정하지 않았다. 75개/하루는 대략 일일 50~100세션 패턴에서 자연스럽다고 추정.
- Windows의 `os.Remove`가 unlocked 파일에서 항상 성공하는지는 실측 안 함 — `UnlockFileEx` + `file.Close()` 후라면 OS가 lock을 풀어 Remove가 가능하다고 가정.

---

## 1. 구조

본 변경은 새 모듈 없이 네 기존 파일 안에서 끝난다.

- **`file_lock_unix.go` / `file_lock_windows.go`의 release closure 확장** (SPEC §5.1, §5.5). 두 플랫폼이 동일한 순서로 동작해야 한다 — `Unlock` → `file.Close()` → `os.Remove(lockPath)`. Close 후 Remove여야 Windows에서 "file in use" 에러를 피한다. Remove 실패는 closure 반환값에 영향을 주지 않고 `debugLog`로만 노출한다 (다른 프로세스가 이미 cleanup으로 지웠거나 race로 사라진 케이스가 정상이다).
- **`api.go` `cleanOldCaches`의 glob 확장** (SPEC §5.2, §5.3). 기존 `cache-*.json` 외에 `cache-*.json.lock`도 동일한 staleness 기준(`time.Hour`)으로 정리한다. cache family의 lock은 cache cleanup이 자기 책임으로 본다.
- **`cache.go` `cleanOldSessionStates`의 glob 확장** (SPEC §5.2, §5.3). 기존 `session-state-*.json` 외에 `session-state-*.json.lock`도 동일한 staleness(`sessionStateTTL`)로 정리. session-state family의 lock은 session-state cleanup이 자기 책임.

cleanup 분리는 두 family의 staleness 임계값이 다르고(`time.Hour` vs `sessionStateTTL=300s`) 책임 경계를 흐리지 않기 위함 (SPEC §3: 기존 시간축 재사용).

## 2. 데이터 흐름

### 2.1 정상 acquire-release (SPEC §5.1)

```
acquireCacheFileLock(path.lock, ...)
  ├─ os.OpenFile(path.lock, O_CREATE|O_RDWR)
  ├─ syscall.Flock(fd, LOCK_EX|LOCK_NB)  (또는 LockFileEx)
  └─ return release closure

호출자: defer release()
release closure:
  ├─ syscall.Flock(fd, LOCK_UN)  (또는 UnlockFileEx)
  ├─ file.Close()
  └─ os.Remove(path.lock)         ← 신규
       (실패 시 debugLog로 무시)
```

closure 직후 `os.Stat(path.lock)`은 `fs.ErrNotExist`를 반환한다 (SPEC §5.1).

### 2.2 비정상 종료 후 stale 회수 (SPEC §5.2, §5.3, §5.4)

```
acquireCacheFileLock(path.lock, ...)
  └─ ...  (정상 흐름)
[프로세스 SIGKILL — closure 실행 못 함, .lock 디스크에 남음]

다음 cc-usage 호출:
  ├─ go cleanOldCaches()            (api.go가 자기 cache family 처리)
  │    └─ glob cache-*.json, cache-*.json.lock
  │         → ModTime > 1시간 이면 Remove
  └─ go cleanOldSessionStates()     (cache.go가 자기 session-state family 처리)
       └─ glob session-state-*.json, session-state-*.json.lock
            → ModTime > sessionStateTTL(300s) 이면 Remove
```

진행 중 lock(다른 프로세스가 보유 중이라 ModTime이 신선)은 cleanup에 의해 영향받지 않는다 (SPEC §5.4) — staleness 가드가 read-time에 자동 적용.

### 2.3 race 분석

이론적 race:
- 프로세스 A: OpenFile → Flock 사이에 프로세스 B: Remove
- A의 fd는 unlinked inode를 가리킴
- 새 acquire가 OpenFile하면 새 inode 생성 → B가 새 inode에 Flock
- 결과: A와 B가 서로 다른 inode를 잡아 mutual exclusion 실패

이 race는 release closure의 Remove와 동시에 다른 프로세스의 OpenFile이 일어날 때 가능하다. 실제 영향:
- cc-usage는 status line 호출마다 fork-exec — 동시 호출 빈도 매우 낮음
- 호출 간 OpenFile→Flock 사이 시간은 microsecond 단위
- cleanup의 Remove는 ModTime 기준 stale만 대상이라 진행 중 lock은 못 잡음
- 따라서 release closure의 Remove만 race window를 가짐, 그 window는 acquire 직후 Flock 직전 — 시간적으로 거의 0
- 실측 충돌 가능성은 무시할 수준

trade-off 자세한 비교는 §5 결정 A.

## 3. 인터페이스

- **외부 contract 변화 없음**. CLI 플래그, stdin/stdout 프로토콜, Config 스키마, locale 키, 위젯 인터페이스 모두 보존.
- **`withCacheFileLock` 시그니처 변화 없음** — release closure 안의 동작만 바뀐다. 호출자(api.go·cache.go) 코드 무변경.
- **cache 디렉터리 레이아웃 변화 없음** — `.lock` 파일이 사라지는 게 정상화이며, 기존에도 advisory였다.

## 4. 영향 범위

| 파일 | 변경 형태 |
|------|-----------|
| `file_lock_unix.go` | release closure에 `os.Remove(lockPath)` 추가 (Close 뒤, 에러 무시·debugLog) |
| `file_lock_windows.go` | 동일 — `UnlockFileEx` + `file.Close()` 뒤에 `os.Remove(lockPath)` |
| `api.go` | `cleanOldCaches` glob에 `cache-*.json.lock` 추가, 동일한 ModTime 기준(`time.Hour`)으로 정리 |
| `cache.go` | `cleanOldSessionStates` glob에 `session-state-*.json.lock` 추가, 동일한 ModTime 기준(`sessionStateTTL`)으로 정리 |

호환성:
- `withCacheFileLock` 시그니처와 호출자 무변경.
- v0.3.1 사용자가 남긴 stale `.lock` 75개는 본 패치 적용 후 첫 cleanup 사이클에서 ModTime 기준 stale로 분류되어 회수된다 (대다수가 1시간+ 또는 5분+ stale이므로 첫 호출에 정리 시작).
- session-state·api cache JSON 파일 자체와 무관 — 본체 정리 흐름은 변경 없다.
- v0.3.2 bump는 본 spec 자체로는 트리거하지 않는다. projectinfo-display와 같은 commit·release에 묶음 처리하므로 version 갱신은 projectinfo-display task-004가 단독 책임.

## 5. Decision Points

### A. 정리 시점 선택 — release immediate vs cleanup-only vs both (SPEC §5.1, §5.2)

옵션:
- **A1 — release closure에서 즉시 Remove**: 정상 흐름의 lock 파일을 즉시 정리. cleanup 없이 잘 작동하지만 비정상 종료된 stale은 영원히 남음.
- **A2 — cleanup glob 확장만**: release closure는 손대지 않고 cleanup이 stale만 회수. 정상 흐름의 lock 파일이 cleanup 사이클(1시간 throttle + ModTime 임계값) 사이에 누적됨.
- **A3 — 둘 다 (Option C)**: 정상 흐름은 즉시 정리, 비정상 종료 stale은 cleanup으로 회수.

트레이드오프:
- A1: 가장 단순. race 우려(§2.3) 존재하나 실측 무시 가능. 단, 비정상 종료 stale을 청소하지 않으면 SPEC §5.2 미달.
- A2: race 없음. 그러나 정상 종료된 lock도 1시간 throttle + ModTime 안에 누적 — SPEC §5.1 ("release 직후 디스크에 남지 않는다") 미달.
- A3: 정상 흐름 SPEC §5.1과 비정상 종료 SPEC §5.2를 동시 충족. race window는 release closure 안 Remove와 cleanup의 stale Remove 두 곳에 존재하나 둘 다 실효 영향 무시 가능.

**채택: A3**. SPEC §5.1과 §5.2를 모두 도달 가능하게 만드는 유일한 옵션. race 우려는 cc-usage의 fork-exec per call 특성상 동시 호출 빈도가 매우 낮아 실측 0에 가깝다.

### B. cleanup 글로브 확장 위치 (SPEC §5.3)

옵션:
- **B1 — 각 cleanup 함수에 자기 family lock 추가**: `cleanOldCaches`가 `cache-*.json.lock` 추가, `cleanOldSessionStates`가 `session-state-*.json.lock` 추가.
- **B2 — 새 `cleanOldLocks` 통합 함수**: 두 family lock을 한 곳에서 정리.

트레이드오프:
- B1: 각 cleanup이 자기 cache family의 lock도 책임 — 책임 경계 일치. 두 family의 staleness 임계값이 다르므로(`time.Hour` vs `sessionStateTTL`) 같은 함수에서 다른 임계값을 분기 처리할 필요 없음.
- B2: 통합 위치는 단일하지만 임계값 분기가 필요해진다. cache.go가 api cache까지 알게 되거나 그 반대 — 경계 흐려짐.

**채택: B1**. 각 cleanup 함수가 자기 cache family의 lock을 같은 staleness로 정리한다. 함수 시그니처·호출 위치 변화 없음.

### C. Remove 실패 처리 (SPEC §5.1, §5.5)

옵션:
- **C1 — 완전 무시** (`_ = os.Remove(...)`).
- **C2 — debugLog로 노출**, 그러나 closure 반환값은 변경 없음.

트레이드오프:
- C1: 가장 짧다. 그러나 lock leak 디버깅 시 silent.
- C2: 비용 한 줄. DEBUG=cc-usage 활성 시에만 노출되어 일반 stdout은 오염 없음 (CLAUDE.md §출력: "stderr는 모든 debug/error 출력").

**채택: C2**. lock cleanup 진단 가치 vs 비용이 명확히 C2 우세. Remove 실패가 이미 다른 프로세스(cleanup or 동시 release)가 지운 정상 race 가능성도 있으므로 closure 반환값은 영향받지 않게 분리.

### D. release closure 단계 순서 (SPEC §5.5)

옵션:
- **D1 — Unlock → Close → Remove**.
- **D2 — Unlock → Remove → Close**.
- **D3 — Close → Unlock → Remove** (close가 unlock을 부수효과로 한다고 가정).

트레이드오프:
- D2: open fd가 있는 파일을 Remove. POSIX는 inode가 fd close까지 살아 있어 정상 동작하지만, Windows에서 unlink가 "sharing violation" 에러를 낼 수 있다 — 이식성 위험.
- D3: close가 unlock을 보장하는지는 POSIX/Windows 둘 다 명확한 contract가 아니다. 명시 unlock이 안전.
- D1: 가장 보수적. Unlock으로 lock 해제 명시, Close로 fd 정리, 그 다음 unlinked inode가 아닌 일반 path로 Remove 시도. Windows에서도 안전.

**채택: D1**. 두 플랫폼 file_lock 모두 같은 순서를 적용해 SPEC §5.5(플랫폼 동등) 충족.
