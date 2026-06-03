# project-widget-split — IMPLEMENT

## Section: 공유 헬퍼 추출

- [x] task-001: git branch 조회를 공유 헬퍼로 추출
  - 목적: 디렉토리 하나를 받아 git branch 이름(없으면 빈 문자열)을 돌려주는 단일 경로가
    생기고, git 미설치·명령 실패·타임아웃·detached HEAD에서 모두 빈 branch로 degrade한다
  - 접근: `widgets_project.go`에 `gitBranch(dir) string` 추출 — `exec.LookPath("git")`
    실패·`git status --porcelain=v2 --branch` 실패/500ms 타임아웃 시 빈 문자열, 성공 시
    `# branch.head` 라인에서 이름을 취하되 `(detached)`는 빈 문자열로 둔다. ahead/behind
    파싱은 가져오지 않는다
  - 검증 조건:
    - 결과: git 저장소에서 branch 이름이 반환되고, git 미설치·실패·타임아웃·detached에서
      빈 문자열이 반환된다
    - 확인: `go build ./...`·`go vet ./...` 통과. 헬퍼 동작은 task-002·task-003의 위젯
      회귀 테스트로 간접 커버
  - 참조: SPEC §5.5, ANALYSIS §5 D1

## Section: projectInfo 단순화

- [x] task-002: projectInfo에서 ahead/behind·worktree 제거
  - 목적: projectInfo 출력이 path와 (git 저장소일 때) branch만 포함하고, ahead/behind
    카운트(`↑`/`↓`)와 worktree 토큰(`[...]`)은 어떤 stdin 입력에도 출력되지 않는다
  - 접근: `projectInfoData`를 `DisplayPath`·`Branch`로 축소(`Ahead`/`Behind`/`Worktree`
    필드 제거), GetData에서 worktree 채움과 ahead/behind 파싱 제거 후 branch는
    task-001 헬퍼로 조회, Render에서 `↑n ↓n`·`[worktree]` 출력 제거. cwd 결정은 기존
    `detectCurrentCwd()` 공유를 유지
  - 검증 조건:
    - 결과: ahead/behind·worktree가 채워질 stdin(예: upstream 있는 저장소,
      `worktree.name` 포함 입력)에서도 출력에 `↑`/`↓`/`[...]`가 없고 path·branch만 남는다
    - 확인: `go build ./...`·`go vet ./...`·`go test ./...` 통과. ahead/behind·worktree가
      출력되지 않음을 검증하는 회귀 테스트를 `widgets_project_test.go`에 추가하고, 기존
      cwd fallback·path 헬퍼 테스트가 깨지지 않음을 확인
  - 참조: SPEC §5.1, SPEC §5.5, ANALYSIS §5 D2

## Section: projectName 신규 위젯

- [x] task-003: projectName 위젯 구현 및 등록
  - 목적: 현재 디렉토리의 base name과 (git 저장소일 때) branch만 출력하고 상위 경로
    세그먼트·홈 압축 마커(`~`)·축약 마커(`…`)·경로 구분자를 포함하지 않는 위젯이 존재하며,
    git 부재 시 branch 없이 name만 출력하고 무출력 stdin에서는 아무것도 출력하지 않는다
  - 접근: `widgets_project.go`에 `projectNameWidget`(ID `projectName`) 추가 —
    cwd는 `detectCurrentCwd()` 공유로 결정(빈 문자열이면 GetData가 nil 반환해 skip),
    표시 텍스트는 `filepath.Base(currentDir)`로 산출(경로 가공 헬퍼 비경유), branch는
    task-001 헬퍼로 조회, Render는 `name (branch)`. 같은 파일 `init()`에서 registerWidget
  - 검증 조건:
    - 결과: projectName 출력이 base name으로 시작하고 `~`·`…`·경로 구분자를 포함하지
      않으며, git 미설치·비저장소에서 branch 없이 name만, cwd가 결정 불가한 무출력 stdin에서
      위젯이 생략된다
    - 확인: `go build ./...`·`go vet ./...`·`go test ./...` 통과. base name 단일 토큰 출력과
      마커 미포함, git 부재 degrade, cwd 결정 불가 시 skip을 검증하는 테스트를
      `widgets_project_test.go`에 추가
  - 참조: SPEC §5.2, SPEC §5.5, ANALYSIS §5 D1, ANALYSIS §5 D2

- [x] task-004: projectName 위젯 선택 메커니즘 연결
  - 목적: preset char `N`과 위젯 ID `projectName`으로 projectName 위젯을 켤 수 있고,
    `disabledWidgets`에 `projectName`을 넣으면 출력에서 사라지며, 기존 projectInfo 선택
    방식(`P` / `projectInfo`)도 그대로 동작한다
  - 접근: `widget.go`의 `presetCharToWidget`에 `'N': "projectName"` 추가. `'P' →
    projectInfo` 매핑과 `displayPresets.compact` 레이아웃은 그대로 둔다(projectName은
    기본 비노출 옵트인)
  - 검증 조건:
    - 결과: preset `"N"` 또는 lines에 `projectName` 지정 시 위젯이 렌더되고,
      `disabledWidgets: ["projectName"]`에서 사라지며, `"P"`/`projectInfo` 선택과 compact
      기본 레이아웃은 변경 전과 동일하게 동작한다
    - 확인: `go test ./...` 통과(기존 `widget_test.go`의 `projectInfo`·preset 케이스 유지).
      preset char `N` 해석과 `P` 보존을 검증하는 케이스를 추가하고, 수동 실행으로 preset
      `"N"` 출력 확인
  - 참조: SPEC §5.3, ANALYSIS §5 D5

## Section: progress bar 폭 축소

- [x] task-005: context progress bar 폭 10→7
  - 목적: context 위젯 progress bar의 전체 칸 수(채움 `█` + 빈 칸 `░`의 합)가 10에서
    7로 고정 렌더되고, percent→칸수 비례 매핑은 동일하게 유지된다
  - 접근: `render.go`의 `renderProgressBar` 내부 `const width = 10`을 `7`로 변경
    (시그니처·호출부 불변, 파라미터화하지 않음)
  - 검증 조건:
    - 결과: 임의 percent 입력에서 bar의 `█`+`░` 합이 항상 7이고 0%·100% 양끝이 올바르게
      렌더된다
    - 확인: `go test ./...` 통과. bar 전체 칸 수가 7임을 검증하는 케이스를
      `widgets_core_test.go`(또는 render 대상 테스트)에 추가/갱신
  - 참조: SPEC §5.4, ANALYSIS §5 D3
