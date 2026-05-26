# transcript-truth-source

## 요약
Claude Code의 transcript 파일을 cc-usage status line의 진실 소스로 받아들여, 빈/degraded stdin이 와도 model/context/projectInfo 위젯이 흔들리지 않고 유지되도록 한다. session-state 캐시의 5분 TTL에 의존하지 않는다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [ ] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-05-26: SPEC 작성
- 2026-05-26: ANALYSIS 작성 + SPEC §3에 D1=(b)/D2=(ii) 결정 commit, §5.4 도입 출처 정정
