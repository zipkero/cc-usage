# spec: test-portability-and-ci

## 1. 범위

저장소의 검증 명령이 개발 머신에서 실제로 통과하게 만들고, 그 상태가 자동으로 지켜지게 한다. 두 갈래다.

- **테스트 이식성**: Windows에서만 실패하는 테스트 4개를 fixture 쪽에서 플랫폼 중립으로 고친다.
  제품 코드의 동작은 바꾸지 않는다.
- **CI 신설**: GitHub Actions 워크플로를 추가해 ubuntu·macOS·Windows 세 runner에서 `go test ./...`와
  `go vet ./...`를 실행한다.

### 입력 맥락

`20260730-001-session-start-placeholder` 착수 시점(`ea393fb`)에 이미 실패하고 있던 테스트 4개다. 깨끗한
워킹 트리에서 `go test ./...`를 돌려 확인했다.

```
TestConfigHomeDir/defaultConfigPath_falls_back_to_home
TestProjectPathCompressHome/current_under_home_gets_tilde_prefix
TestProjectPathShrink/tilde_path_over_budget_collapses_middle_to_ellipsis
TestProjectPathShrink/absolute_path_over_budget_keeps_leading_separator
```

원인은 둘이며 모두 테스트 쪽이다.

- **경로 fixture가 POSIX 전용**: fixture가 `/var/aaaaaaaa/.../proj`, `~/aaaaaaaa/.../proj`,
  `/Users/alice/projects/cc-usage` 같은 forward slash 경로를 쓰는데 `shrinkPath`·`compressHome`은
  `filepath.Separator`로 분기한다. Windows에서 그 값은 `\`이므로 fixture 경로가 구분자 없는 한 덩어리로
  취급되어 축약·틸데 치환이 일어나지 않는다.
- **`HOME` override가 Windows에 안 먹음**: 테스트가 `HOME`을 임시 디렉토리로 덮지만 Windows의
  `os.UserHomeDir()`는 `USERPROFILE`을 읽으므로 override가 무시되고 실제 홈 경로가 반환된다.

**배포 동작의 결함이 아니라는 점을 확인했다.** 실제 Windows 세션에서 Claude Code가 넘기는
`workspace.current_dir`은 `C:\Users\zipke\GolandProjects\cc-usage` 형태이고, 설치된 v0.5.5 바이너리로
실행했을 때 `~\GolandProjects\cc-usage`로 정상 축약된다. 같은 바이너리에 forward slash 경로를 넣으면
축약이 일어나지 않아 위 실패와 같은 양상이 재현되는데, 이는 실사용에서 도달하지 않는 입력이다.

저장소에 CI가 없다(`.github/workflows` 부재). 개발 머신의 `go test`가 유일한 관문인데 그것이 빨간 상태로
오래 방치됐고, 그 결과 `20260730-001`은 완료 조건을 "착수 시점 대비 실패 집합 불변"으로 우회해야 했다.

`/spec-init` 전 논의에서 검토하고 접은 접근 하나. **제품 코드를 구분자 중립으로 만드는 방향**(`shrinkPath`·
`compressHome`이 `/`와 `\`를 모두 구분자로 인정) 은 fixture를 그대로 둘 수 있지만, Windows에 forward slash
경로가 실제로 들어오는 경로가 확인되지 않아 사용자 가치가 불분명하고 M1의 성격("검증 기반 정비")을 제품
동작 변경으로 넓힌다. 이번 범위에서 제외한다(§4).

## 2. 목표

`go test ./...`와 `go vet ./...`가 개발 머신에서 실제로 통과하는 상태로 되돌리고, 이후 feature가 완료
조건을 baseline 우회로 쓰지 않게 한다. 지금은 실패 4개가 상수처럼 깔려 있어 새 회귀가 그 안에 섞여도
드러나지 않는다.

그 상태를 사람의 기억이 아니라 자동 게이트가 지키게 한다. 이번 실패가 Windows 전용이었으므로 게이트도
세 OS를 함께 봐야 같은 유형을 다시 놓치지 않는다.

## 3. 제약

- **테스트 코드만 고친다.** `config.go`·`widgets_project.go` 등 제품 코드의 동작을 바꾸지 않는다.
- Zero dependency 유지 — 테스트를 포함해 외부 모듈을 쓰지 않으며 `go.mod`에 `require` 블록이 생기지 않는다.
- 단일 `main` 패키지 유지 — 서브 패키지를 만들지 않는다.
- fixture의 플랫폼 중립화는 Go 표준 라이브러리로만 한다(`filepath.Join`, `filepath.FromSlash` 등).
  OS별 분기를 테스트에 흩뿌리는 대신 fixture 생성 자리에서 흡수한다.
- 홈 디렉토리를 덮는 테스트는 POSIX의 `HOME`과 Windows의 `USERPROFILE`을 함께 덮는다.
- CI는 GitHub Actions로 하고 runner는 `ubuntu-latest`·`macos-latest`·`windows-latest` 세 개다.
  실행 명령은 `go test ./...`와 `go vet ./...`이며, Go 버전은 `go.mod`의 선언을 따른다.
- 워크플로는 `main`으로의 push와 pull request 모두에서 동작한다.
- 이 변경은 사용자에게 노출되는 동작을 바꾸지 않으므로 CLAUDE.md §버전 정책상 SemVer bump 대상이 아니다.
  `Makefile` VERSION과 `.claude-plugin/plugin.json` version을 올리지 않고, `bin/` 재빌드와 release 브랜치
  동기화도 하지 않는다.
- 검증 명령은 `go test ./...`와 `go vet ./...`이다. 이 환경에 `make`가 없으므로 `Makefile`의 `test` 타깃과
  같은 명령을 직접 부른다.

## 4. 제외 범위

- **제품 코드를 구분자 중립으로 만드는 것.** §1에서 접은 접근이다. `shrinkPath`·`compressHome`·
  `configHomeDir`의 동작은 그대로 둔다.
- **`make release` 타깃 신설.** CLAUDE.md §배포가 미구현으로 남겨둔 항목이지만 배포 자동화는 검증 게이트와
  관심사가 달라 같은 feature에 넣으면 완료 조건이 두 갈래로 갈린다. 이 환경에 `make`가 없어 완료 조건을
  검증할 수단도 없다. 별도로 다룬다.
- **버전 정합 검사 자동화.** `Makefile` VERSION과 `plugin.json` version이 어긋나는 것을 CI가 잡게 하는 것.
  이번 CI 범위는 테스트·vet 두 명령으로 한정한다.
- **CI에 linter·보안 스캔·릴리스 자동화·커버리지 리포트 추가.** 이번에는 실패를 잡는 게이트만 만든다.
- **기존 테스트의 커버리지 확대.** 실패 4개를 통과시키는 것 외에 새 테스트 케이스를 발굴하지 않는다.
- **stdin 파싱 전면 실패 블랙아웃과 `resets_at` 처리.** ROADMAP M2 소관이며 별도 feature로 다룬다.

## 5. 완료 조건

1. Windows 개발 머신에서 `go test ./...`가 실패 없이 통과한다.
2. §1에 나열한 테스트 4개가 각각 통과한다.
3. `go vet ./...`가 통과한다.
4. `config.go`와 `widgets_project.go`가 변경되지 않는다 — 제품 코드의 동작이 그대로임이 diff로 확인된다.
5. `go.mod`에 `require` 블록이 없다.
6. `.github/workflows/` 아래 워크플로가 존재하고, `ubuntu-latest`·`macos-latest`·`windows-latest` 세
   runner에서 `go test ./...`와 `go vet ./...`를 실행한다.
7. 그 워크플로가 `main`으로의 push와 pull request 두 이벤트에서 동작하도록 선언되어 있다.
8. 워크플로가 GitHub Actions에서 실제로 실행되어 세 runner 모두 성공한다.
9. `Makefile`의 VERSION과 `.claude-plugin/plugin.json`의 version이 이 feature 전후로 동일하다.
