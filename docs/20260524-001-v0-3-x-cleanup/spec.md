# v0-3-x-cleanup — SPEC

## 1. 범위

v0.3.0 release 이후 발견된 두 가지 정합성·위생 항목을 하나의 patch release(v0.3.x)로 정리한다.

- **설계 문서 정합성**: `DESIGN.md`에 남아있는 historical `Translations` 예시 스키마가 v0.3.0 hardening의 task-007에서 제거된 라벨(`Errors.NoContext`, `Labels.OneM`, `Labels.SevenDAll`)을 그대로 노출하고 있어 현 코드와 어긋난다. 그 부분만 사실에 맞게 정리한다.
- **빌드 부산물 위생**: 프로젝트 루트에서 `go build ./...`가 실행될 때 module-name(`cc-usage`)으로 떨어지는 바이너리가 `.gitignore`에 잡히지 않아 매 빌드/테스트 후 `git status`에 untracked로 노출된다. `.gitignore`에 패턴을 추가해 무시한다.

## 2. 목표

- **광고와 실제의 일치**: 외부에 노출되는 설계 스펙(DESIGN.md)이 현 코드와 어긋난 부분으로 새 합류자를 혼동시키지 않는다.
- **워크플로 청결**: 매 빌드/테스트 사이클에서 `git status`가 거짓 untracked 파일로 더럽혀지지 않고, 실수로 stray binary가 commit될 위험이 0이 된다.

## 3. 제약

- `DESIGN.md`의 historical 의도·아키텍처 설명은 보존한다. 본 작업은 현 코드와 어긋난 사실 부분만 동기화하며, 문서 전반의 재구성·재작성은 하지 않는다.
- `.gitignore`의 기존 규칙(`dist/`, `bin/` 명시 허용, `.idea`, `.vscode`, `.env`, `*.exe`, `*.test`, `go.work` 등)을 약화시키지 않는다. 신규 패턴 1~2줄 추가만 허용.
- Zero dependency 유지. 새 외부 의존, 새 도구체인, 새 빌드 타깃 도입 없음.
- 본 작업은 v0.3.x patch release에 들어가며, 사용자에게 노출되는 동작 변경(새 위젯, 새 옵션, 새 CLI 플래그)은 없다.
- bin/ 재빌드는 코드/빌드 산출물 변경이 없으므로 본 spec 작업만으로는 트리거되지 않는다. 필요 시 별도 판단.

## 4. 제외 범위

- `DESIGN.md` 전반의 재작성·재구성 — task-007 관련 historical Translations 예시 외의 차이는 별도 spec.
- `ROADMAP.md` / `PLAN.md` / `IMPLEMENT.md`의 정합성 — 본 spec 밖.
- `.gitignore`의 기존 규칙 재정렬·재작성 — 신규 패턴 추가만 허용.
- 새 분석 위젯, 새 캐시 인프라, 새 외부 의존, 새 빌드 절차 — 본 spec 밖.

## 5. 완료 조건

1. v0.3.0의 `widget.go` `Translations` 정의(`Model` / `Labels{FiveH, SevenD, SevenDSonnet}` / `Time` / `Widgets{ApiDuration, BurnRate, Cache, Performance, Session}`, `Errors` 서브구조 없음)와 `DESIGN.md`의 `Translations` 스키마 설명이 일치한다. `ripgrep`으로 `NoContext|OneM|SevenDAll`을 비-`docs` 영역(`DESIGN.md` 포함)에서 검색했을 때 0건이다.

2. 클린 working tree 상태에서 `go build ./...`를 실행한 직후 `git status`가 `nothing to commit, working tree clean`을 반환한다 (module-root `cc-usage` stray binary가 `.gitignore`에 의해 무시되어 untracked로 노출되지 않는다).

3. 위 변경 후 `make build`, `make build-local`, `go test ./...` 세 명령 모두 변경 전과 동일하게 정상 종료한다 (exit 0, 출력 형태 보존). 본 spec이 기존 빌드/테스트 흐름을 깨지 않는다.
