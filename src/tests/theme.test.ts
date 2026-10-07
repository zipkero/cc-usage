import { describe, expect, test } from 'claude-code/testing'
import { resolveSettings } from '../hooks/settings'
import { percentRole, type Role, type Style, THEME_NAMES, THEMES, type ThemeName, tokenRole } from '../hooks/themes'
import { buildLines, type Part, renderWidget, type Snapshot } from '../hooks/widgets'

// deprecated-go render.go의 themes 표에서 SGR 코드를 그대로 옮겼다
const GO_THEMES: Record<ThemeName, Record<Role, string>> = {
  default: { model: '38;5;117', folder: '38;5;222', branch: '38;5;218', safe: '38;5;151', warning: '38;5;222', danger: '38;5;210', secondary: '38;5;249', accent: '38;5;222', barEmpty: '38;5;240' },
  minimal: { model: '37', folder: '37', branch: '37', safe: '90', warning: '37', danger: '1;37', secondary: '90', accent: '37', barEmpty: '90' },
  catppuccin: { model: '38;2;137;180;250', folder: '38;2;249;226;175', branch: '38;2;245;194;231', safe: '38;2;166;227;161', warning: '38;2;250;179;135', danger: '38;2;243;139;168', secondary: '38;2;127;132;156', accent: '38;2;249;226;175', barEmpty: '38;2;69;71;90' },
  dracula: { model: '38;2;189;147;249', folder: '38;2;255;184;108', branch: '38;2;255;121;198', safe: '38;2;80;250;123', warning: '38;2;241;250;140', danger: '38;2;255;85;85', secondary: '38;2;98;114;164', accent: '38;2;255;184;108', barEmpty: '38;2;68;71;90' },
  gruvbox: { model: '38;2;215;153;33', folder: '38;2;250;189;47', branch: '38;2;211;134;155', safe: '38;2;184;187;38', warning: '38;2;250;189;47', danger: '38;2;204;36;29', secondary: '38;2;168;153;132', accent: '38;2;250;189;47', barEmpty: '38;2;80;73;69' },
  nord: { model: '38;2;136;192;208', folder: '38;2;235;203;139', branch: '38;2;180;142;173', safe: '38;2;163;190;140', warning: '38;2;235;203;139', danger: '38;2;191;97;106', secondary: '38;2;76;86;106', accent: '38;2;235;203;139', barEmpty: '38;2;59;66;82' },
  tokyoNight: { model: '38;2;122;162;247', folder: '38;2;224;175;104', branch: '38;2;187;154;247', safe: '38;2;158;206;106', warning: '38;2;224;175;104', danger: '38;2;247;118;142', secondary: '38;2;86;95;137', accent: '38;2;224;175;104', barEmpty: '38;2;59;66;82' },
  solarized: { model: '38;2;38;139;210', folder: '38;2;181;137;0', branch: '38;2;211;54;130', safe: '38;2;133;153;0', warning: '38;2;181;137;0', danger: '38;2;220;50;47', secondary: '38;2;88;110;117', accent: '38;2;181;137;0', barEmpty: '38;2;88;110;117' },
}

const ROLES: Role[] = ['model', 'folder', 'branch', 'safe', 'warning', 'danger', 'secondary', 'accent', 'barEmpty']

const hex = (...rgb: number[]) => '#' + rgb.map((v) => v.toString(16).padStart(2, '0')).join('')

// xterm 256색: 16–231은 6단계 큐브, 232–255는 회색 24단계
function xterm256(n: number): string {
  if (n >= 232) return hex(8 + 10 * (n - 232), 8 + 10 * (n - 232), 8 + 10 * (n - 232))
  const step = (v: number) => (v === 0 ? 0 : 55 + 40 * v)
  const i = n - 16
  return hex(step(Math.floor(i / 36)), step(Math.floor(i / 6) % 6), step(i % 6))
}

// Go SGR 코드를 Text color 형태로 옮긴 기대값
function expectedStyle(sgr: string): Style {
  const p = sgr.split(';').map(Number)
  if (p[0] === 38 && p[1] === 5) return { color: xterm256(p[2]!) }
  if (p[0] === 38 && p[1] === 2) return { color: hex(p[2]!, p[3]!, p[4]!) }
  const basic: Record<string, Style> = { '37': { color: 'white' }, '90': { color: 'gray' }, '1;37': { color: 'white', bold: true } }
  return basic[sgr]!
}

describe('THEMES', () => {
  test('8종 테마가 모두 있다', () => {
    expect([...THEME_NAMES].sort()).toEqual(Object.keys(GO_THEMES).sort())
    expect(Object.keys(THEMES).sort()).toEqual(Object.keys(GO_THEMES).sort())
  })

  for (const name of THEME_NAMES) {
    test(`${name}의 역할 색이 Go 판 값과 같다`, () => {
      for (const role of ROLES) {
        expect({ role, ...THEMES[name][role] }).toEqual({ role, ...expectedStyle(GO_THEMES[name][role]) })
      }
    })
  }

  test('default는 xterm 256색 hex다', () => {
    expect(THEMES.default).toEqual({
      model: { color: '#87d7ff' },
      folder: { color: '#ffd787' },
      branch: { color: '#ffafd7' },
      safe: { color: '#afd7af' },
      warning: { color: '#ffd787' },
      danger: { color: '#ff8787' },
      secondary: { color: '#b2b2b2' },
      accent: { color: '#ffd787' },
      barEmpty: { color: '#585858' },
    })
  })
})

describe('percentRole', () => {
  test('50 이하 safe, 80 이하 warning, 그 초과 danger', () => {
    expect([0, 50, 51, 80, 81, 100, 130].map(percentRole)).toEqual([
      'safe',
      'safe',
      'warning',
      'warning',
      'danger',
      'danger',
      'danger',
    ])
  })
})

describe('tokenRole', () => {
  test('256K 이상 warning, 512K 이상 danger, 그 밖에는 secondary', () => {
    expect([255_999, 256_000, 511_999, 512_000].map(tokenRole)).toEqual(['secondary', 'warning', 'warning', 'danger'])
  })
})

describe('resolveSettings theme', () => {
  test('8종 이름은 그대로 쓴다', () => {
    for (const name of THEME_NAMES) expect(resolveSettings({ theme: name }).theme).toBe(name)
  })

  test('없거나 모르는 값이면 default이고 다른 항목은 그대로다', () => {
    expect(resolveSettings({}).theme).toBe('default')
    expect(resolveSettings({ theme: 'Dracula' }).theme).toBe('default')
    expect(resolveSettings({ theme: 3 }).theme).toBe('default')
    expect(resolveSettings({ theme: 'nope', layout: 'M|C' }).lines).toEqual([['model'], ['context']])
  })
})

function snap(percent: number | undefined, rate: number, tokens = 80_000): Snapshot {
  return {
    usage: {
      context: { tokens, window: 1_000_000, ...(percent !== undefined && { percent }) },
      rateLimits: [
        { kind: 'five_hour', percentUsed: rate, resetsAt: new Date(3 * 3_600_000 + 4 * 60_000).toISOString() },
        { kind: 'seven_day', percentUsed: rate },
        { kind: 'spend_limit', percentUsed: rate },
      ],
      cost: { usd: 1.23 },
    },
    model: 'claude-opus-5-5',
    root: '/work/cc-usage',
    branch: 'main',
    repoSlug: 'zipke/cc-usage',
    homes: [],
    language: 'en',
    now: 0,
  }
}

const settingsFor = (theme: ThemeName) => resolveSettings({ theme })
const styled = (theme: ThemeName, role: Role) => THEMES[theme][role]

describe('위젯 색', () => {
  for (const theme of THEME_NAMES) {
    test(`${theme}: 위젯마다 그 테마의 역할 색을 쓴다`, () => {
      const s = snap(40, 12)
      const st = settingsFor(theme)
      expect(renderWidget('projectName', s, st)).toEqual([
        { text: 'cc-usage', ...styled(theme, 'folder') },
        { text: ' (main)', ...styled(theme, 'branch') },
      ])
      expect(renderWidget('projectInfo', s, st)?.[0]).toEqual({ text: '/work/cc-usage', ...styled(theme, 'folder') })
      expect(renderWidget('repoInfo', s, st)).toEqual([{ text: 'zipke/cc-usage', ...styled(theme, 'secondary') }])
      expect(renderWidget('model', s, st)).toEqual([{ text: '◆ claude-opus-5-5', ...styled(theme, 'model') }])
      expect(renderWidget('cost', s, st)).toEqual([{ text: '$1.23', ...styled(theme, 'accent') }])
      expect(renderWidget('context', s, st)).toEqual([
        { text: '███', ...styled(theme, 'safe') },
        { text: '░░░░░', ...styled(theme, 'barEmpty') },
        { text: ' ' },
        { text: '40%', ...styled(theme, 'safe') },
        { text: ' 80K', ...styled(theme, 'secondary') },
      ])
      // Go 판에서 Dim이던 남은 시간 괄호는 색 없이 dimColor다
      expect(renderWidget('rateLimit5h', s, st)).toEqual([
        { text: '5h: ', ...styled(theme, 'secondary') },
        { text: '12%', ...styled(theme, 'safe') },
        { text: ' (3h4m)', dimColor: true },
      ])
      // 구분자는 기호만 Dim이다
      const [line] = buildLines(s, resolveSettings({ theme, layout: 'M$' }))
      expect(line?.slice(1, 4)).toEqual([{ text: ' ' }, { text: '│', dimColor: true }, { text: ' ' }])
    })
  }
})

describe('퍼센트 색 경계', () => {
  const RATE_WIDGETS = ['rateLimit5h', 'rateLimit7d', 'spendLimit'] as const
  const CASES: [number, Role][] = [
    [50, 'safe'],
    [51, 'warning'],
    [80, 'warning'],
    [81, 'danger'],
  ]

  for (const theme of ['default', 'dracula', 'minimal'] as const) {
    for (const [pct, role] of CASES) {
      test(`${theme}: ${pct}%는 ${role} 색이다`, () => {
        const s = snap(pct, pct)
        const st = settingsFor(theme)
        const ctx = renderWidget('context', s, st)!
        const percentPart = ctx.find((p) => p.text === pct + '%')
        const barFilled = ctx.find((p) => p.text.startsWith('█'))
        expect(percentPart).toEqual({ text: pct + '%', ...styled(theme, role) })
        expect(barFilled).toEqual({ text: barFilled!.text, ...styled(theme, role) })
        for (const id of RATE_WIDGETS) {
          expect(renderWidget(id, s, st)?.[1]).toEqual({ text: pct + '%', ...styled(theme, role) })
        }
      })
    }
  }

  test('rate limit 소수 퍼센트는 버린 정수로 판정한다', () => {
    expect(renderWidget('rateLimit5h', snap(0, 50.9), settingsFor('default'))?.[1]).toEqual({
      text: '50%',
      ...styled('default', 'safe'),
    })
  })

  test('막대가 다 차면 빈 칸 조각이 없고 비면 채운 칸 조각이 없다', () => {
    const full = renderWidget('context', snap(100, 0), settingsFor('nord'))!
    expect(full.slice(0, 2)).toEqual([{ text: '████████', ...styled('nord', 'danger') }, { text: ' ' }])
    const empty = renderWidget('context', snap(0, 0), settingsFor('nord'))!
    expect(empty.slice(0, 2)).toEqual([{ text: '░░░░░░░░', ...styled('nord', 'barEmpty') }, { text: ' ' }])
  })
})

describe('컨텍스트 토큰 색', () => {
  const tokenPart = (tokens: number, theme: ThemeName) =>
    renderWidget('context', snap(10, 0, tokens), settingsFor(theme))!.at(-1) as Part

  for (const theme of ['default', 'gruvbox'] as const) {
    test(`${theme}: 256K 미만 secondary, 256K 이상 warning, 512K 이상 danger`, () => {
      expect(tokenPart(255_999, theme)).toEqual({ text: ' 256K', ...styled(theme, 'secondary') })
      expect(tokenPart(256_000, theme)).toEqual({ text: ' 256K', ...styled(theme, 'warning') })
      expect(tokenPart(511_999, theme)).toEqual({ text: ' 512K', ...styled(theme, 'warning') })
      expect(tokenPart(512_000, theme)).toEqual({ text: ' 512K', ...styled(theme, 'danger') })
    })
  }
})

describe('placeholder', () => {
  test('퍼센트가 아직 없으면 빈 막대와 Secondary 색에 dimColor인 -다', () => {
    expect(renderWidget('context', snap(undefined, 0), settingsFor('dracula'))).toEqual([
      { text: '░░░░░░░░', ...styled('dracula', 'barEmpty') },
      { text: ' ' },
      { text: '-', ...styled('dracula', 'secondary'), dimColor: true },
    ])
  })

  test('빈 막대도 설정한 폭을 따른다', () => {
    expect(renderWidget('context', snap(undefined, 0), resolveSettings({ contextBarWidth: 3 }))?.[0]).toEqual({
      text: '░░░',
      ...styled('default', 'barEmpty'),
    })
  })
})
