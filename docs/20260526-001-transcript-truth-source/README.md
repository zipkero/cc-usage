# transcript-truth-source

## 요약
Claude Code의 transcript 파일을 cc-usage status line의 진실 소스로 받아들여, 빈/degraded stdin이 와도 model/context/projectInfo 위젯이 흔들리지 않고 유지되도록 한다. session-state 캐시의 5분 TTL에 의존하지 않는다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [x] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-05-26: SPEC 작성
- 2026-05-26: ANALYSIS 작성 + SPEC §3에 D1=(b)/D2=(ii) 결정 commit, §5.4 도입 출처 정정
- 2026-05-27: 인코딩 규칙 정정 — `/`·`.`만 → `/`,`\`,`:`,`.` 네 구분자 치환(Windows backslash·드라이브 콜론 포함). SPEC §3 + analysis 실측/인터페이스 전파
- 2026-05-27: IMPLEMENT 체크리스트 작성
- 2026-05-27: task-001~015 구현·verify 완료 (전 Task approved, v0.3.11 bump)
- 2026-05-27: v0.3.11 회귀 수정 — transcript root에 CLAUDE_CONFIG_DIR 반영(task-016, v0.3.12). `.claude` 고정으로 config 홈 이동 환경(.claude-triptopaz)의 워크스페이스 transcript를 놓쳐 rate-limit only로 떨어지던 문제 해결
