# cc-usage 구현 체크리스트

DESIGN.md + ROADMAP.md 기반 단계별 구현 플랜. 각 단계는 이전 단계 완료 후 진행.

## 진행 규칙

- **한 Step씩** 순서대로 진행한다. 다음 Step으로 넘어가기 전에 현재 Step을 완료한다.
- 각 Step 완료 후 **상세 설명**을 제공한다:
  - 생성/수정한 파일 목록
  - 주요 구현 내용과 설계 판단 근거
  - DESIGN.md/ROADMAP.md 스펙 대비 달라진 점이 있으면 명시
  - 검증 결과
- 완료된 항목은 `[x]`로 체크한다.

> **Phase 번호 참조:** 이 문서의 Phase 번호는 의존성 순서에 맞게 재배치됨. ROADMAP.md 원본 Phase와의 매핑:
> | CHECKLIST | ROADMAP | 내용 |
> |-----------|---------|------|
> | Phase 1 | (DESIGN.md) | MVP 코어 |
> | Phase 2 | Phase 2 | 세션/분석 |
> | Phase 3 | Phase 6 | Transcript 파싱 |
> | Phase 4 | Phase 3 | 모니터링 위젯 |
> | Phase 5 | Phase 4 | 부가 위젯 |
> | Phase 6 | Phase 5 | 외부 CLI 통합 |
> | Phase 7 | Phase 7 | check-usage CLI |
> | Phase 8 | Phase 8 | 추가 커맨드 |
>
> **미구현 위젯 동작:** display preset에 포함된 위젯이 아직 구현되지 않은 경우, 오케스트레이터의 graceful skip(nil/error → 건너뜀)에 의해 자동 생략됨. 예: Phase 2 완료 시 normal 모드의 `todoProgress`는 transcript 미구현으로 자동 숨김.

---

## Phase 1: MVP — 코어 파이프라인 (DESIGN.md)

### Step 1.1: 프로젝트 초기화 + stdin 파싱
- [ ] `go.mod` 생성 (`module cc-usage`, go 1.22+)
- [ ] `main.go` — 엔트리포인트: stdin 읽기 → 파싱 → 위젯 실행 → stdout
  - [ ] `--config <path>` CLI 인자 파싱 (`flag` 패키지)
  - [ ] configDir 추론 (`dirname(configPath)`, 기본값 `~/.claude/`)
- [ ] 디버그 유틸리티 (`debugLog`, stderr 출력, `DEBUG=cc-usage` 환경변수) — 이후 모든 Step에서 사용
- [ ] `stdin.go` — `StdinInput` 구조체 정의 + `json.Decoder` 파싱
  - [ ] 모든 필드 매핑 (model, workspace, worktree, context_window, cost, rate_limits 등)
  - [ ] 선택 필드는 포인터 또는 omitempty
- [ ] `config.go` — Config 구조체 + 파일 로드
  - [ ] `{configPath}` 로드 (없으면 기본값 사용, 에러 아님)
  - [ ] 기본값 병합 (language=auto, plan=max, displayMode=compact, cache.ttlSeconds=300)
  - [ ] Preset 문자열 파싱 (`|`로 라인 분리, 문자→위젯ID 매핑)
- [ ] **검증:** `echo '{"model":{"id":"test","display_name":"Test"}}' | go run .` → 패닉 없이 실행

### Step 1.2: 위젯 시스템 + 렌더링
- [ ] `widget.go` — Widget 인터페이스 (`ID`, `GetData`, `Render`)
  - [ ] `Context` 구조체 (Stdin, Config, Translations, RateLimits)
  - [ ] 위젯 레지스트리 (ID → Widget 매핑)
  - [ ] 오케스트레이터: display mode → 위젯 배열 → GetData → Render → 조인
  - [ ] `disabledWidgets` 필터링
  - [ ] nil/error 위젯 graceful skip
  - [ ] 빈 라인 자동 제거
- [ ] `render.go` — ANSI 렌더링
  - [ ] `ThemeColors` 구조체 + 8개 테마 정의 (default, minimal, catppuccin, dracula, gruvbox, nord, tokyoNight, solarized)
  - [ ] `getColorForPercent()` (≤50 Safe, ≤80 Warning, >80 Danger)
  - [ ] 세퍼레이터 4종 (pipe, space, dot, arrow)
  - [ ] `renderProgressBar()` (Width=10, █/░)
  - [ ] RESET 상수 (`\x1b[0m`)
- [ ] **검증:** 하드코딩 위젯으로 ANSI 컬러 출력 확인

### Step 1.3: 포매팅 유틸리티
- [ ] `format.go`
  - [ ] `formatTokens()` — 1500→"1.5K", 150000→"150K", 1500000→"1.5M"
  - [ ] `formatCost()` — 0.5→"$0.50", 1.234→"$1.23"
  - [ ] `formatTimeRemaining()` — resetAt → "3d2h", "2h30m", "45m"
  - [ ] `formatDuration()` — ms → "1h30m", "5m"
  - [ ] `shortenModelName()` — "Claude 3.5 Sonnet"→"Sonnet"
  - [ ] `calculatePercent()` — 0-100 클램프
  - [ ] `truncate()` — maxLen 초과 시 "…" 추가
  - [ ] `clampPercent()` — float→int, 0-100 범위
  - [ ] `osc8Link()` — OSC8 하이퍼링크
- [ ] **검증:** 각 함수 경계값 테스트 (0, 음수, 매우 큰 값)

### Step 1.4: 코어 위젯 (model, context, cost)
- [ ] `widgets_core.go`
  - [ ] `model` 위젯: 이모지(◆Opus/◇Sonnet/○Haiku) + display_name + effort(H/M/L) + fast(↯) — ⚠️ DESIGN.md TODO(model) 참조: effort/fast 필드 실측 필요
  - [ ] `context` 위젯: 프로그레스바 + used_percentage + formatTokens(total) — ⚠️ DESIGN.md TODO(context) 참조: used_percentage 폴백 계산
  - [ ] `cost` 위젯: formatCost(total_cost_usd)
- [ ] Display mode에 위젯 등록 (compact 첫 3개)
- [ ] **검증:** DESIGN.md 검증 커맨드 실행 → `◆ Opus │ ████░░░░ 30% 60K │ $1.25`

### Step 1.5: API 인증 + 클라이언트
- [ ] `credentials.go` — OAuth 토큰 추출
  - [ ] Windows/Linux: `{configDir}/.credentials.json` → `claudeAiOauth.accessToken`
  - [ ] macOS: `security find-generic-password` → JSON 파싱 → 파일 폴백
  - [ ] mtime 기반 캐시 무효화 (파일)
  - [ ] macOS: TTL 10초, 실패 시 60초 backoff
- [ ] `api.go` — Usage API 클라이언트
  - [ ] 3-tier 캐시: 메모리 → 파일(`~/.cache/cc-usage/cache-{hash}.json`) → API
  - [ ] `hashToken()` — SHA-256 앞 16자
  - [ ] 캐시 TTL: 정상=config.ttlSeconds, 에러=30초, stale=1시간
  - [ ] Request deduplication (동일 토큰 해시)
  - [ ] 에러 처리: 429(retry-after), 403(curl 폴백), 기타(negative cache)
  - [ ] 캐시 파일 정리 (1시간+, 1시간 간격 throttle)
  - [ ] `User-Agent: cc-usage/{VERSION}`, `anthropic-beta: oauth-2025-04-20`
- [ ] **검증:** 실제 credential로 API 호출 → 응답 로깅 (DEBUG=cc-usage)

### Step 1.6: Rate Limit 위젯
- [ ] `widgets_core.go` 추가
  - [ ] `rateLimit5h` — stdin.rate_limits 우선, 없으면 API 폴백
  - [ ] `rateLimit7d` — 동일 로직 (Max 플랜만)
  - [ ] `rateLimit7dSonnet` — API only (seven_day_sonnet)
  - [ ] 리셋 시간 표시 (formatTimeRemaining)
  - [ ] 사용량 색상 (getColorForPercent)
- [ ] **검증:** rate_limits 포함 JSON → `5h: 42% │ 7d: 69%`

### Step 1.7: projectInfo 위젯
- [ ] `widgets_project.go`
  - [ ] 디렉토리 basename 표시
  - [ ] git branch 감지 (`git rev-parse --abbrev-ref HEAD`)
  - [ ] ahead/behind 카운트 (`git rev-list --count --left-right @{u}...HEAD`)
  - [ ] project_dir ≠ current_dir일 때 subpath 표시
  - [ ] worktree 세션 표시
- [ ] **검증:** git 프로젝트 디렉토리에서 branch + ahead/behind 출력 확인

### Step 1.8: i18n
- [ ] `locales/en.json` — 영어 번역
- [ ] `locales/ko.json` — 한국어 번역
- [ ] `//go:embed` 로 바이너리 내장
- [ ] `Translations` 구조체 + 언어 감지 (config → LANG/LC_ALL/LC_MESSAGES → "ko" 판별)
- [ ] 기존 위젯에 Translations 적용 (라벨, 시간 단위)
- [ ] **검증:** `LANG=ko_KR.UTF-8`에서 한국어 출력 확인

### Step 1.9: 빌드 + 플러그인 패키징
- [ ] `Makefile` — build, build-local, test, clean
  - [ ] 크로스 컴파일: darwin/arm64, darwin/amd64, linux/amd64, windows/amd64
  - [ ] `-ldflags="-s -w -X main.version=VERSION"`
- [ ] `.claude-plugin/plugin.json` — 플러그인 매니페스트
- [ ] `.claude-plugin/marketplace.json` — 마켓플레이스 메타데이터
- [ ] `commands/setup.md` — `/cc-usage:setup` 커맨드
- [ ] **검증:** `make build` → 4개 플랫폼 바이너리 생성 확인

### Step 1.10: 통합 테스트
- [ ] DESIGN.md 검증 커맨드 전체 통과
  - [ ] 기본 JSON → `◆ Opus │ ████░░░░ 30% 60K │ $1.25`
  - [ ] rate_limits 포함 → `◆ Opus │ ████░░░░ 30% 60K │ $1.25 │ 5h: 42% │ 7d: 69%`
- [ ] 빈 stdin → 패닉 없이 graceful 종료
- [ ] 필수 필드만 있는 최소 JSON → 정상 출력
- [ ] `--config` 커스텀 경로 → 해당 경로의 config/credentials 사용
- [ ] settings.json에 등록 후 실제 Claude Code에서 status line 동작 확인

---

## Phase 2: 세션/분석 위젯 (ROADMAP §2)

### Step 2.1: Session Tracking 시스템
- [ ] 세션 디렉토리 생성 (`~/.cache/cc-usage/sessions/`)
- [ ] 세션 파일 읽기/쓰기 (`{sessionId}.json` → `{ "startTime": ... }`)
- [ ] atomic 생성 (`O_CREATE|O_EXCL` → 실패 시 기존 파일 읽기)
- [ ] 메모리 캐시 (`map[string]int64`)
- [ ] 세션 파일 정리 (7일+, 1시간 throttle)
- [ ] `getSessionElapsedMinutes()` 헬퍼

### Step 2.2: sessionDuration 위젯
- [ ] `widgets_project.go`에 추가
- [ ] `⏱` + formatDuration(elapsed)
- [ ] stdin.session_id 없으면 `"default"` 사용
- [ ] **검증:** 세션 파일 생성 → 재실행 시 경과시간 증가 확인

### Step 2.3: burnRate 위젯
- [ ] `widgets_analytics.go`
- [ ] `🔥` + formatTokens(tokensPerMinute) + "/m"
- [ ] 세션 1분 미만 → null (위젯 숨김)
- [ ] **검증:** total_input + total_output / elapsed minutes 정확성

### Step 2.4: cacheHit 위젯
- [ ] `widgets_analytics.go`
- [ ] `📦` + hitPercentage%
- [ ] 색상 반전: 70%+ Safe, 40-70% Warning, <40% Danger
- [ ] 0으로 나누기 방지
- [ ] **검증:** cache_read_input_tokens 비율에 따른 색상 변화

---

## Phase 3: Transcript 파싱 시스템 (ROADMAP §6)

> Phase 4~5 위젯들의 선행 의존성

### Step 3.1: Incremental 파서 코어
- [ ] `transcript.go` (신규 파일)
- [ ] `TranscriptEntry`, `ContentBlock` 구조체
- [ ] `ParsedTranscript` 상태 구조체
- [ ] Incremental 파싱: stat → offset 비교 → 새 바이트만 읽기
- [ ] Truncation 감지 → 전체 re-parse
- [ ] JSONL 라인별 `json.Unmarshal`

### Step 3.2: 엔트리 처리 로직
- [ ] assistant + tool_use → ToolUses/RunningToolIDs에 추가
- [ ] user + tool_result → CompletedToolCount++, RunningToolIDs에서 제거
- [ ] Task/Agent 추적 (name=="Task" → ActiveAgentIDs)
- [ ] TaskCreate/TaskUpdate → PendingTaskCreates/Updates → tool_result에서 확정
- [ ] TodoWrite → LastTodoWriteInput 갱신
- [ ] SessionStartTime (첫 timestamp), SessionName (customTitle)
- [ ] 완료된 ToolUses 삭제 (메모리 절약)

### Step 3.3: 검증
- [ ] 샘플 transcript.jsonl로 파싱 정확성 테스트
- [ ] 파일 append 후 incremental 파싱 → 기존 상태 유지 + 신규만 추가
- [ ] 파일 truncation → 전체 re-parse 정상 동작

---

## Phase 4: 모니터링 위젯 (ROADMAP §3)

### Step 4.1: todoProgress 위젯
- [ ] `widgets_analytics.go`
- [ ] Tasks API 우선 → TodoWrite 폴백
- [ ] `✓ completed/total` 표시
- [ ] status 정규화 (not_started→pending, running→in_progress 등)
- [ ] transcript 없으면 null

### Step 4.2: toolActivity 위젯
- [ ] `widgets_analytics.go`
- [ ] `⚙️` + running tool name(target) + `│` + completed count + "done"
- [ ] target 추출: Read/Write/Edit→basename, Glob/Grep→pattern, Bash→command
- [ ] truncate 적용

### Step 4.3: agentStatus 위젯
- [ ] `widgets_analytics.go`
- [ ] `🤖 Agent:` + running count + "running," + completed count + "done"
- [ ] subagent_type 표시 (없으면 "Agent")

### Step 4.4: 나머지 Group B 위젯
- [ ] `tokenBreakdown` — `📊 In/Out/CW/CR` 분류 표시
- [ ] `tokenSpeed` — `⚡` + tok/s (total_output / api_duration)
- [ ] `depletionTime` — `⏳` + 5h/7d 중 먼저 도달하는 시간
- [ ] **검증:** normal/detailed display mode에서 2번째 라인 위젯 정상 출력

---

## Phase 5: 부가 위젯 (ROADMAP §4)

### Step 5.1: Budget Tracking 시스템
- [ ] `~/.cache/cc-usage/budget.json` 파일 관리
- [ ] Delta 추적: date 불일치→리셋, delta=max(0, current-previous)
- [ ] Request deduplication (budget + todayCost 동시 조회)
- [ ] fire-and-forget 저장

### Step 5.2: Group C 위젯
- [ ] `configCounts` — CLAUDE.md, AGENTS.md, Rules, MCPs, Hooks, +Dirs 카운트
- [ ] `forecast` — `📈 ~$X/h` (비용/시간 추정)
- [ ] `budget` — `💵 $X/$Y` (dailyBudget 설정 시만)
- [ ] `todayCost` — `Today: $X` (일일 합계)
- [ ] `linesChanged` — `+N -N` (stdin 우선, git 폴백)

### Step 5.3: 기타 간단 위젯
- [ ] `version` — stdin.version
- [ ] `sessionName` — stdin.session_name 또는 transcript
- [ ] `sessionId` / `sessionIdFull` — session_id 앞 8자 / 전체
- [ ] `vimMode` — NORMAL/INSERT (vim 활성화 시만)
- [ ] `outputStyle` — "default" 아닐 때만
- [ ] `apiDuration` — api_duration / total_duration * 100
- [ ] `peakHours` — PT 기준 피크(평일 5-11 AM) / 오프피크
- [ ] `performance` — cache hit + output ratio 복합 배지
- [ ] `lastPrompt` — transcript 마지막 user text

### Step 5.4: 검증
- [ ] detailed display mode 전체 라인 출력 확인
- [ ] budget.json 날짜 변경 시 리셋 동작
- [ ] configCounts가 실제 파일시스템 스캔 결과와 일치

---

## Phase 6: 외부 CLI 통합 (ROADMAP §5)

### Step 6.1: Provider 감지
- [ ] `detectProvider()` — ANTHROPIC_BASE_URL → "zai" / "zhipu" / "anthropic"

### Step 6.2: Codex CLI 클라이언트
- [ ] `~/.codex/auth.json` 존재 감지
- [ ] 인증: access_token + account_id 추출, mtime 캐시
- [ ] API: `GET chatgpt.com/backend-api/wham/usage`
- [ ] 모델 감지: config.toml → 캐시 → `codex exec` 폴백
- [ ] `codexUsage` 위젯: `🔷 codex 5h:X% 7d:Y%`
- [ ] 메모리 캐시 + negative cache 30초

### Step 6.3: Gemini CLI 클라이언트
- [ ] `~/.gemini/oauth_creds.json` 존재 감지 (macOS: Keychain 우선)
- [ ] OAuth 토큰 갱신 (expiry 5분 전, Google OAuth endpoint)
- [ ] 프로젝트 ID 조회 (환경변수 → settings.json → API)
- [ ] Quota API 호출 → remainingFraction → usedPercent
- [ ] 모델 감지 (settings.json)
- [ ] `geminiUsage` 위젯: 현재 모델 버킷
- [ ] `geminiUsageAll` 위젯: 전체 버킷

### Step 6.4: z.ai/ZHIPU 클라이언트
- [ ] `detectProvider()` == "zai"/"zhipu" 조건
- [ ] 인증: ANTHROPIC_AUTH_TOKEN 환경변수
- [ ] API: `{base_url_origin}/api/monitor/usage/quota/limit`
- [ ] 사용률 파싱 (percentage → currentValue/remaining → currentValue/usage)
- [ ] `zaiUsage` 위젯: `Z: 5h X% │ 1m Y%`

### Step 6.5: 검증
- [ ] Codex 미설치 → codexUsage null (위젯 숨김)
- [ ] Gemini 토큰 만료 → 자동 갱신 → 정상 데이터
- [ ] z.ai provider가 아닐 때 → zaiUsage null

---

## Phase 7: check-usage CLI (ROADMAP §7)

### Step 7.1: 서브커맨드 구조
- [ ] `cc-usage check` 엔트리포인트 (stdin 없이 독립 실행)
- [ ] `--json` 플래그 (JSON 출력)
- [ ] `--lang` 플래그 (언어 오버라이드)

### Step 7.2: 대시보드 출력
- [ ] Claude 사용량 조회 (기존 API 클라이언트 재사용)
- [ ] Codex 사용량 조회
- [ ] Gemini 사용량 조회
- [ ] z.ai 사용량 조회
- [ ] Pretty 포맷 출력 (테이블 + 구분선)
- [ ] JSON 포맷 출력

### Step 7.3: 추천 알고리즘
- [ ] 각 CLI의 5h 사용률 비교 → 가장 낮은 CLI 추천
- [ ] 미설치/에러 CLI 제외
- [ ] **검증:** 여러 CLI 조합으로 추천 정확성 확인

### Step 7.4: 커맨드 파일
- [ ] `commands/check-usage.md` 작성

---

## Phase 8: 추가 커맨드 (ROADMAP §8)

### Step 8.1: update 커맨드
- [ ] `commands/update.md` — 플러그인 업데이트 + settings.json 경로 갱신
- [ ] OS별 바이너리 경로 감지 패턴

### Step 8.2: setup-alias 커맨드
- [ ] `commands/setup-alias.md` — `check-ai` 쉘 함수 등록
- [ ] bash/zsh/PowerShell 분기

---

## 공통 품질 기준

각 Phase 완료 시 확인:
- [ ] `go build ./...` 성공 (컴파일 에러 없음)
- [ ] `go vet ./...` 경고 없음
- [ ] 빈/최소/전체 stdin JSON에 대해 패닉 없음
- [ ] DEBUG=cc-usage로 디버그 로그 확인 가능
- [ ] 실제 Claude Code status line에서 동작 확인 (Phase 1 이후)
