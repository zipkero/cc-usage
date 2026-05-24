# projectinfo-display — SPEC

## 1. 범위

projectInfo 위젯의 출력 형식과 위치 결정 방식을 두 갈래로 갱신한다.

- **path 표시 확장**: 현재 `filepath.Base(currentDir)`로 base name만 노출하는 출력을 home-tilde 압축된 full path 노출로 바꾼다. home 하위 경로는 `~/`로 압축하고, home 외부는 절대 경로 그대로 둔다. 너무 긴 경로는 앞쪽 segment를 `…/`로 줄여 status line 폭을 깨뜨리지 않게 한다. 같은 base name을 가진 서로 다른 디렉터리(`project-a/web` vs `project-b/web` 등) 사이의 시각적 구분이 가능해진다.
- **위치 자유화**: `widget.go`의 orchestrate 루프가 projectInfo를 라인 안에서 빼서 `result.ProjectInfo` 단일 필드로 격리하고, `main.go`의 출력 단계가 partsOutput 앞에 항상 prefix하는 현재 동작을 제거한다. projectInfo도 다른 위젯과 동일하게 preset의 선언 위치 그대로 라인에 합류한다. preset에서 빼면 출력에 등장하지 않는 기존 동작은 유지한다.

이 변경은 v0.3.2 patch 릴리스로 묶는다 (CLAUDE.md §배포 §버전 정책).

## 2. 목표

- **위치 식별성**: 사용자가 같은 base name을 갖는 여러 프로젝트·작업 디렉터리 사이에서 status line만 보고 어느 위치에서 작업 중인지 분간할 수 있다.
- **preset 의도 반영**: 사용자가 custom preset에서 projectInfo의 위치를 자유롭게 둘 수 있고, 출력 순서가 선언 순서와 일치한다.
- **회귀 없는 릴리스**: 기존 branch / ahead-behind / worktree 표시 동작과 다른 위젯, config 스키마, locale key, 출력 형식의 나머지 요소는 변하지 않는다.

## 3. 제약

- status line 한 줄 폭을 깨뜨리지 않도록 path 표시에 길이 임계값을 둔다. 임계값을 초과하면 base name을 보존한 채 앞쪽 segment를 `…/`로 줄인다. (정확한 임계값과 자르기 규칙은 ANALYSIS §5에서 commit한다.)
- Zero dependency 유지, 단일 `main` 패키지 유지.
- Config 스키마, locale JSON 키, CLI 플래그 변경 없음. path 표시 토글이나 임계값 조정용 새 옵션을 도입하지 않는다. 새 위젯도 도입하지 않는다.
- `~/` 압축은 home dir 결정 시 `os.UserHomeDir()` 결과만 사용한다 (환경변수 직접 조회는 별도 도입 안 함).
- v0.3.1 → v0.3.2 SemVer patch bump를 동반한다 (Makefile VERSION, `.claude-plugin/plugin.json` version, `api.go` userAgent 세 곳). 동등 hash 재빌드만 push하지 않는다.
- bin/ 재빌드와 release 브랜치 sync는 본 spec 작업 결과에 따라 절차대로 수행한다.

## 4. 제외 범위

- 새 위젯, 새 config 옵션, 새 외부 통합 — 본 spec 밖.
- branch / ahead-behind / worktree 표시 자체의 동작 변경 — 본 spec 밖 (full path 뒤에 기존 형식 그대로 이어붙인다).
- subpath 별도 표시 — full path에 이미 포함되므로 제거. 별도 옵션 부활시키지 않는다.
- `main.go` degraded-input 복원 로직과 noIdentity 가드의 의미·동작 — 본 spec 밖.
- 다국어 라벨 추가 — projectInfo는 path와 git 정보만 표시하며 로컬라이즈할 텍스트가 없다.

## 5. 완료 조건

1. `workspace.current_dir`가 `os.UserHomeDir()` 결과의 하위 경로면 출력의 디렉터리 부분이 `~/`로 시작하는 압축 형식으로 보인다 (예: home이 `/Users/zipkero`, current_dir이 `/Users/zipkero/GolandProjects/cc-usage`면 `~/GolandProjects/cc-usage`).

2. `workspace.current_dir`가 home 외부면 절대 경로 그대로 보인다 (예: `/tmp/foo` → `/tmp/foo`). home dir 조회가 실패해도 절대 경로로 fallback해서 base name 단독은 절대 노출되지 않는다.

3. 압축 후 표시 길이가 임계값을 초과하면 base name(마지막 segment)은 보존된 채 앞쪽 segment 중 일부가 `…/`로 대체되어 임계값 이내로 줄어든다. base name이 단독으로도 임계값보다 길면 base name 자체가 그대로 노출되며 잘라내지 않는다.

4. `project_dir`과 `current_dir`이 다른 상태에서 출력에 별도 subpath 토큰이 추가로 등장하지 않는다 (full path가 그 정보를 이미 포함).

5. branch / ahead / behind / worktree 표시 형식은 v0.3.1과 동일하다 (path 뒤에 공백으로 이어지는 `(<branch>)`, `↑n`, `↓n`, `[<worktree>]` 토큰).

6. preset에서 projectInfo가 첫 위치가 아닌 곳에 선언되어 있으면 출력에서도 그 위치에 등장한다 (예: `MCP$RHF` preset의 `P`가 4번째에 있으면 출력 4번째 토큰이 projectInfo). preset에 projectInfo가 없으면 출력에 등장하지 않는다.

7. `make build`, `make build-local`, `go test ./...` 세 명령이 변경 전과 동일하게 exit 0으로 종료한다 (회귀 없음).

8. `Makefile` VERSION, `.claude-plugin/plugin.json` version, `api.go` userAgent 세 곳이 모두 `0.3.2`로 일치한다. `dist/cc-usage --version`이 `0.3.2`를 출력한다.
