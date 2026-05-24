# lock-leak-cleanup

## 요약
`~/.cache/cc-usage/*.lock` 파일이 release 흐름에서 `os.Remove`되지 않아 영구 누적되는 결함을 정리. 정상 흐름은 release 즉시 제거, 비정상 종료 leak은 백그라운드 cleanup 글로브 확장으로 회수. projectinfo-display(v0.3.2)와 같은 commit·release에 묶음 처리.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [x] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-05-24: SPEC 작성
- 2026-05-24: ANALYSIS 작성
- 2026-05-24: IMPLEMENT 체크리스트 작성
- 2026-05-24: IMPLEMENT 완료 (task-001~003 verify approved)
