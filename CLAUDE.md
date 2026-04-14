# cc-usage — Project Instructions

Go 기반 Claude Code status line plugin. stdin JSON → ANSI 텍스트 변환.

## 문서 구조

| 파일 | 용도 | 언제 읽나 |
|------|------|-----------|
| `DESIGN.md` | 코어 시스템 스펙 (Phase 1 MVP) | Phase 1 구현 시 |
| `ROADMAP.md` | 확장 기능 스펙 (Phase 2~8) | 해당 Phase 구현 시 |
| `CHECKLIST.md` | 구현 순서 + 체크리스트 | 항상 — 현재 진행 상황 확인용 |

구현 시 CHECKLIST.md의 현재 Phase → 해당 소스 문서(DESIGN 또는 ROADMAP) 순서로 참조.

## 진행 방식

- CHECKLIST.md의 Step을 **한 번에 하나씩** 순서대로 진행.
- 각 Step 완료 후 반드시 **상세 설명**을 제공:
  - 생성/수정한 파일 목록
  - 주요 구현 내용과 설계 판단 근거
  - 스펙 대비 달라진 점
  - 검증 결과
- 완료된 체크리스트 항목은 `[x]`로 업데이트.

## 프로젝트 규칙

### 의존성
- **Zero dependency.** Go 표준 라이브러리만 사용. 외부 모듈 추가 금지.
- `go.mod`에 `require` 블록이 생기면 안 됨.

### 패키지 구조
- 단일 `main` 패키지. 서브 패키지 생성 금지.
- 파일 분리는 DESIGN.md "프로젝트 구조" 섹션을 따름.

### 출력 규칙
- **stdout**: 위젯 렌더링 결과만. 그 외 모든 출력은 stderr.
- **debugLog**: `DEBUG=cc-usage` 또는 `DEBUG=1` 환경변수로 활성화. stderr로만 출력.

### 위젯 구현 규칙
- Widget 인터페이스(`ID`, `GetData`, `Render`)를 구현.
- `GetData`에서 nil/error 반환 시 오케스트레이터가 자동 skip. 패닉 금지.
- 위젯 파일 배치: `widgets_core.go`(model, context, cost, rateLimit), `widgets_project.go`(projectInfo, sessionDuration 등), `widgets_analytics.go`(burnRate, cacheHit, toolActivity 등).

### 경로 규칙
- 설정: `--config` CLI 인자 또는 `~/.claude/cc-usage.json`
- 인증: `{configDir}/.credentials.json` (`configDir` = config 파일의 dirname)
- 캐시: `~/.cache/cc-usage/` (전역, configDir과 무관)

### 빌드
```bash
make build-local  # 로컬 바이너리
make build        # 크로스 컴파일 (darwin/arm64, darwin/amd64, linux/amd64, windows/amd64)
```

### 테스트
```bash
# 기본 동작 확인
echo '{"model":{"id":"claude-opus-4-6","display_name":"Opus"},"workspace":{"current_dir":"/tmp"},"context_window":{"total_input_tokens":50000,"total_output_tokens":10000,"context_window_size":200000,"current_usage":{"input_tokens":50000,"output_tokens":10000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"cost":{"total_cost_usd":1.25}}' | ./dist/cc-usage

# 기대 출력: ◆ Opus │ ████░░░░ 30% 60K │ $1.25
```

### TODO 마커
설계 문서 내 `> **TODO(...)**` 블록은 구현 시 판단이 필요한 항목. 해당 위젯 구현 전에 반드시 읽고 결정할 것.
