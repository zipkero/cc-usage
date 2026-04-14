# cc-usage — 확장 기능 로드맵

DESIGN.md의 MVP 이후에 구현할 기능들. 각 항목은 Go 구현에 필요한 모든 정보를 포함한다.

---

## Phase 2: 세션/분석 위젯 (Group A)

### 2.1 sessionDuration 위젯

**기능:** 세션 경과 시간 표시 (예: `⏱ 1h30m`)

**데이터 소스:** 파일 기반 세션 시작 시간 추적

**Session Tracking 시스템 (신규 구현 필요):**

```
~/.cache/cc-usage/sessions/{sessionId}.json
```

```json
{ "startTime": 1713100000000 }
```

**동작:**
1. `stdin.session_id`로 세션 식별 (없으면 `"default"`)
2. 세션 파일 존재 → `startTime` 읽기
3. 세션 파일 없음 → 현재 시간으로 생성 (atomic: `O_CREATE|O_EXCL` 플래그)
4. 경과시간 = `now - startTime`
5. `formatDuration()`으로 포매팅

**경쟁 조건 처리:**
- 여러 프로세스가 동시에 세션 파일 생성 시도할 수 있음
- `O_EXCL` 플래그로 atomic 생성 → 실패하면 기존 파일 읽기
- Go: `os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)`

**세션 파일 정리:**
- 7일(`604800초`) 이상 된 `*.json` 파일 삭제
- 1시간 간격 throttling (time-based)
- 세션 파일 저장 성공 시 fire-and-forget 정리

**캐싱:**
- 메모리 캐시: `map[string]int64` (sessionId → startTime)
- Request deduplication: 동일 sessionId 동시 조회 방지

---

### 2.2 burnRate 위젯

**기능:** 분당 토큰 소비량 (예: `🔥 5.2K/m`)

**계산:**
```
totalTokens = stdin.context_window.total_input_tokens + stdin.context_window.total_output_tokens
elapsedMinutes = getSessionElapsedMinutes(ctx) // 최소 1분 이상일 때만
tokensPerMinute = totalTokens / elapsedMinutes
```

**렌더링:** `formatTokens(tokensPerMinute) + "/m"`

**조건:** 세션 1분 미만이면 null 반환 (위젯 숨김)

---

### 2.3 cacheHit 위젯

**기능:** 캐시 히트율 (예: `📦 85%`)

**계산:**
```
usage = stdin.context_window.current_usage
cacheRead = usage.cache_read_input_tokens
totalInput = usage.input_tokens + usage.cache_creation_input_tokens + cacheRead
hitPercentage = (cacheRead / totalInput) * 100  // 0으로 나누기 방지
```

**렌더링:** 색상은 높을수록 좋으므로 반전:
- 70%+ → Safe (초록)
- 40-70% → Warning (노랑)
- <40% → Danger (빨강)

---

### 2.4 todoProgress 위젯

**기능:** Task/Todo 완료율 (예: `✓ 3/5`)

**데이터 소스:** transcript.jsonl 파싱 필요 (→ Phase 6 Transcript 참조)

**우선순위:** Tasks API (TaskCreate/TaskUpdate) → TodoWrite 폴백

**TaskCreate/TaskUpdate 추적:**
- `tool_use` 블록에서 `TaskCreate` → pending에 저장, `tool_result` 에서 확정
- `TaskUpdate` → pending에 저장, `tool_result`에서 기존 task 업데이트
- 순차 ID 부여 (1, 2, 3...)

**TodoWrite 추적 (레거시 폴백):**
- 마지막 `TodoWrite` tool_use의 `input.todos` 배열 사용
- `status` 정규화: `not_started`→`pending`, `running`→`in_progress`, `complete`/`done`→`completed`

---

## Phase 3: 모니터링 위젯 (Group B)

### 3.1 toolActivity 위젯

**기능:** 실행 중/완료 도구 + 대상 표시 (예: `⚙️ Read(app.ts) │ 12 done`)

**데이터 소스:** transcript.jsonl

**Running tool 추적:**
- `assistant` 메시지의 `tool_use` 블록 → `runningToolIds`에 추가
- `user` 메시지의 `tool_result` 블록 → `runningToolIds`에서 제거 + `completedCount++`

**Tool target 추출:**
```
Read/Write/Edit → basename(input.file_path)
Glob/Grep       → truncate(input.pattern, 20)
Bash            → truncate(input.command, 25)
기타            → undefined (target 표시 안 함)
```

---

### 3.2 agentStatus 위젯

**기능:** 서브에이전트 상태 (예: `🤖 Agent: 2 running, 3 done`)

**데이터 소스:** transcript.jsonl

**추적:**
- `tool_use` 블록에서 `name === "Task"` → `activeAgentIds`에 추가
- `tool_result` 에서 해당 ID → `activeAgentIds`에서 제거 + `completedAgentCount++`
- active agent 정보: `input.subagent_type` (없으면 `"Agent"`), `input.description`

---

### 3.3 tokenBreakdown 위젯

**기능:** 토큰 분류 표시 (예: `📊 In 30K · Out 8K · CW 2K · CR 50K`)

**계산:**
```
usage = stdin.context_window.current_usage
input = usage.input_tokens
output = usage.output_tokens  // current_usage.output_tokens 우선, 없으면 total_output_tokens 폴백
cacheWrite = usage.cache_creation_input_tokens
cacheRead = usage.cache_read_input_tokens
```

---

### 3.4 tokenSpeed 위젯

**기능:** 출력 토큰 생성 속도 (예: `⚡ 67 tok/s`)

**계산:**
```
outputTokens = stdin.context_window.total_output_tokens
apiDurationMs = stdin.cost.total_api_duration_ms
tokensPerSecond = outputTokens / (apiDurationMs / 1000)
```

**조건:** `total_api_duration_ms`가 없거나 0이면 null

---

### 3.5 depletionTime 위젯

**기능:** Rate limit 도달 예상 시간 (예: `⏳ 2h15m`)

**계산:**
```
// 5h limit 기준 (stdin의 used_percentage 또는 API의 utilization — 통합된 퍼센트 값 사용)
usedPercent = getRateLimitPercent(ctx, "five_hour")  // stdin 우선, API 폴백
elapsedMinutes = getSessionElapsedMinutes(ctx)
ratePerMinute = usedPercent / elapsedMinutes
minutesToLimit = (100 - usedPercent) / ratePerMinute

// 7d limit도 동일하게 계산 → 더 빨리 도달하는 쪽 표시
```

**조건:** burnRate 가 0이거나 세션 1분 미만이면 null

---

## Phase 4: 부가 위젯 (Group C)

### 4.1 configCounts 위젯

**기능:** 설정 파일 수 표시 (예: `CLAUDE.md: 2 │ Rules: 3 │ MCP: 1`)

**데이터 소스:** 파일시스템 스캔

**카운트 대상:**
| 항목 | 탐색 경로 |
|------|-----------|
| CLAUDE.md | `~/.claude/CLAUDE.md`, `{project_dir}/CLAUDE.md`, `{project_dir}/.claude/CLAUDE.md` |
| AGENTS.md | 동일 패턴 |
| Rules | `{project_dir}/.claude/rules/**` 파일 수 |
| MCPs | `~/.claude/settings.json` → `mcpServers` 키 수 |
| Hooks | `~/.claude/settings.json` → `hooks` 키 수 |
| +Dirs | `stdin.workspace.added_dirs` 길이 |

---

### 4.2 forecast 위젯

**기능:** 시간당 예상 비용 (예: `📈 ~$8/h`)

**계산:**
```
elapsedMinutes = getSessionElapsedMinutes(ctx)
currentCost = stdin.cost.total_cost_usd
hourlyCost = (currentCost / elapsedMinutes) * 60
```

---

### 4.3 budget 위젯

**기능:** 일일 예산 대비 지출 (예: `💵 $5.20/$15`)

**Budget Tracking 시스템 (신규 구현 필요):**

파일: `~/.cache/cc-usage/budget.json`

```json
{
  "date": "2026-04-14",
  "dailyTotal": 5.20,
  "sessions": {
    "session-abc": 3.10,
    "session-xyz": 2.10
  }
}
```

**Delta 추적 알고리즘:**
1. 파일에서 state 로드 (date 불일치 → 리셋)
2. `delta = max(0, currentSessionCost - sessions[sessionId])`
3. `dailyTotal += delta`
4. `sessions[sessionId] = currentSessionCost`
5. 파일에 저장 (fire-and-forget)

**이유:** statusline은 매 갱신마다 실행됨. 단순 누적하면 중복 카운팅. delta 방식으로 "이전 확인 이후 새로 발생한 비용"만 더함.

**Request deduplication:** budget + todayCost 위젯이 같은 렌더 사이클에서 동시 호출 → 한번만 실행

**Config 의존:** `config.dailyBudget` 설정 시에만 표시

---

### 4.4 todayCost 위젯

**기능:** 오늘 전체 세션 합계 비용 (예: `Today: $12.30`)

**데이터 소스:** budget tracking과 동일 (`recordCostAndGetDaily()` 결과)

---

### 4.5 linesChanged 위젯

**기능:** 코드 변경량 (예: `+150 -30`)

**데이터 소스:** `stdin.cost.total_lines_added/removed`

**stdin에 없을 경우 git 폴백:**
```bash
git diff --stat HEAD  # tracked 파일
git diff --stat       # unstaged
# untracked: git ls-files --others --exclude-standard | wc -l
```

---

### 4.6 기타 간단 위젯

| ID | 설명 | 데이터 | 조건 |
|----|------|--------|------|
| version | Claude Code 버전 | `stdin.version` | 항상 |
| sessionName | `/rename` 세션명 | `stdin.session_name` 우선, 없으면 transcript의 `customTitle` 폴백 | 설정 시만 |
| sessionId | 세션 ID (8자) | `stdin.session_id[:8]` | 항상 |
| sessionIdFull | 세션 ID (전체) | `stdin.session_id` | 항상 |
| vimMode | NORMAL/INSERT | `stdin.vim.mode` | vim 활성화 시만 |
| outputStyle | 출력 스타일 | `stdin.output_style.name` | `"default"` 아닐 때만 |
| apiDuration | API 시간 비율 | `total_api_duration_ms / total_duration_ms * 100` | 둘 다 존재할 때 |
| peakHours | 피크 시간대 | 시스템 시계 → PT 변환 | 항상 |
| performance | 효율성 배지 | cache hit + output ratio 복합 | 데이터 있을 때 |

> **TODO(performance):** 구체적 계산식과 배지 종류를 구현 시 정의할 것. 초안: cacheHit 70%+ AND outputRatio(output/input) 20%+ → `⚡ Efficient`, 그 외 → `📊 Normal`. 임계값은 실측 후 조정.
| lastPrompt | 마지막 프롬프트 | transcript에서 마지막 user text | transcript 있을 때 |

**peakHours 상세:**
- Anthropic API 피크: 평일 5-11 AM PT (Pacific Time)
- `America/Los_Angeles` 기준 변환
- 피크 중: `🔴 Peak (2h left)`, 오프피크: `🟢 Off-Peak (3h to peak)`

---

## Phase 5: 외부 CLI 통합

### 5.1 Provider 감지

**기능:** `ANTHROPIC_BASE_URL` 환경변수로 프로바이더 판별

```go
func detectProvider() string {
    baseURL := os.Getenv("ANTHROPIC_BASE_URL")
    if strings.Contains(baseURL, "api.z.ai") { return "zai" }
    if strings.Contains(baseURL, "bigmodel.cn") { return "zhipu" }
    return "anthropic"
}
```

---

### 5.2 Codex CLI (codexUsage 위젯)

**기능:** OpenAI Codex CLI 사용량 표시 (예: `🔷 codex 5h:30% 7d:45%`)

**설치 감지:** `~/.codex/auth.json` 존재 여부

**인증:**
```
파일: ~/.codex/auth.json
구조: { "tokens": { "access_token": "...", "account_id": "..." } }
```
- mtime 기반 캐시

**API:**
```
GET https://chatgpt.com/backend-api/wham/usage
Authorization: Bearer {access_token}
ChatGPT-Account-Id: {account_id}
User-Agent: cc-usage/{VERSION}
```

**응답:**
```json
{
  "plan_type": "plus",
  "rate_limit": {
    "primary_window": {
      "used_percent": 30,
      "reset_at": 1713200000
    },
    "secondary_window": {
      "used_percent": 45,
      "reset_at": 1713800000
    }
  }
}
```

**모델 감지 (우선순위):**
1. `~/.codex/config.toml` → `model = "value"` 파싱 (간단 TOML: 루트 레벨만)
2. 캐시된 모델 (`~/.cache/cc-usage/codex-model-cache.json`, config.toml mtime 기반)
3. `codex exec 1+1=` 실행 → stdout에서 `model: xxx` 파싱 (5분 backoff)

**캐싱:** 메모리만 (파일 캐시 없음). negative cache 30초.

---

### 5.3 Gemini CLI (geminiUsage / geminiUsageAll 위젯)

**기능:** Google Gemini CLI 사용량 (예: `💎 gemini 42%`)

**설치 감지:**
1. macOS Keychain: `security find-generic-password -s gemini-cli-oauth -a main-account -w`
2. 파일: `~/.gemini/oauth_creds.json`

**인증:**
```
파일: ~/.gemini/oauth_creds.json
구조: { "access_token": "...", "refresh_token": "...", "expiry_date": 1713200000 }
```

**토큰 갱신 (중요!):**
- `expiry_date < now + 5분` → refresh 필요
- Google OAuth 토큰 갱신:
  ```
  POST https://oauth2.googleapis.com/token
  Content-Type: application/x-www-form-urlencoded

  grant_type=refresh_token
  refresh_token={token}
  client_id=681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com
  client_secret=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
  ```
  (Google installed app 가이드라인상 client_secret 임베딩 안전)
- 갱신 성공 → `oauth_creds.json` 업데이트 (mode 0600)
- 갱신 실패 → null 반환

**프로젝트 ID 조회 (quota API에 필요):**
1. `GOOGLE_CLOUD_PROJECT` 또는 `GOOGLE_CLOUD_PROJECT_ID` 환경변수
2. `~/.gemini/settings.json` → `cloudaicompanionProject`
3. API 호출:
   ```
   POST https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist
   Authorization: Bearer {token}
   Body: { "metadata": { "ideType": "GEMINI_CLI", "platform": "PLATFORM_UNSPECIFIED", "pluginType": "GEMINI" } }
   ```
   → 응답의 `cloudaicompanionProject`

**Quota API:**
```
POST https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota
Authorization: Bearer {token}
Body: { "project": "{projectId}" }
```

**응답:**
```json
{
  "buckets": [
    {
      "modelId": "gemini-2.5-pro",
      "remainingFraction": 0.58,
      "resetTime": "2026-04-15T00:00:00Z"
    }
  ]
}
```

**사용률 계산:** `usedPercent = round((1 - remainingFraction) * 100)`

**모델 감지:** `~/.gemini/settings.json` → `selectedModel` 또는 `model.name`

**geminiUsage:** 현재 모델의 버킷만 표시
**geminiUsageAll:** 모든 버킷 표시

---

### 5.4 z.ai/ZHIPU (zaiUsage 위젯)

**기능:** z.ai/ZHIPU 사용량 (예: `Z: 5h 30% │ 1m 15%`)

**조건:** `detectProvider() == "zai" || "zhipu"` 일 때만 활성화

**인증:** `ANTHROPIC_AUTH_TOKEN` 환경변수

**API:**
```
GET {ANTHROPIC_BASE_URL의 origin}/api/monitor/usage/quota/limit
Authorization: Bearer {ANTHROPIC_AUTH_TOKEN}
```

**응답:**
```json
{
  "data": {
    "limits": [
      {
        "type": "TOKENS_LIMIT",
        "currentValue": 5000,
        "remaining": 10000,
        "percentage": 33,
        "nextResetTime": 1713200000
      },
      {
        "type": "TIME_LIMIT",
        "currentValue": 30,
        "remaining": 70,
        "nextResetTime": 1713800000
      }
    ]
  }
}
```

**사용률 파싱 (우선순위):**
1. `percentage` 필드 (직접)
2. `currentValue / (currentValue + remaining) * 100`
3. `currentValue / usage * 100` (usage = 총 한도)

---

## Phase 6: Transcript 파싱 시스템

toolActivity, agentStatus, todoProgress, sessionName, lastPrompt 위젯이 의존.

### 6.1 Transcript 파일

경로: `stdin.transcript_path` → `transcript.jsonl` (JSONL 형식)

### 6.2 Incremental 파싱 (핵심 최적화)

```
상태:
  path: string       // 현재 파싱 중인 파일 경로
  offset: int64      // 마지막으로 읽은 바이트 위치
  data: ParsedData   // 누적 파싱 결과

로직:
  1. stat(transcriptPath) → fileSize
  2. 같은 파일이고 offset == fileSize → 캐시 반환 (변경 없음)
  3. 같은 파일이고 offset < fileSize → offset부터 새 바이트만 읽기 (incremental)
  4. 다른 파일이거나 fileSize < offset (truncation) → 전체 re-parse
```

Go에서: `os.Open` → `file.Seek(offset, io.SeekStart)` → `bufio.Scanner`

### 6.3 JSONL 엔트리 구조

```go
type TranscriptEntry struct {
    Type        string `json:"type"`         // "assistant" | "user" | "system"
    Timestamp   string `json:"timestamp"`
    CustomTitle string `json:"customTitle"`  // /rename 세션명
    Message     *struct {
        Content []ContentBlock `json:"content"`
    } `json:"message"`
}

type ContentBlock struct {
    Type      string `json:"type"`          // "tool_use" | "tool_result" | "text"
    ID        string `json:"id"`            // tool_use의 고유 ID
    ToolUseID string `json:"tool_use_id"`   // tool_result가 참조하는 tool_use ID
    Name      string `json:"name"`          // tool 이름 (Read, Write, Bash, Task 등)
    Input     any    `json:"input"`         // tool 입력 데이터
    Text      string `json:"text"`          // text 블록 내용
}
```

### 6.4 ParsedTranscript 상태

```go
type ParsedTranscript struct {
    // tool_use 맵 (ID → tool info). 완료 후 삭제됨.
    ToolUses map[string]ToolInfo

    // 완료된 도구 수 (단순 카운터, 무한 증가)
    CompletedToolCount int

    // 현재 실행 중인 tool ID (tool_result 수신 시 제거)
    RunningToolIDs map[string]bool

    // 마지막 TodoWrite input (todoProgress용)
    LastTodoWriteInput any

    // 활성 에이전트 ID (name=="Task"인 tool_use)
    ActiveAgentIDs map[string]bool

    // 완료된 에이전트 수
    CompletedAgentCount int

    // Tasks (TaskCreate/TaskUpdate로 생성된 task 목록)
    Tasks map[string]TaskInfo  // seqId → {subject, status}
    NextTaskID int

    // Pending (tool_result 수신 전 버퍼)
    PendingTaskCreates map[string]PendingCreate  // tool_use_id → info
    PendingTaskUpdates map[string]PendingUpdate

    // 세션 정보
    SessionStartTime int64   // 첫 엔트리의 timestamp
    SessionName      string  // customTitle 필드
}
```

### 6.5 엔트리 처리 로직

```
for each entry:
  1. 첫 timestamp → SessionStartTime
  2. customTitle → SessionName
  3. type=="assistant" + tool_use 블록:
     - ToolUses[id] = {name, timestamp, input}
     - RunningToolIDs에 추가
     - name=="Task" → ActiveAgentIDs에 추가
     - name=="TaskCreate" → PendingTaskCreates에 추가 (seqId 부여)
     - name=="TaskUpdate" → PendingTaskUpdates에 추가
  4. type=="user" + tool_result 블록:
     - CompletedToolCount++
     - RunningToolIDs에서 제거
     - ActiveAgentIDs에 있으면 → 제거 + CompletedAgentCount++
     - ToolUses에서 name=="TodoWrite" → LastTodoWriteInput = input
     - PendingTaskCreates에 있으면 → Tasks에 확정
     - PendingTaskUpdates에 있으면 → Tasks 업데이트
     - ToolUses에서 삭제 (메모리 절약)
```

---

## Phase 7: check-usage CLI

### 7.1 기능

독립 실행 가능한 CLI 대시보드. 모든 AI CLI (Claude, Codex, Gemini, z.ai)의 사용량을 한눈에 보여주고, 가장 여유 있는 CLI를 추천.

### 7.2 엔트리 포인트

별도 바이너리 또는 같은 바이너리의 서브커맨드:
```bash
cc-usage check    # pretty 출력
cc-usage check --json   # JSON 출력
cc-usage check --lang ko
```

### 7.3 출력 형식

```
════════════════════════════════════════
          CLI Usage Dashboard
════════════════════════════════════════

[Claude]
  5h: 42% (2h30m)  |  7d: 69% (3d2h)

[Codex]
  5h: 30% (1h45m)  |  7d: 45% (5d)  |  Plan: plus

[Gemini]
  gemini-2.5-pro   58% (12h)
  gemini-2.5-flash 20% (12h)

[z.ai]
  5h: 15% (3h)  |  1m: 8%

════════════════════════════════════════
Recommendation: codex (Lowest usage: 30% used)
════════════════════════════════════════
```

### 7.4 추천 알고리즘

```
각 CLI의 5h 사용률을 비교 (숫자가 낮을수록 좋음)
- Claude: fiveHourPercent (z.ai provider면 제외)
- Codex: primaryWindow.usedPercent
- Gemini: usedPercent
- z.ai: tokensPercent
→ 가장 낮은 CLI 추천
```

### 7.5 JSON 출력 구조

```json
{
  "claude": { "name": "Claude", "available": true, "error": false, "fiveHourPercent": 42, "sevenDayPercent": 69, ... },
  "codex": { "name": "Codex", "available": true, ... },
  "gemini": null,
  "zai": null,
  "recommendation": "codex",
  "recommendationReason": "Lowest usage (30% used)"
}
```

### 7.6 커맨드 파일

`commands/check-usage.md`:
```markdown
---
description: Check all AI CLI usage limits and get recommendations
allowed-tools: Bash
---
# Tasks
1. Find plugin binary and run: `cc-usage check $ARGUMENTS`
```

---

## Phase 8: 추가 커맨드

### 8.1 update 커맨드

`commands/update.md`: 플러그인 업데이트 후 settings.json의 statusLine 경로를 최신 버전으로 갱신.

**Go 바이너리 특화:**
- Node.js 경로 대신 OS별 바이너리 경로 감지
- `ls -d ~/.claude/plugins/cache/cc-usage/cc-usage/*/dist/cc-usage-{os}-{arch}` 패턴

### 8.2 setup-alias 커맨드

`commands/setup-alias.md`: `check-ai` 쉘 함수를 `.zshrc`/`.bashrc`/PowerShell profile에 추가.

**Go 바이너리 특화:**
```bash
# bash/zsh function
check-ai() {
  "$(ls -d ~/.claude/plugins/cache/cc-usage/cc-usage/*/dist/cc-usage-$(uname -s | tr A-Z a-z)-$(uname -m) 2>/dev/null | sort -V | tail -1)" check "$@"
}
```

---

## 구현 우선순위 요약

| 순서 | Phase | 내용 | 의존성 |
|------|-------|------|--------|
| 1 | 2.1-2.3 | sessionDuration, burnRate, cacheHit | session tracking |
| 2 | 6 | Transcript 파싱 시스템 | 없음 |
| 3 | 2.4 | todoProgress | transcript |
| 4 | 3.1-3.2 | toolActivity, agentStatus | transcript |
| 5 | 3.3-3.5 | tokenBreakdown, tokenSpeed, depletionTime | 없음 |
| 6 | 4.1-4.6 | Group C 위젯 전체 | budget tracking |
| 7 | 5.1 | Provider 감지 | 없음 |
| 8 | 5.2 | Codex 클라이언트 | 없음 |
| 9 | 5.3 | Gemini 클라이언트 | 없음 |
| 10 | 5.4 | z.ai 클라이언트 | provider |
| 11 | 7 | check-usage CLI | 5.2-5.4 |
| 12 | 8 | 추가 커맨드 | 없음 |
