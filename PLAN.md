# cc-usage — Completion Checkpoint Plan

각 Task는 "무엇이 동작하면 완료인가"를 정의한다. 구현 방법/순서를 나열하지 않는다.

> **전역 규칙:** Decision Point가 남은 Phase는 사용자 승인 없이 구현 Task로 넘어가지 않는다.

---

## Phase 1: MVP — stdin JSON → ANSI status line

이전 Phase Exit Criteria: 없음 (첫 Phase).

stdin으로 JSON을 받아 ANSI 컬러 status line을 stdout으로 출력하는 최소 파이프라인.
이 Phase가 끝나면 사용자는 Claude Code settings.json에 바이너리를 등록하고 실제 status line을 볼 수 있다.

---

### ─── 한 호흡: 입력 파이프라인 ───

- [ ] **T1.1** CLI에 최소 JSON을 stdin으로 넣으면 패닉 없이 프로세스가 종료된다
  - 목적: stdin 파싱과 config 로드의 경계가 독립적으로 동작하는지 확인
  - 입력: `echo '{"model":{"id":"test","display_name":"Test"}}' | go run .`
  - 산출물: `StdinInput` 구조체, `Config` 구조체, `--config` CLI 인자 처리, `debugLog` 유틸리티
  - Exit Criteria: 위 커맨드가 exit 0으로 종료되고 stderr에 패닉/스택트레이스 없음

- [ ] **T1.2** `--config <path>` 지정 시 해당 경로의 설정을 로드하고, configDir이 인증 경로의 기준이 된다
  - 목적: 멀티 프로필(~/.claude/, ~/.claude-triptopaz/ 등) 지원의 기반
  - 입력: 존재하는/존재하지 않는 config 경로
  - 산출물: config 로드 로직, configDir 추론, 기본값 병합
  - Exit Criteria: `--config /tmp/test.json`으로 실행 시 `DEBUG=cc-usage`에서 해당 경로 로드 로그 확인. 파일 없어도 기본값으로 정상 실행

### ─── 한 호흡: 위젯 시스템 + 렌더링 ───

- [ ] **T1.3** Widget 인터페이스로 등록된 위젯이 display mode에 따라 순서대로 렌더링된다
  - 목적: 위젯 시스템(레지스트리, 오케스트레이터, graceful skip)이 확장 가능한 구조로 동작하는지 확인
  - 입력: 하드코딩 테스트 위젯 + display mode 설정
  - 산출물: `Widget` 인터페이스, 위젯 레지스트리, 오케스트레이터, `disabledWidgets` 필터링
  - Exit Criteria: nil 반환 위젯은 건너뛰고, 유효한 위젯만 세퍼레이터로 조인되어 stdout에 출력

- [ ] **T1.4** 테마와 세퍼레이터 설정에 따라 ANSI 컬러 출력이 변한다
  - 목적: 8개 테마, 4종 세퍼레이터, 프로그레스바가 config에 따라 전환되는지 확인
  - 입력: config에서 theme/separator 변경
  - 산출물: `ThemeColors` 구조체, 8개 테마 정의, 세퍼레이터 4종, `renderProgressBar()`, `getColorForPercent()`
  - Exit Criteria: `"theme":"catppuccin"` 설정 시 default와 다른 ANSI 코드 출력. 프로그레스바가 퍼센트에 비례하여 █/░ 비율 변화

- [ ] **T1.5** 포매팅 함수들이 경계값에서 올바른 결과를 반환한다
  - 목적: 토큰/비용/시간/퍼센트 포매팅이 위젯 전체에서 일관되게 사용되는 기반
  - 입력: 0, 음수, 매우 큰 값, 소수점 경계
  - 산출물: `formatTokens`, `formatCost`, `formatTimeRemaining`, `formatDuration`, `shortenModelName`, `calculatePercent`, `truncate`, `clampPercent`, `osc8Link`
  - Exit Criteria: `go test`에서 경계값 케이스 전수 통과 — 1500→"1.5K", 1500000→"1.5M", cost 0.5→"$0.50", 음수/초과 퍼센트→0-100 클램프

### ─── 한 호흡: 코어 위젯 ───

- [ ] **T1.6** model/context/cost 위젯이 stdin JSON으로부터 올바르게 렌더링된다
  - 목적: 가장 기본적인 3개 위젯이 compact display mode의 첫 출력을 구성
  - 입력: `{"model":{"id":"claude-opus-4-6","display_name":"Opus"},"context_window":{...},"cost":{"total_cost_usd":1.25}}`
  - 산출물: model 위젯(이모지+이름+effort+fast), context 위젯(프로그레스바+%+토큰), cost 위젯
  - Exit Criteria: `echo '{...}' | ./cc-usage` → `◆ Opus │ ████░░░░ 30% 60K │ $1.25`

### Decision Point 1.A: model 위젯의 effort/fast 필드

stdin JSON에 `effort`, `fast` 필드가 실제로 존재하는지 실측 필요 (DESIGN.md TODO(model)).

**선택지:**
1. **필드 존재 시** — `effort` → `(H)/(M)/(L)`, `fast` → `↯` 접미사 표시
2. **필드 부재 시** — 해당 표시 생략, 모델명만 출력

**Trade-off:** 1은 정보 밀도 높지만 실측 없이 구현하면 dead code. 2는 안전하지만 나중에 추가 작업 필요.

→ 실측 후 결정. 필드가 없으면 2로 진행.

### Decision Point 1.B: context 위젯 used_percentage 폴백

`used_percentage`가 optional일 때 폴백 계산 방식 (DESIGN.md TODO(context)).

**선택지:**
1. **폴백 계산** — `(total_input + total_output) / context_window_size * 100`
2. **위젯 숨김** — `used_percentage` 없으면 context 위젯 전체 skip

**Trade-off:** 1은 항상 표시되지만 공식 값과 미세한 차이 가능. 2는 정확하지만 정보 손실.

→ DESIGN.md는 1을 명시. `context_window_size`도 없을 때만 숨김.

### Decision Point 1.C: rateLimit7d/7dSonnet의 "Max 플랜만" 조건

`rateLimit7d`와 `rateLimit7dSonnet`이 Max 플랜에서만 표시되어야 하는지 (DESIGN.md TODO(plan)).

**선택지:**
1. **config.plan 분기** — `config.plan == "max"`일 때만 7d 위젯 표시, pro면 숨김
2. **데이터 기반 자동 숨김** — API/stdin에 데이터가 없으면 graceful skip (plan 분기 불필요)

**Trade-off:** 1은 명시적이지만 plan 설정을 잘못하면 데이터가 있어도 안 보임. 2는 설정 없이 동작하지만 pro 플랜에서도 데이터가 오면 표시됨.

→ DESIGN.md는 "우선 2로 구현, 필요 시 1 추가"를 제안. 사용자 확인 필요.

---

- [ ] **T1.7** OAuth 토큰을 추출하고 API에서 rate limit 데이터를 가져올 수 있다
  - 목적: 인증 + API 호출 + 3-tier 캐시가 연동되어 rate limit 위젯의 데이터 소스 역할
  - 입력: 실제 `{configDir}/.credentials.json` 또는 macOS Keychain
  - 산출물: OAuth 토큰 추출(파일/Keychain), API 클라이언트, 3-tier 캐시(메모리→파일→API), request dedup, 에러 처리(429/403/기타)
  - Exit Criteria: `DEBUG=cc-usage`에서 `[cc-usage:api] fetch` 패턴 로그 출력. 두 번째 실행 시 `[cc-usage:api] cache hit` 로그. credential 없으면 패닉 없이 nil 반환

- [ ] **T1.8** rate limit 위젯이 stdin 데이터 우선, 없으면 API 폴백으로 표시된다
  - 목적: stdin과 API 두 데이터 소스의 우선순위 로직이 올바르게 동작
  - 입력: rate_limits 있는 JSON / 없는 JSON
  - 산출물: `rateLimit5h`, `rateLimit7d`, `rateLimit7dSonnet` 위젯
  - Exit Criteria: rate_limits 포함 JSON → `5h: 42% │ 7d: 69%`. rate_limits 없는 JSON → API 호출 시도 (credential 없으면 위젯 숨김)

### ─── 한 호흡: 프로젝트 정보 + i18n ───

- [ ] **T1.9** projectInfo 위젯이 디렉토리, git branch, ahead/behind을 표시한다
  - 목적: 현재 작업 컨텍스트를 status line에서 확인
  - 입력: git 프로젝트 디렉토리의 stdin JSON
  - 산출물: `projectInfo` 위젯 (디렉토리명, git branch, ↑↓ 카운트, subpath, worktree 표시)
  - Exit Criteria: git repo에서 실행 시 `📁 dirname (branch)` 형태 출력. non-git 디렉토리에서도 패닉 없이 디렉토리명만 표시

- [ ] **T1.10** 언어 설정에 따라 라벨과 시간 단위가 전환된다
  - 목적: 한국어/영어 i18n 지원
  - 입력: `config.language="ko"` 또는 `LANG=ko_KR.UTF-8`
  - 산출물: `locales/en.json`, `locales/ko.json` (바이너리 내장), `Translations` 구조체, 언어 감지 로직
  - Exit Criteria: `LANG=ko_KR.UTF-8`에서 시간 단위가 한국어로 출력

### ─── 한 호흡: 빌드 + 배포 ───

- [ ] **T1.11** 4개 플랫폼(darwin/arm64, darwin/amd64, linux/amd64, windows/amd64) 바이너리가 생성된다
  - 목적: 크로스 컴파일 + 플러그인 패키징
  - 입력: `make build`
  - 산출물: Makefile, `dist/` 바이너리 4종, `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, `commands/setup.md`
  - Exit Criteria: `make build` 후 `dist/` 에 4개 바이너리 존재. 각각 `file` 명령으로 아키텍처 확인

- [ ] **T1.12** 실제 Claude Code에서 status line이 동작한다
  - 목적: Phase 1 전체 통합 검증
  - 입력: settings.json에 바이너리 등록 후 Claude Code 실행
  - 산출물: 동작하는 status line
  - Exit Criteria:
    - `echo '{기본JSON}' | ./cc-usage` → `◆ Opus │ ████░░░░ 30% 60K │ $1.25`
    - `echo '{rate_limits포함}' | ./cc-usage` → `◆ Opus │ ████░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%`
    - 빈 stdin → 패닉 없이 종료
    - 필수 필드만 있는 최소 JSON → 정상 출력

---

## Phase 2: 세션/분석 위젯

이전 Phase Exit Criteria 회귀: T1.12의 4가지 검증 항목 통과.

세션 경과 시간 추적과 토큰 분석 위젯. 이 Phase가 끝나면 사용자는 세션 시간, 분당 소비량, 캐시 히트율을 볼 수 있다.

---

- [ ] **T2.1** 세션 시작 시간이 파일로 기록되고, 재실행 시 동일 세션의 경과 시간이 증가한다
  - 목적: sessionDuration, burnRate, depletionTime 등 시간 기반 위젯의 공통 인프라
  - 입력: `session_id` 포함 JSON을 두 번 실행
  - 산출물: 세션 파일(`~/.cache/cc-usage/sessions/{id}.json`), atomic 생성(`O_EXCL`), 메모리 캐시, 7일 정리
  - Exit Criteria: 첫 실행 시 `~/.cache/cc-usage/sessions/{id}.json` 생성. 재실행 시 `cat` 으로 파일 내용 확인 → `startTime` 값 동일. `session_id` 없으면 `default.json` 사용

- [ ] **T2.2** sessionDuration, burnRate, cacheHit 위젯이 올바르게 렌더링된다
  - 목적: 세션/분석 정보를 status line에 표시
  - 입력: session tracking이 동작하는 상태에서 context_window 데이터 포함 JSON
  - 산출물: `sessionDuration`(⏱ 경과시간), `burnRate`(🔥 토큰/분), `cacheHit`(📦 히트율%) 위젯
  - Exit Criteria:
    - sessionDuration: 세션 경과에 따라 `⏱ 1h30m` 형태 변화
    - burnRate: 세션 1분 미만이면 숨김, 이후 `🔥 5.2K/m` 형태
    - cacheHit: 70%+ → 초록, 40-70% → 노랑, <40% → 빨강

---

## Phase 3: Transcript + 모니터링 위젯

이전 Phase Exit Criteria 회귀: T1.12 + T2.2.

transcript.jsonl을 incremental 파싱하고, 그 위에서 동작하는 위젯들을 제공한다. 이 Phase가 끝나면 사용자는 도구 실행 현황, 에이전트 상태, todo 진행률, 토큰 상세 분석을 볼 수 있다.

---

### ─── 한 호흡: Transcript 파싱 ───

- [ ] **T3.1** transcript.jsonl의 새로 추가된 부분만 파싱하고, 기존 상태를 유지한다
  - 목적: 매 호출마다 전체 파일을 읽지 않는 incremental 파싱으로 성능 보장
  - 입력: transcript.jsonl 파일 경로가 포함된 stdin JSON
  - 산출물: `TranscriptEntry`/`ContentBlock` 구조체, `ParsedTranscript` 상태, offset 기반 incremental 읽기
  - Exit Criteria: 파일 append 후 재실행 시 `[cc-usage:transcript] incremental` 로그 확인. 파일 truncation 시 `[cc-usage:transcript] re-parse` 로그와 함께 전체 재파싱

- [ ] **T3.2** tool_use/tool_result 쌍이 올바르게 추적되어 running/completed 상태를 알 수 있다
  - 목적: toolActivity, agentStatus, todoProgress 위젯의 데이터 정확성
  - 입력: assistant tool_use + user tool_result가 포함된 샘플 transcript.jsonl
  - 산출물: ToolUses 맵, RunningToolIDs, CompletedToolCount, ActiveAgentIDs, CompletedAgentCount, Tasks(TaskCreate/Update), LastTodoWriteInput, SessionStartTime, SessionName
  - Exit Criteria: tool_use 후 running 카운트 증가 → tool_result 후 completed 카운트 증가 + running 감소. 완료된 ToolUses 메모리에서 삭제

### ─── 한 호흡: Transcript 기반 위젯 ───

- [ ] **T3.3** todoProgress가 Tasks API와 TodoWrite 데이터를 올바르게 반영한다
  - 목적: task 완료율을 status line에서 확인
  - 입력: TaskCreate/TaskUpdate 또는 TodoWrite가 포함된 transcript
  - 산출물: `todoProgress` 위젯 (`✓ completed/total`)
  - Exit Criteria: TaskCreate 3개 중 2개 completed → `✓ 2/3`. Tasks 없고 TodoWrite만 → 같은 형식. transcript 없으면 숨김

- [ ] **T3.4** toolActivity와 agentStatus가 현재 실행 중인 도구/에이전트를 표시한다
  - 목적: 실시간 작업 현황 파악
  - 입력: running tool_use가 있는 transcript
  - 산출물: `toolActivity`(⚙️ 도구명(타겟) + 완료수), `agentStatus`(🤖 running/done 카운트)
  - Exit Criteria: Read(app.ts) 실행 중 → `⚙️ Read(app.ts) │ 12 done`. Agent 2개 running → `🤖 Agent: 2 running, 3 done`

### ─── 한 호흡: 토큰 분석 위젯 ───

- [ ] **T3.5** tokenBreakdown, tokenSpeed, depletionTime이 올바르게 계산된다
  - 목적: 토큰 사용의 상세 분석 정보 제공
  - 입력: context_window.current_usage + cost.total_api_duration_ms + session elapsed
  - 산출물: `tokenBreakdown`(📊 In/Out/CW/CR), `tokenSpeed`(⚡ tok/s), `depletionTime`(⏳ 도달 예상 시간)
  - Exit Criteria:
    - tokenBreakdown: 4개 항목이 모두 표시
    - tokenSpeed: `total_api_duration_ms` 없으면 숨김
    - depletionTime: 5h/7d 중 먼저 도달하는 쪽 표시, burnRate 0이면 숨김

---

## Phase 4: 부가 위젯

이전 Phase Exit Criteria 회귀: T1.12 + T3.4 + T3.5.

예산 추적, 설정 카운트, 간단 정보 위젯. 이 Phase가 끝나면 detailed display mode의 모든 줄이 채워진다.

---

- [ ] **T4.1** configCounts, forecast, linesChanged 위젯이 동작한다
  - 목적: 프로젝트 설정과 독립적 부가 정보 표시 (budget 시스템 무관)
  - 입력: 실제 파일시스템 + stdin + git repo
  - 산출물: `configCounts`(CLAUDE.md/Rules/MCPs 카운트), `forecast`(📈 시간당 비용), `linesChanged`(+N -N)
  - Exit Criteria:
    - configCounts: 실제 CLAUDE.md 파일 수와 일치
    - forecast: 세션 1분+ 후 `📈 ~$X/h` 표시
    - linesChanged: stdin에 `total_lines_added/removed` 있으면 해당 값, 없으면 `git diff --stat` 폴백

### ─── 한 호흡: Budget 위젯 ───

- [ ] **T4.2** budget.json이 세션 간 일일 비용을 정확하게 누적한다
  - 목적: delta 추적으로 중복 카운팅 없는 일일 비용 집계
  - 입력: 동일 session_id로 비용이 증가하는 여러 번의 실행
  - 산출물: `~/.cache/cc-usage/budget.json`, delta 추적 알고리즘, 날짜 변경 시 리셋, request dedup
  - Exit Criteria: session cost $3 → $5 → dailyTotal에 $5 반영(중복 없음). 다른 세션 $2 추가 → dailyTotal $7. 날짜 변경 → 리셋

- [ ] **T4.3** budget과 todayCost 위젯이 budget tracking 데이터를 반영한다
  - 목적: 일일 예산 대비 지출 현황 표시
  - 입력: budget.json이 존재하는 상태 + config.dailyBudget 설정/미설정
  - 산출물: `budget`(💵 $X/$Y), `todayCost`(Today: $X)
  - Exit Criteria:
    - budget: `dailyBudget` 미설정 시 숨김, 설정 시 `💵 $5.20/$15` 형태
    - todayCost: budget.json의 dailyTotal 반영

- [ ] **T4.4** 기타 간단 위젯 전체가 조건에 따라 표시/숨김된다
  - 목적: detailed mode의 나머지 슬롯 채움
  - 입력: 각 위젯의 조건 필드가 있는/없는 stdin JSON
  - 산출물: version, sessionName, sessionId, sessionIdFull, vimMode, outputStyle, apiDuration, peakHours, performance, lastPrompt
  - Exit Criteria:
    - version: `stdin.version` 값 그대로 출력
    - sessionId: `stdin.session_id` 앞 8자 출력. sessionIdFull: 전체 출력
    - vimMode: vim 비활성화 시 숨김, 활성화 시 `NORMAL`/`INSERT`
    - outputStyle: `"default"` 일 때 숨김, 그 외 스타일명 출력
    - peakHours: `TZ=America/Los_Angeles` 고정 기준으로 평일 5-11 AM → `🔴 Peak`, 그 외 → `🟢 Off-Peak`
    - lastPrompt: transcript 있을 때 마지막 user text 표시, 없으면 숨김
    - Preset 전용 위젯(sessionIdFull 등): `"preset":"i"` config로 검증 가능

### Decision Point 4.A: performance 위젯 임계값

배지 종류와 계산식 결정 필요 (ROADMAP.md TODO(performance)).

**선택지:**
1. **초안 기준** — cacheHit 70%+ AND outputRatio 20%+ → `⚡ Efficient`, 그 외 → `📊 Normal`
2. **3단계** — Efficient / Normal / Inefficient (low cache hit 경고)
3. **실측 후 결정** — 실제 세션 데이터 수집 후 임계값 조정

**Trade-off:** 1은 단순하지만 임계값이 임의적. 2는 정보 많지만 "Inefficient" 표시가 사용자에게 부정적. 3은 정확하지만 출시 지연.

→ 사용자 결정 필요.

---

## Phase 5: 외부 CLI 통합

이전 Phase Exit Criteria 회귀: T1.12 + T4.4.

Codex, Gemini, z.ai CLI의 사용량을 status line에 표시. 이 Phase가 끝나면 사용자는 여러 AI CLI의 rate limit을 한 곳에서 볼 수 있다.

---

- [ ] **T5.1** ANTHROPIC_BASE_URL로 provider가 감지되고, 해당 provider의 위젯만 활성화된다
  - 목적: z.ai/ZHIPU 등 대체 provider 지원의 기반
  - 입력: `ANTHROPIC_BASE_URL` 환경변수 설정/미설정
  - 산출물: `detectProvider()` 함수 ("anthropic" / "zai" / "zhipu")
  - Exit Criteria: `ANTHROPIC_BASE_URL=https://api.z.ai/...` → `"zai"` 반환. 미설정 → `"anthropic"`

- [ ] **T5.2** Codex CLI가 설치되어 있으면 사용량이 표시되고, 미설치면 위젯이 숨겨진다
  - 목적: Codex CLI rate limit 모니터링
  - 입력: `~/.codex/auth.json` 존재/부재
  - 산출물: `codexUsage` 위젯, 인증(access_token + account_id), API 호출, 모델 감지, 메모리 캐시
  - Exit Criteria: auth.json 있으면 `🔷 codex 5h:X% 7d:Y%`. 없으면 위젯 숨김. negative cache 30초 동작

- [ ] **T5.3** Gemini CLI 토큰이 만료되면 자동 갱신되고, quota가 표시된다
  - 목적: Gemini CLI rate limit 모니터링 + OAuth 토큰 lifecycle 관리
  - 입력: `~/.gemini/oauth_creds.json` (만료된/유효한 토큰)
  - 산출물: `geminiUsage`(현재 모델), `geminiUsageAll`(전체 버킷), OAuth 갱신, 프로젝트 ID 조회, Quota API
  - Exit Criteria:
    - 만료된 토큰 → 자동 갱신 → quota 표시
    - 갱신 실패 → 위젯 숨김(패닉 없음)
    - `remainingFraction 0.58` → `42%`
    - geminiUsageAll: `"preset":"g"` config로 전체 버킷 나열 확인

- [ ] **T5.4** z.ai/ZHIPU provider일 때만 zaiUsage 위젯이 나타난다
  - 목적: z.ai/ZHIPU 전용 사용량 모니터링
  - 입력: `detectProvider()=="zai"` + `ANTHROPIC_AUTH_TOKEN` 환경변수
  - 산출물: `zaiUsage` 위젯, API 호출, 사용률 파싱(percentage → currentValue/remaining 폴백)
  - Exit Criteria: anthropic provider → 위젯 숨김. zai provider + 유효 토큰 → `Z: 5h 30% │ 1m 15%`

---

## Phase 6: check-usage CLI

이전 Phase Exit Criteria 회귀: T1.12 + T5.2 + T5.3 + T5.4.

모든 AI CLI 사용량을 한눈에 보는 독립 실행 대시보드. 이 Phase가 끝나면 사용자는 `cc-usage check`으로 어떤 CLI가 가장 여유 있는지 확인할 수 있다.

---

- [ ] **T6.1** `cc-usage check`이 stdin 없이 독립 실행되어 대시보드를 출력한다
  - 목적: status line 컨텍스트 밖에서도 사용량 확인 가능
  - 입력: `cc-usage check` (stdin 없음)
  - 산출물: `check` 서브커맨드, Claude/Codex/Gemini/z.ai 조회, pretty 포맷 + `--json` 포맷, `--lang` 오버라이드
  - Exit Criteria:
    - pretty 출력: 구분선 + 각 CLI 섹션 + Recommendation 라인
    - `--json`: 유효한 JSON 출력 (jq 파싱 가능)
    - 미설치 CLI → 해당 섹션 `null` 또는 `"available": false`
    - 가장 낮은 5h 사용률 CLI가 추천됨

- [ ] **T6.2** `/cc-usage:check-usage` 커맨드가 Claude Code 안에서 동작한다
  - 목적: Claude Code slash command로 대시보드 접근
  - 입력: Claude Code에서 `/cc-usage:check-usage` 실행
  - 산출물: `commands/check-usage.md`
  - Exit Criteria: slash command 실행 시 바이너리를 찾아 `cc-usage check` 결과 출력

---

## Phase 7: 추가 커맨드

이전 Phase Exit Criteria 회귀: T6.1.

플러그인 업데이트와 쉘 별칭 설정. 이 Phase가 끝나면 사용자는 플러그인을 업데이트하고, 터미널에서 `check-ai`로 바로 대시보드를 볼 수 있다.

---

- [ ] **T7.1** `/cc-usage:update` 실행 시 플러그인이 업데이트되고 settings.json 경로가 갱신된다
  - 목적: Go 바이너리 경로가 버전마다 바뀌므로 자동 경로 갱신 필요
  - 입력: Claude Code에서 `/cc-usage:update` 실행
  - 산출물: `commands/update.md`, OS별 바이너리 경로 감지 패턴
  - Exit Criteria: 업데이트 후 settings.json의 statusLine.command가 최신 바이너리 경로를 가리킴

- [ ] **T7.2** `/cc-usage:setup-alias` 실행 시 쉘에 `check-ai` 함수가 등록된다
  - 목적: `cc-usage check`을 간편하게 호출
  - 입력: Claude Code에서 `/cc-usage:setup-alias` 실행
  - 산출물: `commands/setup-alias.md`, bash/zsh/PowerShell 분기
  - Exit Criteria: 새 터미널에서 `check-ai` 실행 시 대시보드 출력
