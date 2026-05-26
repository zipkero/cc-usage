# ANALYSIS — transcript-truth-source

## 근거

기준 코드: v0.3.10 HEAD (`c17043a`). 직전 두 feature(`safe-empty-stdin-fallback` v0.3.7, `atomic-degraded-restore` v0.3.9, `warmup-leak` v0.3.10)가 도입한 흐름 위에 본 feature를 얹는다.

읽은 spec 범위: `docs/20260526-001-transcript-truth-source/spec.md` 전체(§1–§5, 15개 완료 조건, §3에 D1=(b)/D2=(ii) 결정 commit).

읽은 코드 사실:

- `main.go:78–151`. v0.3.10의 흐름은 `parseStdin` → `sessionCacheKey` → `resolveCachedSessionState` → 1차 `orchestrate` → `shouldRestoreFromSession` 통과 시 `fillFromSessionCache` + 2차 `orchestrate` → `shouldSuppressOutput` → `isWarmupExceptionPath`이면 `renderRateLimitOnly`로 교체 → 저장 직전 `stripRestoredFields`로 atomic 보장. 본 feature는 1·2차 orchestrate 사이의 단일 eligibility 결정에 **transcript 단계**를 추가한다.
- `main.go:171–177` `resolveCachedSessionState`. v0.3.8에서 `cacheKey == "" && cached == nil` → `cached == nil` 단일 가드로 완화. session_id가 있어도 per-key 캐시가 비면 `loadByWorkspaceCwd` 폴백. 본 feature는 이 함수에는 손대지 않고 main의 backfill 단계만 확장.
- `cache.go:13–28` TTL. `sessionStateTTL = 300s`(파일 유효 기간), `workspaceRestoreTTL = 60s`(단일 atomic-restore eligibility 게이트). 본 feature는 두 상수 모두 무변경. transcript 폴백은 두 TTL 어느 쪽에도 종속되지 않는다.
- `cache.go:386–415` `shouldRestoreFromSession`. 4단 게이트(cached 존재 + SavedAt + 60s 이내 + cwd 일치 + 최소 1필드 비어있음). 본 feature의 transcript 단계는 **이 게이트가 false 반환했거나 fillFromSessionCache 이후에도 model/ContextWindow가 여전히 비어있을 때**만 발동한다(중복 작업 방지).
- `cache.go:424–459` `fillFromSessionCache`. 필드별 emptiness 룰로 atomic backfill, `restoredFieldMask` 반환. 본 feature는 같은 mask 구조를 transcript 분에도 재사용해 `stripRestoredFields`의 self-perpetuation 차단을 통합한다.
- `cache.go:104–120` `detectCurrentCwd`/`detectCurrentCwdWithSource`. `CLAUDE_PROJECT_DIR` env → `os.Getwd()`. `normalizeCwd`로 `EvalSymlinks` 정규화. 본 feature의 transcript 디렉토리 인코딩 입력으로 동일 함수 재사용.
- `widgets_core.go:20–28, 68–88, 116–122`. `modelWidget`은 ID·DisplayName 둘 다 비면 nil. `contextWidget`은 `ContextWindowSize <= 0`이면 nil. `costWidget.GetData`는 항상 non-nil(음수만 0으로 정규화) — D6에서 변경 대상.
- `widgets_project.go:33–37`. v0.3.10 시점 `projectInfoWidget.GetData`는 `Workspace.CurrentDir == ""`이면 nil. SPEC §5.4가 명시한 `detectCurrentCwd` fresh fallback은 본 feature에서 처음 도입.
- `stdin.go`. `StdinInput.TranscriptPath`(정상 stdin 제공)·`Model.ID`·`ContextWindow.{ContextWindowSize, TotalInputTokens, TotalOutputTokens, CurrentUsage.*}`·`Cost.TotalCostUsd`가 본 feature가 다루는 필드. 스키마 변경 없음.

실측 확인된 transcript 포맷(2026-05-26, Claude Code v2.1.150 표본):

- 경로: `~/.claude/projects/-<encoded_cwd>/<session_id>.jsonl`. cwd 인코딩은 `/`와 `.`을 모두 `-`로 치환하는 forward-only 함수. lossy → reverse 디코딩 불가. forward만 필요(현재 cwd를 cc-usage가 알고 있음).
- 각 line은 JSON entry, `type` 분류. 본 feature 소비 대상은 `type == "assistant"`. 다른 type(`user`, `system`, `attachment`, `file-history-snapshot`, `permission-mode`, `ai-title`, `last-prompt`, `queue-operation`)은 skip.
- `assistant` entry top-level: `parentUuid, isSidechain, message, requestId, type, uuid, timestamp, userType, entrypoint, cwd, sessionId, version, gitBranch`. cost·duration 필드 **없음** — D1 (b) 정책의 단가표 합산 필요성 근거.
- `message`: `model, id, type, role, content, stop_reason, stop_sequence, stop_details, usage, diagnostics`.
- `message.model` 예: `"claude-opus-4-7"`. stdin이 보내는 `[1m]` suffix가 transcript에는 **없음** — D2 (ii) cwd별 last-known `[1m]` 캐시 필요성 근거.
- `message.usage`: `input_tokens, cache_creation_input_tokens, cache_read_input_tokens, output_tokens, cache_creation.{ephemeral_5m_input_tokens, ephemeral_1h_input_tokens}, service_tier`. stdin `context_window.current_usage`의 4개 핵심 필드와 동치.
- 파일 크기: 같은 cwd 안에서 5MB+ 관찰. 매 호출 풀 read 불가 — D5 tail window 제약.
- 같은 cwd 디렉토리 안에 여러 jsonl 공존(세션 하나당 하나). 빈 stdin이면 mtime 가장 최근 파일을 후보로(D3).
- Claude Code가 append-only로 쓴다. read 시점에 마지막 partial line 가능 — D6 처리.

추정과 사실 분리:

- **확인됨**: 위 포맷·필드·디렉토리 규칙·`[1m]` suffix 부재·append-only·파일 크기 관찰값.
- **추정**: tail read window 최적 크기(D5에서 64KB 시작 + 2배 확장 + 상한 1MB로 commit). 단가표의 ephemeral 가격 분기 처리 디테일(D11에서 hardcode embed 형태 commit, 데이터값 자체는 implement 단계에서 Anthropic 공식 가격 참조).

## 1. 구조

기존 단일 `main` 패키지 안에서 끝난다. 서브 패키지 신설 없음(CLAUDE.md §패키지). 신규 파일 두 개와 기존 함수 확장.

| 신규/변경 | 경계 | 역할 |
|---|---|---|
| `transcript.go` (신규) | transcript 소비자 — read-only | cwd → transcript 디렉토리 인코딩(forward-only), 후보 jsonl 선택, tail read window로 마지막 assistant entry 파싱, model + usage + entry.cwd만 추출 |
| `pricing.go` (신규) | 단가표 embed | 모델 ID → {input, output, cache_read, cache_creation_5m, cache_creation_1h} USD per MTok. hardcode embed(zero-dep). 알려지지 않은 모델은 lookup 실패로 표현 |
| `last_model_cache.go` (신규 또는 cache.go 확장) | cwd별 last-known `[1m]` 신호 영속화 | stdin이 `[1m]` 신호 제공 시 cwd별로 저장. transcript 폴백 시 read |
| `main.go` 흐름 확장 | source-of-truth 우선순위 | session-state(Layer 1, 60s 이내) → transcript(Layer 2, TTL 무관) → warmup/묵음(Layer 3, 기존) |
| `widgets_project.go` (변경) | projectInfo 위젯 | `Workspace.CurrentDir == ""`일 때 `detectCurrentCwd()`로 fresh fallback (SPEC §5.4) |
| `widgets_core.go` (변경) | cost 위젯 | estimated cost 분기와 시각 마커 |

경계 원칙:

- transcript 진실 소스는 **read-only**. cc-usage가 쓰지 않고 lock도 걸지 않는다. 동시성은 partial-line skip + 실패 graceful skip으로(SPEC §5.10).
- Layer 1(session-state)과 Layer 2(transcript)는 **순차 적용**. Layer 1이 채운 필드는 Layer 2가 덮어쓰지 않는다(field-local emptiness 룰). 같은 mask 구조 공유로 `stripRestoredFields`의 self-perpetuation 차단을 통합.
- `pricing.go`의 단가표 lookup 실패 시 cost 위젯은 skip(D11). 잘못된 cost 표시 위험 0.
- last-known `[1m]` 캐시는 cwd-exact-match 가드 아래에서만 read·write. cross-cwd 노출 0회(D10).

## 2. 데이터 흐름

```
parseStdin
  └─> StdinInput (정상 / degraded)
sessionCacheKey(input)
resolveCachedSessionState(cacheKey, now)        [v0.3.10 무변경]
  ├─ loadSessionState(key) -> hit -> return
  └─ miss -> fallbackByWorkspaceCwd(now)        [v0.3.8 완화 형태 유지]

ctx 구성
result = orchestrate(ctx)                        [1차]

restoredMask := empty

# Layer 1: session-state (60s 이내, 정확)
if shouldRestoreFromSession(ctx.Stdin, cached, now):
    mask1 = fillFromSessionCache(&ctx.Stdin, cached)
    restoredMask = merge(restoredMask, mask1)
    result = orchestrate(ctx)                    [2차]

# Layer 2: transcript (TTL 무관, estimated)
if needsTranscriptBackfill(ctx.Stdin):           [model/ContextWindow 여전히 비었나]
    cwd = detectCurrentCwd()
    transcriptPath = ctx.Stdin.TranscriptPath
    if transcriptPath == "" and cwd != "":
        transcriptPath = selectTranscriptCandidate(encodeCwdToTranscriptDir(cwd))
    if transcriptPath != "":
        entry = readLastAssistantEntry(transcriptPath)
        if entry != nil and entry.cwd matches cwd (D4):
            oneMSignal = loadLastKnownOneM(cwd)   [D10]
            mask2 = applyTranscriptToStdin(&ctx.Stdin, entry, oneMSignal)
            restoredMask = merge(restoredMask, mask2)
            ctx.CostEstimated = mask2.Cost        [D6]
            result = orchestrate(ctx)             [3차]

# Layer 3: warmup / 묵음 (기존)
if shouldSuppressOutput(ctx.Stdin, ctx.RateLimits):
    return
if isWarmupExceptionPath(ctx.Stdin, ctx.RateLimits):
    result = renderRateLimitOnly(ctx)

print result.Lines

# save: layer 1·2 모두 stripRestoredFields로 vacate (정상 stdin 값만 보존)
if !warmupOnly and result.WidgetCount >= 2:
    snapshot := ctx.Stdin
    snapshot.RateLimits = nil
    stripRestoredFields(&snapshot, restoredMask)
    saveSessionState(...)

# last-known [1m] 저장: stdin이 [1m]을 제공했고 cwd 식별되면 갱신
if stdin.Model.ID contains "[1m]" and cwd != "":
    saveLastKnownOneM(cwd, true)
```

상태 전이:

| Layer 1 | Layer 2 | 결과 |
|---|---|---|
| 통과 | model/context 채워졌으면 skip | full (session 값, cost 정확) |
| 통과 | model/context 여전히 비면 (드문 케이스) 보강 | full mixed (가능성 낮음) |
| 미통과 | 통과 | full (transcript, cost estimated) |
| 미통과 | 미통과 | warmup/묵음 (기존) |

실패 경로(SPEC §5.6, §5.10):

| 실패 | Layer 2 동작 |
|---|---|
| cwd 식별 불가 | 미발동 → Layer 3 |
| transcript 디렉토리 부재 | 미발동 → Layer 3 |
| 디렉토리 있으나 jsonl 0개 | 미발동 → Layer 3 |
| 후보 read 실패 | debugLog + 미발동 → Layer 3 |
| tail window 상한(1MB) 도달 후 entry 못 찾음 | 미발동 → Layer 3 |
| 마지막 assistant entry 부재 | 미발동 → Layer 3 |
| JSON 파싱 실패 | 미발동 → Layer 3 |
| partial 마지막 line | skip, 그 앞 완전 line로 매칭 |
| entry.cwd ≠ detectCurrentCwd (D4) | 미발동 |
| 단가표 lookup 실패 | cost mask만 비활성, model/context는 채움 |
| `[1m]` 캐시 부재 | 기본값 200K(D2) |

orchestrate 호출 횟수: 정상 stdin은 1회. Layer 1 적용 시 2회. Layer 2 적용 시 3회. status line 호출당 200ms 예산 안에 머무는지 D5에서 검토.

## 3. 인터페이스

외부 API 추가 없음. 패키지 내 함수.

| 함수 | 시그니처(가이드) | 책임 |
|---|---|---|
| `encodeCwdToTranscriptDir` | `func(home, cwd string) string` | cwd → `<home>/.claude/projects/-<encoded>`. `/`·`.` → `-` |
| `selectTranscriptCandidate` | `func(dir string) (string, error)` | 디렉토리 안의 `*.jsonl` 중 newest mtime, 동률은 lex sort |
| `readLastAssistantEntry` | `func(path string, initialWindow, maxWindow int) (*transcriptEntry, error)` | tail read window 역방향 line scan. partial line skip |
| `applyTranscriptToStdin` | `func(stdin *StdinInput, entry *transcriptEntry, oneMSignal bool) restoredFieldMask` | model + usage + cost(estimated) backfill |
| `loadLastKnownOneM` / `saveLastKnownOneM` | `func(cwd string) bool` / `func(cwd string, val bool)` | cwd별 `[1m]` 캐시 read/write |
| `lookupPricing` | `func(modelID string) (modelPricing, bool)` | 단가표 lookup, miss는 false |
| `estimateCost` | `func(usage transcriptUsage, p modelPricing) float64` | 4개 토큰 카테고리 × 단가 합산 |

`transcriptEntry`는 `Model string` + `Usage transcriptUsage` + `Cwd string`만 보유(파싱 표면 최소화). `transcriptUsage`는 4개 카테고리(`input, output, cache_read, cache_creation`) + ephemeral 분기는 단가표 적용 시 단가표에서 결정.

기존 인터페이스 무변경: `StdinInput` 스키마(`Cost.Estimated`는 stdin에 추가하지 않고 `Context.CostEstimated`로 ctx에 표현), `Widget` 인터페이스, session-state 캐시 파일 포맷, API 캐시.

## 4. 영향 범위

직접 영향:

- `main.go` — backfill 흐름에 Layer 2(transcript) 추가. Layer 1·3 본문은 무변경. `restoredMask` 통합.
- `transcript.go` (신규) — §3 함수 + `transcriptEntry`.
- `pricing.go` (신규) — 단가표 embed + lookup.
- `last_model_cache.go` 또는 `cache.go` 확장 — last-known `[1m]` cwd별 영속화.
- `widgets_project.go` — `detectCurrentCwd` fresh fallback(SPEC §5.4).
- `widgets_core.go` — `costWidget.Render`에 estimated 마커. `GetData`는 무변경 가능성(D6).
- `widget.go` — `Context`에 `CostEstimated bool` 추가(D6).
- `Makefile` / `.claude-plugin/plugin.json` / `api.go` — SemVer bump 세 곳 동시(v0.3.10 → v0.3.11).

간접 영향:

- `shouldSuppressOutput` / `isWarmupExceptionPath` (main.go:233, 249) — backfill 이후 stdin을 본다. Layer 2 backfill로 noIdentity flip 가능(기대 동작). 본문 무변경(SPEC §5.14).
- `saveSessionState` / `stripRestoredFields` — Layer 2 분도 mask로 추적해 self-perpetuation 차단(D8).
- `loadByWorkspaceCwd` — 호출 빈도 감소(transcript가 일부 케이스를 흡수), 동작 무변경.
- `cleanOldSessionStates` / `cleanOldCaches` — transcript는 Claude Code가 관리하므로 대상 0. last-known `[1m]` 파일에 cleanup 정책은 D10에서 결정.

하위 호환·마이그레이션: 해당 없음.

## 5. Decision Points

### D1. cost 정책 (SPEC §3, §5.5)

채택: **(b) 단가표 + estimated 마커**. SPEC §3에 commit됨.

근거: TTL 초과 시 cost가 사라지는 (a)의 단점이 본 feature의 핵심 목표(빈 stdin에서도 full 출력)와 직접 충돌. (b)의 단가표 stale 위험은 마커로 사용자에게 신호.

세부:
- stdin이 `cost.total_cost_usd > 0`을 제공하면 그 값을 그대로 사용(estimated 아님).
- 빈 stdin + transcript usage 사용 시 단가표로 합산, estimated 마커.
- 단가표 lookup 실패(알려지지 않은 모델)는 cost 위젯 skip(정확성 우선).

### D2. context_window_size 추론 (SPEC §3, §5.3)

채택: **(ii) cwd별 last-known `[1m]` 캐시 + 기본값 200K**. SPEC §3에 commit됨.

근거: (i) 휴리스틱은 cache_read 누적으로 인한 false-positive 위험 큼. (iii) skip은 SPEC §2 partial 제거 목표와 충돌. (ii)는 정확성·UX 균형이 가장 좋고 cwd-exact-match 가드로 cross-cwd 노출 0.

### D3. transcript 후보 선택 우선순위 (SPEC §5.9)

채택: **stdin.transcript_path 우선, 없으면 cwd 디렉토리 내 newest mtime jsonl, mtime 동률은 lex sort**. SPEC §5.9 명시 사항.

### D4. cwd 일치 가드 강도 (SPEC §5.7, §5.11)

채택: **(β) entry.cwd 추가 검증**.

근거: 인코딩이 forward-only lossy(`/foo.bar` vs `/foo-bar` 모두 `-foo-bar`로 충돌 가능)이므로 디렉토리 매칭만으로 cross-cwd 노출 0%를 보장 못 한다. entry.cwd × `normalizeCwd` 정확 일치로 0 보장. 비용 무시 가능.

### D5. tail read window 크기 (SPEC §3, §5.8)

채택: **64KB 시작 + 부족 시 2배 확장 + 상한 1MB. 상한 초과 시 미발동(다음 단계)**.

근거: SPEC §3가 `cacheLockTimeout = 200ms` 차수 안에 머물 것을 요구. 일반 assistant entry는 수 KB(64KB 충분). 1MB는 매우 긴 message도 cover하면서 SSD read ≈ 1–3ms + JSON parse 수십 ms로 200ms 차수 유지. 상한 초과 시 정확성 우선 원칙에 따라 미발동.

### D6. cost 위젯 변경 표면

채택: **`costWidget.GetData` 무변경, `Render`에서 `ctx.CostEstimated` 확인 후 마커 표시**.

근거: GetData 시그니처를 흔들지 않고 추가. `ctx.CostEstimated`는 main이 Layer 2 backfill 직후 설정. 정상 stdin 경로에서는 항상 false → 출력 동일(SPEC §5.11).

마커 형식 후보: `~$3.14`, `$3.14*`, `$3.14 (est)`. implement 단계에서 i18n locale 파일과의 정합을 보고 선택.

### D7. projectInfo §5.4 도입

채택: **`projectInfoWidget.GetData` 첫 가드를 `currentDir = detectCurrentCwd()` 폴백 후 그래도 비면 nil 반환**.

근거: SPEC §5.4 본문이 명시. v0.3.10까지의 동작은 nil 반환으로 위젯 통째 사라짐 → 사용자가 첫 보고한 회귀. cwd 자체는 cc-usage 프로세스 내부 신호라 cross-workspace 위험 0.

### D8. transcript backfill의 `restoredFieldMask` 통합

채택: **기존 `restoredFieldMask` 재사용**. Layer 1·2 모두 같은 mask 구조에 누적. `stripRestoredFields`로 vacate.

근거: atomic-degraded-restore의 self-perpetuation 차단 패턴을 그대로 유지(SPEC §5.14). 두 layer가 같은 필드를 채울 수 있으나 fillFromSessionCache의 emptiness 룰로 중복 쓰기는 발생하지 않음.

### D9. session-state 캐시의 잔여 역할

채택 후 정리:

- 60s 이내 cost 정확값 보관(Layer 1) — 기존.
- workspace 정규화 사본 보관 — 기존.
- last-known `[1m]` 신호는 **별도 파일**(D10)에 두어 atomic restore TTL 게이트에 종속시키지 않음.

session-state 캐시 자체의 제거는 SPEC §4가 본 feature 범위 밖으로 둠. 잔여 역할 유지.

### D10. last-known `[1m]` 캐시 저장 위치·정리 정책

옵션:

- (α) session-state-`<key>`.json에 boolean 필드 추가.
- (β) 별도 파일 `~/.cache/cc-usage/one-m-by-cwd.json` (cwd-hash → bool 단일 파일).
- (γ) 별도 파일 `last-model-<cwd-hash>.json` (cwd 하나당 파일 하나).

채택: **(β) 단일 파일**.

근거:
- session-state 의 atomic-restore TTL(60s) 게이트와 분리되어야 함(transcript 폴백 시점에 60s 초과여도 1M 신호는 필요). (α) 부적합.
- (γ)는 cwd 수에 비례해 파일 수 증가. cleanup 패턴 복잡.
- (β)는 atomic write 한 번으로 정합 유지 가능. cwd-hash로 cross-cwd 노출 0(read·write 모두 cwd-exact-match 가드 통과 후 진입).

cleanup 정책: 파일 단일이므로 별도 cleanup goroutine 불필요. 같은 파일에 누적 추가/덮어쓰기. cwd가 더 이상 존재하지 않아도 파일 크기는 무시 가능한 수준(cwd당 ~100B).

### D11. 단가표 데이터 형식과 lookup 정책

채택: **`pricing.go`에 `map[string]modelPricing` hardcode embed**. 키는 transcript의 `message.model` 형태(`claude-opus-4-7`, `claude-sonnet-4-6` 등 base ID).

`modelPricing` 필드: `Input`, `Output`, `CacheRead`, `CacheCreation5m`, `CacheCreation1h` (모두 USD per MTok).

lookup 실패 정책: cost 위젯 skip(GetData에서 nil 반환은 아님 — Render에서 `ctx.CostEstimated`가 true이고 단가표 miss면 빈 문자열 반환 → orchestrator가 자동 skip). 잘못된 cost 표시 위험 0.

가격값 자체는 implement 단계에서 Anthropic 공식 가격 페이지 참조해 채움. 가격 변경 시 cc-usage 패치(SemVer bump 동반) — SPEC §3에 명시됨.
