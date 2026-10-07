import { describe, expect, test } from 'claude-code/testing'
import { resolveSettings } from '../hooks/settings'
import { THEMES } from '../hooks/themes'
import { type Part, renderWidget, type Snapshot } from '../hooks/widgets'

// 컨텍스트 막대 폭 (SPEC §5.7): 1–40 정수만 받고 그 밖은 8칸이다

function snap(percent: number | undefined): Snapshot {
  return {
    usage: {
      context: { tokens: 80_000, window: 200_000, ...(percent !== undefined && { percent }) },
      rateLimits: [],
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
}

const count = (parts: Part[], ch: string) => parts.reduce((n, p) => n + [...p.text].filter((c) => c === ch).length, 0)

// 막대 조각만 — 퍼센트 앞 공백 조각 전까지다
function bar(percent: number | undefined, contextBarWidth: unknown): { filled: number; empty: number } {
  const parts = renderWidget('context', snap(percent), resolveSettings({ contextBarWidth } as never))!
  const cells = parts.slice(0, parts.findIndex((p) => p.text === ' '))
  return { filled: count(cells, '█'), empty: count(cells, '░') }
}

describe('resolveSettings contextBarWidth', () => {
  test('1–40 정수는 그대로 쓴다', () => {
    for (const w of [1, 8, 23, 40]) expect(resolveSettings({ contextBarWidth: w }).contextBarWidth).toBe(w)
  })

  test('설정이 없거나 범위 밖·정수 아님·숫자 아님이면 8이다', () => {
    expect(resolveSettings({}).contextBarWidth).toBe(8)
    for (const w of [0, 41, -1, 2.5, 40.5, Number.NaN, Number.POSITIVE_INFINITY, '12', true]) {
      expect(resolveSettings({ contextBarWidth: w } as never).contextBarWidth).toBe(8)
    }
  })

  test('폭이 틀려도 다른 항목은 기본값으로 돌아가지 않는다', () => {
    for (const w of [0, 41, -1, 2.5]) {
      const s = resolveSettings({
        layout: 'M|C',
        disabledWidgets: '$',
        theme: 'nord',
        separator: 'dot',
        language: 'ko',
        contextBarWidth: w,
      })
      expect(s.contextBarWidth).toBe(8)
      expect(s.lines).toEqual([['model'], ['context']])
      expect([...s.disabled]).toEqual(['cost'])
      expect(s.theme).toBe('nord')
      expect(s.separator).toBe('dot')
      expect(s.language).toBe('ko')
    }
  })

  test('다른 항목이 틀려도 폭은 그대로다', () => {
    expect(resolveSettings({ theme: 'neon', separator: 'slash', layout: '', contextBarWidth: 12 }).contextBarWidth).toBe(12)
  })
})

describe('컨텍스트 막대 칸 수', () => {
  test('폭 1, 8, 40에서 █와 ░ 칸 수 합이 그 폭이다', () => {
    for (const w of [1, 8, 40]) {
      for (const pct of [0, 1, 40, 50, 99, 100]) {
        const { filled, empty } = bar(pct, w)
        expect(filled + empty).toBe(w)
      }
    }
  })

  test('폭 0, 41, -1, 2.5에서는 8칸이다', () => {
    for (const w of [0, 41, -1, 2.5]) {
      const { filled, empty } = bar(40, w)
      expect(filled + empty).toBe(8)
      expect(filled).toBe(3)
    }
  })

  test('채운 칸은 round(percent/100*폭)을 0..폭으로 자른 값이다', () => {
    const CASES: [number, number, number][] = [
      // [폭, 퍼센트, 채운 칸]
      [1, 0, 0],
      [1, 49, 0],
      [1, 50, 1],
      [1, 100, 1],
      [3, 50, 2],
      [8, 6, 0],
      [8, 7, 1],
      [10, 44, 4],
      [10, 45, 5],
      [40, 1, 0],
      [40, 2, 1],
      [40, 99, 40],
      [40, 100, 40],
      // 100을 넘거나 0보다 작아도 0..폭으로 자른다
      [8, 130, 8],
      [8, -5, 0],
    ]
    for (const [w, pct, filled] of CASES) {
      expect(bar(pct, w)).toEqual({ filled, empty: w - filled })
    }
  })

  test('폭을 바꿔도 막대 뒤 퍼센트·토큰 조각은 그대로다', () => {
    const tail = (w: number) => {
      const parts = renderWidget('context', snap(40), resolveSettings({ contextBarWidth: w }))!
      return parts.slice(parts.findIndex((p) => p.text === ' '))
    }
    expect(tail(1)).toEqual(tail(8))
    expect(tail(40)).toEqual([{ text: ' ' }, { text: '40%', ...THEMES.default.safe }, { text: ' 80K' }])
  })

  test('퍼센트가 아직 없으면 그 폭의 빈 막대와 흐린 -다', () => {
    for (const w of [1, 8, 40]) {
      expect(renderWidget('context', snap(undefined), resolveSettings({ contextBarWidth: w }))).toEqual([
        { text: '░'.repeat(w), ...THEMES.default.barEmpty },
        { text: ' ' },
        { text: '-', ...THEMES.default.secondary, dimColor: true },
      ])
    }
  })
})
