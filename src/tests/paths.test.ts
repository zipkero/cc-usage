import { describe, expect, test } from 'claude-code/testing'
import { baseName, compressHome, parseRemoteSlug, PATH_MAX, shrinkPath } from '../hooks/paths'

describe('compressHome', () => {
  test('홈 자체는 ~, 홈 아래는 ~/…로 줄인다', () => {
    expect(compressHome('/home/kero', ['/home/kero'])).toBe('~')
    expect(compressHome('/home/kero/work/cc-usage', ['/home/kero'])).toBe('~/work/cc-usage')
  })

  test('끝 구분자가 붙은 홈도 같은 홈이다', () => {
    expect(compressHome('/home/kero/', ['/home/kero'])).toBe('~')
    expect(compressHome('/home/kero/a', ['/home/kero/'])).toBe('~/a')
  })

  test('이름이 홈으로 시작할 뿐인 형제 디렉토리는 줄이지 않는다', () => {
    expect(compressHome('/home/kero2/a', ['/home/kero'])).toBe('/home/kero2/a')
  })

  test('홈 밖 경로는 그대로다', () => {
    expect(compressHome('/srv/app', ['/home/kero'])).toBe('/srv/app')
  })

  test('홈이 없으면 압축하지 않는다', () => {
    expect(compressHome('/home/kero/a', [])).toBe('/home/kero/a')
    expect(compressHome('C:\\Users\\kero\\a', [''])).toBe('C:\\Users\\kero\\a')
  })

  test('Windows 경로는 구분자가 섞이고 드라이브 대소문자가 달라도 줄인다', () => {
    expect(compressHome('C:\\Users\\kero\\GolandProjects\\cc-usage', ['C:\\Users\\kero'])).toBe(
      '~/GolandProjects/cc-usage',
    )
    expect(compressHome('c:/Users/kero/GolandProjects\\cc-usage', ['C:\\Users\\kero'])).toBe(
      '~/GolandProjects/cc-usage',
    )
    expect(compressHome('C:\\USERS\\Kero', ['c:/users/kero'])).toBe('~')
  })

  test('드라이브 없는 경로는 대소문자를 구분한다', () => {
    expect(compressHome('/Home/Kero/a', ['/home/kero'])).toBe('/Home/Kero/a')
  })

  test('앞의 홈이 맞지 않으면 다음 홈을 본다', () => {
    expect(compressHome('C:\\Users\\kero\\a', ['/c/Users/kero', 'C:\\Users\\kero'])).toBe('~/a')
  })
})

describe('shrinkPath', () => {
  const long = (n: number) => 'x'.repeat(n)

  test('50자 이하는 그대로다', () => {
    const p = '~/' + long(48)
    expect(p).toHaveLength(PATH_MAX)
    expect(shrinkPath(p, PATH_MAX)).toBe(p)
  })

  test('50자를 넘으면 <head>/…/<base>로 줄인다', () => {
    expect(shrinkPath('~/' + long(30) + '/' + long(20) + '/cc-usage', PATH_MAX)).toBe('~/…/cc-usage')
    expect(shrinkPath('/' + long(30) + '/' + long(20) + '/cc-usage', PATH_MAX)).toBe('/…/cc-usage')
    expect(shrinkPath('D:\\' + long(30) + '\\' + long(20) + '\\cc-usage', PATH_MAX)).toBe('D:\\…\\cc-usage')
  })

  test('줄여도 넘치면 base만 남긴다', () => {
    const base = long(PATH_MAX)
    expect(shrinkPath('~/a/' + base, PATH_MAX)).toBe(base)
  })

  test('가운데 조각이 없으면 base만 남긴다', () => {
    expect(shrinkPath('~/' + long(60), PATH_MAX)).toBe(long(60))
  })

  test('길이는 code point로 센다', () => {
    const p = '~/' + '프'.repeat(48)
    expect(shrinkPath(p, PATH_MAX)).toBe(p)
  })
})

describe('baseName', () => {
  test('마지막 경로 조각을 돌려준다', () => {
    expect(baseName('/work/cc-usage')).toBe('cc-usage')
    expect(baseName('C:\\Users\\kero\\cc-usage\\')).toBe('cc-usage')
  })
})

describe('parseRemoteSlug', () => {
  test('https remote에서 owner/name을 뽑는다', () => {
    expect(parseRemoteSlug('https://github.com/zipke/cc-usage.git')).toBe('zipke/cc-usage')
    expect(parseRemoteSlug('https://github.com/zipke/cc-usage')).toBe('zipke/cc-usage')
    expect(parseRemoteSlug('https://token@gitlab.example.com/group/sub/cc-usage.git')).toBe('sub/cc-usage')
  })

  test('scp 형식 remote에서 owner/name을 뽑는다', () => {
    expect(parseRemoteSlug('git@github.com:zipke/cc-usage.git')).toBe('zipke/cc-usage')
    expect(parseRemoteSlug('git@github.com:zipke/cc-usage')).toBe('zipke/cc-usage')
  })

  test('ssh:// remote에서 owner/name을 뽑는다', () => {
    expect(parseRemoteSlug('ssh://git@github.com/zipke/cc-usage.git')).toBe('zipke/cc-usage')
    expect(parseRemoteSlug('ssh://git@host.example.com:2222/zipke/cc-usage.git')).toBe('zipke/cc-usage')
  })

  test('owner/name을 뽑을 수 없으면 null이다', () => {
    expect(parseRemoteSlug('')).toBeNull()
    expect(parseRemoteSlug('https://github.com/cc-usage.git')).toBeNull()
    expect(parseRemoteSlug('git@github.com:cc-usage.git')).toBeNull()
    expect(parseRemoteSlug('file:///srv/git/cc-usage.git')).toBeNull()
    expect(parseRemoteSlug('C:\\repos\\cc-usage')).toBeNull()
    expect(parseRemoteSlug('/srv/git/cc-usage.git')).toBeNull()
  })
})
