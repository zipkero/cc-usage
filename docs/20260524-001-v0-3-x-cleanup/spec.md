# v0-3-x-cleanup — SPEC

## 1. 범위

v0.3.0 release 이후 발견된 historical 문서 정리와 빌드 부산물 위생 항목을 하나의 patch release(v0.3.x)로 정리한다.

- **Historical 설계 문서 제거**: `docs/<feature-dir>/` spec-flow 도입 이후 `DESIGN.md`와 `ROADMAP.md`는 reference 가치를 잃었다. DESIGN.md의 모든 섹션이 README / CLAUDE.md / 코드와 중복이고 일부 내용은 stale(예: `Translations` 예시 스키마가 task-007 정리 이전 라벨을 그대로 노출, API 클라이언트 설명이 task-005 이전 memCache 구조 기준). ROADMAP.md의 Phase 2~6 스펙은 v0.2.0 정리에서 명시 결정한 외부 의존 제외 / 데이터 부재 제거와 충돌하고, 향후 새 위젯 도입은 새 spec-flow를 거치므로 actionable 가치 0. 두 파일을 통째로 제거하고, `CLAUDE.md`의 "설계 문서" 섹션도 두 파일을 reference로 가리키지 않도록 갱신한다.
- **빌드 부산물 위생**: 프로젝트 루트에서 `go build ./...`가 실행될 때 module-name(`cc-usage`)으로 떨어지는 바이너리가 `.gitignore`에 잡히지 않아 매 빌드/테스트 후 `git status`에 untracked로 노출된다. `.gitignore`에 패턴을 추가해 무시한다.

## 2. 목표

- **단일 출처 원칙**: README(사용자) / CLAUDE.md(개발자) / 코드(런타임 진실) 세 layer만 유지한다. historical 설계 문서가 stale 상태로 남아 새 합류자를 혼동시키거나 코드와 어긋난 정보를 믿게 만들지 않는다.
- **워크플로 청결**: 매 빌드/테스트 사이클에서 `git status`가 거짓 untracked 파일로 더럽혀지지 않고, 실수로 stray binary가 commit될 위험이 0이 된다.

## 3. 제약

- `.gitignore`의 기존 규칙(`dist/`, `bin/` 명시 허용, `.idea`, `.vscode`, `.env`, `*.exe`, `*.test`, `go.work` 등)을 약화시키지 않는다. 신규 패턴 1~2줄 추가만 허용.
- Zero dependency 유지. 새 외부 의존, 새 도구체인, 새 빌드 타깃 도입 없음.
- 본 작업은 v0.3.x patch release에 들어가며, 사용자에게 노출되는 동작 변경(새 위젯, 새 옵션, 새 CLI 플래그)은 없다.
- bin/ 재빌드는 코드/빌드 산출물 변경이 없으므로 본 spec 작업만으로는 트리거되지 않는다.
- CLAUDE.md의 "설계 문서" 섹션은 historical 두 파일을 가리키지 않게 정리하되, README / CLAUDE.md / 코드가 진실 layer임을 명시한다.

## 4. 제외 범위

- README.md / CLAUDE.md 본문의 다른 섹션 재작성 — 본 spec 밖.
- `.gitignore`의 기존 규칙 재정렬·재작성 — 신규 패턴 추가만 허용.
- 새 분석 위젯, 새 캐시 인프라, 새 외부 의존, 새 빌드 절차 — 본 spec 밖.
- 진행 중·미해결 feature spec(`docs/<feature-dir>/`) 자체 — 본 spec 밖.

## 5. 완료 조건

1. v0.3.0의 `widget.go` `Translations` 정의(`Model` / `Labels{FiveH, SevenD, SevenDSonnet}` / `Time` / `Widgets{ApiDuration, BurnRate, Cache, Performance, Session}`, `Errors` 서브구조 없음)와 `DESIGN.md`의 `Translations` 스키마 설명이 일치한다. `ripgrep`으로 `NoContext|OneM|SevenDAll`을 비-`docs` 영역(`DESIGN.md` 포함)에서 검색했을 때 0건이다.

2. 클린 working tree 상태에서 `go build ./...`를 실행한 직후 `git status`가 `nothing to commit, working tree clean`을 반환한다 (module-root `cc-usage` stray binary가 `.gitignore`에 의해 무시되어 untracked로 노출되지 않는다).

3. 위 변경 후 `make build`, `make build-local`, `go test ./...` 세 명령 모두 변경 전과 동일하게 정상 종료한다 (exit 0, 출력 형태 보존). 본 spec이 기존 빌드/테스트 흐름을 깨지 않는다.

4. `DESIGN.md`와 `ROADMAP.md`가 repo에 존재하지 않는다 (`git ls-files | grep -E '^(DESIGN|ROADMAP)\.md$'` 0건). `CLAUDE.md`의 "설계 문서" 섹션은 두 파일을 reference로 가리키는 행을 포함하지 않으며, README / CLAUDE.md / 코드가 진실 layer임을 명시한다.
