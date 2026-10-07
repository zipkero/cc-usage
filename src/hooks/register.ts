// cc-usage 기본 줄(project │ model │ context │ cost │ 5h │ 7d)을 프롬프트 위 band에 그린다.
// 데이터 출처는 stdin이 아니라 mods API다 — $.session.usage()가 status line과 같은 수치를 준다.
import type { EngineInterface, On, SessionRateLimit, SessionUsage } from 'claude-code'

// Go 쪽 modelSymbolTable과 같은 순서·기호
const MODEL_SYMBOLS: [string, string][] = [
  ['opus', '◆'],
  ['sonnet', '◇'],
  ['haiku', '○'],
  ['fable', '◈'],
  ['mythos', '◎'],
]
const DEFAULT_MODEL_SYMBOL = '●'

const BAR_WIDTH = 8
const SEPARATOR = ' │ '

// 리셋 카운트다운과 브랜치를 세션이 쉬는 동안에도 갱신하는 주기
const TICK_MS = 30_000

// session.measure 입력에는 startedAt이 없어 SessionUsage 전체를 담을 수 없다
type Figures = Pick<SessionUsage, 'context' | 'rateLimits' | 'cost'>

let usage: Figures | null = null
let model = ''
// cwd()는 셸 cd를 따라가 이름·브랜치가 흔들리므로 cd로 움직이지 않는 root()를 쓴다
let root = ''
let branch = ''
// PoC 진단용: 브랜치를 못 읽은 이유를 band에 그대로 보인다
let branchError = ''

export function register(on: On) {
  on('session.start', async ($, e, next) => {
    await refreshAll($)
    $.clock.every(TICK_MS, async () => {
      await refreshAll($)
      $.ui.invalidate('ui.render')
    })
    return next(e)
  })

  // 턴이 끝날 때마다, 그리고 rate limit이 1포인트 움직일 때 발생한다
  on('session.measure', async ($, e, next) => {
    usage = { context: e.context, rateLimits: e.rateLimits, cost: e.cost }
    $.ui.invalidate('ui.render')
    return next(e)
  })

  // /model 전환과 브랜치 이동은 measure에 잡히지 않으므로 턴 끝에서 다시 읽는다
  on('turn.complete', async ($, e, next) => {
    await refreshSession($)
    $.ui.invalidate('ui.render')
    return next(e)
  })

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    if (e.props.hasSurvey) return next(e)
    const { Box, Text } = $.ui.resolve(e)

    const segments = buildSegments()
    if (segments.length === 0) return next(e)

    const children: ReturnType<typeof Text>[] = []
    segments.forEach((seg, i) => {
      if (i > 0) children.push(Text({ dimColor: true, children: [SEPARATOR] }))
      for (const part of seg) children.push(Text(part))
    })
    // band는 spinner 바로 아래에 붙어 그려진다 — 위 한 줄을 비워 떼어 놓는다
    return Box({ flexDirection: 'row', marginTop: 1, paddingLeft: 1, children })
  })
}

async function refreshAll($: EngineInterface) {
  usage = await $.session.usage()
  await refreshSession($)
}

async function refreshSession($: EngineInterface) {
  model = await $.session.model()
  root = await $.session.root()
  await readBranch($)
}

async function readBranch($: EngineInterface) {
  try {
    const r = await $.process.run(['git', 'branch', '--show-current'], { cwd: root, timeoutMs: 2000 })
    branch = r.exitCode === 0 ? r.stdout.trim() : ''
    branchError = r.exitCode === 0 ? '' : 'exit ' + r.exitCode + ': ' + r.stderr.trim()
  } catch (err) {
    branch = ''
    branchError = String(err)
  }
}

type Part = { children: string[]; color?: string; dimColor?: boolean; bold?: boolean }

// 위젯 하나가 Text 조각 여러 개로 이뤄진다 — 색이 조각마다 다르기 때문이다
function buildSegments(): Part[][] {
  const segs: Part[][] = []

  const project = projectSegment()
  if (project) segs.push(project)
  if (model) segs.push(modelSegment(model))

  if (usage) {
    const ctx = contextSegment(usage)
    if (ctx) segs.push(ctx)
    if (usage.cost) segs.push([{ children: [formatCost(usage.cost.usd)], color: 'yellow' }])
    for (const kind of ['five_hour', 'seven_day', 'spend_limit']) {
      const rl = usage.rateLimits.find((r) => r.kind === kind)
      if (rl) segs.push(rateLimitSegment(rl))
    }
  }
  return segs
}

function projectSegment(): Part[] | null {
  if (!root) return null
  const name = root.split(/[\\/]/).filter(Boolean).pop() ?? root
  const parts: Part[] = [{ children: [name], color: 'cyan' }]
  if (branch) parts.push({ children: [' (' + branch + ')'], color: 'magenta' })
  else if (branchError) parts.push({ children: [' (git: ' + branchError.slice(0, 60) + ')'], dimColor: true })
  return parts
}

function modelSegment(id: string): Part[] {
  const lower = id.toLowerCase()
  const symbol = MODEL_SYMBOLS.find(([sub]) => lower.includes(sub))?.[1] ?? DEFAULT_MODEL_SYMBOL
  return [{ children: [symbol + ' ' + id], color: 'blue' }]
}

function contextSegment(u: Figures): Part[] | null {
  const { percent, tokens } = u.context
  // 첫 응답 전에는 percent가 없다 — Go 쪽 placeholder와 같은 자리
  if (percent === undefined) return [{ children: ['ctx: -'], dimColor: true }]
  const filled = Math.round((percent / 100) * BAR_WIDTH)
  const bar = '█'.repeat(filled) + '░'.repeat(BAR_WIDTH - filled)
  const parts: Part[] = [{ children: [bar + ' ' + percent + '%'], color: levelColor(percent) }]
  if (tokens !== undefined) parts.push({ children: [' ' + formatTokens(tokens)] })
  return parts
}

const RATE_LIMIT_LABELS: Record<string, string> = {
  five_hour: '5h',
  seven_day: '7d',
  spend_limit: 'spend',
}

function rateLimitSegment(rl: SessionRateLimit): Part[] {
  const label = RATE_LIMIT_LABELS[rl.kind] ?? rl.kind
  // spend_limit은 한도를 넘으면 100을 넘는다 — 자르지 않고 그대로 보인다
  const pct = Math.floor(rl.percentUsed)
  const parts: Part[] = [{ children: [label + ': ' + pct + '%'], color: levelColor(pct) }]
  const remaining = rl.resetsAt ? formatTimeRemaining(Date.parse(rl.resetsAt) - Date.now()) : ''
  if (remaining) parts.push({ children: [' (' + remaining + ')'], dimColor: true })
  return parts
}

function levelColor(pct: number): string {
  if (pct >= 80) return 'red'
  if (pct >= 50) return 'yellow'
  return 'green'
}

// format.go의 formatTokens와 같은 규칙
function formatTokens(n: number): string {
  if (n < 1000) return String(Math.max(0, n))
  if (n < 1_000_000) {
    const v = n / 1000
    return (v < 10 ? v.toFixed(1) : v.toFixed(0)) + 'K'
  }
  const v = n / 1_000_000
  return (v < 10 ? v.toFixed(1) : v.toFixed(0)) + 'M'
}

function formatCost(usd: number): string {
  return '$' + usd.toFixed(2)
}

// format.go의 formatTimeRemaining과 같은 규칙 — 1분 미만이나 지난 시각은 표시하지 않는다
function formatTimeRemaining(ms: number): string {
  const total = Math.floor(ms / 60_000)
  if (!(total > 0)) return ''
  const days = Math.floor(total / (60 * 24))
  const hours = Math.floor((total % (60 * 24)) / 60)
  const minutes = total % 60
  if (days > 0) return days + 'd ' + hours + 'h'
  if (hours > 0) return hours + 'h' + minutes + 'm'
  return minutes + 'm'
}
