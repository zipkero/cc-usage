# test-portability-and-ci — ANALYSIS

## 근거

spec.md 전체(§1 범위·입력 맥락, §2 목표, §3 제약, §4 제외 범위, §5 완료 조건 9개)를 읽었다. spec.md에는
승인 전 확인 섹션이 없고 미답 항목도 없다.

코드·실행으로 직접 확인한 사실이다.

1. **실패 재현.** 깨끗한 워킹 트리에서 `go test ./...`를 windows/amd64 · go1.26.5로 돌려 서브테스트
   4개가 실패함을 확인했다. got/want이 메커니즘 판별에 직접 쓰이므로 그대로 남긴다.

   ```
   main_test.go:88            got "C:\Users\zipke\.claude\cc-usage.json"      (want: TempDir 아래 경로)
   widgets_project_test.go:294 compressHome("/Users/alice/projects/cc-usage", "/Users/alice")
                               got "/Users/alice/projects/cc-usage"           want "~/projects/cc-usage"
   widgets_project_test.go:364 shrinkPath("~/aaaaaaaa/.../proj", 50)  got "proj"  want "~/…/proj"
   widgets_project_test.go:364 shrinkPath("/var/aaaaaaaa/.../proj", 50) got "proj" want "/…/proj"
   ```

2. **원인 1 — 구분자.** `filepath.Separator`는 이 환경에서 `\`다(별도 프로그램으로 확인).
   `compressHome`(widgets_project.go:179–191)은 `home + sep` 접두로 판정하므로 POSIX fixture는
   어느 분기에도 걸리지 않고 입력을 그대로 돌려준다. `shrinkPath`(같은 파일 198–232)는 `rest`를 `sep`로
   split하는데 결과가 1개라 `len(segs) < 2` 경로로 떨어지고, 거기서 `filepath.Base(s)`를 반환한다.
   Windows의 `filepath.Base`는 `/`도 구분자로 인정하므로(`Base("~/a/b/proj")` → `"proj"`) got이
   입력 그대로가 아니라 `"proj"`로 나온다. 두 함수의 got 차이가 여기서 갈린다.

3. **원인 2 — 홈 조회.** `os/file.go:605–624`의 `UserHomeDir`은 GOOS별로 **단 하나의** 환경변수를
   본다 — unix `HOME`, windows `USERPROFILE`, plan9 `home`. 실행으로도 확인했다: Windows에서 `HOME`만
   덮으면 반환값이 실제 홈 그대로이고, `USERPROFILE`을 덮으면 그 값이 나오며, `HOMEDRIVE`/`HOMEPATH`는
   전혀 관여하지 않는다. `USERPROFILE`이 빈 문자열이면 `%userprofile% is not defined` 오류가 되고,
   `defaultConfigPath`(main.go:34–40)는 그 경우 `""`를 반환한다.

4. **왜 이 4개만 실패하는가.** 다른 POSIX 리터럴 fixture는 `filepath.Base`·`filepath.Clean`·
   `strings.Contains`만 거친다. 이들은 Windows에서 `/`를 구분자로 받아주거나 구분자와 무관하다 —
   예: widgets_project_test.go:191의 `/Users/alice/projects/cc-usage`는 `filepath.Base`를 통해
   `cc-usage`로 잘 떨어져 통과한다. `filepath.Separator` 기반 접두 판정·split만 깨진다.

5. **저장소에 이미 있는 관례.** main_test.go:66·76·86은 기대값을 `filepath.Join`으로 만든다. 그래서
   `TestConfigHomeDir`의 다른 서브테스트 2개는 Windows에서도 이미 통과한다 — "기대값을 리터럴로 쓰지
   않고 `path/filepath`로 조립한다"는 방향이 이 파일에 선례로 존재한다.

6. **`filepath.FromSlash`의 성질.** `internal/filepathlite/path.go:184–189`에서 `Separator == '/'`면
   입력을 그대로 반환한다 — linux·darwin에서 항등이다. Windows에서는 `/`를 `\`로 바꾸는 바이트 치환이라
   rune 수와 세그먼트 수가 보존된다. 실행으로 확인: 세 장문 fixture의 rune 수가 변환 후에도 51·54·65로
   현재와 같고, 50 rune 예산을 전제로 손계산된 precondition이 그대로 성립한다.

7. **시험 패치 검증.** 스크래치패드에 모듈 사본을 만들어 fixture를 `FromSlash`/`Join`으로 중립화하고
   홈 override에 `USERPROFILE`을 추가한 결과, `go test ./...`가 `ok`, `go vet ./...`가 무출력으로
   통과했다(실패 0). 추가로 `GOOS=linux|darwin|windows`에 대해 `go vet ./...`와
   `go test -c`가 모두 성공해 세 OS에서 컴파일·vet이 깨지지 않음을 확인했다. 저장소 파일은 건드리지
   않았고 검증 후 다시 `go test ./...`를 돌려 실패 4개 baseline이 그대로임을 확인했다.

8. **테스트 헬퍼 관례.** 헬퍼는 필요한 파일 상단에 doc 주석과 함께 두고, `*testing.T`를 받으면
   `t.Helper()`를 부른다 — `stripANSI`(widgets_project_test.go:9), `captureStderr`·`writeTempConfig`
   (config_test.go:13·35), `newContextRenderCtx`·`splitContextRender`(widgets_core_test.go:11·22).
   여러 테스트 파일이 공유하는 헬퍼 파일은 없다.

9. **저장소·툴체인 사실.** `go.mod`는 `go 1.26.2`를 선언하고 `require` 블록이 없다. `go.sum`이 없다.
   `Makefile`의 `test` 타깃은 `go test ./...` 한 줄이고 `PLATFORMS`는 darwin/linux/windows를 덮지만
   CI가 쓰는 명령과는 무관하다. `.github/` 디렉토리가 없다. `Makefile` VERSION과
   `.claude-plugin/plugin.json` version은 둘 다 `0.5.5`다(SPEC §5.9의 기준선).

10. **줄바꿈.** `git ls-files --eol`은 `i/lf w/crlf`, `core.autocrlf`는 `true`, `.gitattributes`는
    없다. 그래서 이 작업 트리에서 `gofmt -l .`은 `.go` 파일 14개 전부를 뱉는다(`gofmt -d`로 확인한
    차이는 줄바꿈뿐). 반면 `go vet ./...`와 `go test ./...`는 CRLF 트리에서 정상 통과한다.

11. **git 의존.** `gitBranch`(widgets_project.go:53–66)는 `exec.LookPath("git")` 실패와 명령 실패를
    모두 빈 브랜치로 흡수한다. git을 건드리는 테스트의 판정이 러너에 git이 있는지에 매이지 않는다.

12. **외부 action 사실**(upstream README 확인). `actions/checkout`·`actions/setup-go`의 현재 상위
    major는 둘 다 `v7`이다. setup-go의 `cache` 기본값은 `true`이고, 캐시를 수행할 수 없으면 warning을
    남기고 계속한다. `go-version-file`은 `go.mod`를 입력으로 받는다.

추정으로 남는 것은 세 가지다. SPEC §5.8은 원격 실행을 관측해야 닫히므로 이 환경에서 확인할 수 없다.
setup-go가 `go 1.26.2` 선언을 정확히 어떤 패치 버전으로 해석하는지는 로컬에서 확인할 수 없다.
windows-latest 러너의 `core.autocrlf` 설정도 확인할 수 없다(다만 §10에서 두 검증 명령이 EOL에 무관함을
확인했으므로 판정에 영향이 없다).

## 1. 구조

새 제품 모듈·레이어가 없다. 테스트 fixture 층과 저장소 밖 실행 환경, 두 방향의 경계만 조정한다.

**경계 A — fixture 층 ↔ 제품 함수.** `compressHome`·`shrinkPath`는 "OS 고유 구분자로 표기된 경로
문자열"을 받는 순수 문자열 함수다. 이 계약은 지금까지 문서화되지 않았고 테스트가 POSIX 표기로 어겼다.
경계를 다시 그으면, **OS 중립 표기 → OS 고유 표기 변환의 소유자는 fixture 정의 자리**다. 변환은 입력과
기대값 양쪽에 같은 규칙으로 걸리며, 테스트 본문(호출·비교 로직)은 그대로 둔다. SPEC §5.2가 요구하는
"각 테스트가 통과한다"는 이 경계 재배치만으로 달성된다.

**경계 B — 테스트 프로세스 ↔ 프로세스 환경.** `defaultConfigPath`는 홈 위치를 인자가 아니라 프로세스
환경에서 읽는다. 테스트가 가진 개념은 "홈을 임시 디렉토리로 바꾼다" 하나인데, 그 개념을 표현하는 키가
GOOS마다 다르다. 이 GOOS 지식을 테스트 함수마다 흩뜨리지 않고 `main_test.go` 파일 수준의 한 자리가
소유한다. 여기가 §근거 3의 지식이 남는 유일한 지점이다.

**경계 C — 저장소 ↔ CI 실행 환경.** `.github/workflows/` 아래 워크플로 파일 하나가 전부다. 이 경계는
저장소 쪽에 새 스크립트나 `Makefile` 타깃을 요구하지 않고, 이미 있는 것(`go.mod`의 Go 버전 선언, 두
검증 명령)만 소비한다. SPEC §5.6·§5.7이 요구하는 게이트가 이 파일 하나에 들어간다.

세 경계 모두 기존 파일·기존 디렉토리 관례 안에서 끝난다. 공용 test helper 파일이나 CI용 스크립트 층을
새로 만들지 않는다.

## 2. 데이터 흐름

**흐름 1 — 경로 fixture 판정.** POSIX 리터럴 → `filepath.FromSlash`(또는 이미 중립화된 base에서
`filepath.Join`) → OS 고유 문자열 → `compressHome` → `shrinkPath` → 기대값 비교. 기대값도 같은 변환을
통과한다. OS 분기는 오직 `FromSlash` 내부의 `Separator == '/'` 판정 한 곳에서 일어난다.

- linux·darwin: 변환이 항등이라 fixture 바이트가 현재와 동일하다. 즉 오늘 통과하는 판정이 그대로
  유지되고, POSIX 쪽에서 새로 검증해야 할 동작이 생기지 않는다(§근거 6).
- windows: `/`가 `\`로 바뀌면서 `HasPrefix(home+sep)`와 `Split(sep)` 분기가 처음으로 성립한다.
  `compressHome`은 `~`+`\` 접두를 붙이고, `shrinkPath`는 세그먼트가 6개로 잡혀 `~\…\proj` /
  `\…\proj` 후보를 만들고 예산(50 rune) 안이라 그것을 반환한다.
- 현재의 실패 경로: 구분자 불일치 → `compressHome`은 입력을 그대로 통과시키고, `shrinkPath`는
  `len(segs) < 2` 분기로 떨어져 `filepath.Base` 결과만 남긴다. 이것이 §근거 1의 got이다.
- 예산 초과 base 케이스(`longBase`)는 변환 후에도 최소 축약형이 예산을 넘어 base 단독을 반환하는
  분기에 그대로 남는다 — 이 케이스는 변환 전후로 판정이 같다.

**흐름 2 — 홈 override 판정.** `t.Setenv`로 `HOME`과 `USERPROFILE`을 같은 임시 디렉토리로 설정 →
`defaultConfigPath()` → `os.UserHomeDir()`가 GOOS에 맞는 키 하나를 선택 → `configHomeDir(home)`이
`CLAUDE_CONFIG_DIR`를 우선 검사(공백만이면 미설정 취급) → `filepath.Join(..., "cc-usage.json")`.
기대값도 `filepath.Join`으로 만들어 구분자 선택을 표준 라이브러리에 맡긴다. 실패 경로는 하나뿐이다 —
선택된 키가 비어 있으면 `os.UserHomeDir()`가 오류를 돌려주고 `defaultConfigPath()`가 `""`가 된다.
두 키를 함께 덮으면 어느 GOOS에서도 이 경로에 들어가지 않는다. `CLAUDE_CONFIG_DIR`가 이기는
서브테스트에서도 `os.UserHomeDir()`는 여전히 먼저 호출되므로(main.go:35) 같은 override를 두는 편이
판정이 환경에 매이지 않는다.

**흐름 3 — CI 파이프라인.** `main` push 또는 `main`을 base로 하는 PR → 단일 job이 OS 3개 matrix로
전개 → 각 leg에서 checkout → setup-go(`go-version-file`로 `go.mod` 선언을 따르고 캐시는 끈다) →
`go vet ./...` → `go test ./...`. leg은 서로 독립이며(fail-fast 해제) vet이 깨진 leg에서도 test가
실행된다. **워크플로 자체에는 OS 분기가 없다** — 세 leg이 완전히 같은 step 목록과 같은 명령을 돌리고,
OS 차이는 전부 흐름 1의 `Separator` 판정, 즉 Go 표준 라이브러리 안에서 흡수된다. CI는 배포 산출물을
만들지 않으므로 `Makefile`의 크로스 빌드 경로와 만나지 않는다.

## 3. 인터페이스

**제품 코드와 테스트 사이의 계약은 바뀌지 않는다.** `compressHome(current, home string) string`,
`shrinkPath(s string, max int) string`, `configHomeDir(home string) string`,
`defaultConfigPath() string` 네 함수의 signature와 동작이 그대로다. 테스트는 호출 방식이 아니라
넘기는 값의 표기만 바꾼다. SPEC §5.4가 요구하는 "제품 코드 무변경"은 diff에 `config.go`·
`widgets_project.go`가 아예 등장하지 않는 것으로 확인된다. 위 함수들이 OS 고유 표기를 전제한다는
암묵 계약을 제품 코드 주석으로 명시하는 것도 하지 않는다 — 동작 변경은 아니지만 spec §3이 테스트
코드만 고치도록 정했고 SPEC §5.4를 diff로 판정하기 때문이다.

**환경 계약.** 홈을 덮는 테스트는 `HOME`과 `USERPROFILE`을 같은 값으로 함께 설정한다. 두 키는 각각
POSIX와 Windows에서 홈의 유일한 출처이며 서로 배타적이므로, 같은 값을 넣어두면 GOOS 분기 없이 정확하다.
plan9의 `home` 키는 이 저장소의 배포 대상(`Makefile` PLATFORMS)에 없어 다루지 않는다.

**CI 워크플로가 저장소에 요구하는 것.** (a) 모듈 루트에 유효한 `go` 지시를 가진 `go.mod`가 있어야
한다 — `go-version-file`의 입력이며 Go 버전의 단일 출처다. (b) 검증이 `go vet ./...`·`go test ./...`
두 명령으로 완결되어야 한다 — `make`나 셸 스크립트, 별도 셸 지정을 요구하지 않는다. (c) 트리거는
`main` push와 `main`을 base로 하는 PR 두 이벤트다. (d) 권한은 저장소 콘텐츠 읽기만 필요하다.

## 4. 영향 범위

- `main_test.go` — `TestConfigHomeDir`의 홈 override 서브테스트 2개, 그리고 그 override를 소유하는
  파일 수준 헬퍼.
- `widgets_project_test.go` — `TestProjectPathCompressHome`·`TestProjectPathShrink`의 fixture와
  기대값, `path/filepath` import 추가.
- `.github/workflows/` — 디렉토리와 워크플로 파일 신설.
- 이 feature 디렉토리의 `README.md`(상태·히스토리)와 `analysis.md`.

불변임을 탐색으로 확인한 것(흔적은 §근거 1·4·7·9). 네 대상 함수의 호출자를 전수 검색한 결과 제품
코드 쪽 호출자는 `widgets_project.go:113`과 `main.go:39`·`main.go:43`뿐이고 모두 이번 변경과 무관하다.
바꾸는 fixture는 전부 테스트 함수 로컬 값이라 파일 밖 참조가 없다. `config.go`·`widgets_project.go`·
`main.go`·`go.mod`·`Makefile`·`.claude-plugin/plugin.json`·`bin/`·`locales/`와 `release` 브랜치는
건드리지 않는다(SPEC §5.4·§5.5·§5.9). 루트 `README.md`·`CLAUDE.md`에 CI 존재를 알리는 문장을 넣는 것은
spec §1 범위 밖이라 하지 않는다.

하위 호환·마이그레이션: 해당 없음. 노출 동작, 설정 스키마, 바이너리 산출물이 모두 그대로다.

## 5. Decision Points

**A. fixture 중립화 수단.** 옵션은 (1) POSIX 리터럴을 `filepath.FromSlash`로 변환, (2) 세그먼트를
`filepath.Join`으로 조립, (3) OS별 기대값 테이블. **(1)을 기본으로 채택하고, 이미 중립화된 base
변수에서 파생되는 fixture에만 (2)를 쓴다**(`compressHome`의 home 하위 경로 케이스). 근거: `FromSlash`는
`Separator == '/'`일 때 입력을 그대로 반환함이 표준 라이브러리 소스에서 확인되므로(§근거 6) POSIX 동작이
바이트 단위로 불변임을 증명할 수 있고, rune 수·세그먼트 수가 보존되어 50 rune 예산을 손계산한
precondition을 다시 검토하지 않아도 된다. `Join`은 `Clean`을 거치므로 항등이 보장되지 않아 fixture마다
정규화 영향을 따로 따져야 하고, 장문 경로를 세그먼트 목록으로 쪼개면 "예산을 넘는 경로"라는 의도가
리터럴에서 안 읽힌다. 반대로 home 하위 경로는 `Join(home, ...)`이 base와 어긋날 수 없다는 이점이 있어
그 자리만 (2)를 쓴다. (3)은 B에서 기각한다.

**B. 기대값의 중립화와 `~` 표현.** `compressHome`은 `"~" + string(filepath.Separator)`로 결과를
조립하고(widgets_project.go:188) `shrinkPath`도 `head + sep`로 후보를 만든다(같은 파일 221–226).
따라서 `~` 접두 결과의 구분자는 OS마다 갈린다 — Windows에서는 `~\projects\cc-usage`, `~\…\proj`다.
**기대 문자열도 입력과 같은 변환을 통과시킨다.** 입력만 중립화하고 기대값을 POSIX 리터럴로 두면 판정이
여전히 깨진다는 점이 이 결정의 핵심이다. 반면 구분자가 없는 기대값(`~` 단독, base 단독)은 리터럴 그대로
둔다 — 불필요한 변환은 "여기에도 구분자가 있나"라는 오독을 만든다. 대안이었던 OS별 기대값 테이블
(`runtime.GOOS` 분기 또는 `_windows_test.go` 분리)은 기각한다: 같은 판정을 두 벌 유지해야 하고 두
리터럴이 갈라지면 한쪽만 검증되며, spec §3의 "OS별 분기를 테스트에 흩뿌리지 않는다"와 정면으로
어긋난다. 참고로 `…`는 비ASCII rune이지만 `FromSlash`는 `/` 바이트만 치환하므로 UTF-8이 깨지지 않고,
이는 §근거 7의 실행으로 확인됐다.

**C. 경로 중립화 코드의 배치.** 옵션은 (1) 각 테스트 함수의 fixture 정의 자리에 인라인, (2) 파일 수준
래퍼 헬퍼 신설, (3) 공용 test helper 파일 신설. **(1)을 채택한다.** 근거: 변환이 표준 라이브러리
호출 한 개라 래퍼는 이름만 늘리고 무엇이 일어나는지를 감춘다. 저장소의 헬퍼 관례(§근거 8)는 여러 단계
setup이나 파싱처럼 로직이 있을 때만 헬퍼를 만들고, `path/filepath`로 기대값을 조립하는 것은
main_test.go에 이미 인라인 선례가 있다(§근거 5). spec §3의 "fixture 생성 자리에서 흡수한다"와도 맞는다.
(3)은 공용 헬퍼 파일이 아예 없는 현재 구조에 새 층을 만든다.

**D. 홈 override의 키 집합과 배치.** 옵션은 (1) `HOME`·`USERPROFILE`을 둘 다 `t.Setenv`,
(2) `runtime.GOOS`로 분기해 하나만, (3) 제품 코드에 홈 조회 indirection을 넣어 테스트가 주입.
**(1)을 채택한다.** `os/file.go:605–624`에서 GOOS별 출처가 서로 배타적임을 확인했으므로 두 키를 같은
값으로 두면 어느 GOOS에서도 정확하고 분기가 생기지 않는다. `HOMEDRIVE`/`HOMEPATH`는 관여하지 않음을
실행으로 확인했고(§근거 3) plan9의 `home`은 배포 대상이 아니므로 둘 다 다루지 않는다. (2)는 분기
흩뿌리기 금지에 걸리고, (3)은 제품 코드 무변경(SPEC §5.4)에 걸린다.
배치는 C와 결론이 다르다 — **`main_test.go` 파일 수준 헬퍼로 둔다**(`t.Helper()` + doc 주석).
여기서 감추는 것은 호출 한 개가 아니라 "왜 키가 두 개인가"라는 비자명한 지식이고, 호출 지점이 2곳이며,
다음 사람이 `HOME`만 덮어 같은 실패로 돌아오는 것을 막는 것이 이 헬퍼의 목적이다. 이 헬퍼는
`CLAUDE_CONFIG_DIR`가 이기는 서브테스트에도 함께 쓴다 — 그 서브테스트는 지금 우연히 통과하지만
`os.UserHomeDir()`가 여전히 먼저 호출되므로(§2 흐름 2) override를 두는 편이 판정이 환경에 매이지 않는다.

**E. 통과 중인 POSIX fixture를 함께 손댈지.** 옵션은 (1) 실패 4개에 관련된 fixture만, (2) 두 테스트
파일의 POSIX 리터럴을 일괄 중립화. **(1)을 채택한다.** spec §1이 대상을 실패 4개로 한정하고 §4가
커버리지 확대를 제외한다. 통과 중인 리터럴은 `filepath.Base`·`Clean`·`strings.Contains`만 거쳐
구분자에 무관하다는 것을 확인했으므로(§근거 4) 일괄 변경은 diff를 넓히면서 판정을 하나도 바꾸지 않는다.

**F. CI job 구성.** 옵션은 (1) 단일 job + OS 3개 matrix, (2) OS별 job 3개. **(1)을 채택하고
`fail-fast`를 해제한다.** 세 leg이 완전히 같은 명령을 돌리므로(§2 흐름 3) job을 나누면 같은 step
목록을 3벌 유지해야 하고 step 하나를 고칠 때 세 곳을 건드려야 한다. `fail-fast` 기본값은 한 leg이
깨지면 나머지를 취소하는데, 그러면 "어느 OS에서 깨지는가"가 로그에서 사라진다 — 이번 실패가 정확히
그 유형이었으므로(spec §2) 해제 쪽이 이 게이트의 목적에 맞는다. 대가는 실패 시 러너 사용량이 늘어나는
것인데, 두 명령이 초 단위로 끝나는 규모라 무시할 수 있다.

**G. 외부 action 지정, Go 버전, 캐시.** `actions/checkout`·`actions/setup-go`를 major 태그로
고정한다(현재 상위 major는 둘 다 `v7` — §근거 12). commit SHA 고정은 기각한다: 이 저장소에 다른
워크플로가 없어 SHA 갱신을 지탱할 관례가 아직 없고, 둘 다 GitHub 공식 action이다.
Go 버전은 **`go-version-file: go.mod`**를 쓴다 — 리터럴을 적으면 버전 출처가 두 곳이 되어 spec §3이
요구하는 "`go.mod`의 선언을 따른다"가 시간이 지나면 깨진다.
캐시는 **`cache: false`를 명시한다.** 이 모듈은 `require` 블록이 없고 `go.sum`도 없어(§근거 9) 복원할
의존이 없고, setup-go는 캐시를 수행할 수 없으면 warning을 남기고 계속하므로(§근거 12) 기본값 true를
두면 leg 3개 × 매 실행마다 의미 없는 warning이 로그에 상시로 남는다.

**H. vet·test step 구성.** 옵션은 (1) 한 step에서 두 명령을 이어 실행, (2) 두 step으로 분리,
(3) 분리하고 test step을 vet 결과와 무관하게 실행. **(3)을 채택한다.** (1)은 로그에서 어느 명령이
깨졌는지가 step 이름으로 드러나지 않는다. (2)는 vet이 먼저 깨진 leg에서 test 결과를 잃는데, 그것은
F에서 `fail-fast`를 해제한 이유와 같은 종류의 마스킹이다. 순서는 vet → test로 두고(더 짧게 끝나는
쪽을 먼저), test step에 취소되지 않았을 때 실행하는 조건을 붙여 마스킹을 없앤다. 추가 비용은 조건
한 줄이다.

**I. 트리거 범위.** `main` push는 spec §3이 직접 정한다. PR은 spec이 base 브랜치를 말하지 않는데,
이 저장소의 GitHub default branch가 소스 없는 orphan `release`라서 PR 생성 시 base 기본값이 `release`가
된다. 옵션은 (1) `pull_request` 무필터, (2) base를 `main`으로 한정. **(2)를 채택한다.** 이 워크플로는
Go 모듈 루트를 전제하는데 `release`에는 소스가 없고, 릴리스 동기화는 PR이 아니라 직접 push로 하므로
(CLAUDE.md §릴리스 절차) `main` 한정이 실제 사용 경로를 모두 덮는다. 대가는 `release`를 base로 하는
PR에 게이트가 걸리지 않는 것인데, 그 브랜치에는 검증할 Go 소스가 없어 손실이 없다.

**J. Windows leg이 실제로 초록이 될 수 있는지.** 확인한 것: 같은 OS·같은 툴체인에서 시험 패치본이
`go test ./...`와 `go vet ./...`를 모두 통과했다(§근거 7). 그 통과가 **CRLF 작업 트리 상태에서**
나왔으므로 러너의 EOL 설정과 무관하며, `.gitattributes`를 새로 두지 않는다(§근거 10). windows-latest의
기본 셸은 `pwsh`지만 `./...`는 go 툴이 해석하는 패키지 패턴이라 셸 글롭 확장 대상이 아니고 두 명령이
셸 고유 문법을 쓰지 않으므로 `shell:`을 지정하지 않는다 — OS별 step을 만들지 않는다는 §2 흐름 3의
성질이 여기서 유지된다. git을 건드리는 테스트는 `LookPath` 실패와 명령 실패를 모두 빈 브랜치로 흡수하므로
(§근거 11) 러너의 git 유무와 무관하다. 남는 미확인은 원격 실행 관측 하나뿐이며 L에서 다룬다.

**K. 형식 검사를 CI에 넣지 않는다.** 이 작업 트리에서 `gofmt -l .`이 `.go` 파일 14개 전부를 뱉는데
차이가 줄바꿈뿐임을 확인했다(§근거 10). 형식 검사를 step으로 넣으면 결과가 러너의 EOL 설정에 따라
갈리고, 그것을 맞추려면 `.gitattributes` 신설이나 제품 파일 재기록이 필요해 spec §4(linter 추가 제외)와
§3(테스트 코드만 고친다)을 넘는다. CI 명령은 spec §3이 정한 두 개로 고정한다. 이 항목을 명시하는 이유는
"CI를 만드는 김에 포맷 검사도"가 자연스러운 확장으로 보이기 때문이다.

**L. SPEC §5.8 관측 경로.** 옵션은 (1) `main`에 직접 push해 push 트리거로 관측, (2) 브랜치를 올려 PR을
열고 pull_request 트리거로 관측한 뒤 머지, (3) `workflow_dispatch`를 추가해 수동 실행. **(1)을
채택한다.** 근거 — 이 저장소는 전 이력에서 모든 작업을 `main`에 직접 commit·push해 왔고 PR이 쓰인 적이
없다(`git log`로 확인). (2)는 이 관측 하나를 위해 없던 PR 워크플로를 들이면서도 두 트리거를 다 보려면
머지 후 push 실행까지 확인해야 해 절차가 길어진다. (3)은 spec §3이 정한 트리거 구성을 넓히고 관측
목적이면 (1)로 충분하다. 대가 — `pull_request` 트리거는 실제 실행이 아니라 워크플로 선언으로만 확인된다
(SPEC §5.7이 요구하는 것도 "동작하도록 선언되어 있다"이므로 완료 조건과 어긋나지 않는다).
push는 원격에 영향을 주므로 CLAUDE.md §사전 확인에 따라 실행 직전에 확인받는다.
