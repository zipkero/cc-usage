# project-widget-split — ANALYSIS

## 근거

읽은 spec 범위: spec.md §1–§5 전체. §1 범위(projectInfo 단순화 + projectName 신규 +
progress bar 폭 축소 + 위젯 선택 메커니즘만으로 분기), §3 제약(zero dependency·단일
main 패키지·전용 config 키 금지·`projectInfo`/`'P'` 보존·무출력 조건 유지·git 500ms
타임아웃 graceful degrade), §4 제외 범위(model/cost/rateLimit 불변, 제거 정보 재배치
금지, bar 폭 config 노출 금지), §5 완료 조건 5개를 근거로 삼았다.

코드에서 확인한 사실:

- `projectInfoWidget`는 `widgets_project.go`에 단독 구현. `projectInfoData`는
  `DisplayPath`·`Branch`·`Ahead`·`Behind`·`Worktree` 5필드를 보유한다. GetData는
  ① cwd 결정(`Workspace.CurrentDir` → 비면 `detectCurrentCwd()`), ② home 압축 +
  세그먼트 축약(`compressHome`/`shrinkPath`/`pathDisplayMaxRunes=50`), ③ worktree
  이름 채움, ④ `git status --porcelain=v2 --branch` 단일 호출로 `branch.head`(branch)와
  `branch.ab`(ahead/behind)를 함께 파싱한다. Render는 path → `(branch ↑n ↓n)` →
  `[worktree]` 순으로 조립한다.
- ahead/behind는 `branch.ab` 파싱 블록과 Render의 `↑`/`↓` 출력에서만, worktree는
  `Stdin.Worktree.Name` 소비와 Render의 `[...]` 출력에서만 쓰인다. 두 토큰을 소비하는
  다른 위젯은 없다(grep `Worktree` → `stdin.go` 스키마 + `widgets_project.go`만).
- branch 조회의 핵심은 `git status --porcelain=v2 --branch` 실행과 `# branch.head`
  라인에서 이름을 뽑되 `(detached)`를 제외하는 부분이다. ahead/behind 제거 후에도 이
  branch 추출은 그대로 필요하다.
- cwd fallback 진입점은 `detectCurrentCwd()`(내부 `detectCurrentCwdWithSource`/
  `normalizeCwd`, env hook `detectCwdEnv`/`detectCwdGetwd`). 현재 projectInfo만
  호출한다(grep `detectCurrentCwd` → `widgets_project.go`와 테스트).
- `compressHome`/`shrinkPath`/`pathDisplayMaxRunes`는 전체 경로 표시 전용 헬퍼다.
  projectName(base name만)은 이들을 쓰지 않는다.
- `renderProgressBar`(`render.go`)는 `const width = 10`을 내부에 고정하고 percent와
  theme만 받는다. 호출자는 context 위젯 한 곳뿐이다(grep `renderProgressBar` →
  `render.go` 정의 + `widgets_core.go:93`).
- 위젯 등록/노출 지점: `widget.go`의 `displayPresets.compact`(첫 원소가
  `"projectInfo"`), `presetCharToWidget`(`'P' → "projectInfo"`), `registry`(각 위젯
  `init()`). orchestrate는 `disabledWidgets` 셋으로 위젯을 거르고 GetData=nil/error
  또는 Render="" 면 자동 skip한다.
- 무출력 조건은 main.go 소관이며 본 변경 범위 밖이다. 위젯 단의 graceful degrade
  (GetData nil 반환 시 skip)는 그대로 유지된다.
- README는 위젯 테이블에서 `projectInfo`를 "디렉토리 + git branch (+ worktree,
  subpath)"로 기술하고, preset 예시·troubleshooting·git 호출 설명에서 참조한다.

추정과 사실의 분리: 위 호출자/소비자 목록은 grep 탐색으로 확인한 사실이다. progress
bar 폭 "7"은 spec이 "줄어든 고정 값"만 요구하므로(§5.4) 본 analysis가 합의에 따라
확정하는 설계값이다(근거가 아닌 결정).

## 1. 구조

이 변경은 새 모듈·레이어·서브 패키지를 만들지 않는다. 기존 단일 `main` 패키지의
파일 경계 안에서 위젯 하나를 단순화하고, 위젯 하나를 추가하며, 공유 git 헬퍼를
추출하고, 렌더 헬퍼 한 곳의 폭을 조정한다.

위젯 경계:

- **projectInfo 위젯** (전체 경로 표시): cwd 결정 → 경로 표시 가공(home 압축 +
  세그먼트 축약) → branch 조회 → `path (branch)` 렌더. ahead/behind·worktree는
  데이터 모델과 렌더에서 모두 사라진다(SPEC §5.1). 경로 가공 헬퍼
  (`compressHome`/`shrinkPath`)는 이 위젯 전용으로 유지된다.
- **projectName 위젯** (신규, base name 표시): cwd 결정 → base name 추출
  (`filepath.Base`) → branch 조회 → `name (branch)` 렌더. 상위 경로·`~`·`…`·구분자를
  일절 포함하지 않으므로 경로 가공 헬퍼를 거치지 않는다(SPEC §5.2).

공유 경계(§5 Decision Point에서 확정):

- **branch 조회**: 두 project 위젯이 동일하게 `git status --porcelain=v2 --branch`
  실행 후 `branch.head`에서 branch 이름을 얻는다(`(detached)` 제외, git 미설치·실패
  시 빈 branch로 degrade). 이 로직을 공유 헬퍼로 추출해 한 곳에서 소유한다.
- **cwd 결정**: 두 위젯 모두 `Workspace.CurrentDir`가 비면 `detectCurrentCwd()`로
  보강한다. 이미 함수로 분리돼 있어 양쪽이 직접 호출해 공유한다.

렌더 경계:

- **progress bar 폭**: `renderProgressBar`의 고정 폭을 10에서 7로 낮춘다. 호출자가
  context 위젯 하나뿐이므로 시그니처 변경 없이 내부 const만 바꾼다(SPEC §5.4).

## 2. 데이터 흐름

진입은 기존과 동일하다. main → stdin 파싱 → orchestrate → 각 위젯 GetData/Render →
separator 조인 → stdout. 본 변경은 orchestrate 아래 두 project 위젯의 내부 흐름과
context 위젯의 bar 렌더만 바꾼다.

project 위젯 공통 흐름(projectInfo·projectName):

1. cwd 결정: `Stdin.Workspace.CurrentDir`가 비어 있지 않으면 그대로, 비면
   `detectCurrentCwd()`(CLAUDE_PROJECT_DIR → os.Getwd, symlink 정규화). 둘 다 빈
   문자열이면 GetData가 nil을 반환하고 orchestrator가 위젯을 skip한다(SPEC §5.5).
2. 표시 텍스트 산출:
   - projectInfo → home 압축 + 세그먼트 축약을 거친 전체 경로.
   - projectName → `filepath.Base(currentDir)` 한 토큰. 경로 가공 헬퍼를 타지
     않으므로 `~`·`…`·구분자가 끼어들 여지가 없다(SPEC §5.2).
3. branch 조회: `exec.LookPath("git")` 실패면 branch 없이 표시 텍스트만 가진 채로
   진행한다. git이 있으면 500ms 타임아웃 컨텍스트로 `git status --porcelain=v2
   --branch`를 실행한다. 명령 실패·타임아웃이면 branch 없이 degrade한다. 성공이면
   `# branch.head` 라인에서 이름을 취하되 `(detached)`는 branch 미설정으로 둔다
   (SPEC §3 git 동작 특성 유지, §5.5 git 부재 degrade).
4. 렌더: 표시 텍스트를 Folder 색으로, branch가 있으면 ` (branch)`를 Branch 색으로
   덧붙인다. projectInfo에서 `↑n ↓n`과 `[worktree]`는 더 이상 출력되지 않는다
   (SPEC §5.1).

상태·동시성: 위젯은 무상태이며 호출당 git을 1회 실행한다. project 위젯을 둘 다 켜면
git 호출이 두 번 발생하지만(각 위젯이 독립적으로 GetData), 둘은 상호 배타적으로
선택해 쓰는 표시 방식이므로 일반적 사용에서는 한 번이다. 에러 경로는 모두 branch
없는 표시로 수렴(graceful degrade)하며 패닉하지 않는다.

context 위젯 흐름은 불변이되, `renderProgressBar`가 채움+빈칸 합 7칸을 반환한다
(SPEC §5.4). percent→칸수 매핑은 동일 비례식이며 폭만 7로 바뀐다.

## 3. 인터페이스

경계를 가로지르는 계약만 기술한다(내부 helper 시그니처는 범위 밖).

- **Widget 인터페이스**: projectName 위젯은 기존 `Widget`(`ID()`/`GetData`/`Render`)을
  그대로 구현한다. 새 인터페이스를 도입하지 않는다.
- **위젯 ID 계약**: `projectInfo`는 보존되고(SPEC §3), `projectName`이 신규 ID로
  추가된다. orchestrate·`disabledWidgets`·`lines`가 이 ID 문자열로 위젯을 지목한다
  (SPEC §5.3).
- **preset char 계약**: `presetCharToWidget`에 projectName용 char를 신규 매핑하고
  `'P' → projectInfo`는 그대로 둔다. preset 문자열로 둘 중 하나를 선택할 수 있어야
  한다(SPEC §5.3). 채택 char는 §5에서 확정한다.
- **config 스키마**: 변경 없음. path/name 분기를 위한 전용 키를 추가하지 않는다
  (SPEC §3). 분기는 위젯 ID/preset char/`disabledWidgets`로만 이뤄진다(SPEC §2, §5.3).
- **stdin 스키마**: `StdinInput.Worktree` 필드는 스키마에서 제거하지 않는다. 소비처가
  사라질 뿐이며(SPEC §4 — 재배치 아닌 단순 제거), 입력 프로토콜 정의를 좁히는 것은
  범위 밖이다.

## 4. 영향 범위

직접 수정 파일:

- `widgets_project.go` — projectInfo의 ahead/behind 파싱·worktree 소비·렌더 제거,
  데이터 모델 축소. projectName 위젯 신규 구현 및 `init()` 등록. branch 조회 공유
  헬퍼 추출(§5).
- `render.go` — `renderProgressBar` 내부 고정 폭 10 → 7(SPEC §5.4).
- `widget.go` — `presetCharToWidget`에 projectName char 추가. `displayPresets.compact`
  노출 여부는 §5에서 결정.

직접 의존(탐색 확인):

- `widgets_core.go:93` — `renderProgressBar` 유일 호출자. 시그니처 불변이라 호출부
  수정 불필요. 출력 폭만 7칸으로 바뀐다.
- `widget.go`의 `displayPresets`/`presetCharToWidget` — `projectInfo` 참조 보존.
  projectName 노출 추가만 검토 대상.

테스트:

- `widgets_project_test.go` — projectInfo의 ahead/behind·worktree 검증 케이스가 있으면
  제거 후 동작과 어긋난다. cwd fallback·path 표시 헬퍼 테스트는 유지된다. projectName
  케이스 추가 여지가 있으나 테스트 작성은 implement.md 소관이며, 여기서는 영향 지점만
  기록한다.
- `widget_test.go` — `{"projectInfo"}` 케이스는 ID 보존으로 그대로 유효하다.

문서:

- `README.md` — 위젯 테이블의 `projectInfo` 설명에서 "worktree·subpath" 문구가 실제
  출력과 어긋나므로 갱신 대상이고, projectName 행 추가가 필요하다(SPEC §5.3 — 사용자
  문서 반영). troubleshooting의 `projectInfo` 언급은 cwd fallback 동작이 불변이므로
  손대지 않는다. README 갱신 자체는 본 analysis 산출이 아니며 후속 단계 소관이다.

하위 호환·마이그레이션: `projectInfo` ID와 `'P'` char를 보존하므로 기존
`lines`/`preset`/`disabledWidgets` 사용자 설정은 깨지지 않는다(SPEC §3). 그 외 깨지는
기존 호출자·저장 데이터·외부 contract는 탐색에서 확인되지 않았다 — 해당 없음.

## 5. Decision Points

### D1. branch 조회 로직 공유 방식

- 옵션 A: 두 위젯이 각자 git 호출·파싱을 중복 보유.
- 옵션 B: branch 이름만 돌려주는 공유 헬퍼(예: `gitBranch(dir) string`)로 추출하고
  두 위젯이 호출.
- 채택: B. 근거 — ahead/behind 제거 후 두 위젯이 필요로 하는 git 결과는 branch
  이름 하나로 동일하다. 중복 보유는 500ms 타임아웃·`(detached)` 처리·실패 degrade
  규칙(SPEC §3)을 두 곳에서 동기화해야 해 회귀 위험이 크다. 헬퍼는 입력 dir, 출력
  branch 문자열(없으면 빈 문자열)로 경계가 자명하다. zero dependency·단일 패키지
  제약 안에서 파일 내 함수 추출로 끝난다.

### D2. cwd 결정 공유

- 옵션 A: projectName이 cwd fallback 로직을 다시 구현.
- 옵션 B: 기존 `detectCurrentCwd()`를 두 위젯이 공유 호출.
- 채택: B. 근거 — `detectCurrentCwd`는 이미 함수로 분리돼 있고 CLAUDE_PROJECT_DIR
  우선·symlink 정규화 동작을 담는다. 재구현은 idle 시 fallback 동작(README
  troubleshooting에 문서화)을 갈라지게 만든다. 신규 추상화 도입 없이 그대로 호출한다.

### D3. progress bar 폭의 고정값과 적용 방식

- 폭 값: spec은 "10에서 줄어든 고정 값"만 요구한다(SPEC §5.4). 채택값 **7**. 근거 —
  status line 가로 절감(SPEC §2)을 달성하면서, percent→칸수 비례에서 0%·100% 양끝과
  중간 구간이 시각적으로 구분되는 최소 수준을 유지한다.
- 적용 방식 옵션:
  - 옵션 A: `renderProgressBar` 내부 `const width`를 7로 변경.
  - 옵션 B: 폭을 파라미터로 받도록 시그니처 변경.
- 채택: A. 근거 — 호출자가 context 위젯 하나뿐이고(탐색 확인), spec은 폭의 사용자
  설정 노출을 명시적으로 범위 밖으로 둔다(SPEC §4). 파라미터화는 호출부마다 폭을
  전달하는 표면을 늘릴 뿐 현재 요구에 기여하지 않는다. 미래에 폭 차등이 필요하면
  그때 파라미터화하면 되고, 지금은 직접 const 변경이 최소 변경이다.

### D4. config 키 추가 vs 위젯 분리

- 옵션 A: `cc-usage.json`에 path 표시 방식(전체 경로/이름) 분기 키 추가.
- 옵션 B: 위젯을 둘로 나누고 위젯 ID/preset char/`disabledWidgets`로만 선택.
- 채택: B. 근거 — SPEC §3이 전용 config 키 추가를 명시 금지하고, §2가 기존 "위젯을
  골라 켠다" 메커니즘 위에서 해결해 설정 표면을 늘리지 않기를 목표로 둔다. 위젯
  분리는 두 표시 방식을 독립 위젯으로 만들어 선택을 기존 메커니즘에 흡수시킨다.
  이 항목은 선택의 여지가 spec으로 닫혀 있으나, 근거를 §5에 명시 commit한다.

### D5. projectName preset char 및 displayPresets 노출

- char 옵션: projectName에 부여할 단일 char. `'P'`는 projectInfo가 점유한다(SPEC §3).
- 채택: **`'N'`** (name). 근거 — 의미 연상(name)이 명확하고 기존
  `presetCharToWidget`(`M`/`C`/`$`/`R`/`7`/`P`)과 충돌하지 않는다.
- displayPresets.compact 노출 옵션:
  - 옵션 A: 기본 compact 레이아웃에 projectName도 추가.
  - 옵션 B: compact 기본 레이아웃은 `projectInfo`만 유지하고, projectName은
    preset/lines/disabledWidgets로 사용자가 명시 선택했을 때만 노출.
- 채택: B. 근거 — 두 위젯은 같은 위치를 차지하는 상호 배타 표시 방식이라 둘 다
  기본 노출하면 path와 name이 중복 출력된다. 기본값은 기존 동작(projectInfo) 유지가
  사용자 영향 최소이고, projectName은 옵트인 위젯으로 둔다. 이 경우 README 위젯
  테이블에는 두 위젯을 모두 등재하되 기본 레이아웃 변화는 없음을 반영한다.
