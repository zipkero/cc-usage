# test-portability-and-ci — IMPLEMENT

- [x] task-001: 경로 축약 테스트 fixture를 OS 중립 표기로 전환
  - 목적: 홈 하위 경로가 `~` 접두로 축약되는지, 예산을 넘는 경로가 중간을 `…`로 접는지 검사하는
    세 서브테스트가 Windows 개발 머신에서 통과하고, POSIX에서는 지금 통과하는 판정이 바이트
    단위로 그대로 유지된다.
  - 접근: `TestProjectPathCompressHome`의 `home` 상수와 `TestProjectPathShrink`의 장문 경로 두
    개를 `filepath.FromSlash`로 감싸고, `home`에서 파생되는 입력·기대값은 `filepath.Join`으로
    조립하며, 구분자를 담은 기대 문자열(`~/projects/cc-usage`, `~/…/proj`, `/…/proj`)에도 같은
    `FromSlash` 변환을 적용한다. 구분자가 없는 기대값(`~` 단독, `longBase` 단독)과 파생되지
    않는 통과 중 리터럴은 그대로 둔다.
  - 검증 조건:
    - 결과: `TestProjectPathCompressHome`·`TestProjectPathShrink`의 모든 서브테스트가 통과한다.
      실패하던 세 서브테스트(`current under home gets tilde prefix`,
      `tilde path over budget collapses middle to ellipsis`,
      `absolute path over budget keeps leading separator`)가 Windows에서 각각
      `~\projects\cc-usage`·`~\…\proj`·`\…\proj`를 만들어 통과하고, 이미 통과하던 서브테스트
      (`current equals home`, `current outside home`, `empty home`, `length within budget`,
      `base name alone`)도 그대로 통과한다. 50 rune 예산을 전제한 precondition 검사 세 개가
      변환 후에도 성립해 `t.Fatalf`로 끊기지 않는다. 변경은 테스트 파일 안에서 끝나고
      `compressHome`·`shrinkPath`의 signature·동작은 그대로다.
    - 확인: `go test -run "TestProjectPathCompressHome|TestProjectPathShrink" -v ./...`로 각
      서브테스트가 PASS인지 본다. `go vet ./...` 무출력. `git diff --name-only`에
      `widgets_project.go`·`config.go`가 나타나지 않는 것으로 제품 코드 무변경을 대조한다.
      POSIX 쪽 불변은 `filepath.FromSlash`가 `Separator == '/'`에서 항등이라는 성질에 의존하며,
      `GOOS=linux`·`GOOS=darwin`에서 `go vet ./...`와 `go test -c`가 성공하는 것으로 컴파일
      수준까지 확인한다(실제 POSIX 실행 관측은 CI 실행에서 닫힌다).
  - 참조: SPEC §5.2, §5.4 / ANALYSIS §5 A, B, C, E

- [x] task-002: 홈 디렉토리 override를 두 환경변수로 확장하고 전체 스위트를 초록으로 만든다
  - 목적: 홈을 임시 디렉토리로 덮는 설정 경로 테스트가 Windows에서도 실제 홈이 아니라 그 임시
    디렉토리를 기준으로 판정되고, 저장소 전체 테스트 스위트가 실패 없이 통과한다.
  - 접근: `main_test.go` 파일 수준에 `HOME`과 `USERPROFILE`을 같은 값으로 함께 `t.Setenv`하는
    헬퍼를 두고(`t.Helper()` 호출 + 왜 키가 두 개인지 적은 doc 주석), `defaultConfigPath`를
    부르는 서브테스트 두 곳의 `t.Setenv("HOME", ...)`를 그 헬퍼 호출로 바꾼다. `runtime.GOOS`
    분기를 만들지 않고 기대값은 기존 `filepath.Join` 조립을 유지한다.
  - 검증 조건:
    - 결과: `TestConfigHomeDir`의 네 서브테스트가 모두 통과하고, `defaultConfigPath falls back
      to home`이 실제 사용자 홈이 아니라 `t.TempDir()` 아래
      `<temp>/.claude/cc-usage.json`(OS 고유 구분자)을 얻는다. `defaultConfigPath uses
      CLAUDE_CONFIG_DIR`도 같은 헬퍼로 홈을 덮은 상태에서 통과한다. `go test ./...`가 실패
      0으로 `ok`이고 `go vet ./...`가 무출력이다. `go.mod`에 `require` 블록이 없고 `go.sum`이
      생기지 않는다. 이 시점까지의 변경 파일은 테스트 파일 두 개와 feature 문서뿐이다.
    - 확인: `go test -run TestConfigHomeDir -v ./...`로 네 서브테스트 PASS를 보고, `go test
      ./...` 전체를 돌려 출력에 `FAIL`이 없는 것을 확인한다(이 feature는 착수 시점 baseline
      우회를 쓰지 않으므로 실제로 `ok`여야 한다). `go vet ./...` 실행. `go.mod`를 읽어
      `require` 부재를 확인하고 `git status --short`로 `go.sum`이 생기지 않은 것을 본다.
      `git diff --name-only`에 `config.go`·`widgets_project.go`가 없음을 대조한다.
  - 참조: SPEC §5.1, §5.2, §5.3, §5.4, §5.5 / ANALYSIS §5 D

- [x] task-003: 세 OS에서 vet·test를 돌리는 GitHub Actions 워크플로 신설
  - 목적: 저장소에 워크플로 파일이 생겨, `main`으로의 push와 `main`을 base로 하는 pull request
    에서 ubuntu·macOS·Windows 세 러너가 각각 `go vet ./...`와 `go test ./...`를 돌리도록
    선언된다.
  - 접근: `.github/workflows/` 아래 워크플로 파일 하나를 만든다. 단일 job을 OS 3개 matrix로
    전개하고 `fail-fast`를 끄며, `actions/checkout` → `actions/setup-go`(major 태그 고정,
    `go-version-file: go.mod`, `cache: false`) → vet step → test step 순으로 두고 test step에는
    취소되지 않았을 때 실행하는 조건을 붙인다.
  - 검증 조건:
    - 결과: 워크플로가 `on.push`의 브랜치로 `main`, `on.pull_request`의 base 브랜치로 `main`을
      선언한다. 단일 job의 matrix OS 값이 `ubuntu-latest`·`macos-latest`·`windows-latest` 셋이고
      `fail-fast`가 `false`다. `actions/checkout`·`actions/setup-go`가 major 태그(`v7`)로
      고정되고 setup-go가 `go-version-file: go.mod`와 `cache: false`를 받는다. `go vet ./...`와
      `go test ./...`가 서로 다른 step으로 있고 vet이 먼저 오며, test step에 vet 실패에도
      실행되는 조건이 붙는다. 파일 전체에 `shell:` 키, OS별 조건 분기, gofmt·linter·커버리지
      step이 없고 권한은 저장소 콘텐츠 읽기로 한정된다. 저장소 쪽에 새 스크립트나 `Makefile`
      타깃이 생기지 않는다.
    - 확인: 워크플로 파일을 읽어 위 키·값을 하나씩 대조하고, `git status --short`로 새로 생기는
      경로가 `.github/workflows/` 아래 파일 하나뿐임을 확인한다. `go test ./...`와
      `go vet ./...`를 다시 돌려 task-002의 초록 상태가 유지되는 것을 본다. YAML 자체의 유효성과
      step의 실제 성공은 이 선언 검사로 닫지 않고 원격 실행 관측에 맡긴다.
  - 참조: SPEC §5.6, §5.7 / ANALYSIS §5 F, G, H, I, J, K

- [ ] task-004: 워크플로 실제 실행으로 세 러너 성공을 관측
  - 목적: 이 feature의 변경이 `main`에 올라가 워크플로가 실제로 한 번 실행되고 ubuntu·macOS·
    Windows 세 러너가 모두 성공하며, 배포 버전 문자열은 이 feature 전후로 그대로다.
  - 접근: 이 feature의 변경(테스트 파일 2개 + 워크플로 + feature 문서)을 commit한 뒤 `main`에
    직접 push해 push 트리거 실행을 관측한다. push와 원격 실행 조회는 원격에 영향을 주는 작업
    이므로 commit까지만 먼저 하고, push 실행 직전에 사용자 확인을 받는다.
  - 검증 조건:
    - 결과: push 대상 diff에 `Makefile`·`.claude-plugin/plugin.json`·`bin/`이 등장하지 않고 두
      파일의 버전 문자열이 여전히 `0.5.5`로 서로 같다. push 후 워크플로 실행 하나가 세 leg으로
      전개되고 세 leg의 결론이 모두 success이며, 각 leg에서 vet step과 test step이 둘 다 성공
      으로 남는다. `release` 브랜치는 갱신되지 않고 `bin/` 재빌드도 일어나지 않는다.
      `pull_request` 트리거는 실행이 아니라 선언으로만 확인된다.
    - 확인: push 전에 `git diff --name-only <feature 시작 커밋>..HEAD`로 버전 파일·`bin/`·
      제품 코드 부재를 대조하고, `Makefile`의 `VERSION`과 `.claude-plugin/plugin.json`의
      `version`을 직접 읽어 둘 다 `0.5.5`임을 확인한다. 사용자 확인을 받은 뒤 push하고,
      `gh run list --branch main --limit 1`과 `gh run view <run-id>`로 세 leg의 결론과 step
      결과를 본다. `git branch -a`로 `release`가 그대로인 것을 확인한다.
  - 참조: SPEC §5.8, §5.9 / ANALYSIS §5 J, L
