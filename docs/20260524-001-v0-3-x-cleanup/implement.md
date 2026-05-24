# v0-3-x-cleanup — IMPLEMENT

- [ ] task-001: `DESIGN.md` Translations 예시 스키마 동기화
  - 목적: 설계 문서의 `Translations` 예시 코드 블록이 현 코드(`widget.go`)의 구조와 일치하여, 외부 독자가 본 라벨 목록과 실제 코드가 어긋나지 않는다
  - 접근: `DESIGN.md`의 "i18n / 다국어" 섹션 안 Translations 코드 블록(`Labels` 라인 / `Errors` 라인 / `Widgets` 블록)만 수술적으로 편집한다. `Labels`는 `FiveH, SevenD, SevenDSonnet` 3필드로 줄이고, `Errors struct{ NoContext string }` 줄은 통째로 제거하며, `Widgets` 블록은 현 코드 5필드(`ApiDuration, BurnRate, Cache, Performance, Session`)로 축소한다. 같은 섹션의 산문은 손대지 않는다
  - 검증 조건:
    - 결과: `rg -n "NoContext|OneM|SevenDAll" --glob '!docs/**'`가 0건을 반환하고, `DESIGN.md`의 Translations 블록이 `widget.go:17-41`의 `Translations` 정의와 필드 단위로 일치한다. 또한 `make build`, `make build-local`, `go test ./...` 세 명령이 변경 전과 동일하게 exit 0으로 종료한다 (DESIGN.md 편집은 Go 컴파일 대상이 아니므로 실측 영향은 없지만 회귀 가드로 함께 확인)
    - 확인: `rg -n "NoContext|OneM|SevenDAll" --glob '!docs/**'` 실행 후 출력 비어 있음을 직접 확인 / `DESIGN.md`의 변경된 블록과 `widget.go:17-41`을 side-by-side diff로 대조 / `make build && make build-local && go test ./...` 순차 실행하여 모두 exit 0
  - 참조: SPEC §5.1, SPEC §5.3, ANALYSIS §4, ANALYSIS §5.2

- [ ] task-002: `.gitignore`에 module-root stray binary 패턴 추가
  - 목적: 깨끗한 트리에서 `go build ./...`를 실행한 직후 `git status`가 `nothing to commit, working tree clean`을 반환하여, module-name(`cc-usage`) 바이너리가 거짓 untracked로 노출되지 않는다
  - 접근: `.gitignore` 파일 끝에 `# go build ./... 산출물 (module root)` 주석 1줄과 `/cc-usage` 패턴 1줄을 단독 라인으로 추가한다. 기존 규칙(`*.exe`, `*.test`, `dist/`, `!bin/` allow-list, IDE 무시 등)은 손대지 않으며, 패턴은 module root에 한정되어 `bin/cc-usage-*` 등 다른 경로의 동명 파일에 영향이 없다
  - 검증 조건:
    - 결과: 깨끗한 트리에서 `go build ./...` 직후 `git status --porcelain` 출력이 비어 있고, 동시에 `git check-ignore -v cc-usage`가 `.gitignore:<line>:/cc-usage cc-usage`로 매칭을 보고한다. `bin/` 안의 기존 tracked 바이너리(`bin/cc-usage-darwin-arm64` 등)는 `git check-ignore`에 의해 ignored 보고되지 않는다. 또한 `make build`, `make build-local`, `go test ./...` 세 명령이 변경 전과 동일하게 exit 0으로 종료한다
    - 확인: 작업 트리를 정리한 뒤 `go build ./...` 실행 / `git status --porcelain` 출력이 빈 줄임을 직접 확인 / `git check-ignore -v cc-usage`로 신규 패턴 적용 여부 확인 / `git check-ignore -v bin/cc-usage-darwin-arm64` 등 기존 tracked 파일이 ignored로 잘못 잡히지 않음을 확인 / `make build && make build-local && go test ./...` 순차 실행하여 모두 exit 0
  - 참조: SPEC §5.2, SPEC §5.3, ANALYSIS §4, ANALYSIS §5.1
