# lock-leak-cleanup — IMPLEMENT

- [x] task-001: `file_lock_unix.go` / `file_lock_windows.go` release closure에 `os.Remove(lockPath)` 추가
  - 목적: 정상 acquire-release 흐름이 끝난 직후 해당 `<path>.lock` 파일이 디스크에 남지 않는다. release closure 실행 후 `os.Stat(lockPath)`이 `fs.ErrNotExist`를 반환한다. 두 플랫폼(Unix, Windows)이 동일한 단계 순서(`Unlock → Close → Remove`)로 동작한다. Remove 실패는 closure 반환값에 영향을 주지 않고 `debugLog`로 stderr에만 기록되어 다른 프로세스가 이미 cleanup이나 동시 release로 지운 정상 race 케이스를 silent로 두지 않는다
  - 접근: 두 파일의 release closure 안에서 기존 `Unlock` + `file.Close()` 뒤에 `if err := os.Remove(lockPath); err != nil && !errors.Is(err, fs.ErrNotExist) { debugLog("lock", "remove %s failed: %v", lockPath, err) }`를 추가한다. `lockPath`는 `acquireCacheFileLock` 매개변수로 이미 closure scope에 있다. 단계 순서를 정확히 `syscall.Flock(LOCK_UN)` → `file.Close()` → `os.Remove(lockPath)`로 둔다 (Windows에서도 `UnlockFileEx` → `file.Close()` → `os.Remove(lockPath)`). 기존 `Unlock` 실패·`Close` 실패의 closure 반환 로직은 그대로 둔다. Remove는 별도 분기로 추가하며 기존 에러 우선순위를 깨지 않는다
  - 검증 조건:
    - 결과:
      - 두 파일의 release closure가 동일 단계 순서로 동작 (직접 코드 inspection)
      - 정상 release 후 `~/.cache/cc-usage/<path>.json.lock` 파일이 존재하지 않음
      - Remove 실패 시 stderr에 `[cc-usage:lock]` debug 메시지가 나오고 closure는 정상 종료
    - 확인: `go vet ./...`, `go build ./...`, `go test ./...` 모두 exit 0. task-003의 release 회귀 테스트 PASS
  - 참조: SPEC §5.1, SPEC §5.5, SPEC §5.6, ANALYSIS §5.A, ANALYSIS §5.C, ANALYSIS §5.D

- [x] task-002: `api.go` `cleanOldCaches`와 `cache.go` `cleanOldSessionStates` glob 확장
  - 목적: 비정상 종료로 디스크에 남은 stale `.lock` 파일이 다음 cc-usage 호출의 백그라운드 cleanup에 의해 staleness 임계값 초과 시 제거된다. cleanup은 `session-state-*.json.lock`과 `cache-*.json.lock` 양쪽 모두를 대상으로 한다. 진행 중인(ModTime이 신선한) `.lock` 파일은 cleanup에 의해 잘못 제거되지 않는다. 각 cleanup 함수가 자기 cache family의 lock만 처리해 책임 경계가 흐려지지 않는다
  - 접근: `api.go`의 `cleanOldCaches`에서 `filepath.Glob`을 `cache-*.json`과 `cache-*.json.lock` 두 패턴 결과를 합쳐 처리하도록 확장한다. staleness 임계값은 기존 `time.Hour` 그대로 사용. 동일하게 `cache.go`의 `cleanOldSessionStates`에서도 `session-state-*.json`과 `session-state-*.json.lock` 두 패턴을 처리하며 임계값 `sessionStateTTL`(300s) 그대로 사용. ModTime > 임계값 가드는 기존 흐름 재사용 — 신선한 lock은 자동 보호됨. 두 cleanup 함수의 시그니처·호출 위치·fire-and-forget 패턴 모두 변경 없음
  - 검증 조건:
    - 결과:
      - `cleanOldCaches`가 stale `cache-*.json.lock`을 제거하고 fresh한 것은 보존
      - `cleanOldSessionStates`가 stale `session-state-*.json.lock`을 제거하고 fresh한 것은 보존
      - 각 함수가 다른 family의 파일(`session-state-*.json.lock`은 cleanOldCaches가 보면 안 됨, 반대도 동일)에는 영향을 주지 않음
      - 두 함수 모두 호출자(main.go)에서 fire-and-forget goroutine으로 그대로 실행되며 stdin 출력 경로를 차단하지 않음
    - 확인: `go vet ./...`, `go build ./...`, `go test ./...` 모두 exit 0. task-003의 cleanup 회귀 테스트 PASS
  - 참조: SPEC §5.2, SPEC §5.3, SPEC §5.4, SPEC §5.6, ANALYSIS §5.A, ANALYSIS §5.B

- [x] task-003: lock 정리 회귀 테스트 작성
  - 목적: task-001의 release 시점 Remove와 task-002의 cleanup glob 확장이 의도한 케이스(정상 release / stale lock 회수 / fresh lock 보존 / family 격리)에 대해 회귀하지 않음을 자동 검증한다
  - 접근: 두 회귀 영역을 분리해 두 테스트 함수로 작성한다.
    - `TestWithCacheFileLockRemovesLockFile` — `t.TempDir()` 기반 임시 경로에서 `withCacheFileLock`을 정상 흐름으로 호출 후, 호출 종료 시점에 `<path>.lock`이 `fs.ErrNotExist`를 반환하는지 확인. Unix/Windows 양쪽에서 동일 동작 — build tag 분기 없이 한 테스트로 검증 가능.
    - `TestCleanOldSessionStatesRemovesStaleLock` 확장 또는 신규 `TestCleanOldSessionStatesHandlesLocks` — 기존 `TestCleanOldSessionStates`(v0.3.1) 구조를 따라 `t.Setenv("HOME", t.TempDir())`로 격리하고, fixture 4종(`session-state-stale.json` stale / `session-state-fresh.json` fresh / `session-state-stale.json.lock` stale / `session-state-fresh.json.lock` fresh / `cache-old.json.lock` 다른 family)을 만들어 ModTime을 `os.Chtimes`로 명시. 호출 후 stale 2종은 삭제, fresh와 다른 family는 보존됨을 단언. throttle 변수(`lastSessionStateCleanup`)를 zero로 리셋 + `t.Cleanup`으로 복원하는 기존 패턴 그대로.
    - 동일 패턴으로 `cleanOldCaches`도 `cache-*.json.lock` 처리 회귀 테스트 — `api_test.go` 또는 `cache_test.go`에 추가.
  - 검증 조건:
    - 결과: 위 세 시나리오(release 후 lock 부재 / cleanup이 stale lock 제거 / cleanup이 fresh와 다른 family 보존)가 모두 자동 검증에 PASS한다
    - 확인: `go test -run TestWithCacheFileLockRemovesLockFile -v ./...` PASS / `go test -run TestCleanOldSessionStatesHandlesLocks -v ./...` PASS / `go test ./...` 전체 회귀 없음
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.4, SPEC §5.6
