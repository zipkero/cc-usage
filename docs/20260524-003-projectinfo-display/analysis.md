# projectinfo-display — ANALYSIS

## 근거

### 읽은 자료
- `docs/20260524-003-projectinfo-display/spec.md` 전체 (§5.1–§5.8).
- `widgets_project.go` 전체 (projectInfoData, GetData, Render, init).
- `widget.go` — `OrchestrateResult`(151–158)와 `orchestrate`의 projectInfo 분기(200–205).
- `main.go` — 출력 단계(132–147)의 `HasProject` prefix 분기.
- `format.go` — 다른 위젯도 쓰는 `truncate(s, 25)` 헬퍼.
- `Makefile`, `.claude-plugin/plugin.json`, `api.go:61` — v0.3.x bump 위치.
- `CLAUDE.md` §위젯 추가 절차, §배포, §버전 정책.

### 코드베이스에서 확인한 사실
- `result.ProjectInfo` / `result.HasProject`의 외부 참조: `widget.go` 정의 + 할당과 `main.go` 출력 분기뿐. 다른 파일·테스트 0건. 두 필드 제거 시 컴파일 영향 범위가 같은 두 파일에 갇힌다.
- `projectInfoData.Subpath`의 외부 참조: `widgets_project.go` 내부 4곳뿐. 다른 파일에 정의·소비 없음.
- `format.go`의 `truncate(s, 25)`는 모델명 등 다른 위젯도 사용 — projectInfo 전용으로 임의 길이를 받게 일반화하면 다른 호출자에 영향. path 전용 helper는 widget 내부에 따로 두는 게 충돌 없음.
- 사용자 결정(AskUserQuestion): home-tilde 압축 + v0.3.2 patch bump + spec-flow.
- `os.UserHomeDir()`은 cc-usage의 다른 경로(api.go, cache.go)에서 표준 사용 중. 같은 호출로 home 결정.

### 추정과 미확인
- v0.3.1에서 저장된 session-state cache의 `widget_count`는 projectInfo를 제외한 값이다. v0.3.2 첫 호출은 projectInfo를 포함한 새 widget_count로 비교하므로 한 번 degraded로 오인될 수 있다. 그러나 이 분기는 "cache 복원 → 재orchestrate" 안전 방향으로 흐르고, 같은 호출 끝의 `saveSessionState`가 새 카운트로 덮어써 자기-회복한다. 별도 마이그레이션 불필요.
- Windows path 분리자(`\`) 환경의 실 사용은 측정하지 않았다. `filepath.Separator`로 처리되므로 압축·잘림 알고리즘 정합성은 유지된다고 본다.

---

## 1. 구조

본 변경은 새 모듈을 만들지 않고 세 기존 파일 안에서 끝난다.

- **`widgets_project.go`가 path 표시 로직을 소유**한다. home-tilde 압축과 길이 정규화는 widget 내부의 private helper 두 개로 둔다(`format.go`로 끌어올리지 않음 — 다른 위젯이 동일 변환을 필요로 하지 않고, format.go의 `truncate`는 단일 토큰용으로 남겨둔다). `projectInfoData`에서 `Subpath` 필드를 제거하고 그 대신 표시용 `DisplayPath` 필드를 둔다 (SPEC §5.1, §5.2, §5.3, §5.4).
- **`widget.go`의 `OrchestrateResult`에서 projectInfo 특수 필드 두 개(`ProjectInfo`, `HasProject`)를 제거**하고, `orchestrate` 루프의 분기를 일반 위젯과 동일하게 합친다 (SPEC §5.6). 결과적으로 `WidgetCount`의 의미가 "projectInfo 제외 → projectInfo 포함"으로 한 토큰 단위로 이동한다.
- **`main.go`의 출력 단계에서 `HasProject` prefix 분기 제거**. `partsOutput` 한 줄 print로 단순화 (SPEC §5.6).
- **v0.3.2 patch bump**는 본 변경과 같은 commit에 묶는다. `Makefile` VERSION, `.claude-plugin/plugin.json` version, `api.go:61` userAgent 세 곳을 같은 값으로 동시 갱신한다 (SPEC §5.8, CLAUDE.md §버전 정책).

## 2. 데이터 흐름

### 2.1 path 표시 경로 (SPEC §5.1–§5.5)

```
Stdin.Workspace.CurrentDir
        │
        ▼
projectInfoWidget.GetData(ctx)
   ├─ 1) home := os.UserHomeDir() (실패 시 빈 문자열)
   ├─ 2) compressed := compress(currentDir, home)
   │        current == home          → "~"
   │        current가 home + sep로 시작 → "~" + tail
   │        그 외                    → currentDir 그대로
   ├─ 3) display := shrink(compressed, maxLen=50)
   │        길이 OK                   → compressed
   │        base name만으로도 초과     → base name 단독
   │        그 외                    → 앞 segment들을 "…/"로 일괄 치환 (base 보존)
   ├─ 4) Worktree from ctx.Stdin.Worktree
   └─ 5) git status --porcelain=v2 --branch → Branch / Ahead / Behind
        │
        ▼
projectInfoWidget.Render
   ├─ DisplayPath (theme.Folder)
   ├─ " (Branch [↑Ahead] [↓Behind])" — v0.3.1 형식 그대로
   └─ " [Worktree]" — v0.3.1 형식 그대로
        │  (subpath 토큰 미등장, SPEC §5.4)
        ▼
orchestrate loop의 일반 분기로 합류 (SPEC §5.6)
        │
        ▼
main.go: strings.Join(result.Lines, "\n") → stdout (prefix 분기 없음)
```

`~` prefix 보존: shrink 단계에서 `~/very/deep/path/proj` 가 50자 초과 시 `~/…/path/proj` 형태로 줄어 "home 안" 신호를 잃지 않는다. 절대 경로 케이스(`/Users/x/very/deep/path/proj`)도 같은 알고리즘으로 `/Users/…/path/proj` 형태가 된다.

### 2.2 home 조회 실패 경로 (SPEC §5.2)

`os.UserHomeDir()` 에러 시 home이 빈 문자열로 흘러 compress가 currentDir 그대로 반환한다. 그래서 base name 단독 노출은 어떤 경로로도 발생하지 않는다 — SPEC §5.2 후반 가드를 자동 만족.

### 2.3 cache 호환성 경로

v0.3.1 cache(projectInfo 제외 widget_count) → v0.3.2 첫 호출(projectInfo 포함 widget_count) 비교:

```
cached.WidgetCount = N
result.WidgetCount = N + 1   (projectInfo 포함)
usageDegraded = (N+1 < N) = false  → 정상 동작
```

오히려 반대 방향(`cached.WidgetCount = N+1`, `result.WidgetCount = N`)이 우려되는데, 이는 v0.3.2 cache가 먼저 쌓인 뒤 v0.3.1 바이너리로 되돌리는 극단 케이스에서만 발생하고 본 spec 범위 밖이다. v0.3.1 → v0.3.2 일반 업그레이드 경로에서 cache는 한 호출 사이클 안에 새 카운트로 자기-덮어쓰기된다.

## 3. 인터페이스

- **외부 contract 변화 없음**. CLI 플래그, stdin/stdout 프로토콜, Config 스키마, locale JSON 키, 위젯 등록 인터페이스 모두 보존 (SPEC §3).
- **세션 cache JSON 포맷 변화 없음**. `SessionState`는 stdin 그대로 보관하며 `OrchestrateResult`를 보관하지 않는다 — 따라서 `OrchestrateResult.ProjectInfo`/`HasProject` 제거가 디스크 호환성에 영향이 없다.
- **새 export 없음**. path 변환 helper 두 개는 같은 `main` 패키지 내부 private 함수.

## 4. 영향 범위

| 파일 | 변경 형태 |
|------|-----------|
| `widgets_project.go` | `projectInfoData.Subpath` 제거 / `DisplayPath` 추가 / GetData에 home-tilde 압축 + 길이 정규화 추가 / Render에서 subpath 분기 제거 |
| `widget.go` | `OrchestrateResult.ProjectInfo`/`HasProject` 제거 / orchestrate의 `if widgetID == "projectInfo"` 특수 분기 제거 |
| `main.go` | 출력 단계의 `HasProject` prefix 분기 제거 (partsOutput 단일 print) |
| `Makefile` | VERSION `0.3.1` → `0.3.2` |
| `.claude-plugin/plugin.json` | version `0.3.1` → `0.3.2` |
| `api.go` | userAgent `cc-usage/0.3.1` → `cc-usage/0.3.2` |

**호환성 확인**:
- `result.ProjectInfo`/`HasProject` 외부 참조: 없음 (grep 확인).
- `projectInfoData.Subpath` 외부 참조: 없음 (grep 확인).
- userAgent 문자열을 직접 검증하는 테스트 여부는 IMPLEMENT 단계에서 grep 후, 있으면 같은 commit에서 갱신.
- session-state cache widget_count 의미 변화: §2.3에서 다룸 (한 cycle 자기-회복).

bin/ 재빌드와 release 브랜치 sync는 본 spec 작업의 소스 변경 결과로 별도 Task에서 처리한다 (CLAUDE.md §배포 §릴리스 절차).

## 5. Decision Points

### A. 길이 임계값과 잘림 알고리즘 (SPEC §5.3)

옵션:
- **A1 — 문자 단위 단일 truncate**: `format.go`의 `truncate(s, n)`을 그대로 호출. segment 경계를 무시하고 rune 단위로 잘라 `~/GolandProjec…` 같은 의미 없는 출력이 나옴.
- **A2 — segment-aware 압축, 임계값 50 rune, base 보존**: 길이가 임계값을 넘으면 앞 segment들을 통째로 `…/`로 치환. base name이 단독으로도 임계값보다 길면 그대로 둔다. `~` prefix는 보존 — `~/very/deep/path/proj` → `~/…/path/proj`.
- **A3 — segment-aware + 사용자 설정 임계값**: Config에 새 옵션 추가. SPEC §3가 "임계값 조정용 새 옵션을 도입하지 않는다"를 명시하므로 위반.

**채택: A2, 임계값 50 rune**.
- A1: rune 단위 자르기가 path semantics를 파괴한다. 기각.
- A3: SPEC §3 위반. 기각.
- 임계값 50 근거: 다른 위젯이 합쳐 약 35–50자(`◆ Opus │ ████░░░░ 30% 60K │ $1.25` ≈ 40자, rate-limit 추가 시 +20–25자)를 차지. path 50 + 다른 위젯 60 ≈ 110자로 일반 터미널 폭 안. 40은 깊은 경로(`~/work/2026/repo/sub`)에서도 `…/` 발동이 잦아 정보 손실 큼, 60은 status line 폭 균형 경계.
- helper와 임계값은 widget 내부 상수/private 함수로 둔다 (SPEC §3: 새 옵션 도입 금지와 정합).

### B. `OrchestrateResult.ProjectInfo` / `HasProject` 처리 (SPEC §5.6)

옵션:
- **B1 — 두 필드 모두 제거, orchestrate 분기와 main의 prefix 분기도 함께 제거**.
- **B2 — 필드를 유지하되 populate 안 함** (defensive backwards-compat).
- **B3 — projectInfo가 라인의 첫 위치면 `result.ProjectInfo`를 채우는 fast path 유지**.

**채택: B1**.
- B2: `OrchestrateResult`는 외부 export 아님(`main` 패키지 내부 struct). dead state로 남길 이유 없음.
- B3: SPEC §5.6이 "preset 선언 위치 = 출력 위치"를 명시. 첫 위치 특수화 자체가 SPEC 위반.
- B1의 안전성: 외부 참조 없음을 grep으로 확인. main.go의 출력 분기 제거가 컴파일 단계에서 검증된다.

### C. home-tilde 경계 처리 (SPEC §5.1, §5.2)

옵션:
- **C1 — `strings.HasPrefix(current, home + filepath.Separator)`만 검사**. `current == home` 정확 일치 시 prefix 매치가 실패해 절대 경로로 떨어진다.
- **C2 — `filepath.Rel(home, current)`로 외부 여부 판정**. 결과가 `..`로 시작하면 외부. Windows에서 다른 drive 경계 시 error 가능성 + 추가 정규화 비용.
- **C3 — `current == home` short-circuit + `HasPrefix(home + sep)` 결합**. 두 분기 모두 명시.

**채택: C3**.
- C1: home 디렉터리 자체에서 호출하면 path가 절대 경로로 떨어져 SPEC §5.1의 "~ 압축" 정신과 어긋남.
- C2: 명료성과 이식성에서 prefix 비교가 우월하다. `filepath.Rel`은 error 표면이 추가되어 home 조회 실패와 경계 case 처리가 섞임.
- C3은 코드 두 줄로 명확. 비용 무시 가능 (같은 호출에서 `git status` 500ms timeout 서브프로세스가 함께 도는데 string 비교는 nanosecond 단위).

### D. `projectInfoData.Subpath` 제거 (SPEC §5.4)

옵션:
- **D1 — 필드·population·Render 분기 모두 제거**.
- **D2 — 필드 유지, Render에서만 분기 스킵**.

**채택: D1**.
- 외부 참조 없음 (grep 확인 — widgets_project.go 내부 4곳뿐).
- SPEC §4 명시: "subpath 별도 표시 — full path에 이미 포함되므로 제거".
- D2: 죽은 상태값. 캐시 직렬화에도 등장하지 않으므로 제거에 호환성 부담 없음.

### E. v0.3.2 bump 패키징 (SPEC §5.8)

옵션:
- **E1 — feature 변경과 v0.3.2 bump를 같은 commit에 묶음**. CLAUDE.md §버전 정책 ("user-facing 변경 commit에 version bump 포함, version-only commit은 권장하지 않음")과 정합.
- **E2 — 두 commit 분리** (feature → bump). traceability 측면 미세 이점이지만 정책 위반.

**채택: E1**.
- 세 곳(`Makefile`, `.claude-plugin/plugin.json`, `api.go:61`) 동시 갱신.
- `dist/cc-usage --version` → `0.3.2` 확인 (SPEC §5.8).
- `bin/` 재빌드와 release 브랜치 sync는 binary 산출물 commit이라 feature diff와 섞지 않는다 — IMPLEMENT 후반의 별개 Task로 분리한다 (CLAUDE.md §릴리스 절차의 "main에서 bin/ 갱신 → release worktree sync" 흐름).
