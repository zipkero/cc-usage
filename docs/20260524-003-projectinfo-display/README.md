# projectinfo-display

## 요약
projectInfo를 base name only → home-tilde 압축 full path로 확장하고, orchestrate의 projectInfo 특수 분기를 제거해 preset 선언 위치가 곧 출력 위치가 되도록 한다. v0.3.2 patch 릴리스.

샘플 출력 (home 하위 + custom preset 위치 자유화):
```
~/GolandProjects/cc-usage (main ↑2) │ ◆ Opus │ ████░░ 30% 60K │ $1.25
```

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
- 2026-05-24: IMPLEMENT 완료 (task-001~004 verify approved)
