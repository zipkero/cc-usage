import { describe, expect, test } from 'claude-code/testing'
import { resolveSettings } from '../hooks/settings'
import { buildLines, type Part, SEPARATORS, type Snapshot } from '../hooks/widgets'
import { displayWidth, fitLine } from '../hooks/width'

describe('displayWidth', () => {
  const WIDE: [string, string][] = [
    ['한글 음절', '가'],
    ['한글 음절 끝', '힣'],
    ['한글 초성 자모', 'ᄀ'],
    ['한글 호환 자모', 'ㄱ'],
    ['CJK 한자', '漢'],
    ['CJK 확장 A', '㐀'],
    ['CJK 구두점', '。'],
    ['히라가나', 'あ'],
    ['가타카나', 'ア'],
    ['전각 라틴', 'Ａ'],
    ['전각 기호', '￥'],
    ['emoji', '😀'],
    ['교통 emoji', '🚀'],
    ['그림 기호 emoji', '🌟'],
    ['CJK 확장 B', '\u{20000}'],
  ]

  for (const [label, ch] of WIDE) {
    test(`${label} ${JSON.stringify(ch)}는 2칸이다`, () => {
      expect(displayWidth(ch)).toBe(2)
    })
  }

  test('범위 바로 밖의 문자는 1칸이다', () => {
    // 한글 초성 자모 끝 다음, 한글 음절 끝 다음, 전각 형태 끝 다음(반각 구두점)
    expect(displayWidth('ᅠ')).toBe(1)
    expect(displayWidth('힤')).toBe(1)
    expect(displayWidth('｡')).toBe(1)
  })

  test('band가 쓰는 Ambiguous 기호 ◆ █ ░ │ › · …와 ASCII는 1칸이다', () => {
    expect(displayWidth('◆█░│›·…')).toBe(7)
    expect(displayWidth('◇○●◈◎')).toBe(5)
    expect(displayWidth('abc $1.23')).toBe(9)
  })

  test('제어문자는 0칸이다', () => {
    expect(displayWidth('\x00\x07\x1b\x1f\x7f')).toBe(0)
    expect(displayWidth('a\tb')).toBe(2)
  })

  test('한글이 섞인 라벨은 문자 수가 아니라 표시폭으로 잰다', () => {
    expect(displayWidth('5시간: 12%')).toBe(10)
    expect('5시간: 12%'.length).toBe(8)
  })
})

const PIPE = SEPARATORS.pipe
const lineText = (line: Part[]) => line.map((p) => p.text).join('')

describe('fitLine', () => {
  // 4 + 3 + 4 + 3 + 2 = 16칸
  const WIDGETS: Part[][] = [[{ text: 'aaaa', color: 'red' }], [{ text: 'bbbb' }], [{ text: 'cc' }]]

  test('예산 이하면 위젯과 구분자를 그대로 둔다', () => {
    expect(fitLine(WIDGETS, PIPE, 16)).toEqual([
      { text: 'aaaa', color: 'red' },
      ...PIPE,
      { text: 'bbbb' },
      ...PIPE,
      { text: 'cc' },
    ])
  })

  test('예산을 넘으면 오른쪽 위젯을 앞 구분자와 함께 뺀다', () => {
    const line = fitLine(WIDGETS, PIPE, 15)
    expect(line).toEqual([{ text: 'aaaa', color: 'red' }, ...PIPE, { text: 'bbbb' }])
    // 줄 끝에 구분자 조각이 남지 않는다
    expect(line[line.length - 1]).toEqual({ text: 'bbbb' })
  })

  test('남은 줄이 예산 이하가 되는 순간 멈춘다', () => {
    expect(lineText(fitLine(WIDGETS, PIPE, 11))).toBe('aaaa │ bbbb')
    expect(lineText(fitLine(WIDGETS, PIPE, 10))).toBe('aaaa')
    expect(lineText(fitLine(WIDGETS, PIPE, 4))).toBe('aaaa')
  })

  test('space 구분자도 위젯과 함께 빠진다', () => {
    expect(fitLine(WIDGETS, SEPARATORS.space, 11)).toEqual([
      { text: 'aaaa', color: 'red' },
      { text: '  ' },
      { text: 'bbbb' },
    ])
  })

  test('하나만 남아도 넘치면 예산-1칸과 …로 자른다', () => {
    expect(fitLine(WIDGETS, PIPE, 3)).toEqual([{ text: 'aa…', color: 'red' }])
    expect(fitLine([[{ text: 'aaaaaaaa' }], [{ text: 'bb' }]], PIPE, 5)).toEqual([{ text: 'aaaa…' }])
  })

  test('조각 경계에서 자르면 …는 잘린 조각의 꾸밈을 이어받고 뒤 조각은 버린다', () => {
    const widget: Part[] = [{ text: 'ab', color: 'red' }, { text: 'cd', dimColor: true }, { text: 'ef' }]
    expect(fitLine([widget], PIPE, 6)).toEqual(widget)
    expect(fitLine([widget], PIPE, 5)).toEqual([
      { text: 'ab', color: 'red' },
      { text: 'cd', dimColor: true },
      { text: '…' },
    ])
    expect(fitLine([widget], PIPE, 3)).toEqual([
      { text: 'ab', color: 'red' },
      { text: '…', dimColor: true },
    ])
  })

  test('2칸 문자는 반으로 자르지 않는다', () => {
    const widget: Part[] = [{ text: '가나다' }]
    // 남은 칸 3에 가(2)만 들어가고 나(2)는 넘친다
    expect(fitLine([widget], PIPE, 4)).toEqual([{ text: '가…' }])
    expect(fitLine([widget], PIPE, 5)).toEqual([{ text: '가나…' }])
    expect(displayWidth(lineText(fitLine([widget], PIPE, 4)))).toBe(3)
  })

  test('예산이 1이면 …만, 0 이하면 아무것도 남기지 않는다', () => {
    expect(fitLine(WIDGETS, PIPE, 1)).toEqual([{ text: '…', color: 'red' }])
    expect(fitLine(WIDGETS, PIPE, 0)).toEqual([])
    expect(fitLine(WIDGETS, PIPE, -1)).toEqual([])
  })

  test('자른 줄의 표시폭은 예산을 넘지 않는다', () => {
    const widget: Part[] = [{ text: '5시간: ' }, { text: '12%' }]
    for (let budget = 1; budget <= 10; budget++) {
      expect(displayWidth(lineText(fitLine([widget], PIPE, budget)))).toBeLessThanOrEqual(budget)
    }
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
  repoSlug: null,
  homes: [],
  language: 'en',
  now: 0,
}

describe('buildLines 폭 맞춤', () => {
  test('예산을 주지 않으면 맞추지 않는다', () => {
    expect(buildLines(SNAP, resolveSettings({ layout: 'NMC$R7L' })).map(lineText)).toEqual([
      'cc-usage (main) │ ◆ claude-opus-5-5 │ ███░░░░░ 40% 80K │ $1.23 │ 5h: 12% │ 7d: 34% │ spend: 56%',
    ])
  })

  test('여러 줄 배치는 줄마다 따로 맞춘다', () => {
    const lines = buildLines(SNAP, resolveSettings({ layout: 'NM|C$|R7L' }), 29)
    expect(lines.map(lineText)).toEqual(['cc-usage (main)', '███░░░░░ 40% 80K │ $1.23', '5h: 12% │ 7d: 34%'])
  })

  test('끈 위젯은 맞추기 전에 빠져 폭을 차지하지 않는다', () => {
    const lines = buildLines(SNAP, resolveSettings({ layout: 'NMC$', disabledWidgets: 'M' }), 39)
    expect(lines.map(lineText)).toEqual(['cc-usage (main) │ ███░░░░░ 40% 80K'])
  })

  test('한국어 라벨 줄은 한글을 2칸으로 재서 맞춘다', () => {
    const ko = { ...SNAP, language: 'ko' as const }
    const settings = resolveSettings({ layout: 'R7L' })
    // 34칸, 31자
    expect(buildLines(ko, settings, 34).map(lineText)).toEqual(['5시간: 12% │ 7일: 34% │ spend: 56%'])
    expect(buildLines(ko, settings, 33).map(lineText)).toEqual(['5시간: 12% │ 7일: 34%'])
    expect(buildLines(ko, settings, 4).map(lineText)).toEqual(['5시…'])
  })

  test('예산이 0 이하면 줄을 남기지 않는다', () => {
    expect(buildLines(SNAP, resolveSettings({ layout: 'N|M' }), 0)).toEqual([])
  })
})
