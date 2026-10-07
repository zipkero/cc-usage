// 프로젝트 칸의 경로·이름과 repoInfo의 owner/name 문자열을 만든다.
// $·런타임 import 없는 순수 계층이라 테스트가 직접 import한다.

// Go 판 pathDisplayMaxRunes와 같은 projectInfo 경로 길이 상한
export const PATH_MAX = 50

const SEPARATORS = /[\\/]/
const DRIVE = /^[A-Za-z]:$/

// 루트 `/`와 `C:/`의 끝 구분자는 남긴다 — 떼면 홈 접두사 비교가 어긋난다
function normalize(path: string): string {
  const p = path.replace(/\\/g, '/')
  const trimmed = p.replace(/\/+$/, '')
  if (trimmed === '') return p ? '/' : ''
  return DRIVE.test(trimmed) ? trimmed + '/' : trimmed
}

// homes 중 처음 맞는 홈을 `~`로 바꾼다. 맞는 홈이 없으면 path를 그대로 돌려준다.
// 드라이브 문자가 있는 경로는 Windows 경로로 보고 대소문자를 무시한다.
export function compressHome(path: string, homes: readonly string[]): string {
  const p = normalize(path)
  for (const raw of homes) {
    if (!raw) continue
    const h = normalize(raw)
    const fold = /^[A-Za-z]:/.test(p) && /^[A-Za-z]:/.test(h)
    const same = (a: string, b: string) => (fold ? a.toLowerCase() === b.toLowerCase() : a === b)
    if (same(p, h)) return '~'
    const prefix = h.endsWith('/') ? h : h + '/'
    if (p.length > prefix.length && same(p.slice(0, prefix.length), prefix)) {
      return '~/' + p.slice(prefix.length)
    }
  }
  return path
}

// max자(code point)를 넘으면 `<head>/…/<base>`로, 그래도 넘으면 base만 남긴다.
// head는 `~`, 루트 `/`, 드라이브 문자 중 path가 가진 것이다.
export function shrinkPath(path: string, max: number): string {
  if ([...path].length <= max) return path
  const sep = path.match(SEPARATORS)?.[0] ?? '/'
  const segs = path.split(SEPARATORS)
  let prefix = ''
  const first = segs[0] ?? ''
  if (first === '~' || DRIVE.test(first)) {
    prefix = first + sep
    segs.shift()
  } else if (first === '') {
    prefix = sep
  }
  const rest = segs.filter(Boolean)
  const base = rest[rest.length - 1] ?? path
  if (rest.length < 2) return base
  const candidate = prefix + '…' + sep + base
  return [...candidate].length <= max ? candidate : base
}

// 마지막 경로 조각이다. 조각이 없으면(루트 등) path를 그대로 돌려준다.
export function baseName(path: string): string {
  return path.split(SEPARATORS).filter(Boolean).pop() ?? path
}

// origin remote URL에서 마지막 두 경로 조각을 owner/name으로 뽑는다.
// https://host/owner/name(.git), git@host:owner/name(.git), ssh://…/owner/name(.git)만 받고 나머지는 null이다.
export function parseRemoteSlug(url: string): string | null {
  const u = url.trim()
  let path: string | undefined
  const withScheme = u.match(/^[A-Za-z][A-Za-z0-9+.-]*:\/\/[^/]+\/(.+)$/)
  if (withScheme) {
    path = withScheme[1]
  } else if (!u.includes('://')) {
    // scp 형식: 호스트가 한 글자면 Windows 드라이브(C:\…)라 받지 않는다
    const scp = u.match(/^(?:[^@/\\]+@)?([^:/\\]{2,}):(.+)$/)
    path = scp?.[2]
  }
  if (!path) return null
  const segs = path.replace(/[?#].*$/, '').split('/').filter(Boolean)
  if (segs.length < 2) return null
  const owner = segs[segs.length - 2] ?? ''
  const name = (segs[segs.length - 1] ?? '').replace(/\.git$/, '')
  if (!owner || !name) return null
  return owner + '/' + name
}
