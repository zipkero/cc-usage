import type { On, PluginOptions, ProcessRunResult, RenderElement, SessionRepo, SessionUsage } from 'claude-code'
import { expect, mock, test, type Engine } from 'claude-code/testing'
import { THEME_NAMES, THEMES } from '../hooks/themes'

const PLUGIN = 'cc-usage-mod'
const ROOT = '/work/cc-usage'

const USAGE: SessionUsage = {
  startedAt: 0,
  context: { tokens: 80_000, window: 200_000, percent: 40 },
  rateLimits: [
    { kind: 'five_hour', percentUsed: 12 },
    { kind: 'seven_day', percentUsed: 34 },
    { kind: 'spend_limit', percentUsed: 56 },
  ],
  cost: { usd: 1.23 },
}

const BAND_PROPS = {
  hasSurvey: false,
  isWorking: false,
  maxRows: 20,
  bodyColumns: 200,
  scroll: { offset: 0, bodyRows: 20 },
  view: {},
}

type Stubs = {
  root?: string
  usage?: SessionUsage
  env?: Record<string, string>
  // null이면 process.run에 답하지 않아 호출이 거부된다
  git?: ProcessRunResult | null
  // undefined면 session.repo에 답하지 않아 호출이 거부된다
  repo?: SessionRepo | null
}

const GIT_MAIN = { exitCode: 0, stdout: 'main\n', stderr: '' } as ProcessRunResult

// 실제 세션 대신 플러그인이 부르는 $ 호출에 고정값으로 답하고, process.run·session.repo 호출을 기록한다.
// 돌려준 git·gitDelayMs를 바꾸면 그 뒤 process.run 응답이 바뀐다.
function stubSession(on: On, stubs: Stubs = {}) {
  const clock = mock.clock(on)
  const calls = {
    run: [] as { argv: readonly string[]; cwd: string | undefined }[],
    repo: 0,
    clock,
    git: stubs.git === undefined ? GIT_MAIN : stubs.git,
    // 0보다 크면 process.run이 mock clock으로 그만큼 지나야 답한다
    gitDelayMs: 0,
  }
  mock.env(on, stubs.env ?? {})
  on('session.usage', async () => ({ value: stubs.usage ?? USAGE }))
  on('session.model', async () => ({ value: 'claude-opus-5-5' }))
  on('session.root', async () => ({ value: stubs.root ?? ROOT }))
  on('process.run', async (_$, e, next) => {
    calls.run.push({ argv: e.argv, cwd: e.init?.cwd })
    const git = calls.git
    if (calls.gitDelayMs > 0) await clock.sleep(calls.gitDelayMs)
    return git ? { value: git } : next(e)
  })
  on('session.repo', async (_$, e, next) => {
    calls.repo++
    return stubs.repo !== undefined ? { value: stubs.repo } : next(e)
  })
  on('session.start', async (_$, e) => ({ cwd: e.cwd }))
  on('turn.complete', async (_$, e) => ({ text: e.answer }))
  return calls
}

// 턴 하나가 끝난다 — 플러그인의 갱신 경로 하나다
function completeTurn($: Engine) {
  return $.turn.complete({ answer: '', durationMs: 0, isAborted: false, turnId: 't', reason: 'answer' })
}

async function mountBand($: Engine, props: Partial<typeof BAND_PROPS> = {}) {
  await $.session.start({ cwd: ROOT, surface: 'terminal', isInteractive: true })
  return $.ui.mount({ plugin: PLUGIN, surface: 'terminal', component: 'AbovePrompt', props: { ...BAND_PROPS, ...props } })
}

function textOf(node: unknown): string {
  if (typeof node === 'string') return node
  const children = (node as { children?: unknown[] }).children ?? []
  return children.map(textOf).join('')
}

// band Box 아래 자식 하나가 한 행이다
function rows(tree: RenderElement): string[] {
  expect(tree.type).toBe('Box')
  expect((tree as { props?: Record<string, unknown> }).props?.['flexDirection']).toBe('column')
  return ((tree as { children?: unknown[] }).children ?? []).map(textOf)
}

test('설정을 주지 않으면 v0.6.2와 같은 한 줄로 그린다', async ($, on) => {
  stubSession(on)
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual([
    'cc-usage (main) │ ◆ claude-opus-5-5 │ ███░░░░░ 40% 80K │ $1.23 │ 5h: 12% │ 7d: 34% │ spend: 56%',
  ])
})

test('로드 때 먼저 그려진 뒤 session.start가 끝나면 band를 다시 그린다', async ($, on) => {
  stubSession(on)
  // 데이터가 오기 전 첫 render는 next(e)로 비키므로 엔진 자리의 이 응답이 그려진다
  on('ui.render', async () => ({ type: 'Text', children: ['engine band'] }))
  const ui = await $.ui.mount({ plugin: PLUGIN, surface: 'terminal', component: 'AbovePrompt', props: BAND_PROPS })
  expect(textOf(await ui.drawn())).toBe('engine band')
  await $.session.start({ cwd: ROOT, surface: 'terminal', isInteractive: true })
  expect(rows(await ui.drawn())).toEqual([
    'cc-usage (main) │ ◆ claude-opus-5-5 │ ███░░░░░ 40% 80K │ $1.23 │ 5h: 12% │ 7d: 34% │ spend: 56%',
  ])
})

test('layout이 여러 줄이면 그 줄 수만큼 행으로 그린다', { options: { layout: 'NM|C$|R7L' } }, async ($, on) => {
  stubSession(on)
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual([
    'cc-usage (main) │ ◆ claude-opus-5-5',
    '███░░░░░ 40% 80K │ $1.23',
    '5h: 12% │ 7d: 34% │ spend: 56%',
  ])
})

test(
  'layout의 위젯 ID 토큰을 적은 순서로 그린다',
  { options: { layout: 'spendLimit, model | rateLimit7d cost' } },
  async ($, on) => {
    stubSession(on)
    const ui = await mountBand($)
    expect(rows(await ui.drawn())).toEqual(['spend: 56% │ ◆ claude-opus-5-5', '7d: 34% │ $1.23'])
  },
)

test(
  'disabledWidgets에 적은 위젯은 그리지 않고 비는 줄은 버린다',
  { options: { layout: 'NM|C$|R7L', disabledWidgets: 'model, C $' } },
  async ($, on) => {
    stubSession(on)
    const ui = await mountBand($)
    expect(rows(await ui.drawn())).toEqual(['cc-usage (main)', '5h: 12% │ 7d: 34% │ spend: 56%'])
    expect(await ui.find({ type: 'Text', text: /claude-opus/ })).toBeUndefined()
  },
)

test(
  '그릴 줄이 하나도 없으면 band를 그리지 않는다',
  { options: { layout: 'M|C', disabledWidgets: 'MC' } },
  async ($, on) => {
    stubSession(on)
    // 플러그인이 next(e)로 비키면 엔진 자리의 이 응답이 그려진다
    on('ui.render', async () => ({ type: 'Text', children: ['engine band'] }))
    const ui = await mountBand($)
    expect(textOf(await ui.drawn())).toBe('engine band')
  },
)

const HOME = '/home/kero'
const HOME_ROOT = HOME + '/work/cc-usage'
const REPO: SessionRepo = { root: HOME_ROOT, remote: 'git@github.com:zipke/cc-usage.git', internal: false, name: null }

// 프로젝트 칸 테스트는 홈 아래 root와 origin remote가 있는 저장소를 쓴다
function stubProject(on: On, stubs: Stubs = {}) {
  return stubSession(on, { root: HOME_ROOT, env: { HOME }, repo: REPO, ...stubs })
}

test('P·N·G 문자를 적은 순서로 그린다', { options: { layout: 'GPN|M' } }, async ($, on) => {
  stubProject(on)
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual([
    'zipke/cc-usage │ ~/work/cc-usage (main) │ cc-usage (main)',
    '◆ claude-opus-5-5',
  ])
})

test(
  '프로젝트·저장소 위젯 ID를 적은 순서로 그린다',
  { options: { layout: 'projectName repoInfo, projectInfo' } },
  async ($, on) => {
    stubProject(on)
    const ui = await mountBand($)
    expect(rows(await ui.drawn())).toEqual(['cc-usage (main) │ zipke/cc-usage │ ~/work/cc-usage (main)'])
  },
)

test('Windows 홈(USERPROFILE)은 구분자와 드라이브 대소문자가 달라도 ~로 줄인다', { options: { layout: 'P' } }, async ($, on) => {
  stubSession(on, { root: 'c:/Users/kero\\GolandProjects\\cc-usage', env: { USERPROFILE: 'C:\\Users\\Kero' } })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual(['~/GolandProjects/cc-usage (main)'])
})

test('홈 환경변수가 없으면 경로를 그대로 보인다', { options: { layout: 'P' } }, async ($, on) => {
  stubProject(on, { env: {} })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual([HOME_ROOT + ' (main)'])
})

test('50자를 넘는 프로젝트 경로는 줄여서 보인다', { options: { layout: 'P' } }, async ($, on) => {
  stubProject(on, { root: HOME + '/' + 'x'.repeat(30) + '/' + 'y'.repeat(20) + '/cc-usage' })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual(['~/…/cc-usage (main)'])
})

test('git이 비0으로 끝나면 브랜치 괄호만 빼고 오류는 보이지 않는다', { options: { layout: 'PN' } }, async ($, on) => {
  const fail = { exitCode: 128, stdout: '', stderr: 'fatal: not a git repository' } as ProcessRunResult
  stubProject(on, { git: fail })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual(['~/work/cc-usage │ cc-usage'])
  expect(await ui.find({ type: 'Text', text: /git|fatal/ })).toBeUndefined()
})

test('git 실행이 거부되면 브랜치 괄호만 뺀다', { options: { layout: 'PN' } }, async ($, on) => {
  const calls = stubProject(on, { git: null })
  const ui = await mountBand($)
  expect(calls.run.length).toBeGreaterThan(0)
  expect(rows(await ui.drawn())).toEqual(['~/work/cc-usage │ cc-usage'])
})

test('브랜치가 비면(detached HEAD) 괄호를 뺀다', { options: { layout: 'PN' } }, async ($, on) => {
  stubProject(on, { git: { exitCode: 0, stdout: '\n', stderr: '' } as ProcessRunResult })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual(['~/work/cc-usage │ cc-usage'])
})

test('remote가 없으면 repoInfo만 생략한다', { options: { layout: 'NGM' } }, async ($, on) => {
  stubProject(on, { repo: { ...REPO, remote: null } })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual(['cc-usage (main) │ ◆ claude-opus-5-5'])
})

test('저장소 밖이면(repo null) repoInfo만 생략한다', { options: { layout: 'NGM' } }, async ($, on) => {
  stubProject(on, { repo: null })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual(['cc-usage (main) │ ◆ claude-opus-5-5'])
})

test('remote에서 owner/name을 뽑을 수 없으면 repoInfo만 생략한다', { options: { layout: 'NGM' } }, async ($, on) => {
  stubProject(on, { repo: { ...REPO, remote: '/srv/git/cc-usage.git' } })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual(['cc-usage (main) │ ◆ claude-opus-5-5'])
})

test('session.repo가 거부되면 repoInfo만 생략한다', { options: { layout: 'NGM' } }, async ($, on) => {
  const calls = stubProject(on, { repo: undefined })
  const ui = await mountBand($)
  expect(calls.repo).toBeGreaterThan(0)
  expect(rows(await ui.drawn())).toEqual(['cc-usage (main) │ ◆ claude-opus-5-5'])
})

test('repoInfo가 배치에 없으면 session.repo를 부르지 않는다', async ($, on) => {
  const calls = stubProject(on)
  await mountBand($)
  expect(calls.repo).toBe(0)
})

test(
  '끈 repoInfo는 배치에 있어도 session.repo를 부르지 않는다',
  { options: { layout: 'NGM', disabledWidgets: 'repoInfo' } },
  async ($, on) => {
    const calls = stubProject(on)
    const ui = await mountBand($)
    expect(calls.repo).toBe(0)
    expect(rows(await ui.drawn())).toEqual(['cc-usage (main) │ ◆ claude-opus-5-5'])
  },
)

test(
  '셸 cd로 cwd가 하위 디렉토리여도 프로젝트 칸과 git cwd는 root 기준이다',
  { options: { layout: 'PN' } },
  async ($, on) => {
    const calls = stubProject(on)
    on('session.cwd', async () => ({ value: HOME_ROOT + '/src/hooks' }))
    const ui = await mountBand($)
    expect(rows(await ui.drawn())).toEqual(['~/work/cc-usage (main) │ cc-usage (main)'])
    expect(calls.run.length).toBeGreaterThan(0)
    for (const call of calls.run) {
      expect(call).toEqual({ argv: ['git', 'branch', '--show-current'], cwd: HOME_ROOT })
    }
  },
)

// 브랜치 조회 빈도 제한 (SPEC §5.12): 같은 root에서 git·session.repo는 5초 안에 한 번이다

const gitCalls = (calls: { run: { argv: readonly string[] }[] }) => calls.run.filter((c) => c.argv[0] === 'git').length

test(
  '5초 안의 갱신은 git·session.repo를 다시 부르지 않고 5초가 지나면 한 번 더 부른다',
  { options: { layout: 'NG' } },
  async ($, on) => {
    const calls = stubProject(on)
    await mountBand($)
    expect(gitCalls(calls)).toBe(1)
    expect(calls.repo).toBe(1)

    await completeTurn($)
    await completeTurn($)
    await calls.clock.advance(4_999)
    await completeTurn($)
    expect(gitCalls(calls)).toBe(1)
    expect(calls.repo).toBe(1)

    await calls.clock.advance(1)
    await completeTurn($)
    await completeTurn($)
    expect(gitCalls(calls)).toBe(2)
    expect(calls.repo).toBe(2)
  },
)

test('30초 주기 갱신도 같은 5초 제한을 거친다', { options: { layout: 'N' } }, async ($, on) => {
  const calls = stubProject(on)
  await mountBand($)
  await calls.clock.advance(29_000)
  await completeTurn($)
  expect(gitCalls(calls)).toBe(2)
  // 30초 tick은 1초 전 턴 끝 조회의 TTL 안이다
  await calls.clock.advance(1_000)
  expect(gitCalls(calls)).toBe(2)
  await calls.clock.advance(30_000)
  expect(gitCalls(calls)).toBe(3)
})

test('동시에 들어온 갱신은 진행 중인 git 하나를 함께 기다린다', { options: { layout: 'NG' } }, async ($, on) => {
  const calls = stubProject(on)
  const ui = await mountBand($)
  calls.git = { exitCode: 0, stdout: 'feature\n', stderr: '' } as ProcessRunResult
  calls.gitDelayMs = 1_000
  await calls.clock.advance(5_000)

  let finished = 0
  const turns = [completeTurn($), completeTurn($), completeTurn($)].map((t) => t.then(() => finished++))
  await calls.clock.settle()
  // 나중에 온 갱신도 git이 답할 때까지 끝나지 않는다 — 옛 브랜치로 먼저 그리지 않는다
  expect(gitCalls(calls)).toBe(2)
  expect(finished).toBe(0)
  await calls.clock.advance(1_000)
  await Promise.all(turns)
  expect(finished).toBe(3)
  expect(gitCalls(calls)).toBe(2)
  expect(calls.repo).toBe(2)
  expect(rows(await ui.drawn())).toEqual(['cc-usage (feature) │ zipke/cc-usage'])
})

const GIT_FAILURES: [string, ProcessRunResult | null][] = [
  ['비0 종료', { exitCode: 128, stdout: '', stderr: 'fatal: not a git repository' } as ProcessRunResult],
  ['거부', null],
]

for (const [label, git] of GIT_FAILURES) {
  test(`git이 ${label}면 빈 브랜치로 남고 5초 안에는 다시 실행하지 않는다`, { options: { layout: 'N' } }, async ($, on) => {
    const calls = stubProject(on, { git })
    const ui = await mountBand($)
    await calls.clock.advance(4_999)
    await completeTurn($)
    expect(gitCalls(calls)).toBe(1)
    expect(rows(await ui.drawn())).toEqual(['cc-usage'])

    await calls.clock.advance(1)
    await completeTurn($)
    expect(gitCalls(calls)).toBe(2)
  })
}

test(
  'projectInfo·projectName이 유효 배치에 없으면 git을 실행하지 않는다',
  { options: { layout: 'PNM|C', disabledWidgets: 'projectInfo N' } },
  async ($, on) => {
    const calls = stubProject(on)
    const ui = await mountBand($)
    await completeTurn($)
    await calls.clock.advance(30_000)
    await completeTurn($)
    expect(calls.run).toEqual([])
    expect(rows(await ui.drawn())).toEqual(['◆ claude-opus-5-5', '███░░░░░ 40% 80K'])
  },
)

test('브랜치가 바뀌면 5초가 지난 뒤 갱신에서 band에 새 브랜치가 보인다', { options: { layout: 'N' } }, async ($, on) => {
  const calls = stubProject(on)
  const ui = await mountBand($)
  calls.git = { exitCode: 0, stdout: 'feature\n', stderr: '' } as ProcessRunResult

  await calls.clock.advance(4_000)
  await completeTurn($)
  expect(rows(await ui.drawn())).toEqual(['cc-usage (main)'])

  await calls.clock.advance(1_000)
  await completeTurn($)
  expect(rows(await ui.drawn())).toEqual(['cc-usage (feature)'])
})

// 테마 (SPEC §5.4, §5.8): 위젯 Text의 color가 고른 테마의 역할 색이다

const THEMED_USAGE: SessionUsage = {
  ...USAGE,
  context: { tokens: 300_000, window: 1_000_000, percent: 50 },
  rateLimits: [
    { kind: 'five_hour', percentUsed: 51 },
    { kind: 'seven_day', percentUsed: 80 },
    { kind: 'spend_limit', percentUsed: 81 },
  ],
}

type Ui = Awaited<ReturnType<typeof mountBand>>

// 조각 Text 하나의 색 — 바깥 줄 Text도 포함 검색에 걸리므로 보이는 문자열이 정확히 같은 것만 남긴다
async function styleOf(ui: Ui, text: string) {
  const found = (await ui.findAll({ type: 'Text', text })).filter((el) => el.text === text)
  expect(found).toHaveLength(1)
  const { color, bold, dimColor } = found[0]!.props as { color?: string; bold?: boolean; dimColor?: boolean }
  return { color, bold, dimColor }
}

for (const theme of THEME_NAMES) {
  test(`theme ${theme}이면 위젯 Text 색이 그 테마의 역할 색이다`, { options: { theme, layout: 'NGM|C$|R7L' } }, async ($, on) => {
    stubProject(on, { usage: THEMED_USAGE })
    const ui = await mountBand($)
    expect(rows(await ui.drawn())).toEqual([
      'cc-usage (main) │ zipke/cc-usage │ ◆ claude-opus-5-5',
      '████░░░░ 50% 300K │ $1.23',
      '5h: 51% │ 7d: 80% │ spend: 81%',
    ])
    const t = THEMES[theme]
    const as = (role: keyof typeof t) => ({ color: t[role].color, bold: t[role].bold, dimColor: undefined })
    expect(await styleOf(ui, 'cc-usage')).toEqual(as('folder'))
    expect(await styleOf(ui, ' (main)')).toEqual(as('branch'))
    expect(await styleOf(ui, 'zipke/cc-usage')).toEqual(as('secondary'))
    expect(await styleOf(ui, '◆ claude-opus-5-5')).toEqual(as('model'))
    expect(await styleOf(ui, '████')).toEqual(as('safe'))
    expect(await styleOf(ui, '░░░░')).toEqual(as('barEmpty'))
    expect(await styleOf(ui, '50%')).toEqual(as('safe'))
    expect(await styleOf(ui, ' 300K')).toEqual(as('warning'))
    expect(await styleOf(ui, '$1.23')).toEqual(as('accent'))
    expect(await styleOf(ui, '5h: ')).toEqual(as('secondary'))
    expect(await styleOf(ui, '51%')).toEqual(as('warning'))
    expect(await styleOf(ui, '80%')).toEqual(as('warning'))
    expect(await styleOf(ui, '81%')).toEqual(as('danger'))
    const separators = await ui.findAll({ type: 'Text', text: /^│$/ })
    expect(separators).toHaveLength(5)
    for (const sep of separators) expect(sep.props['dimColor']).toBe(true)
  })
}

const SEPARATOR_CASES: [string, string, string][] = [
  ['pipe', ' │ ', '│'],
  ['dot', ' · ', '·'],
  ['arrow', ' › ', '›'],
  ['space', '  ', ''],
]

for (const [separator, text, symbol] of SEPARATOR_CASES) {
  test(`separator ${separator}이면 위젯 사이가 ${JSON.stringify(text)}다`, { options: { separator, layout: 'NM$|R7' } }, async ($, on) => {
    stubSession(on)
    const ui = await mountBand($)
    expect(rows(await ui.drawn())).toEqual([
      'cc-usage (main)' + text + '◆ claude-opus-5-5' + text + '$1.23',
      '5h: 12%' + text + '7d: 34%',
    ])
    // 기호 조각만 dimColor이고 양옆 공백은 꾸미지 않는다
    if (symbol) {
      const symbols = (await ui.findAll({ type: 'Text', text: symbol })).filter((el) => el.text === symbol)
      expect(symbols).toHaveLength(3)
      for (const sym of symbols) expect(sym.props['dimColor']).toBe(true)
      for (const pad of (await ui.findAll({ type: 'Text', text: ' ' })).filter((el) => el.text === ' ')) {
        expect(pad.props['dimColor']).toBeUndefined()
      }
    } else {
      const pads = (await ui.findAll({ type: 'Text', text: '  ' })).filter((el) => el.text === '  ')
      expect(pads).toHaveLength(3)
      for (const pad of pads) expect(pad.props['dimColor']).toBeUndefined()
    }
  })
}

test('theme이 8종 밖이면 default 색으로 그린다', { options: { theme: 'neon', layout: 'M$' } }, async ($, on) => {
  stubSession(on)
  const ui = await mountBand($)
  expect((await styleOf(ui, '◆ claude-opus-5-5')).color).toBe(THEMES.default.model.color)
})

// 언어 (SPEC §5.6): rate limit 라벨과 남은 시간이 고른 언어, auto면 로캘 환경변수를 따른다

const MIN = 60_000
const HOUR = 60 * MIN

// mock clock은 0에서 시작한다 — 5h는 3시간 4분, 7d는 1일 2시간, spend는 5분 뒤 리셋된다
const RESET_USAGE: SessionUsage = {
  ...USAGE,
  rateLimits: [
    { kind: 'five_hour', percentUsed: 12, resetsAt: new Date(3 * HOUR + 4 * MIN).toISOString() },
    { kind: 'seven_day', percentUsed: 34, resetsAt: new Date(26 * HOUR).toISOString() },
    { kind: 'spend_limit', percentUsed: 56, resetsAt: new Date(5 * MIN).toISOString() },
  ],
}

const EN_ROW = '5h: 12% (3h4m) │ 7d: 34% (1d 2h) │ spend: 56% (5m)'
const KO_ROW = '5시간: 12% (3시간4분) │ 7일: 34% (1일 2시간) │ spend: 56% (5분)'

const LANGUAGE_CASES: [string, PluginOptions, Record<string, string>, string][] = [
  ['language ko', { language: 'ko' }, {}, KO_ROW],
  ['language en은 한국어 로캘에서도', { language: 'en' }, { LANG: 'ko_KR.UTF-8' }, EN_ROW],
  ['auto에서 LANG이 ko면', { language: 'auto' }, { LANG: 'ko_KR.UTF-8' }, KO_ROW],
  ['auto에서 LC_ALL만 ko면', { language: 'auto' }, { LC_ALL: 'ko_KR.UTF-8' }, KO_ROW],
  ['auto에서 LC_MESSAGES만 ko면', { language: 'auto' }, { LC_MESSAGES: 'ko' }, KO_ROW],
  ['auto에서 다른 로캘 뒤 LANG이 ko면', { language: 'auto' }, { LC_ALL: 'en_US.UTF-8', LANG: 'ko_KR.UTF-8' }, KO_ROW],
  ['auto에서 로캘이 ko가 아니면', { language: 'auto' }, { LC_ALL: 'C.UTF-8', LANG: 'en_US.UTF-8' }, EN_ROW],
  ['auto에서 로캘 변수가 없으면', { language: 'auto' }, {}, EN_ROW],
  ['설정이 없으면(auto) 한국어 로캘에서', {}, { LANG: 'ko_KR.UTF-8' }, KO_ROW],
  ['선택지 밖 값은 auto로 보아 한국어 로캘에서', { language: 'ja' }, { LANG: 'ko_KR.UTF-8' }, KO_ROW],
]

for (const [label, options, env, row] of LANGUAGE_CASES) {
  test(`${label} rate limit 줄이 ${row}다`, { options: { ...options, layout: 'R7L' } }, async ($, on) => {
    stubSession(on, { usage: RESET_USAGE, env })
    const ui = await mountBand($)
    expect(rows(await ui.drawn())).toEqual([row])
  })
}

test('남은 시간은 mock clock 기준이고 1분 미만이나 지난 리셋은 괄호를 뺀다', { options: { language: 'ko', layout: 'R7L' } }, async ($, on) => {
  const calls = stubSession(on, { usage: RESET_USAGE })
  const ui = await mountBand($)
  // 30초 tick이 다시 그린다 — spend 리셋 30초 전이라 1분 미만으로 생략된다
  await calls.clock.advance(4 * MIN + 30_000)
  expect(rows(await ui.drawn())).toEqual(['5시간: 12% (2시간59분) │ 7일: 34% (1일 1시간) │ spend: 56%'])
  // 5h 리셋 시각이 지났다
  await calls.clock.advance(3 * HOUR)
  expect(rows(await ui.drawn())).toEqual(['5시간: 12% │ 7일: 34% (22시간55분) │ spend: 56%'])
})

test('남은 시간 괄호는 dimColor이고 라벨은 secondary 색이다', { options: { language: 'ko', layout: 'R' } }, async ($, on) => {
  stubSession(on, { usage: RESET_USAGE })
  const ui = await mountBand($)
  expect(await styleOf(ui, '5시간: ')).toEqual({ color: THEMES.default.secondary.color, bold: undefined, dimColor: undefined })
  expect(await styleOf(ui, ' (3시간4분)')).toEqual({ color: undefined, bold: undefined, dimColor: true })
})

// 컨텍스트 막대 폭 (SPEC §5.7): 화면에서도 막대가 설정 폭으로 그려진다

const BAR_CASES: [unknown, string][] = [
  [1, '░'],
  [8, '███░░░░░'],
  [40, '█'.repeat(16) + '░'.repeat(24)],
  // 범위 밖·정수 아님은 8칸
  [0, '███░░░░░'],
  [41, '███░░░░░'],
  [-1, '███░░░░░'],
  [2.5, '███░░░░░'],
]

for (const [width, cells] of BAR_CASES) {
  test(
    `contextBarWidth ${width}이면 막대가 ${[...cells].length}칸이고 다른 설정은 그대로다`,
    { options: { contextBarWidth: width as number, theme: 'nord', separator: 'dot', layout: 'C$|M', disabledWidgets: 'M' } },
    async ($, on) => {
      stubSession(on)
      const ui = await mountBand($)
      expect(rows(await ui.drawn())).toEqual([cells + ' 40% 80K · $1.23'])
      expect(await styleOf(ui, '$1.23')).toEqual({ color: THEMES.nord.accent.color, bold: undefined, dimColor: undefined })
    },
  )
}

test('퍼센트가 아직 없으면 화면에 설정 폭의 빈 막대와 흐린 -를 그린다', { options: { contextBarWidth: 5, layout: 'C$' } }, async ($, on) => {
  stubSession(on, { usage: { ...USAGE, context: { tokens: 0, window: 200_000 } } })
  const ui = await mountBand($)
  expect(rows(await ui.drawn())).toEqual(['░░░░░ - │ $1.23'])
  expect(await styleOf(ui, '░░░░░')).toEqual({ color: THEMES.default.barEmpty.color, bold: undefined, dimColor: undefined })
  expect(await styleOf(ui, '-')).toEqual({ color: THEMES.default.secondary.color, bold: undefined, dimColor: true })
})

// band 폭 맞춤 (SPEC §5.9): 줄마다 bodyColumns - 1(왼쪽 여백 1칸) 표시폭 안에 들어가게 그린다

// 기본 배치 한 줄의 표시폭은 95칸이고, 오른쪽 위젯을 하나씩 빼면 82·72·62·54·35·15칸이다
const DEFAULT_ROW = 'cc-usage (main) │ ◆ claude-opus-5-5 │ ███░░░░░ 40% 80K │ $1.23 │ 5h: 12% │ 7d: 34% │ spend: 56%'

// 줄마다 바깥 Text의 wrap — 표시폭 계산이 빗나가도 다음 행으로 넘어가지 않게 하는 안전망이다
function rowWraps(tree: RenderElement): unknown[] {
  return ((tree as { children?: unknown[] }).children ?? []).map(
    (row) => (row as { props?: Record<string, unknown> }).props?.['wrap'],
  )
}

const FIT_CASES: [number, string][] = [
  [96, DEFAULT_ROW],
  [95, 'cc-usage (main) │ ◆ claude-opus-5-5 │ ███░░░░░ 40% 80K │ $1.23 │ 5h: 12% │ 7d: 34%'],
  [83, 'cc-usage (main) │ ◆ claude-opus-5-5 │ ███░░░░░ 40% 80K │ $1.23 │ 5h: 12% │ 7d: 34%'],
  [82, 'cc-usage (main) │ ◆ claude-opus-5-5 │ ███░░░░░ 40% 80K │ $1.23 │ 5h: 12%'],
  [36, 'cc-usage (main) │ ◆ claude-opus-5-5'],
  [35, 'cc-usage (main)'],
  [16, 'cc-usage (main)'],
  // 위젯 하나만 남아도 넘치면 13칸 + …로 자른다
  [15, 'cc-usage (mai…'],
]

for (const [bodyColumns, row] of FIT_CASES) {
  test(`bodyColumns ${bodyColumns}이면 기본 배치가 ${JSON.stringify(row)}다`, async ($, on) => {
    stubSession(on)
    const ui = await mountBand($, { bodyColumns })
    const tree = await ui.drawn()
    expect(rows(tree)).toEqual([row])
    expect(rowWraps(tree)).toEqual(['truncate-end'])
  })
}

test('잘린 위젯의 …는 잘린 조각의 색을 이어받는다', async ($, on) => {
  stubSession(on)
  const ui = await mountBand($, { bodyColumns: 15 })
  expect(await styleOf(ui, 'cc-usage')).toEqual({ color: THEMES.default.folder.color, bold: undefined, dimColor: undefined })
  expect(await styleOf(ui, ' (mai…')).toEqual({ color: THEMES.default.branch.color, bold: undefined, dimColor: undefined })
})

test('여러 줄 배치는 줄마다 따로 맞추고 모든 줄이 truncate-end다', { options: { layout: 'NM|C$|R7L' } }, async ($, on) => {
  stubSession(on)
  // 예산 29칸: 첫 줄(35칸)과 셋째 줄(30칸)만 넘친다
  const ui = await mountBand($, { bodyColumns: 30 })
  const tree = await ui.drawn()
  expect(rows(tree)).toEqual(['cc-usage (main)', '███░░░░░ 40% 80K │ $1.23', '5h: 12% │ 7d: 34%'])
  expect(rowWraps(tree)).toEqual(['truncate-end', 'truncate-end', 'truncate-end'])
})

// 한국어 줄은 34칸이지만 문자 수로는 31자다 — 한글을 1칸으로 재면 예산 33칸에서 빠지지 않는다
const KO_FIT_CASES: [number, string][] = [
  [35, '5시간: 12% │ 7일: 34% │ spend: 56%'],
  [34, '5시간: 12% │ 7일: 34%'],
  // 5시간: 12%(10칸) 하나만 남아 넘치면 2칸 문자를 반으로 자르지 않는다 — 예산 3칸이면 5와 …뿐이다
  [5, '5시…'],
  [4, '5…'],
]

for (const [bodyColumns, row] of KO_FIT_CASES) {
  test(`언어 ko에서 bodyColumns ${bodyColumns}이면 rate limit 줄이 ${JSON.stringify(row)}다`, { options: { language: 'ko', layout: 'R7L' } }, async ($, on) => {
    stubSession(on)
    const ui = await mountBand($, { bodyColumns })
    expect(rows(await ui.drawn())).toEqual([row])
  })
}
