# v0-3-x-cleanup — ANALYSIS

## 근거

- `docs/20260524-001-v0-3-x-cleanup/spec.md` — 완료 조건 4건 (Translations 정합성 / .gitignore stray binary / 빌드·테스트 회귀 없음 / DESIGN.md+ROADMAP.md 부재 + CLAUDE.md 갱신).
- `widget.go:17-41` — 현 `Translations` 정의. `Labels`는 `FiveH / SevenD / SevenDSonnet` 3개, `Widgets`는 `ApiDuration / BurnRate / Cache / Performance / Session` 5개. `Errors` 서브구조는 존재하지 않음.
- `DESIGN.md` 섹션별 점검 결과, 모든 섹션이 README / CLAUDE.md / 코드와 중복 또는 stale:
  - Overview / Plugin 계약 / Config / 위젯 명세 → README + CLAUDE.md
  - Stdin JSON 예시 / API 응답 구조 → `stdin.go` / `api.go`가 진실 (캐시 파일이 실측 sample)
  - Widget 인터페이스 / 테마 / 렌더링 / 포매팅 / i18n → 각 소스 파일 + `widget.go` 그 자체
  - API 클라이언트 (3-tier 캐시) → CLAUDE.md가 더 최신 (task-005 memCache 제거 반영)
  - i18n Translations 예시 → task-007에서 제거된 라벨이 stale
  - 위젯 목록 (Phase 1 MVP / 확장) → README §Widgets
  - Display Modes / Preset → README §Configuration + `widget.go` presetCharToWidget
  - 빌드 & 배포 → Makefile + CLAUDE.md §배포
- `ROADMAP.md` 섹션별 점검: Phase 2의 burnRate/cacheHit/sessionDuration은 v0.2.0에 이미 구현됨, Phase 3의 tokenSpeed는 v0.2.0 정리에서 데이터 부재로 제외 결정됨, Phase 5(codex/gemini/zai)는 외부 의존 스코프 밖으로 명시 결정됨. 나머지 미구현 위젯은 향후 새 spec-flow를 거칠 예정이라 옛 스펙은 stale 참고로만 의미가 있고 actionable 가치 0.
- `CLAUDE.md:124-133` "설계 문서" 섹션 — `DESIGN.md`와 `ROADMAP.md`를 reference로 표시 중. 두 파일 제거에 맞춰 같이 정리 필요.
- `.gitignore` 전체 35줄 — 12~14줄에 `!bin/` / `!bin/**` allow-list. `*.exe`, `*.test`, `dist/` 등은 있지만 module-name 바이너리(`cc-usage`)에 대응하는 규칙은 없음.
- `go.mod:1` — module path는 `github.com/mypkg/cc-usage`. `go build ./...`는 마지막 path component인 `cc-usage`를 바이너리로 떨군다.
- 재현: 깨끗한 트리에서 `go build ./...` 직후 `git status --porcelain`에 `?? cc-usage`가 노출됨을 직접 확인.
- 패턴 검증: 임시 디렉터리에서 `/cc-usage` 패턴이 `bin/cc-usage-*` allow-list와 충돌하지 않음을 `git status --ignored`로 확인.
- 비-docs 영역 stale 토큰: DESIGN.md 제거로 `rg -n "NoContext|OneM|SevenDAll" --glob '!docs/**'`가 자동으로 0건이 됨.

## 1. 구조

코드 모듈 변경 없음. 문서 3곳 + 메타 1곳.

- `DESIGN.md` 통째 삭제 (SPEC §5.4). 통째 제거가 부분 유지보다 단일 출처 원칙(SPEC §2)에 부합.
- `ROADMAP.md` 통째 삭제 (SPEC §5.4). 향후 위젯 도입은 새 spec-flow를 거친다.
- `CLAUDE.md`의 "설계 문서" 섹션 갱신 — DESIGN/ROADMAP을 가리키는 표 행 제거, README / CLAUDE.md / 코드가 진실 layer임을 명시 (SPEC §5.4).
- `.gitignore`에 module-root stray binary용 패턴 1줄 추가 (SPEC §5.2). 기존 규칙은 보존.

`Translations` 정합성(SPEC §5.1)은 DESIGN.md 통째 제거의 자동 부수효과로 충족된다 — 파일이 사라지면 `NoContext|OneM|SevenDAll` ripgrep 매치도 자동 0건이 된다.

## 2. 데이터 흐름

본 작업은 런타임 데이터 경로를 건드리지 않으므로 흐름 분석은 해당 없음. 검증 흐름만 기록:

1. `git ls-files | grep -E '^(DESIGN|ROADMAP)\.md$'` → 0건 (SPEC §5.4 검증).
2. `rg -n "NoContext|OneM|SevenDAll" --glob '!docs/**'` → 0건 (SPEC §5.1 자동 충족 검증).
3. 깨끗한 트리에서 `go build ./...` → `git status --porcelain` 빈 출력 (SPEC §5.2 검증).
4. `make build`, `make build-local`, `go test ./...` 모두 exit 0 (SPEC §5.3 검증).
5. `CLAUDE.md`의 "설계 문서" 섹션에 `DESIGN.md` / `ROADMAP.md` 행이 없음을 육안 확인 (SPEC §5.4 검증).

## 3. 인터페이스

외부에 노출되는 인터페이스(CLI 플래그, stdin/stdout 프로토콜, Config 스키마, 위젯 API, locale JSON 키) 변경 없음. 해당 없음.

## 4. 영향 범위

| 파일 | 변경 형태 | 범위 |
|------|-----------|------|
| `DESIGN.md` | 삭제 | 파일 통째 |
| `ROADMAP.md` | 삭제 | 파일 통째 |
| `CLAUDE.md` | 편집 | "설계 문서" 섹션의 표 행 두 줄 제거, 본문을 README/CLAUDE/코드가 진실 layer임을 명시하도록 갱신 |
| `.gitignore` | 패턴 1줄 추가 | 파일 끝 또는 `*.test` 인접 위치 |

깨질 수 있는 caller — 없음:

- `Translations` Go 코드 변경하지 않음. locale JSON / Go struct 정합성 그대로.
- `DESIGN.md` / `ROADMAP.md`를 link로 참조하는 다른 문서 없음 (release 브랜치 README는 master 브랜치만 가리킴, marketplaces 인스톨 사용자는 두 파일을 못 보던 상태).
- `.gitignore`는 add-only 변경. 기존 tracked 파일은 영향 없음.
- bin/ 산출물 재빌드는 SPEC §3에 따라 트리거되지 않음.

## 5. Decision Points

### 5.1 `.gitignore` 패턴 선택 (SPEC §5.2)

| 패턴 | 의도 명확성 | side effect 위험 |
|------|-------------|------------------|
| `/cc-usage` | root의 module-name 바이너리만 무시 | 매우 낮음 — 다른 경로의 `cc-usage` 이름 파일/디렉터리에는 영향 없음 |
| `cc-usage` | 임의 경로의 정확한 이름을 무시 | 낮음 — 현재 `bin/cc-usage-*`는 이름이 다르므로 안전. 미래에 다른 위치에 같은 이름이 들어오면 의도와 다른 무시 가능 |

**채택: `/cc-usage`**. SPEC §3 "기존 규칙을 약화시키지 않는다"와 §5.2 "거짓 untracked 방지"를 정확히 만족시키면서 부수효과 최소. 파일 끝에 단독 줄로 두되, 가독성을 위해 `# go build ./... 산출물 (module root)` 1줄 주석 동반.

### 5.2 `DESIGN.md` 편집 범위 (SPEC §5.1)

본 결정은 5.4 채택 결과(통째 제거)로 인해 적용 대상이 사라졌다. SPEC §5.1은 DESIGN.md 부재로 자동 충족되며, 본 결정의 내용 — 최소 수술적 편집 vs 광범위 재작성 — 는 더 이상 implement 단계에서 평가되지 않는다. 결정 기록만 보존.

### 5.3 bin/ 재빌드 여부 (SPEC §3)

`.gitignore` 패턴 추가와 historical 문서 제거 모두 Go 소스 변경이 아니므로 SPEC §3 "본 spec 작업만으로는 트리거되지 않는다"를 그대로 따른다. `make build`는 회귀 검증용으로만 실행하고 산출물을 commit하지 않는다.

### 5.4 `DESIGN.md` / `ROADMAP.md` 처리 방식 (SPEC §5.4)

| 옵션 | 트레이드오프 |
|------|-------------|
| A. 부분 유지 (Translations 등 stale 부분만 동기화, 나머지 보존) | historical reference 보존이라는 명목상 가치는 있지만 — DESIGN.md의 모든 섹션이 이미 다른 곳에 더 정확하게 존재해 가치 0이고, ROADMAP.md는 v0.2.0 정리에서 다수 항목이 명시 폐기됨. 부분 유지는 stale 위험을 영구화함 |
| B. 통째 제거 | 단일 출처 원칙(SPEC §2)에 부합. 향후 새 위젯 도입은 새 spec-flow를 거치므로 ROADMAP 미구현 스펙의 actionable 가치는 0. CLAUDE.md 설계 문서 섹션도 같이 정리 |

**채택: B (통째 제거)**. 근거 — DESIGN.md의 어느 섹션도 README/CLAUDE.md/코드 대비 unique 가치를 더하지 않음을 섹션별로 확인했고, ROADMAP.md는 v0.2.0 정리 시점의 가설이라 현재 결정과 충돌하는 항목이 더 많음. 통째 제거가 SPEC §5.1(Translations 정합성)을 자동 충족시키는 부수효과도 있음.
