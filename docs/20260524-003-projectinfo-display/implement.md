# projectinfo-display — IMPLEMENT

- [x] task-001: `widgets_project.go` path 표시 로직 갱신
  - 목적: workspace.current_dir가 home 하위면 출력의 디렉터리 부분이 `~/`로 시작하는 압축 형식으로, home 외부면 절대 경로 그대로 보인다. home 조회 실패 시에도 절대 경로로 fallback해서 base name 단독은 절대 노출되지 않는다. 압축 후 길이가 임계값(50 rune)을 초과하면 base name은 보존된 채 앞쪽 segment가 `…/`로 일괄 치환되며, base name이 단독으로도 임계값을 넘으면 그대로 노출한다. project_dir과 current_dir이 달라도 별도 subpath 토큰은 등장하지 않는다. branch / ahead / behind / worktree 출력 형식은 v0.3.1과 동일하다
  - 접근: `projectInfoData.Subpath` 필드 제거 및 `DisplayPath` 필드 추가. GetData에서 (1) `os.UserHomeDir()` 호출 (실패 시 빈 문자열), (2) `current == home` short-circuit + `HasPrefix(home + filepath.Separator)`로 home-tilde 압축, (3) segment-aware 길이 정규화 (50 rune 임계, base 보존, 앞쪽 segment를 `…/`로 일괄 치환, `~` prefix 보존) — 둘 다 widget 내부 private helper(`compressHome`, `shrinkPath` 등 분리하거나 한 함수로 묶어도 무방, analysis.md §2.1 절차 그대로)로 둔다. Render에서 `truncate(d.DirName, 25)` → `d.DisplayPath` 출력으로 교체, subpath 출력 분기 제거. branch/ahead-behind/worktree 출력 분기는 손대지 않는다
  - 검증 조건:
    - 결과:
      - `current_dir`이 home 하위인 stdin에서 출력의 path 부분이 `~/`로 시작한다
      - `current_dir`이 home 외부 또는 home 조회 실패인 stdin에서 절대 경로 그대로 나타나고 base name 단독은 나타나지 않는다
      - 50 rune 초과 path에서 base name이 보존된 채 앞쪽 segment가 `…/`로 줄어든다 (`~/very/deep/path/proj` → `~/…/path/proj`)
      - base name이 단독으로 50 rune을 넘으면 그대로 노출된다
      - `project_dir`과 `current_dir`이 다를 때도 출력에 별도 subpath 토큰이 추가되지 않는다
      - branch / ahead / behind / worktree 출력 토큰 형식은 v0.3.1과 동일하다
    - 확인: `go vet ./...`, `go build ./...`, `go test ./...` 모두 exit 0. task-003의 helper 회귀 테스트가 모두 통과
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.4, SPEC §5.5, SPEC §5.7, ANALYSIS §1, ANALYSIS §2.1, ANALYSIS §5.A, ANALYSIS §5.C, ANALYSIS §5.D

- [x] task-002: orchestrate / main.go의 projectInfo 위치 자유화
  - 목적: preset에 projectInfo가 첫 위치가 아닌 곳에 선언되어 있으면 출력의 그 위치에 등장한다. preset에 projectInfo가 없으면 출력에 나타나지 않는다 (기존 동작과 동일). projectInfo가 첫 위치일 때의 출력 순서도 변하지 않는다 (compact preset 동작 보존)
  - 접근: `widget.go`의 `OrchestrateResult` struct에서 `ProjectInfo string` / `HasProject bool` 두 필드를 제거한다. `orchestrate` 루프의 `if widgetID == "projectInfo"` 분기를 제거하고 다른 위젯과 동일하게 `parts = append(parts, rendered)`로 합류시킨다. `main.go`의 출력 단계에서 `result.HasProject` prefix 분기를 제거하고 `partsOutput`이 비지 않으면 그대로 print하는 단일 분기로 단순화한다
  - 검증 조건:
    - 결과:
      - 기본 compact preset(`projectInfo`가 첫 위치)의 출력 형태가 v0.3.1과 동일하다 (`<path> │ <model> │ ...`)
      - custom preset에서 projectInfo를 다른 위치(예: 끝)에 두면 출력에서도 그 위치에 등장한다
      - preset에서 projectInfo를 빼면 출력에 나타나지 않는다
      - `OrchestrateResult.ProjectInfo` / `HasProject` 외부 참조가 모두 사라져 컴파일이 깨끗하게 통과한다 (`grep -rn 'ProjectInfo\|HasProject' --include='*.go'` 결과가 0건)
    - 확인: `go vet ./...`, `go build ./...`, `go test ./...` 모두 exit 0. 수동 확인 — `echo '{...}' | ./dist/cc-usage`로 compact / custom preset 두 가지 출력 비교
  - 참조: SPEC §5.6, SPEC §5.7, ANALYSIS §1, ANALYSIS §2, ANALYSIS §5.B

- [x] task-003: path 변환 helper 회귀 테스트 작성
  - 목적: task-001의 home-tilde 압축과 segment-aware 길이 정규화가 의도한 케이스(home 하위 / 정확히 home / home 외부 / home 조회 실패 / 50 rune 이하 / 50 rune 초과 segment 컷 / base 단독 초과)에 대해 회귀하지 않음을 자동 검증한다
  - 접근: `widgets_project_test.go`(새 파일) 또는 기존 테스트 파일에 unit test를 추가한다. task-001에서 도입한 path 변환 helper(`compressHome`, `shrinkPath` 또는 동등 함수)를 직접 호출해 다음 케이스를 검증한다. (1) `current == home` → `~`, (2) home 하위 → `~/...`, (3) home 외부 → 절대 경로 그대로, (4) home 빈 문자열(조회 실패 시뮬레이션) → 절대 경로 그대로, (5) compressed 길이 ≤ 50 → 변화 없음, (6) compressed 길이 > 50 → base 보존 + 앞 segment `…/`, (7) `~` prefix 경우 → `~/…/` 형태 유지, (8) base 단독 > 50 → 그대로. 각 case는 deterministic 입력 string으로 작성하며 wall clock·home dir 실제 조회에 의존하지 않는다
  - 검증 조건:
    - 결과: 위 8개 시나리오 모두 자동 검증에 PASS한다
    - 확인: `go test -run TestProjectPath ./...` 또는 해당 테스트 이름으로 실행 시 모두 PASS. `go test ./...`로 전체 회귀 없음 확인
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.7

- [x] task-004: v0.3.1 → v0.3.2 SemVer patch bump
  - 목적: `dist/cc-usage --version`이 `0.3.2`를 출력하고, `Makefile` VERSION / `.claude-plugin/plugin.json` version / `api.go` userAgent 세 곳이 모두 `0.3.2`로 일치한다. `/plugin` UI의 update 감지가 정상 동작하는 v0.3.2 release 후보가 된다
  - 접근: `Makefile`의 `VERSION := 0.3.1`을 `0.3.2`로, `.claude-plugin/plugin.json`의 `"version": "0.3.1"`을 `"0.3.2"`로, `api.go:61`의 `userAgent = "cc-usage/0.3.1"`을 `"cc-usage/0.3.2"`로 동시 갱신한다. 동일 commit에 task-001/002/003 변경과 함께 묶는다 (ANALYSIS §5.E). bin/ 재빌드와 release 브랜치 sync는 본 task 범위 밖이며 후속 commit/release 단계에서 별도 처리한다
  - 검증 조건:
    - 결과:
      - 세 파일의 version 문자열이 모두 `0.3.2`다 (`grep -rn '0\.3\.1' Makefile .claude-plugin/plugin.json api.go` 결과 0건)
      - `make build-local` 후 `dist/cc-usage --version`이 `0.3.2`를 출력한다
    - 확인: 세 파일 grep / `make build-local && ./dist/cc-usage --version` / `go test ./...` exit 0
  - 참조: SPEC §5.8, ANALYSIS §5.E
