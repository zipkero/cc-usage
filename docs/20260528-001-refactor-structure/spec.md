# refactor-structure

## 1. 범위
- 단일 `main` 패키지 안에서 파일 단위 책임 재분배.
- 대상은 다음 네 영역이다.
  1. `main.go`의 `main()` 함수 분해.
  2. `api.go`가 보유한 파일 캐시 I/O를 캐시 도메인 파일로 이전.
  3. `cache.go`가 보유한 범용 I/O·경로 유틸을 별도 파일로 분리.
  4. `widget.go`가 보유한 i18n 정의(`Translations`, `loadTranslations`, `detectLanguage`)
     를 별도 파일로 분리.
- 위 네 영역에 한해 기존 함수의 이동·분할·시그니처 정리를 허용한다.

## 2. 목표
- 각 파일이 단일 책임을 갖도록 정리해 신규 기여자가 흐름을 추적하는 비용을 줄인다.
- `main()`이 진입점·플래그·suppress·출력만 다루도록 분해해 비즈니스 흐름(세션 복원, 캐시
  저장)을 별도 단위로 노출한다.
- HTTP 호출과 파일 캐시 책임을 분리해 테스트 단위와 변경 영향 범위를 좁힌다.
- 위젯 오케스트레이션과 번역 로딩 책임을 분리해 `widget.go`를 위젯 시스템 본연에 집중시킨다.

## 3. 제약
- 단일 `main` 패키지를 유지한다. 서브 패키지를 만들지 않는다.
- 외부 의존성을 추가하지 않는다. `go.mod`에 `require` 블록이 생기면 안 된다.
- stdout / stderr / exit code는 동일 stdin 입력에 대해 리팩터 전후가 완전히 동일해야 한다.
- 기존 공개 동작(위젯 출력, 캐시 파일 포맷·경로, API 요청 헤더, 로그 형식)은 변경하지 않는다.
- 기존 테스트는 그대로 통과해야 한다. 이동된 함수가 외부에서 호출되는 위치는 import만 갱신하고
  테스트 코드 자체의 의도는 보존한다.
- 빌드는 `make build` / `make build-local`이 변경 없이 성공해야 한다.

## 4. 제외 범위
- 다음은 이번 feature 범위 밖이며 손대지 않는다.
  - `transcript.go`의 Layer 2 backfill 로직 분리.
  - `widgets_project.go`의 path 헬퍼(`compressHome`, `shrinkPath`)를 `format.go`로 이전.
  - `loadByWorkspaceCwd()`, `projectInfoWidget.GetData()`, `callAPI()` 등 함수 내부의
    세부 분해.
  - 캐시 TTL·세션 복원 동작·rate limit 표시 로직 변경.
  - 새 위젯·새 캐시·새 외부 통합 추가.
  - 성능 최적화, 알고리즘 변경.
  - 의존성 정리(strings/encoding 등 표준 라이브러리 사용 패턴 재검토 포함).
  - README.md·CLAUDE.md 문구 정비 (architecture 섹션의 파일 목록 갱신은 §5.7 참고).

## 5. 완료 조건
1. 동일한 stdin JSON 입력에 대해 리팩터 전/후 `./dist/cc-usage`가 동일한 stdout / stderr /
   exit code를 출력한다.
2. `go test ./...`가 모두 통과한다.
3. `go vet ./...`와 `go build ./...`가 오류 없이 완료된다.
4. `go.mod`에 `require` 블록이 존재하지 않으며, 패키지는 `main` 단일 패키지로 유지된다.
5. `main()` 함수는 진입점·플래그 처리·suppress 판정·최종 출력만 직접 담당하며, Layer 1
   세션 복원과 Layer 2 transcript backfill 흐름을 인라인으로 보유하지 않는다(별도 함수
   호출 한 번으로 위임된다).
6. `api.go`에는 파일 캐시 I/O 정의(`readFileCache`, `writeFileCache`, `cacheFilePath`,
   `cleanOldCaches`, `cacheEntry` 타입, `lastCleanup` 변수, `cleanOldCachesFn`
   indirection 포함)가 더 이상 존재하지 않는다. 해당 정의는 캐시 도메인 파일에 존재한다.
7. `cache.go`에는 세션 상태 도메인 외의 범용 I/O·경로 유틸(`atomicWriteFile`,
   `withCacheFileLock`, `normalizeCwd`, `detectCurrentCwd*`)이 더 이상 존재하지 않는다.
   해당 정의는 별도 파일에 존재한다.
8. `widget.go`에는 i18n 정의(`Translations` 타입, `loadTranslations`, `detectLanguage`,
   `locales` 임베드 포함)가 더 이상 존재하지 않는다. 해당 정의는 별도 파일에 존재한다.
9. 위 §5.6–§5.8 이전 후, 이동된 모든 식별자에 대해 `grep`이 새 파일에서 정의를, 기존 파일에서
   호출만을 보여준다(중복 정의 없음).
