# test-portability-and-ci

## 요약

Windows에서만 실패하는 테스트 4개를 fixture 쪽에서 플랫폼 중립으로 고쳐 `go test ./...`가 실제로 통과하게
만들고, ubuntu·macOS·Windows 세 runner에서 test·vet을 돌리는 GitHub Actions 워크플로를 신설해 그 상태가
자동으로 지켜지게 한다. 제품 코드 동작은 바꾸지 않는다.

## 상태
- [x] SPEC
- [x] ANALYSIS
- [ ] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [analysis.md](./analysis.md) (ANALYSIS 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-07-30: SPEC 작성
- 2026-07-30: ANALYSIS 작성
- 2026-07-30: IMPLEMENT 체크리스트 작성
