import { describe, expect, test } from 'claude-code/testing'
import { parseWidgetTokens, resolveSettings } from '../hooks/settings'
import { buildLines, type Part, type Snapshot } from '../hooks/widgets'

const DEFAULT_LINES = [['projectName', 'model', 'context', 'cost', 'rateLimit5h', 'rateLimit7d', 'spendLimit']]

describe('parseWidgetTokens', () => {
  test('preset 문자를 줄별로 읽는다', () => {
    expect(parseWidgetTokens('PMC$|R7L')).toEqual([
      ['projectInfo', 'model', 'context', 'cost'],
      ['rateLimit5h', 'rateLimit7d', 'spendLimit'],
    ])
  })

  test('쉼표·공백 토큰의 위젯 ID를 읽는다', () => {
    expect(parseWidgetTokens('projectInfo, model | rateLimit5h rateLimit7d')).toEqual([
      ['projectInfo', 'model'],
      ['rateLimit5h', 'rateLimit7d'],
    ])
  })

  test('한 줄 안에서 문자와 ID를 섞어 쓴다', () => {
    expect(parseWidgetTokens('N model C')).toEqual([['projectName', 'model', 'context']])
  })

  test('preset 문자 9개를 모두 매핑한다', () => {
    expect(parseWidgetTokens('PNGMC$R7L')).toEqual([
      ['projectInfo', 'projectName', 'repoInfo', 'model', 'context', 'cost', 'rateLimit5h', 'rateLimit7d', 'spendLimit'],
    ])
  })

  test('모르는 문자, 버린 문자 f T E #, 금지 문자 S V a D B H F는 무시한다', () => {
    expect(parseWidgetTokens('MxfTE#SVaDBHFC?')).toEqual([['model', 'context']])
  })

  test('ID와 정확히 같지 않은 토큰은 문자로 읽는다', () => {
    // Model은 ID가 아니므로 M(model)만 남고 o d e l은 버려진다
    expect(parseWidgetTokens('Model')).toEqual([['model']])
  })

  test('위젯이 없는 줄은 버린다', () => {
    expect(parseWidgetTokens('M||  |xyz|C')).toEqual([['model'], ['context']])
  })

  test('빈 문자열과 위젯 없는 입력은 빈 목록이다', () => {
    expect(parseWidgetTokens('')).toEqual([])
    expect(parseWidgetTokens('fTE# | SVaDBHF')).toEqual([])
  })
})

describe('resolveSettings', () => {
  test('manifest 기본값이면 v0.6.2와 같은 한 줄 배치다', () => {
    const s = resolveSettings({ layout: 'NMC$R7L', disabledWidgets: '' })
    expect(s.lines).toEqual(DEFAULT_LINES)
    expect([...s.disabled]).toEqual([])
  })

  test('남은 줄이 없는 layout은 기본 배치로 돌아간다', () => {
    expect(resolveSettings({ layout: 'xyz|fT', disabledWidgets: '' }).lines).toEqual(DEFAULT_LINES)
    expect(resolveSettings({ layout: '', disabledWidgets: '' }).lines).toEqual(DEFAULT_LINES)
  })

  test('layout이 없거나 문자열이 아니면 기본 배치다', () => {
    expect(resolveSettings({}).lines).toEqual(DEFAULT_LINES)
    expect(resolveSettings({ layout: 3 }).lines).toEqual(DEFAULT_LINES)
  })

  test('disabledWidgets를 문자·ID 집합으로 읽는다', () => {
    const s = resolveSettings({ layout: 'NMC', disabledWidgets: '$, rateLimit7d | C' })
    expect([...s.disabled].sort()).toEqual(['context', 'cost', 'rateLimit7d'])
  })

  test('disabledWidgets가 틀려도 layout은 그대로다', () => {
    const s = resolveSettings({ layout: 'M|C', disabledWidgets: 42 })
    expect(s.lines).toEqual([['model'], ['context']])
    expect([...s.disabled]).toEqual([])
  })
})

const SNAP: Snapshot = {
  usage: {
    context: { tokens: 80_000, window: 200_000, percent: 40 },
    rateLimits: [
      { kind: 'five_hour', percentUsed: 12 },
      { kind: 'seven_day', percentUsed: 34 },
      { kind: 'spend_limit', percentUsed: 56 },
    ],
    cost: { usd: 1.23 },
  },
  model: 'claude-opus-5-5',
  root: '/work/cc-usage',
  branch: 'main',
  repoSlug: 'zipke/cc-usage',
  homes: ['/work'],
  language: 'en',
  now: 0,
}

const lineText = (line: Part[]) => line.map((p) => p.text).join('')

describe('buildLines', () => {
  test('기본 배치는 한 줄에 v0.6.2 순서로 그린다', () => {
    const lines = buildLines(SNAP, resolveSettings({ layout: 'NMC$R7L', disabledWidgets: '' }))
    expect(lines.map(lineText)).toEqual([
      'cc-usage (main) │ ◆ claude-opus-5-5 │ ███░░░░░ 40% 80K │ $1.23 │ 5h: 12% │ 7d: 34% │ spend: 56%',
    ])
  })

  test('여러 줄 배치는 줄별 순서대로 그린다', () => {
    const lines = buildLines(SNAP, resolveSettings({ layout: '$M|L7R', disabledWidgets: '' }))
    expect(lines.map(lineText)).toEqual(['$1.23 │ ◆ claude-opus-5-5', 'spend: 56% │ 7d: 34% │ 5h: 12%'])
  })

  test('끈 위젯은 layout에 있어도 빠진다', () => {
    const lines = buildLines(SNAP, resolveSettings({ layout: 'NMC$R7L', disabledWidgets: 'model, $ R' }))
    expect(lines.map(lineText)).toEqual(['cc-usage (main) │ ███░░░░░ 40% 80K │ 7d: 34% │ spend: 56%'])
  })

  test('끈 뒤 위젯이 남지 않은 줄은 버린다', () => {
    const lines = buildLines(SNAP, resolveSettings({ layout: 'M|R7|C', disabledWidgets: 'R7' }))
    expect(lines.map(lineText)).toEqual(['◆ claude-opus-5-5', '███░░░░░ 40% 80K'])
  })

  test('모든 위젯을 끄면 줄이 없다', () => {
    expect(buildLines(SNAP, resolveSettings({ layout: 'M|C', disabledWidgets: 'MC' }))).toEqual([])
  })

  test('구분자는 위젯 사이에만 들어간다', () => {
    const [line] = buildLines(SNAP, resolveSettings({ layout: 'M$', disabledWidgets: '' }))
    expect(line).toEqual([
      { text: '◆ claude-opus-5-5', color: '#87d7ff' },
      { text: ' ' },
      { text: '│', dimColor: true },
      { text: ' ' },
      { text: '$1.23', color: '#ffd787' },
    ])
  })
})

describe('separator', () => {
  // Go 판 renderSeparator와 같은 문자열 — 기호만 Dim이다
  const CASES: [string, string, Part[]][] = [
    ['pipe', ' │ ', [{ text: ' ' }, { text: '│', dimColor: true }, { text: ' ' }]],
    ['dot', ' · ', [{ text: ' ' }, { text: '·', dimColor: true }, { text: ' ' }]],
    ['arrow', ' › ', [{ text: ' ' }, { text: '›', dimColor: true }, { text: ' ' }]],
    ['space', '  ', [{ text: '  ' }]],
  ]

  for (const [name, text, parts] of CASES) {
    test(`${name}는 위젯 사이에만 ${JSON.stringify(text)}로 들어간다`, () => {
      const settings = resolveSettings({ layout: 'M$|N', separator: name })
      expect(settings.separator).toBe(name)
      const lines = buildLines(SNAP, settings)
      expect(lines.map(lineText)).toEqual(['◆ claude-opus-5-5' + text + '$1.23', 'cc-usage (main)'])
      const [first] = lines
      expect(first?.slice(1, 1 + parts.length)).toEqual(parts)
      // 줄 처음·끝에는 구분자 조각이 없다
      expect(first?.[0]?.text).toBe('◆ claude-opus-5-5')
      expect(first?.[first.length - 1]?.text).toBe('$1.23')
      expect(first).toHaveLength(2 + parts.length)
    })
  }

  test('설정이 없거나 4종 밖이면 pipe다', () => {
    expect(resolveSettings({}).separator).toBe('pipe')
    expect(resolveSettings({ separator: 'slash' }).separator).toBe('pipe')
    expect(resolveSettings({ separator: 1 }).separator).toBe('pipe')
  })

  test('separator가 틀려도 다른 항목은 그대로다', () => {
    const s = resolveSettings({ layout: 'M|C', theme: 'nord', separator: 'slash' })
    expect(s.theme).toBe('nord')
    expect(s.lines).toEqual([['model'], ['context']])
  })
})
