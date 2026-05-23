# v0-3-0-hardening — SPEC

## 1. 범위

cc-usage v0.2.0의 운영 안정성과 코드 정합성을 개선하기 위한 단일 릴리스(v0.3.0)의 변경 범위. 다음 다섯 영역에 한정한다.

- **외부 명령 실행 안전성**: macOS Keychain `security` 명령과 Anthropic Usage API 호출이 status line 렌더를 중단시키지 않도록 timeout 적용 및 단축.
- **Dead state · 미사용 필드 제거**: 단일-shot CLI 모델에서 동작하지 않는 in-process 캐시 변수, 캐시 파일에 남은 stale 필드, 광고만 하고 동작 없는 `Config` 필드, 사용처 없는 i18n 라벨.
- **배포 외연 확장**: `linux/arm64` 빌드 타깃 추가. `bin/run.sh`가 같은 호스트에서 올바른 바이너리를 선택.
- **CLI 운영 편의**: `--version` 플래그 노출.
- **사용자 설정 신뢰성**: `displayMode` / `separator` / `theme` / `language` enum 값에 대한 입력 검증과 stderr 경고.
- **회귀 방지**: 사용자 영향이 큰 preset 파싱, config merge, 새 분석 위젯의 edge case에 대한 단위 테스트 추가.

릴리스 절차(main 코드+bin/ 커밋 → release 브랜치 sync)는 기존 워크플로(CLAUDE.md "배포" 섹션)를 그대로 따른다.

## 2. 목표

- **사용자 관점의 즉시성**: Claude Code가 status line을 호출했을 때 외부 의존성(Keychain prompt, API stall)이 응답을 막지 않는다.
- **광고와 동작의 일치**: README / Config에 노출된 옵션은 실제로 동작하는 것만 남긴다. 동작하지 않는 옵션은 광고하지 않거나 옵션 자체를 제거한다.
- **외연 확대**: linux/arm64 환경(Apple Silicon Asahi, AWS Graviton, Raspberry Pi 등)에서도 plug-and-play로 동작.
- **운영 가시성**: 사용자가 `cc-usage.json`에 잘못된 값을 썼을 때 silent fail이 아니라 stderr로 즉시 인지할 수 있다.
- **변경 회귀 방지**: 다음 릴리스에서 preset 파싱 또는 config 동작이 깨졌을 때 `go test ./...`가 감지한다.

## 3. 제약

- Zero dependency. Go 표준 라이브러리만 사용한다. `go.mod`에 `require` 블록이 생기지 않는다.
- 단일 `main` 패키지 유지. 서브 패키지 신설 금지.
- stdout은 위젯 렌더 결과 + ANSI만 출력하고, 그 외 모든 출력은 stderr.
- **v0.2.0 후방 호환**: 기존 `cc-usage.json`을 그대로 사용한다. 제거되는 `dailyBudget` / `plan` 필드가 들어있어도 status line은 정상 렌더되어야 한다(silent ignore).
- 사용자 settings.json의 statusLine 경로 형식은 손대지 않는다(v0.2.0 hardening의 안정 경로 합의 유지).
- bin/ 빌드 산출물은 git에 커밋되어 release 브랜치로 전파 가능한 상태여야 한다.
- 새 외부 의존 라이브러리, transcript 파싱, 새 캐시 파일 형식은 도입하지 않는다.

## 4. 제외 범위

다음 항목은 v0.3.0 범위 밖이며 이번 작업에서 다루지 않는다. 필요하면 별도 spec으로 분리한다.

- **분석 위젯 신설**: `budget`, `todayCost`, `forecast`, `todoProgress`, `toolActivity`, `lastPrompt`, `configCounts`, `peakHours` 등 v0.2.0에서 의도적으로 보류된 항목.
- **transcript 파싱 인프라**: `~/.claude/projects/.../*.jsonl` 읽기 / 누적 캐시 시스템.
- **일별 누적 cost 캐시**: budget · todayCost를 가능케 하는 daily 캐시.
- **rate limit 위젯 표시 정책 통일**: 5h/7d의 `--` placeholder vs 7d-S의 silent skip 차이.
- **git status 결과 캐싱**: 대형 모노레포에서의 fork+exec 비용 절감.
- **debugLog 호출당 env lookup 최적화**: 시작 시 1회 캐시 등 마이크로 최적화.
- **cacheHit 색상 헬퍼 분리**: `getColorForPercent` inverse 옵션 또는 `getColorForBenefit` 신설.
- **`noIdentity` 출력 정책 변경**: cost / rate_limits만 있고 identity 없는 stdin에서의 동작 변경.
- **Config validation을 별도 `--validate` 서브커맨드로 분리하는 작업**: 이번엔 실행 경로에 inline으로 둔다.

## 5. 완료 조건

1. macOS Keychain `security` 명령이 응답하지 않는 상태(잠겨있거나 prompt 대기)에서 cc-usage를 실행해도 status line stdout 출력이 1초 이내에 완료된다. 첫 호출은 timeout으로 backoff를 트리거하고, 후속 호출은 file fallback으로 즉시 분기한다.

2. Anthropic Usage API endpoint가 응답을 보내지 않는 상황에서 cc-usage를 실행하면 stdout에 status line이 3초 이내에 출력된다. 출력에는 캐시된 rate limit 값이 있을 경우 그 값이, 없을 경우 placeholder가 포함된다.

3. `cc-usage --version` 실행 시 stdout에 ldflags로 주입된 버전 문자열(`0.3.0`)이 출력되고 exit code 0으로 종료한다. 다른 출력은 없다.

4. `make build` 실행 결과로 `bin/`에 darwin/{arm64,amd64}, linux/{amd64,arm64}, windows/amd64 — 총 5개 바이너리가 생성된다. `bin/run.sh`는 호스트가 `linux` + `arm64` (또는 `aarch64`)일 때 `bin/cc-usage-linux-arm64`를 선택한다.

5. 사용자가 cc-usage.json에 알 수 없는 `displayMode`, `separator`, `theme`, 또는 `language` 값을 지정하고 cc-usage를 실행하면, stderr에 어느 필드의 어떤 값이 무효인지 식별 가능한 한 줄 경고가 출력되고, stdout의 status line은 기본값으로 fallback해서 정상 렌더된다.

6. 위 enum 필드 네 가지 모두 유효한 값을 사용한 cc-usage.json으로 실행했을 때 stderr는 비어있다(DEBUG 환경변수 미설정 기준).

7. v0.2.0 시점의 cc-usage.json (특히 `dailyBudget`, `plan` 필드를 포함한 것)을 v0.3.0이 입력으로 받아 실행하면 status line이 정상 렌더되고 stderr에 해당 필드 관련 경고가 출력되지 않는다(silent ignore).

8. `go test ./...`가 다음 영역에 대한 새 테스트를 포함해 모두 PASS한다 — preset 문자열 파싱(`resolvePreset`), 기본값 머지를 포함한 `loadConfig`, 그리고 새 분석 위젯의 GetData가 누락된 cost/duration 입력에 대해 nil을 반환함을 확인. 새 테스트 케이스 수는 최소 3개.

9. v0.3.0의 cc-usage 소스에서 다음 식별자가 ripgrep으로 발견되지 않는다(테스트 데이터, 코멘트, 본 spec 문서는 제외) — `memCache`, `negativeCache`, `DailyBudget`, `Plan`(struct 필드 의미), `last_output`(저장 경로), 그리고 i18n에서 사용처 없는 라벨(`NoContext`, `OneM`, `SevenDAll`).

10. v0.3.0 push 이후 main은 5개 플랫폼 바이너리를 포함한 상태이고, release 브랜치는 main의 v0.3.0 변경분을 sync한 상태이며, GitHub default branch는 `release`를 유지한다.

11. Anthropic Usage API 응답의 `utilization` 값이 정수(`12`)뿐 아니라 부동소수점(`12.5`)으로 와도 cc-usage는 decode 실패 없이 rate limit 위젯에 0~100 사이로 클램핑된 정수 퍼센트를 렌더한다.
