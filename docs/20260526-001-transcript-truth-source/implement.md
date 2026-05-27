# IMPLEMENT — transcript-truth-source

> 실행 체크리스트. 각 Task는 한 verify 사이클로 평가 가능한 단위다. 설계 근거는 analysis.md, 요구사항 완료 조건은 spec.md §5.

## Section: 기반 (transcript 소비 인프라)

- [x] task-001: cwd → transcript 디렉토리 인코딩 함수
  - 목적: 현재 cwd 문자열이 어느 OS에서든 동일 규칙으로 `~/.claude/projects/<encoded>` 디렉토리 이름으로 변환된다
  - 접근: `transcript.go` 신규. `encodeCwdToTranscriptDir(home, cwd string) string` — `/`, `\`, `:`, `.` 네 구분자를 모두 `-`로 치환하는 forward-only 함수. 디코딩 경로는 두지 않음
  - 검증 조건:
    - 결과: `C:\Users\zipke\GolandProjects\cc-usage` → `.../projects/C--Users-zipke-GolandProjects-cc-usage`, POSIX `/home/u/p.q` → `.../projects/-home-u-p-q`
    - 확인: 단위 테스트로 Windows backslash·드라이브 콜론·POSIX 슬래시·dot 케이스 각각 검증
  - 참조: SPEC §5.7, ANALYSIS §3

- [x] task-002: transcript 후보 jsonl 선택
  - 목적: 한 cwd 디렉토리 안에 여러 jsonl이 있어도 가장 최근 세션 파일 하나가 결정적으로 선택되고, 디렉토리 부재·빈 디렉토리는 후보 없음으로 graceful 처리된다
  - 접근: `selectTranscriptCandidate(dir string) (string, error)` — `*.jsonl` 중 newest mtime, mtime 동률은 lex sort. 디렉토리 부재/파일 0개는 빈 문자열+에러로 표현
  - 검증 조건:
    - 결과: 다중 jsonl에서 newest mtime 선택, 동률 시 lex 우선 파일 선택, 빈/부재 디렉토리는 후보 없음
    - 확인: 단위 테스트로 다중 파일·mtime 동률·빈 디렉토리·부재 디렉토리 케이스 검증
  - 참조: SPEC §5.9, SPEC §5.7

- [x] task-003: tail window 역방향 마지막 assistant entry 파싱
  - 목적: 5MB+ transcript여도 파일 전체를 읽지 않고 마지막 assistant entry의 model·usage·cwd만 얻으며, 상한 도달 시 후보 없음으로 폴백한다
  - 접근: `readLastAssistantEntry(path string, initialWindow, maxWindow int) (*transcriptEntry, error)` — 64KB tail 시작, 부족 시 2배 확장, 상한 1MB. 역방향 line scan으로 `type=="assistant"` 첫 매칭. 마지막 partial line skip. `transcriptEntry{Model, Usage, Cwd}`만 보유
  - 검증 조건:
    - 결과: 마지막 assistant entry의 base model ID·4토큰 usage·entry.cwd 추출, 비-assistant type skip, partial 마지막 line skip 후 그 앞 완전 line 매칭, 상한 초과 시 nil
    - 확인: 단위 테스트로 read 바이트 수 상한·partial line·type 필터·entry 부재·상한 초과 케이스 검증
  - 참조: SPEC §5.8, SPEC §5.2, SPEC §5.3, SPEC §5.10

## Section: 보조 캐시·단가표

- [x] task-004: cwd별 last-known `[1m]` 캐시
  - 목적: stdin이 `[1m]` 신호를 줄 때마다 cwd별로 영속 저장되고, transcript-only 경로에서 그 신호를 조회해 1M 여부를 cross-cwd 노출 없이 결정한다
  - 접근: 단일 파일 `~/.cache/cc-usage/one-m-by-cwd.json` (cwd-hash → bool). `loadLastKnownOneM(cwd) bool` / `saveLastKnownOneM(cwd, val)`. read·write 모두 cwd-exact-match 가드 하. atomicWriteFile 재사용
  - 검증 조건:
    - 결과: `[1m]` 저장 후 같은 cwd에서 true 조회, 다른 cwd-hash는 false(기본값 200K 경로), 파일 부재 시 false
    - 확인: 단위 테스트로 저장·조회·cross-cwd 격리·파일 부재 케이스 검증
  - 참조: SPEC §5.3, ANALYSIS §5 (D2, D10)

- [x] task-005: 모델 단가표 embed + lookup + cost 추정
  - 목적: 알려진 모델 ID에 대해 transcript usage로 estimated cost를 계산할 수 있고, 알려지지 않은 모델은 lookup 실패로 표현된다
  - 접근: `pricing.go` 신규. `map[string]modelPricing` hardcode embed (키=base model ID, 필드 Input/Output/CacheRead/CacheCreation5m/CacheCreation1h USD per MTok, Anthropic 공식 가격 참조). `lookupPricing(modelID) (modelPricing, bool)`, `estimateCost(usage, p) float64`
  - 검증 조건:
    - 결과: 알려진 모델은 4토큰 카테고리×단가 합산값 반환, 미등록 모델은 lookup false
    - 확인: 단위 테스트로 알려진 모델 합산·미등록 모델 miss 검증, `go vet` 통과
  - 참조: SPEC §5.5, ANALYSIS §5 (D1, D11)

## Section: cost·projectInfo 위젯

- [x] task-006: cost 위젯 estimated 마커
  - 목적: estimated cost일 때 사용자가 식별 가능한 시각 마커가 표시되고, 정상 stdin 경로(직접 cost)에서는 마커 없이 기존과 동일하게 표시된다
  - 접근: `widget.go` `Context`에 `CostEstimated bool` 추가. `widgets_core.go` `costWidget.Render`에서 `ctx.CostEstimated`가 true면 마커(i18n locale 정합 보고 `$3.14*`/`~$3.14` 중 택1), 단가표 miss로 cost 없으면 빈 문자열. `GetData` 무변경
  - 검증 조건:
    - 결과: estimated일 때 마커 포함 출력, 정상 stdin은 마커 없는 기존 출력, 단가표 miss는 cost 위젯 skip
    - 확인: 단위 테스트로 estimated 마커·정상 경로 무변경·miss skip 검증
  - 참조: SPEC §5.5, SPEC §5.11, ANALYSIS §5 (D6, D11)

- [x] task-007: projectInfo fresh cwd 폴백
  - 목적: stdin workspace가 비어도 `detectCurrentCwd()`로 얻은 cwd에서 projectInfo(branch 포함)가 fresh 조회되어 표시되고, cwd도 못 얻으면 기존대로 nil
  - 접근: `widgets_project.go` `projectInfoWidget.GetData` 첫 가드를 `CurrentDir == ""`이면 `detectCurrentCwd()` 폴백 후, 그래도 비면 nil
  - 검증 조건:
    - 결과: 빈 workspace + 식별 가능 cwd → projectInfo 표시, cwd 식별 불가 → nil(기존 동작)
    - 확인: 단위 테스트로 빈 workspace 폴백·cwd 부재 nil 검증, 수동 빈 stdin smoke로 projectInfo 라인 확인
  - 참조: SPEC §5.4, ANALYSIS §5 (D7)

## Section: main 흐름 통합

- [x] task-008: Layer 2 transcript backfill 흐름 통합
  - 목적: 빈 stdin + 식별 가능 cwd + 읽기 가능 transcript일 때 model/context/cost가 transcript에서 채워져 직전과 동등한 full 출력이 나오고, entry.cwd 불일치 시 미발동한다
  - 접근: `main.go` 1·2차 orchestrate 사이에 Layer 2 추가. `needsTranscriptBackfill`(model/ContextWindow 여전히 빔) → cwd·transcriptPath 결정 → `readLastAssistantEntry` → entry.cwd × `normalizeCwd` 정확 일치 가드(D4) → `loadLastKnownOneM` → `applyTranscriptToStdin`(restoredFieldMask 통합, `ctx.CostEstimated` 설정) → 3차 orchestrate. stdin.transcript_path 우선, 없으면 인코딩 디렉토리 후보
  - 검증 조건:
    - 결과: 빈 stdin + transcript 존재 → full 복원(estimated cost), entry.cwd 불일치 → 미발동, mask로 복원 필드 추적
    - 확인: 통합 테스트로 full 복원·entry.cwd 가드·다중 cwd cross-cwd 노출 0회 검증
  - 참조: SPEC §5.1, SPEC §5.2, SPEC §5.3, SPEC §5.5, SPEC §5.7, ANALYSIS §2, ANALYSIS §5 (D4, D8)

- [x] task-009: Layer 2 실패 경로 graceful 폴백
  - 목적: cwd 식별 불가·디렉토리 부재·jsonl 0개·read 실패·entry 부재·파싱 실패·상한 초과 등 모든 실패에서 panic·hang 없이 v0.3.7+ 보수 출력/묵음으로 폴백한다
  - 접근: task-008 흐름의 각 분기에서 실패 시 debugLog 후 Layer 2 미발동(Layer 3로 진행). 신규 의존이 새 실패 모드를 만들지 않도록 모든 경로 graceful skip
  - 검증 조건:
    - 결과: 손상 transcript·빈 디렉토리·부재 디렉토리·cwd 불가 각각에서 graceful fallback, append-while-read 시뮬레이션 무panic
    - 확인: 회귀 테스트로 각 실패 모드 + append-while-read 시뮬레이션 PASS
  - 참조: SPEC §5.6, SPEC §5.10, SPEC §5.7

- [x] task-010: restoredFieldMask 통합 + self-perpetuation 차단
  - 목적: Layer 2가 채운 필드도 저장 직전 vacate되어 transcript 복원값이 다음 호출에 정상값처럼 재전파되지 않으며, 정상 stdin 값은 보존된다
  - 접근: `applyTranscriptToStdin`이 Layer 1과 같은 `restoredFieldMask` 구조에 누적. 저장 단계의 `stripRestoredFields`가 Layer 1·2 통합 mask로 vacate
  - 검증 조건:
    - 결과: transcript 복원 필드는 저장 스냅샷에서 비워짐, 정상 stdin 필드는 보존
    - 확인: 단위 테스트로 통합 mask vacate·정상 필드 보존 검증
  - 참조: SPEC §5.14, ANALYSIS §5 (D8)

- [x] task-011: last-known `[1m]` 저장 트리거
  - 목적: stdin이 `[1m]` 신호를 제공하고 cwd가 식별되면 매 호출 그 사실이 cwd별로 갱신 저장된다
  - 접근: `main.go` 출력 이후 단계에서 `stdin.Model.ID`가 `[1m]` 포함 + cwd 식별 시 `saveLastKnownOneM(cwd, true)`
  - 검증 조건:
    - 결과: `[1m]` stdin 처리 후 해당 cwd 캐시가 true로 갱신
    - 확인: 단위/통합 테스트로 `[1m]` stdin → 캐시 갱신 검증
  - 참조: SPEC §5.3, ANALYSIS §5 (D2, D10)

- [x] task-012: transcript 결정 디버그 로그
  - 목적: 어떤 transcript를 골랐는지·model을 어디서 채웠는지·폴백 사유가 `DEBUG=cc-usage`에서 stderr로 확인된다
  - 접근: Layer 2 각 분기(후보 선택, entry 매칭, 가드 통과/실패, 폴백 사유)에 `debugLog`. stdout 비오염
  - 검증 조건:
    - 결과: 빈 stdin smoke를 디버그 모드로 실행 시 transcript 결정 로그가 1줄 이상 stderr 기록, stdout 무오염
    - 확인: 수동 `DEBUG=cc-usage` smoke로 stderr 로그 확인
  - 참조: SPEC §5.12

## Section: 회귀·릴리스

- [x] task-013: 정상 stdin 경로 무회귀 보장
  - 목적: 정상 stdin이 올 때 출력이 transcript fallback 도입 이전과 동일하고, safe-empty-stdin-fallback의 §5.1~§5.11 및 v0.3.4+ 회귀 테스트가 변경 없이 PASS 유지된다
  - 접근: 정상 stdin은 1차 orchestrate에서 model/context 채워짐 → `needsTranscriptBackfill` false → Layer 2 미발동. 기존 회귀 테스트 전부 실행
  - 검증 조건:
    - 결과: 정상 stdin 출력 기존과 동일, `ctx.CostEstimated` 항상 false, shouldSuppressOutput/isWarmupExceptionPath 본문 무변경
    - 확인: `go test ./...` 전체 PASS (기존 회귀 + safe-empty-stdin-fallback §5.1~§5.11 테스트 포함)
  - 참조: SPEC §5.11, SPEC §5.14

- [x] task-014: cost 정책 분기 + 통합 경로 회귀 테스트 세트
  - 목적: spec §5.13이 요구한 경로((a) full 복원, (b) 디렉토리 부재, (c) cwd 불가, (d) 손상/entry 부재, (e) cost 정책 분기 각각)가 모두 명시적 테스트로 커버되고 cross-cwd 노출 0회가 보장된다
  - 접근: 여러 구현 Task에 걸치는 e2e 성격 테스트 세트. 직접 cost·estimated cost·단가표 miss skip 분기 각각, 다중 cwd 격리 케이스 포함
  - 검증 조건:
    - 결과: (a)~(e) 각 경로 명시적 케이스 PASS, 모든 케이스에서 cross-cwd 노출 0회
    - 확인: `go test ./...`에서 각 경로 케이스 PASS
  - 참조: SPEC §5.13, SPEC §5.5, SPEC §5.6, SPEC §5.7

- [x] task-015: SemVer bump v0.3.10 → v0.3.11

- [x] task-016: transcript root에 CLAUDE_CONFIG_DIR 반영 (v0.3.11 회귀 수정)
  - 목적: config 홈을 옮긴 환경(CLAUDE_CONFIG_DIR)에서도 워크스페이스 transcript를 찾아 Layer 2가 발동한다 — `~/.claude` 고정으로 datadog-analyzer 등이 rate-limit only로 떨어지던 회귀 제거
  - 접근: `encodeCwdToTranscriptDir`이 root 결정 시 `CLAUDE_CONFIG_DIR`(공백 trim 후 비어있지 않으면) 우선, 없으면 `<home>/.claude`. SemVer v0.3.11→v0.3.12
  - 검증 조건:
    - 결과: CLAUDE_CONFIG_DIR 설정 시 `<cfg>/projects/<encoded>`, 미설정/공백 시 `<home>/.claude/projects/<encoded>`. 실제 바이너리 smoke에서 datadog-analyzer cwd가 full 복원
    - 확인: 단위 테스트(override/blank/fallback) + datadog-analyzer cwd DEBUG smoke로 Layer 2 발동 확인
  - 참조: SPEC §5.1, SPEC §5.7, SPEC §5.15
  - 목적: `/plugin` UI가 update를 감지하도록 버전이 세 곳에서 동일하게 갱신되고 바이너리가 새 버전을 출력한다
  - 접근: `Makefile` VERSION, `.claude-plugin/plugin.json` version, `api.go` userAgent 세 곳을 v0.3.11로 동시 갱신
  - 검증 조건:
    - 결과: `./dist/cc-usage --version`이 v0.3.11 출력, 세 파일 grep 결과 일치
    - 확인: `make build-local` 후 `--version` 확인, 세 파일 grep 일치 확인
  - 참조: SPEC §5.15
