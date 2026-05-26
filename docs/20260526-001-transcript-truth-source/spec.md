# SPEC — transcript-truth-source

## 1. 범위

cc-usage가 status line 출력을 만들 때 의존하는 **진실 소스(source of truth)의 우선순위**와, stdin이 비거나 degraded한 상황에서 그 우선순위에 따라 어떤 데이터로 위젯을 채울지를 결정한다. 대상 위젯: `model`, `context`, `cost`, `projectInfo`, 그리고 그 데이터에 종속된 출력 묵음/표시 판단.

대상 코드 표면 (산출물의 실제 변경 경계는 analysis가 결정): stdin 파싱 결과 → cache key 결정 → degraded restore → 출력 묵음/표시 판단 경로 (현재 `main.go`, `cache.go`, `widgets_*.go`).

본 feature는 Claude Code가 워크스페이스별로 남기는 **transcript 파일**(`~/.claude/projects/-<encoded_cwd>/<session_id>.jsonl`)을 새 진실 소스로 받아들이는 정책을 정의한다. session-state 캐시(`session-state-<key>.json`)의 존재 의의는 transcript로 cover되지 않는 영역에 한해 재정의된다.

## 2. 목표

- **빈/degraded stdin이 와도 사용자가 보던 status line이 그대로 유지된다.** 특히 `sessionStateTTL = 300s` 초과 idle 후 첫 갱신에서 model/context/projectInfo 위젯이 사라지면서 partial로 떨어지는 현행 동작(v0.3.7+)이 제거된다.
- **다른 워크스페이스·세션의 데이터가 현재 화면에 노출되는 사건이 0회**로 유지된다. v0.3.5 cross-workspace 회귀의 재발 방지.
- **stdin 외 진실 소스(transcript)에 새로 의존하더라도 그 부재·손상·포맷 변화에 대해 안전하게 폴백한다.** 새 의존이 새 실패 모드를 만들지 않는다.
- **cost처럼 transcript에서 직접 복원이 불가능한 항목에 대한 정책이 명시적**으로 결정된다 — 무엇이 표시되고 무엇이 표시되지 않는지가 사용자 관점에서 예측 가능하다.

## 3. 제약

- **정확성 우선 원칙**: 잘못된 값(다른 워크스페이스·다른 세션·다른 모델·stale 가격에 기반한 cost)을 표시할 가능성이 0%보다 큰 fallback은 채택하지 않는다. partial 또는 묵음 출력은 잘못된 정보 출력보다 항상 선호된다.
- zero-dependency 유지: Go 표준 라이브러리 외 의존성을 추가하지 않는다.
- 단일 `main` 패키지 유지: 서브 패키지를 만들지 않는다 (CLAUDE.md §패키지).
- 출력 채널 규칙 유지: stdout은 위젯 출력 전용, stderr는 debug/error 전용 (CLAUDE.md §출력).
- **호출당 추가 I/O 상한**: status line은 자주 호출되므로 transcript read가 추가하는 I/O는 기존 `cacheLockTimeout = 200ms`와 같은 차수 안에서 처리되어야 한다. transcript 파일은 5MB+까지 커질 수 있으므로 매 호출 풀 read는 허용되지 않으며, 마지막 assistant entry 근방만 읽는 방식이 강제된다.
- **transcript 파일 포맷·디렉토리 규칙은 외부 변수**로 가정한다: cc-usage가 Claude Code의 포맷을 fix하지 않으며, 포맷 변경 시에도 본 fallback 로직이 silently graceful하게 동작해야 한다.
- **인코딩 방향 제약**: cwd → transcript 디렉토리 이름 변환은 forward-only deterministic 함수로 정의된다 (현재 관찰된 규칙: `/`, `.` 모두 `-`로 치환). 디코딩(인코딩된 dirname → cwd)에 의존하는 코드 경로는 허용되지 않는다 — 인코딩이 lossy이므로 reverse는 모호하다.
- **cost 정책**: (b) 채택. cost 위젯은 항상 표시하되 stdin이 cost를 직접 제공할 때는 그 값을, 빈 stdin·transcript 폴백 경로에서는 transcript usage × 내장 모델 단가표로 계산한 estimated 값을 표시한다. estimated인 경우 사용자가 식별할 수 있는 시각 마커를 함께 표시한다(예: `$3.14*` 또는 `~$3.14`). 단가표는 cc-usage 코드에 hardcode embed(zero-dep 유지)되며, Anthropic 가격 변경 시 cc-usage 자체 패치(SemVer bump 동반)로 갱신한다. 단가표에 없는 모델 ID에 대해서는 cost 위젯을 skip한다(정확성 우선).
- **context_window_size 추론**: (ii) 채택. stdin이 `model.id`에 `[1m]` 같은 컨텍스트 윈도우 신호를 줄 때마다 그 사실을 cwd별로 영속 저장하고, transcript-only 복원 경로에서는 그 캐시를 조회해 1M 여부를 결정한다. 캐시가 없는 cwd에서는 모델 ID의 base size(보통 200K)를 기본값으로 사용한다. 같은 cwd 안에서 사용자가 `/model`로 컨텍스트 윈도우 모드를 토글하면 일시적 stale이 발생할 수 있으나 다음 정상 stdin에서 즉시 보정된다. 캐시는 cwd-exact-match 가드 아래에서만 read·write되어 cross-cwd 노출이 0이어야 한다.
- **TTL 정책 분리**: 기존 `sessionStateTTL = 300s`, `workspaceRestoreTTL`(safe-empty-stdin-fallback에서 결정된 값)은 유지된다. transcript 기반 복원은 TTL 영향을 받지 않는다 — transcript 파일은 Claude Code가 직접 관리하고 cc-usage가 삭제하지 않는다.
- **동시성 안전성**: 다중 cc-usage 인스턴스가 같은 transcript 파일을 read하는 상황에서 race·panic이 없어야 한다. transcript는 Claude Code가 append-only로 쓰는 파일이므로 read 도중 partial line이 보일 수 있다는 점을 가정하고 처리한다.
- **context_window_size 추론**: 모델 ID에서 컨텍스트 사이즈를 결정하는 정책은 알려진 모델 목록 기반이며, 알 수 없는 모델 ID에 대해서는 안전한 기본값을 쓰거나 context 위젯을 skip하는 명시적 정책을 따른다. (예: `claude-opus-4-7[1m]` 같은 stdin-only suffix는 transcript에는 없으므로 transcript-only 경로에서는 1M 컨텍스트 식별 신호가 부재할 수 있다.)

## 4. 제외 범위

- Claude Code가 빈 stdin을 보내는 원인을 추적·수정하는 작업 (Claude Code 본체 영역).
- transcript 파일 포맷·디렉토리 규칙 자체에 대한 변경·확장 제안 (Claude Code 본체 영역).
- 새 위젯 추가, 기존 위젯의 표시 옵션 변경, status line UI/UX 개편.
- 캐시 파일 포맷 변경 또는 마이그레이션 (기존 `session-state-<key>.json` 스키마 유지).
- session-state 캐시 파일 자체의 완전 제거. 본 feature는 transcript가 cover하지 못하는 영역에 한해 session-state 사용을 재정의하되, 캐시 파일을 삭제·폐기하는 결정은 별도 feature.
- OAuth credential 캐시 또는 API rate-limit 캐시(`cache-<tokenHash>.json`)의 동작 변경.
- rate-limit 위젯(5h/7d/7d-Sonnet)의 데이터 소스·표시 방식 변경 — 이미 계정 단위 통합 캐시로 분리되어 있으며 본 feature 범위 밖이다.
- `displayPresets`/`Config.Lines`/`disabledWidgets` 등 사용자 설정 표면의 변경.
- transcript 파일을 cc-usage가 쓰거나 수정하는 동작 — cc-usage는 read-only 소비자로만 동작한다.

## 5. 완료 조건

1. 빈 stdin이 도착했을 때 cc-usage가 현재 cwd를 식별할 수 있고 그 cwd에 대응하는 transcript 파일이 존재·읽기 가능하면, status line은 직전 정상 호출과 동등한 full 출력(projectInfo + model + context + (cost 정책에 따른) + rate-limit)을 표시한다. — UI 관찰: 동일 워크스페이스에서 `sessionStateTTL` 초과 idle 후에도 사용자가 보던 status line이 그대로 유지된다.

2. 빈 stdin이 도착해도 `model` 위젯의 표시 내용이 채워진다. 채움 소스는 해당 cwd 디렉토리 안에서 선택된 transcript의 마지막 assistant entry의 `message.model`이다. — UI 관찰: idle 후 빈 stdin이 와도 model 라인이 사라지지 않는다.

3. 빈 stdin이 도착해도 `context` 위젯의 표시 내용이 채워진다. 채움 소스는 해당 transcript의 마지막 assistant entry의 `message.usage`이다. context_window_size는 §3의 모델 ID 기반 추론 정책에 따라 결정되며, 알 수 없는 모델 ID에 대해서는 context 위젯이 안전하게 skip되거나 정의된 기본값으로 표시된다. — UI 관찰: 빈 stdin 후에도 context 라인이 사라지지 않거나, skip 정책에 따라 일관되게 동작한다.

4. 빈 stdin이 도착해도 `projectInfo` 위젯의 표시 내용이 채워진다. 채움 소스는 `detectCurrentCwd()`로 얻은 현재 cwd이며, git branch는 그 cwd에서 fresh 조회한다. (본 feature에서 도입한다 — v0.3.10까지의 `widgets_project.go`는 stdin의 workspace가 비면 nil을 반환해 위젯이 통째로 사라진다.) — UI 관찰: 새 워크스페이스 첫 실행이나 캐시 부재 시에도 projectInfo 라인이 표시된다.

5. `cost` 위젯은 §3에서 선택한 정책에 따라 작동한다. 어느 정책이든 다른 cwd 또는 다른 세션의 cost 값이 현재 화면에 표시되는 사건은 0회이다. — UI 관찰: 워크스페이스 A → B 전환 후 B의 화면에 A의 cost가 표시되지 않는다.

6. cc-usage가 transcript 파일을 찾지 못하거나 read에 실패하는 경우(cwd 식별 불가, transcript 디렉토리 없음, 디렉토리는 있으나 파일 없음, 모든 후보 파일 read 실패, 마지막 assistant entry 부재, JSON 파싱 실패 등) status line은 `safe-empty-stdin-fallback` v0.3.7+ 동작(보수적 출력 또는 묵음)으로 graceful하게 폴백한다. 새 동작 도입이 v0.3.7+ 동작에 회귀를 만들지 않는다. — Library/CLI 관찰: 손상된 transcript / 빈 디렉토리 등 각 실패 모드에 대해 회귀 테스트가 통과한다.

7. transcript 후보 선택은 현재 cwd 인코딩 결과 **디렉토리 안에서만** 이루어진다. 다른 cwd의 transcript 디렉토리 또는 그 안의 파일이 현재 화면 어느 위젯에도 노출되지 않는다. — Library 관찰: 다중 cwd 시나리오 통합 테스트에서 cross-cwd 노출 0회.

8. transcript 파일 read는 매 status line 호출당 파일 전체를 읽지 않는다. 마지막 assistant entry 또는 그 근방만 읽어서 model/usage 정보를 얻는다. — Library 관찰: 5MB+ transcript에 대한 호출당 read 바이트 수가 정의된 상한(analysis가 수치를 정한다) 안에 머무는 테스트가 통과한다.

9. 같은 cwd 디렉토리 안에 여러 transcript 파일이 있을 때(각 세션 하나씩), stdin이 정상이면 `stdin.transcript_path`를 우선하고, 비어있으면 mtime 가장 최근의 파일을 선택한다. mtime 동률 또는 다중 작성 중인 경우의 안전 동작이 정의되어 있다. — Library 관찰: 다중 transcript 파일 시나리오 테스트가 정의된 선택 규칙대로 통과한다.

10. transcript 파일이 Claude Code에 의해 append-only로 동시 쓰여지는 중에 read해도 cc-usage가 panic·hang 없이 graceful하게 동작한다. partial line이 마지막에 보일 수 있음을 가정해 처리된다. — Library 관찰: append-while-read 시뮬레이션 테스트가 통과한다.

11. 정상 stdin이 도착했을 때의 처리는 transcript-기반 fallback 도입 이전(v0.3.7+)과 동일한 결과를 낸다 — 새 fallback 경로가 정상 경로의 출력을 변경하지 않는다. — Library 관찰: 정상 stdin 입력에 대한 기존 회귀 테스트가 모두 PASS 유지된다.

12. transcript 기반 복원이 발동하거나 발동하지 않는 결정의 근거(어떤 transcript 파일을 골랐는지, model을 어디서 채웠는지, 폴백 사유)가 `DEBUG=cc-usage` 환경에서 stderr 로그로 확인 가능하다. — CLI 관찰: 빈 stdin smoke를 디버그 모드로 실행하면 transcript 결정 로그가 한 줄 이상 기록된다.

13. 자동 회귀 테스트가 최소 다음 경로를 모두 커버한다: (a) cwd 식별 + transcript 존재 → full 복원, (b) cwd 식별 + transcript 디렉토리 부재 → graceful fallback, (c) cwd 식별 불가 → graceful fallback, (d) transcript 파일이 손상 / 마지막 assistant entry 부재 → graceful fallback, (e) §3에서 결정된 cost 정책 분기 각각. 모든 케이스에서 cross-cwd 노출 0회. — Library 관찰: `go test ./...`에서 각 경로가 명시적 테스트 케이스로 PASS.

14. 본 feature가 도입한 동작은 `safe-empty-stdin-fallback` SPEC의 §5.1~§5.11 조건과 v0.3.4 이후 도입된 회귀 테스트(shouldSuppressOutput의 noIdentity + rate-limit OR, restoreUsageFields의 Cost/Context 복원, cleanOldSessionStates의 glob, cleanOldCaches의 무조건 호출 등)에 회귀를 만들지 않는다. — Library 관찰: 기존 회귀 테스트가 변경 없이 PASS 유지.

15. fallback 도입 시 SemVer 정책(CLAUDE.md §버전 정책)에 따라 `Makefile`, `.claude-plugin/plugin.json`, `api.go`의 `userAgent` 세 곳이 동일한 새 버전으로 갱신되어 `/plugin` UI가 update를 감지할 수 있다. — CLI 관찰: `./dist/cc-usage --version`이 새 버전을 출력하고, 세 파일의 grep 결과가 일치한다.
