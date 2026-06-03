# project-widget-split — SPEC

## 1. 범위

현재 단일 `projectInfo` 위젯과 `context` 위젯의 표시 방식을 다음과 같이 재구성한다.

- `projectInfo` 위젯을 **path + branch**만 출력하도록 단순화한다.
- 프로젝트 base name + branch만 출력하는 **`projectName` 위젯을 신규 추가**한다.
- `context` 위젯의 progress bar 가로 폭을 줄인다.
- 두 project 위젯의 선택은 기존 위젯 선택 메커니즘(위젯 ID / preset char /
  `disabledWidgets`)으로만 이뤄지며, 이를 위한 위젯 등록·preset·사용자 문서를
  함께 반영한다.

## 2. 목표

- 사용자가 status line에서 **전체 경로 대신 프로젝트명만 보는 선택지**를 갖게 한다.
  현재는 `projectInfo`가 항상 전체 경로(홈 압축·세그먼트 축약 포함)를 출력해
  프로젝트명만 노출할 방법이 없다.
- status line의 가로 길이를 줄인다 — 잘 채워지지 않는 ahead/behind·worktree 토큰을
  걷어내고, context progress bar를 더 짧게 만들어 한 줄 표시 폭을 절감한다.
- 새 표시 방식을 `cc-usage.json` 설정 키로 분기하지 않고, 이 프로젝트가 이미 쓰는
  "위젯을 골라 켠다"는 선택 메커니즘 위에서 해결해 설정 표면을 늘리지 않는다.

## 3. 제약

- **Zero dependency / 단일 `main` 패키지** 제약을 유지한다. 외부 모듈·서브 패키지
  추가 금지.
- `cc-usage.json`에 path 표시 방식(전체 경로 vs 프로젝트명)을 고르는 **전용 config
  키를 추가하지 않는다.** path/name 구분은 어떤 위젯을 켜는지로만 결정된다.
- 기존 위젯 ID `projectInfo`와 그 preset char `'P'`는 **보존**한다. 이미 이 ID·char를
  `lines`/`preset`/`disabledWidgets`에 쓰고 있는 사용자 설정이 깨지지 않아야 한다.
- 기존 무출력 조건(`workspace.current_dir`·`model`·`context_window_size`가 모두
  비면 출력 전체 생략)을 유지한다.
- git 호출은 500ms 타임아웃·실패 시 graceful degrade 등 기존 동작 특성을 유지한다.

## 4. 제외 범위

- `model` 위젯의 표시 방식 변경은 다루지 않는다. 현행(전체 ID 표시)을 그대로 둔다.
- `cost`·`rateLimit5h`·`rateLimit7d` 위젯의 표시 변경은 다루지 않는다.
- 제거되는 ahead/behind·worktree 정보를 **다른 위젯으로 이전**하는 작업은 범위 밖이다.
  단순 제거이며 재배치가 아니다.
- context progress bar 폭을 사용자가 설정으로 조정하게 만드는 것은 범위 밖이다. 폭은
  고정 값으로 둔다.

## 5. 완료 조건

1. `projectInfo` 위젯 출력에 path와 (git 저장소일 때) branch가 포함되고,
   ahead/behind 카운트(`↑`/`↓`)와 worktree 토큰(`[...]`)은 어떤 stdin 입력에도
   출력되지 않는다.
2. `projectName` 위젯이 존재하며, 그 출력은 현재 디렉토리의 base name과 (git
   저장소일 때) branch만 포함하고 상위 경로 세그먼트나 홈 압축 마커(`~`)·축약
   마커(`…`)·경로 구분자를 포함하지 않는다.
3. `projectName` 위젯을 preset char 및 위젯 ID로 선택할 수 있고,
   `disabledWidgets`에 그 ID를 넣으면 출력에서 사라진다. 기존 `projectInfo`
   선택 방식(`'P'` / `"projectInfo"`)도 그대로 동작한다.
4. `context` 위젯 progress bar의 전체 칸 수(채움 `█` + 빈 칸 `░`의 합)가 기존 10에서
   줄어든 고정 값으로 렌더된다.
5. 두 project 위젯 모두 git 저장소가 아니거나 git 미설치 시 branch 없이 path/name만
   출력하고, 무출력 조건을 만족하는 stdin에서는 아무것도 출력하지 않는다.
