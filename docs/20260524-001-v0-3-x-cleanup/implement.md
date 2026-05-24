# v0-3-x-cleanup — IMPLEMENT

- [ ] task-003: `DESIGN.md` / `ROADMAP.md` 제거 및 `CLAUDE.md` 설계 문서 섹션 정리
  - 목적: 사용 안 되는 historical 설계 문서 두 파일을 repo에서 통째로 제거하고, CLAUDE.md의 설계 문서 섹션을 README / CLAUDE.md / 코드 단일 출처 원칙에 맞춰 정리해 stale reference가 새 합류자를 혼동시키지 않게 한다
  - 접근: `git rm DESIGN.md ROADMAP.md`로 두 파일을 통째 삭제한다. `CLAUDE.md`의 "설계 문서" 섹션에서 `DESIGN.md` / `ROADMAP.md`를 가리키는 표 행 두 줄을 제거하고, 섹션 본문이 "진실 layer는 README(사용자) / CLAUDE.md(개발자) / 코드(런타임)"임을 명시하도록 갱신한다
  - 검증 조건:
    - 결과: `git ls-files | grep -E '^(DESIGN|ROADMAP)\.md$'`가 0건을 반환하고, `CLAUDE.md`의 "설계 문서" 섹션에 `DESIGN.md` / `ROADMAP.md`를 가리키는 행이 없으며, 본문이 단일 출처 layer를 명시한다. 또한 `make build`, `make build-local`, `go test ./...` 세 명령이 변경 전과 동일하게 exit 0으로 종료한다
    - 확인: `git ls-files` + `grep -n` 결과 확인 / `CLAUDE.md`의 "설계 문서" 섹션 육안 검토 / `make build && make build-local && go test ./...` 순차 실행하여 모두 exit 0
  - 참조: SPEC §5.1, SPEC §5.3, SPEC §5.4, ANALYSIS §5.4

- [ ] task-001: `DESIGN.md` Translations 예시 스키마 동기화
  - 목적: 설계 문서의 `Translations` 예시 코드 블록이 현 코드(`widget.go`)의 구조와 일치하여, 외부 독자가 본 라벨 목록과 실제 코드가 어긋나지 않는다
  - 접근: 본 task는 task-003의 통째 제거가 먼저 실행되면 trivially 충족된다 — DESIGN.md 부재로 정합성 문제 자체가 사라지고, `ripgrep`으로 `NoContext|OneM|SevenDAll`을 비-docs 영역에서 검색했을 때 자동으로 0건이 된다. 만약 어떤 사유로 task-003 채택이 뒤집혀 DESIGN.md를 부분 유지하는 시나리오로 회귀하면, ANALYSIS §5.2의 surgical edit (Labels 3필드, Errors 줄 삭제, Widgets 5필드)로 되돌아간다
  - 검증 조건:
    - 결과: `rg -n "NoContext|OneM|SevenDAll" --glob '!docs/**'`가 0건을 반환한다. (task-003 후속 효과)
    - 확인: 해당 ripgrep 실행 후 출력 비어 있음 / `make build && make build-local && go test ./...` 순차 실행하여 모두 exit 0
  - 참조: SPEC §5.1, SPEC §5.3, ANALYSIS §4

- [ ] task-002: `.gitignore`에 module-root stray binary 패턴 추가
  - 목적: 깨끗한 트리에서 `go build ./...`를 실행한 직후 `git status`가 `nothing to commit, working tree clean`을 반환하여, module-name(`cc-usage`) 바이너리가 거짓 untracked로 노출되지 않는다
  - 접근: `.gitignore` 파일 끝에 `# go build ./... 산출물 (module root)` 주석 1줄과 `/cc-usage` 패턴 1줄을 단독 라인으로 추가한다. 기존 규칙(`*.exe`, `*.test`, `dist/`, `!bin/` allow-list, IDE 무시 등)은 손대지 않으며, 패턴은 module root에 한정되어 `bin/cc-usage-*` 등 다른 경로의 동명 파일에 영향이 없다
  - 검증 조건:
    - 결과: 깨끗한 트리에서 `go build ./...` 직후 `git status --porcelain` 출력이 비어 있고, 동시에 `git check-ignore -v cc-usage`가 `.gitignore:<line>:/cc-usage cc-usage`로 매칭을 보고한다. `bin/` 안의 기존 tracked 바이너리(`bin/cc-usage-darwin-arm64` 등)는 `git check-ignore`에 의해 ignored 보고되지 않는다. 또한 `make build`, `make build-local`, `go test ./...` 세 명령이 변경 전과 동일하게 exit 0으로 종료한다
    - 확인: 작업 트리를 정리한 뒤 `go build ./...` 실행 / `git status --porcelain` 출력이 빈 줄임을 직접 확인 / `git check-ignore -v cc-usage`로 신규 패턴 적용 여부 확인 / `git check-ignore -v bin/cc-usage-darwin-arm64` 등 기존 tracked 파일이 ignored로 잘못 잡히지 않음을 확인 / `make build && make build-local && go test ./...` 순차 실행하여 모두 exit 0
  - 참조: SPEC §5.2, SPEC §5.3, ANALYSIS §4, ANALYSIS §5.1
