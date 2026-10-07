import { describe, expect, test } from 'claude-code/testing'
import { detectLanguage, formatTimeRemaining, type Language, resolveLanguage } from '../hooks/i18n'
import { resolveSettings } from '../hooks/settings'
import { buildLines, type Snapshot } from '../hooks/widgets'

const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR

describe('detectLanguage', () => {
  // LC_ALL, LC_MESSAGES, LANG 순서의 값
  const CASES: [(string | undefined)[], Language][] = [
    [[undefined, undefined, undefined], 'en'],
    [[], 'en'],
    [['', '', ''], 'en'],
    [['ko_KR.UTF-8', undefined, undefined], 'ko'],
    [[undefined, 'ko_KR.UTF-8', undefined], 'ko'],
    [[undefined, undefined, 'ko_KR.UTF-8'], 'ko'],
    [[undefined, undefined, 'ko'], 'ko'],
    [['en_US.UTF-8', undefined, 'ko_KR.UTF-8'], 'ko'],
    [['C.UTF-8', 'POSIX', 'en_US.UTF-8'], 'en'],
    // Go strings.HasPrefix처럼 앞부분만, 대소문자를 구분해 본다
    [[undefined, undefined, 'en_KO'], 'en'],
    [[undefined, undefined, 'KO_KR.UTF-8'], 'en'],
  ]
  for (const [values, lang] of CASES) {
    test(`${JSON.stringify(values)}이면 ${lang}다`, () => {
      expect(detectLanguage(values)).toBe(lang)
    })
  }
})

describe('resolveLanguage', () => {
  test('auto는 로캘로 판정한다', () => {
    expect(resolveLanguage('auto', [undefined, undefined, 'ko_KR.UTF-8'])).toBe('ko')
    expect(resolveLanguage('auto', [undefined, undefined, undefined])).toBe('en')
  })

  test('en·ko는 로캘과 상관없이 그대로다', () => {
    expect(resolveLanguage('en', ['ko_KR.UTF-8', 'ko', 'ko'])).toBe('en')
    expect(resolveLanguage('ko', [undefined, undefined, 'en_US.UTF-8'])).toBe('ko')
  })
})

describe('formatTimeRemaining', () => {
  const CASES: [number, string, string][] = [
    [DAY + 2 * HOUR, '1d 2h', '1일 2시간'],
    [DAY + 2 * HOUR + 59 * MIN, '1d 2h', '1일 2시간'],
    [DAY, '1d 0h', '1일 0시간'],
    [DAY - MIN, '23h59m', '23시간59분'],
    [3 * HOUR + 4 * MIN, '3h4m', '3시간4분'],
    [HOUR, '1h0m', '1시간0분'],
    [HOUR - 1, '59m', '59분'],
    [5 * MIN, '5m', '5분'],
    [5 * MIN + 59_999, '5m', '5분'],
    [MIN, '1m', '1분'],
    // 1분 미만, 리셋 시각 정각, 지난 시각, 읽을 수 없는 시각은 생략한다
    [MIN - 1, '', ''],
    [0, '', ''],
    [-5 * MIN, '', ''],
    [Number.NaN, '', ''],
  ]
  for (const [ms, en, ko] of CASES) {
    test(`${ms}ms는 en ${JSON.stringify(en)}, ko ${JSON.stringify(ko)}다`, () => {
      expect(formatTimeRemaining(ms, 'en')).toBe(en)
      expect(formatTimeRemaining(ms, 'ko')).toBe(ko)
    })
  }
})

describe('resolveSettings language', () => {
  test('auto·en·ko는 그대로 쓴다', () => {
    for (const language of ['auto', 'en', 'ko']) expect(resolveSettings({ language }).language).toBe(language)
  })

  test('설정이 없거나 선택지 밖이면 auto다', () => {
    expect(resolveSettings({}).language).toBe('auto')
    expect(resolveSettings({ language: 'ja' }).language).toBe('auto')
    expect(resolveSettings({ language: 'KO' }).language).toBe('auto')
    expect(resolveSettings({ language: 1 }).language).toBe('auto')
  })

  test('language가 틀려도 다른 항목은 그대로다', () => {
    const s = resolveSettings({ layout: 'M|C', theme: 'nord', separator: 'dot', language: 'ja' })
    expect(s.theme).toBe('nord')
    expect(s.separator).toBe('dot')
    expect(s.lines).toEqual([['model'], ['context']])
  })
})

describe('rate limit 위젯 언어', () => {
  const NOW = 1_000_000_000_000
  const at = (ms: number) => new Date(NOW + ms).toISOString()

  function snap(language: Language): Snapshot {
    return {
      usage: {
        context: { tokens: 80_000, window: 200_000, percent: 40 },
        rateLimits: [
          { kind: 'five_hour', percentUsed: 12, resetsAt: at(3 * HOUR + 4 * MIN) },
          { kind: 'seven_day', percentUsed: 34, resetsAt: at(DAY + 2 * HOUR) },
          { kind: 'spend_limit', percentUsed: 56, resetsAt: at(30_000) },
        ],
        cost: { usd: 1.23 },
      },
      model: 'claude-opus-5-5',
      root: '/work/cc-usage',
      branch: 'main',
      repoSlug: null,
      homes: [],
      language,
      now: NOW,
    }
  }

  const lineText = (lang: Language) =>
    buildLines(snap(lang), resolveSettings({ layout: 'R7L' })).map((line) => line.map((p) => p.text).join(''))

  test('ko는 라벨이 5시간·7일이고 남은 시간이 한국어 단위다', () => {
    expect(lineText('ko')).toEqual(['5시간: 12% (3시간4분) │ 7일: 34% (1일 2시간) │ spend: 56%'])
  })

  test('en은 라벨이 5h·7d이고 남은 시간이 d·h·m 단위다', () => {
    expect(lineText('en')).toEqual(['5h: 12% (3h4m) │ 7d: 34% (1d 2h) │ spend: 56%'])
  })
})
