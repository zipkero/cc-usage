# lock-leak-cleanup — SPEC

## 1. 범위

`~/.cache/cc-usage/*.lock` 파일이 acquire-release 흐름에서 디스크에 남아 영구 누적되는 결함을 정리한다.

- `file_lock_unix.go` / `file_lock_windows.go`의 release closure가 `syscall.Flock` / `UnlockFileEx`로 advisory lock은 해제하지만 `.lock` 파일 자체를 `os.Remove`하지 않는다. fd 단위 lock이라 다음 acquire를 막지 않아 기능 회귀는 없지만 inode가 무한 누적된다.
- `api.go`의 `cleanOldCaches`와 `cache.go`의 `cleanOldSessionStates`는 `cache-*.json` / `session-state-*.json` 패턴만 글로브하여 `.lock` 접미사 파일은 어떤 cleanup 경로에서도 회수되지 않는다.

본 spec은 v0.3.2 patch에 `projectinfo-display`(`docs/20260524-003-projectinfo-display/`)와 함께 한 commit·한 release로 묶어 처리한다.

## 2. 목표

- **위생**: 활성 사용자가 cc-usage를 장기간 사용해도 `.lock` 파일이 무한 누적되지 않는다. 정상 흐름의 lock은 release 직후 디스크에서 사라지고, 비정상 종료로 leak된 stale lock도 staleness 임계값 안에 회수된다.
- **기능 보존**: lock 자체의 mutual exclusion 보장은 깨지지 않으며, 다른 프로세스가 진행 중인 lock은 cleanup의 영향을 받지 않는다.

## 3. 제약

- Zero dependency 유지, 단일 `main` 패키지 유지. 새 lock 라이브러리 도입 금지.
- 신규 상수 도입 금지. staleness 임계값은 기존 `sessionStateTTL`(300s) 또는 cleanup 측의 1시간 throttle 같은 기존 시간축을 재사용한다.
- cleanup은 기존 fire-and-forget 패턴(`go cleanOldCaches()` / `go cleanOldSessionStates()`) 안에서 처리하며 stdin 출력 경로에 동기 I/O를 추가하지 않는다.
- `file_lock_unix.go`와 `file_lock_windows.go` 양쪽 모두 동등 동작이어야 한다 (release 직후 `.lock` 제거가 한쪽에만 적용되면 안 됨).
- 본 변경의 version bump는 projectinfo-display와 묶인 v0.3.2 단일 bump로 갈음한다. lock 단독 별도 bump는 두지 않는다.
- bin/ 재빌드와 release 브랜치 sync는 projectinfo-display와 합쳐진 최종 commit/push 단계에서 한 번에 수행한다.

## 4. 제외 범위

- file lock 메커니즘 자체 변경 (`flock` → `fcntl` 등) — 본 spec 밖.
- API cache(`cache-*.json`) 또는 session-state 본체 파일의 정리 정책 변경 — 본 spec 밖.
- 새 lock 종류 추가, lock 디렉터리 위치 변경 — 본 spec 밖.
- projectinfo-display의 path 표시·위치 자유화 — 별도 spec 소관.

## 5. 완료 조건

1. 정상 acquire-release 흐름이 끝난 직후 해당 `<path>.lock` 파일이 디스크에 남지 않는다. release closure 실행 후 `os.Stat`이 `fs.ErrNotExist`를 반환한다.

2. 비정상 종료(release closure 미실행)로 디스크에 남은 stale `.lock` 파일은 다음 cc-usage 호출의 백그라운드 cleanup에 의해 staleness 임계값 초과 시 제거된다.

3. 2번 cleanup은 `session-state-*.json.lock`과 `cache-*.json.lock` 양쪽 모두를 대상으로 한다 (두 cache family의 lock이 모두 정리됨).

4. 다른 프로세스가 현재 보유 중인 `.lock` 파일(ModTime이 staleness 임계값 안)은 cleanup의 영향을 받지 않는다. cleanup 후에도 정상 in-progress lock의 mutual exclusion 보장이 유지된다.

5. `file_lock_unix.go`와 `file_lock_windows.go`의 release closure가 동등하게 `.lock` 파일을 제거한다 (한쪽 플랫폼에서만 정리되는 비대칭이 없다).

6. `make build`, `make build-local`, `go test ./...` 세 명령이 변경 전과 동일하게 exit 0으로 종료한다 (회귀 없음).
