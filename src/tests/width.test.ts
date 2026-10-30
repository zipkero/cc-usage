import { describe, expect, test } from 'claude-code/testing'
import { resolveSettings } from '../hooks/settings'
import { buildLines, type Part, SEPARATORS, type Snapshot } from '../hooks/widgets'
import { displayWidth, wrapLine } from '../hooks/width'

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

describe('wrapLine', () => {
  // 4 + 3 + 4 + 3 + 2 = 16칸
  const WIDGETS: Part[][] = [[{ text: 'aaaa', color: 'red' }], [{ text: 'bbbb' }], [{ text: 'cc' }]]

  test('예산 이하면 한 줄에 위젯과 구분자를 그대로 둔다', () => {
    expect(wrapLine(WIDGETS, PIPE, 16)).toEqual([
      [{ text: 'aaaa', color: 'red' }, ...PIPE, { text: 'bbbb' }, ...PIPE, { text: 'cc' }],
    ])
  })

  test('예산을 넘는 위젯부터 다음 줄로 넘기고 구분자는 줄 끝·줄 앞에 남지 않는다', () => {
    expect(wrapLine(WIDGETS, PIPE, 15)).toEqual([
      [{ text: 'aaaa', color: 'red' }, ...PIPE, { text: 'bbbb' }],
      [{ text: 'cc' }],
    ])
    expect(wrapLine(WIDGETS, PIPE, 10).map(lineText)).toEqual(['aaaa', 'bbbb │ cc'])
    expect(wrapLine(WIDGETS, PIPE, 4).map(lineText)).toEqual(['aaaa', 'bbbb', 'cc'])
  })

  test('space 구분자도 같은 줄의 위젯 사이에만 들어간다', () => {
    expect(wrapLine(WIDGETS, SEPARATORS.space, 11)).toEqual([
      [{ text: 'aaaa', color: 'red' }, { text: '  ' }, { text: 'bbbb' }],
      [{ text: 'cc' }],
    ])
  })

  test('위젯 하나만으로 넘치면 그 위젯만 한 줄에 두고 가운데를 …로 줄인다', () => {
    expect(wrapLine([[{ text: 'aaaabbbb' }], [{ text: 'cc' }]], PIPE, 5).map(lineText)).toEqual(['aa…bb', 'cc'])
    // 예산-1이 홀수면 앞이 1칸 더 남는다
    expect(wrapLine([[{ text: 'abcdefgh' }]], PIPE, 6).map(lineText)).toEqual(['abc…gh'])
  })

  test('가운데를 줄여도 조각마다 꾸밈을 지키고 …는 앞부분이 끊긴 조각의 꾸밈을 따른다', () => {
    const widget: Part[] = [{ text: 'project', color: 'blue' }, { text: ' (branch)', color: 'green' }]
    expect(wrapLine([widget], PIPE, 16)).toEqual([widget])
    expect(wrapLine([widget], PIPE, 10)).toEqual([
      [{ text: 'proje', color: 'blue' }, { text: '…', color: 'blue' }, { text: 'nch)', color: 'green' }],
    ])
    // 앞부분이 조각 경계에서 끝나면 …는 다음 조각의 꾸밈을 따른다
    expect(wrapLine([widget], PIPE, 15)).toEqual([
      [{ text: 'project', color: 'blue' }, { text: '…', color: 'green' }, { text: 'branch)', color: 'green' }],
    ])
  })

  test('2칸 문자는 반으로 자르지 않고 앞에서 남은 칸은 뒤가 쓴다', () => {
    // 남은 칸 4: 앞 2칸에 가, 뒤 2칸에 라
    expect(wrapLine([[{ text: '가나다라' }]], PIPE, 5).map(lineText)).toEqual(['가…라'])
    // 남은 칸 5: 앞 3칸에 가(2)만 들어가 1칸이 뒤로 가고, 뒤 3칸에 라(2)
    expect(wrapLine([[{ text: '가나다라a' }]], PIPE, 6).map(lineText)).toEqual(['가…라a'])
  })

  test('예산이 1이면 …만, 0 이하면 아무 줄도 남기지 않는다', () => {
    expect(wrapLine([[{ text: 'aaaa', color: 'red' }]], PIPE, 1)).toEqual([[{ text: '…', color: 'red' }]])
    expect(wrapLine(WIDGETS, PIPE, 0)).toEqual([])
    expect(wrapLine(WIDGETS, PIPE, -1)).toEqual([])
  })

  test('나눈 줄과 줄인 줄의 표시폭은 예산을 넘지 않는다', () => {
    const widgets: Part[][] = [[{ text: '5시간: ' }, { text: '12%' }], [{ text: '가나다라마바사' }]]
    for (let budget = 1; budget <= 20; budget++) {
      for (const line of wrapLine(widgets, PIPE, budget)) {
        expect(displayWidth(lineText(line))).toBeLessThanOrEqual(budget)
      }
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

  test('여러 줄 배치는 줄마다 따로 맞추고 넘친 위젯은 그 줄 아래로 넘긴다', () => {
    const lines = buildLines(SNAP, resolveSettings({ layout: 'NM|C$|R7L' }), 29)
    expect(lines.map(lineText)).toEqual([
      'cc-usage (main)',
      '◆ claude-opus-5-5',
      '███░░░░░ 40% 80K │ $1.23',
      '5h: 12% │ 7d: 34%',
      'spend: 56%',
    ])
  })

  test('끈 위젯은 맞추기 전에 빠져 폭을 차지하지 않는다', () => {
    const lines = buildLines(SNAP, resolveSettings({ layout: 'NMC$', disabledWidgets: 'M' }), 39)
    expect(lines.map(lineText)).toEqual(['cc-usage (main) │ ███░░░░░ 40% 80K', '$1.23'])
  })

  test('한국어 라벨 줄은 한글을 2칸으로 재서 맞춘다', () => {
    const ko = { ...SNAP, language: 'ko' as const }
    const settings = resolveSettings({ layout: 'R7L' })
    // 34칸, 31자
    expect(buildLines(ko, settings, 34).map(lineText)).toEqual(['5시간: 12% │ 7일: 34% │ spend: 56%'])
    expect(buildLines(ko, settings, 33).map(lineText)).toEqual(['5시간: 12% │ 7일: 34%', 'spend: 56%'])
    // 예산 4칸이면 위젯마다 한 줄이고, 앞 2칸에 시(2)가 들어가지 않아 남은 1칸은 뒤가 쓴다
    expect(buildLines(ko, settings, 4).map(lineText)).toEqual(['5…2%', '7…4%', 'sp…%'])
  })

  test('예산이 0 이하면 줄을 남기지 않는다', () => {
    expect(buildLines(SNAP, resolveSettings({ layout: 'N|M' }), 0)).toEqual([])
  })
})
