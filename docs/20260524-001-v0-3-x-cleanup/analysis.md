# v0-3-x-cleanup — ANALYSIS

## 근거

- `docs/20260524-001-v0-3-x-cleanup/spec.md` — 완료 조건 3건 (DESIGN.md 동기화 / .gitignore stray binary / 빌드·테스트 회귀 없음).
- `widget.go:17-41` — 현 `Translations` 정의. `Labels`는 `FiveH / SevenD / SevenDSonnet` 3개, `Widgets`는 `ApiDuration / BurnRate / Cache / Performance / Session` 5개. `Errors` 서브구조는 존재하지 않음.
- `DESIGN.md:417-432` — historical 예시 스키마. `Labels`에 `SevenDAll`, `OneM`이, 별도로 `Errors struct{ NoContext string }` 줄이 남아 있음. `Widgets` 블록은 v0.2.x 시절 분석 위젯들(`Tools`, `Done`, `ClaudeMd`, `TokenBreakdown`, …)을 포함해 현 코드보다 훨씬 큼.
- `.gitignore` 전체 35줄 — 12~14줄에 `!bin/` / `!bin/**` allow-list. `*.exe`, `*.test`, `dist/` 등은 있지만 module-name 바이너리(`cc-usage`)에 대응하는 규칙은 없음.
- `go.mod:1` — module path는 `github.com/mypkg/cc-usage`. `go build ./...`는 마지막 path component인 `cc-usage`를 바이너리로 떨군다.
- 재현: 깨끗한 트리에서 `go build ./...` 직후 `git status --porcelain`에 `?? cc-usage`가 노출됨을 직접 확인.
- 패턴 검증: 임시 디렉터리에서 `/cc-usage`와 `cc-usage` 둘 다 `bin/cc-usage-darwin-arm64`를 무시하지 않음을 `git status --ignored`로 확인 (`!bin/**`가 살아 있고, `bin/cc-usage-*` 파일명은 `cc-usage`와 정확 매칭이 아님).
- 비-docs 영역 stale 토큰: `rg -n "NoContext|OneM|SevenDAll" --glob '!docs/**'` 결과는 `DESIGN.md:421`, `DESIGN.md:423`만. 다른 파일에는 잔존하지 않음.

## 1. 구조

코드 모듈 변경 없음. 문서 1곳 + 메타 1곳.

- `DESIGN.md` "i18n / 다국어" 안의 **Translations 구조** 코드 블록(`DESIGN.md:417-432`)을 `widget.go:17-41` 정의와 정합시킨다 (SPEC §5.1). 같은 섹션의 산문(언어 감지, 포매팅 유틸리티 등)은 손대지 않는다.
- `.gitignore`에 module-root stray binary용 패턴 1줄 추가 (SPEC §5.2). 기존 규칙(`*.exe`, `*.test`, `dist/`, `!bin/` allow-list, IDE 무시 등)은 보존.

다른 모듈은 영향 받지 않음 — SPEC §5.3의 빌드·테스트 명령은 변경된 파일을 컴파일하지 않는다.

## 2. 데이터 흐름

본 작업은 런타임 데이터 경로를 건드리지 않으므로 흐름 분석은 해당 없음. 검증 흐름만 기록:

1. `rg -n "NoContext|OneM|SevenDAll" --glob '!docs/**'` → 0건 (SPEC §5.1 검증).
2. 깨끗한 트리에서 `go build ./...` → `git status --porcelain` 빈 출력 (SPEC §5.2 검증).
3. `make build`, `make build-local`, `go test ./...` 모두 exit 0 (SPEC §5.3 검증).

## 3. 인터페이스

외부에 노출되는 인터페이스(CLI 플래그, stdin/stdout 프로토콜, Config 스키마, 위젯 API, locale JSON 키) 변경 없음. 해당 없음.

## 4. 영향 범위

| 파일 | 변경 형태 | 범위 |
|------|-----------|------|
| `DESIGN.md` | 편집 (예시 코드 블록 일부 줄 수정) | line 421 `Labels` 필드 목록에서 `SevenDAll`, `OneM` 제거 / line 423 `Errors struct{ NoContext string }` 줄 제거 / line 424-430 `Widgets` 블록을 현 코드 5필드로 축소 |
| `.gitignore` | 패턴 1줄 추가 | 파일 끝 또는 `*.test` 인접 위치 |

깨질 수 있는 caller — 없음:

- `Translations` Go 코드는 변경하지 않음. locale JSON / Go struct 정합성은 그대로.
- `.gitignore`는 add-only 변경. 기존 tracked 파일은 영향 없음 (`git check-ignore`로 사후 검증 가능).
- bin/ 산출물 재빌드는 SPEC §3 (제약)에 따라 트리거되지 않음.

## 5. Decision Points

### 5.1 `.gitignore` 패턴 선택 (SPEC §5.2)

| 패턴 | 의도 명확성 | side effect 위험 |
|------|-------------|------------------|
| `/cc-usage` | root의 module-name 바이너리만 무시 | 매우 낮음 — 다른 경로의 `cc-usage` 이름 파일/디렉터리에는 영향 없음 |
| `cc-usage` | 임의 경로의 정확한 이름을 무시 | 낮음 — 현재 `bin/cc-usage-*`는 이름이 다르므로 안전. 미래에 다른 위치에 같은 이름이 들어오면 의도와 다른 무시 가능 |

**채택: `/cc-usage`**. SPEC §3 "기존 규칙을 약화시키지 않는다"와 §5.2 "거짓 untracked 방지"를 정확히 만족시키면서 부수효과 최소. 파일 끝에 단독 줄로 두되, 가독성을 위해 `# go build ./... 산출물 (module root)` 1줄 주석 동반.

### 5.2 `DESIGN.md` 편집 범위 (SPEC §5.1)

SPEC §3 "historical 의도·아키텍처 설명은 보존, 현 코드와 어긋난 사실 부분만 동기화"를 따라 **최소 수술적 편집**으로 한정:

- `Labels` 라인 — `FiveH, SevenD, SevenDSonnet`만 남긴다.
- `Errors` 줄 — 통째로 삭제.
- `Widgets` 블록 — 현 코드의 5필드(`ApiDuration, BurnRate, Cache, Performance, Session`)로 축소. v0.2.x 흔적 필드(`Tools`, `Done`, `Running`, `Agent`, `Todos`, `ClaudeMd`, `AgentsMd`, `AddedDirs`, `Rules`, `Mcps`, `Hooks`, `ToLimit`, `Forecast`, `Budget`, `TokenBreakdown`, `TodayCost`, `PeakHours`, `OffPeak`)는 모두 제거.

같은 코드 블록 바깥의 산문은 건드리지 않는다. ROADMAP/PLAN/IMPLEMENT 정합성은 SPEC §4(제외)에 따라 본 작업 범위 밖.

### 5.3 bin/ 재빌드 여부 (SPEC §3)

`.gitignore`와 `DESIGN.md`만 바뀌고 Go 소스는 변경 없음. SPEC §3 "본 spec 작업만으로는 트리거되지 않는다"를 그대로 따른다. `make build`는 회귀 검증용으로만 실행하고, 산출물이 byte-equal이 아니더라도 commit하지 않는다.
