<!-- prowl-workflow: v1 -->
# mod-feature-parity

## 요약
TS mod(v0.6.2)에 Go 판에서 빠진 설정·테마·구분자·다국어·폭 맞춤·프로젝트 위젯을 옮긴다.
설정은 플러그인 `userConfig`(`/config`)로 받으며, mods API로 값을 얻을 수 없는 위젯은 버린다.

## 상태
- [x] SPEC
- [x] DESIGN
- [ ] IMPLEMENT

## 문서
- [spec.md](./spec.md)
- [design.md](./design.md) (DESIGN 단계에서 생성)
- [implement.md](./implement.md) (IMPLEMENT 단계에서 생성)

## 작업 히스토리
- 2026-10-07: SPEC 작성
- 2026-10-07: SPEC §3에 pluginConfigs 손 편집 타입 오류 예외 추가
- 2026-10-07: DESIGN 작성
- 2026-10-07: IMPLEMENT 체크리스트 작성
