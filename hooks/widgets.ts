// 세션 스냅샷과 설정으로 band의 줄별 Text 조각을 만든다.
// $·런타임 import 없는 순수 계층이라 테스트가 직접 import한다.
import type { SessionRateLimit, SessionUsage } from 'claude-code'
import { formatTimeRemaining, type Language, TRANSLATIONS } from './i18n'
import { baseName, compressHome, PATH_MAX, shrinkPath } from './paths'
import type { SeparatorName, Settings, WidgetId } from './settings'
import { percentRole, type Role, type Theme, THEMES, tokenRole } from './themes'
import { fitLine } from './width'

// session.measure 입력에는 startedAt이 없어 SessionUsage 전체를 담을 수 없다
export type Figures = Pick<SessionUsage, 'context' | 'rateLimits' | 'cost'>

export type Snapshot = {
  usage: Figures | null
  model: string
  // 셸 cd로 움직이지 않는 $.session.root() — 프로젝트 칸은 cwd가 아니라 이 값을 그린다
  root: string
  // 빈 문자열이면 브랜치 괄호를 생략한다
  branch: string
  // origin remote의 owner/name — null이면 repoInfo를 생략한다
  repoSlug: string | null
  // 홈 압축 후보(HOME, USERPROFILE 순), 없으면 경로를 그대로 보인다
  homes: string[]
  // Settings.language의 auto를 로캘로 풀어 둔 값
  language: Language
  // 리셋까지 남은 시간의 기준 시각(ms)
  now: number
}

// 위젯 하나가 조각 여러 개로 이뤄진다 — 색이 조각마다 다르기 때문이다
export type Part = { text: string; color?: string; dimColor?: boolean; bold?: boolean }

// Go 쪽 modelSymbolTable과 같은 순서·기호
const MODEL_SYMBOLS: [string, string][] = [
  ['opus', '◆'],
  ['sonnet', '◇'],
  ['haiku', '○'],
  ['fable', '◈'],
  ['mythos', '◎'],
]
const DEFAULT_MODEL_SYMBOL = '●'

// Go 판 renderSeparator와 같다 — 기호만 Dim이고 양옆 공백은 꾸미지 않는다
export const SEPARATORS: Readonly<Record<SeparatorName, readonly Part[]>> = {
  pipe: [{ text: ' ' }, { text: '│', dimColor: true }, { text: ' ' }],
  dot: [{ text: ' ' }, { text: '·', dimColor: true }, { text: ' ' }],
  arrow: [{ text: ' ' }, { text: '›', dimColor: true }, { text: ' ' }],
  space: [{ text: '  ' }],
}

// spend는 Go 판에 없던 위젯이라 locale 표에 라벨이 없고 두 언어 모두 spend다
const RATE_LIMIT_KINDS: Partial<Record<WidgetId, { kind: string; label: (lang: Language) => string }>> = {
  rateLimit5h: { kind: 'five_hour', label: (lang) => TRANSLATIONS[lang].labels.fiveH },
  rateLimit7d: { kind: 'seven_day', label: (lang) => TRANSLATIONS[lang].labels.sevenD },
  spendLimit: { kind: 'spend_limit', label: () => 'spend' },
}

// null은 데이터가 없어 그 위젯을 생략한다는 뜻이다
export function renderWidget(id: WidgetId, s: Snapshot, settings: Settings): Part[] | null {
  const theme = THEMES[settings.theme]
  switch (id) {
    case 'projectInfo':
      return s.root ? projectWidget(shrinkPath(compressHome(s.root, s.homes), PATH_MAX), s.branch, theme) : null
    case 'projectName':
      return s.root ? projectWidget(baseName(s.root), s.branch, theme) : null
    case 'repoInfo':
      return s.repoSlug ? [paint(s.repoSlug, theme, 'secondary')] : null
    case 'model':
      return s.model ? modelWidget(s.model, theme) : null
    case 'context':
      return s.usage ? contextWidget(s.usage, settings.contextBarWidth, theme) : null
    case 'cost':
      return s.usage?.cost ? [paint(formatCost(s.usage.cost.usd), theme, 'accent')] : null
    case 'rateLimit5h':
    case 'rateLimit7d':
    case 'spendLimit': {
      const def = RATE_LIMIT_KINDS[id]
      const rl = def && s.usage?.rateLimits.find((r) => r.kind === def.kind)
      return def && rl ? rateLimitWidget(rl, def.label(s.language), s, theme) : null
    }
  }
}

// 끈 위젯과 생략된 위젯을 뺀 줄만 남기고, 위젯 사이에 구분자를 넣어 줄마다 budget 표시폭에 맞춘다.
// budget을 주지 않으면 맞추지 않는다.
export function buildLines(s: Snapshot, settings: Settings, budget = Infinity): Part[][] {
  const separator = SEPARATORS[settings.separator]
  const lines: Part[][] = []
  for (const ids of settings.lines) {
    const widgets: Part[][] = []
    for (const id of ids) {
      if (settings.disabled.has(id)) continue
      const parts = renderWidget(id, s, settings)
      if (parts) widgets.push(parts)
    }
    if (widgets.length === 0) continue
    const line = fitLine(widgets, separator, budget)
    if (line.length > 0) lines.push(line)
  }
  return lines
}

// 테마 역할의 색(과 minimal danger의 bold)을 입힌 조각
function paint(text: string, theme: Theme, role: Role): Part {
  const { color, bold } = theme[role]
  return bold ? { text, color, bold } : { text, color }
}

function projectWidget(label: string, branch: string, theme: Theme): Part[] {
  const parts: Part[] = [paint(label, theme, 'folder')]
  if (branch) parts.push(paint(' (' + branch + ')', theme, 'branch'))
  return parts
}

function modelWidget(id: string, theme: Theme): Part[] {
  const lower = id.toLowerCase()
  const symbol = MODEL_SYMBOLS.find(([sub]) => lower.includes(sub))?.[1] ?? DEFAULT_MODEL_SYMBOL
  return [paint(symbol + ' ' + id, theme, 'model')]
}

function contextWidget(u: Figures, width: number, theme: Theme): Part[] {
  const { percent, tokens } = u.context
  // 첫 응답 전에는 percent가 없다 — Go 판 placeholder처럼 빈 막대와 흐린 - 하나만 그린다
  if (percent === undefined) {
    return [...progressBar(0, width, theme, 'safe'), { text: ' ' }, { ...paint('-', theme, 'secondary'), dimColor: true }]
  }
  const level = percentRole(percent)
  const parts: Part[] = progressBar(percent, width, theme, level)
  parts.push({ text: ' ' }, paint(percent + '%', theme, level))
  if (tokens !== undefined) {
    parts.push(paint(' ' + formatTokens(tokens), theme, tokenRole(tokens)))
  }
  return parts
}

// render.go의 renderProgressBar와 같은 칸 수 — 채운 칸은 round(percent/100*width)를 0..width로 자른다.
// 빈 조각은 만들지 않는다 — 막대가 다 차거나 비면 한쪽 색만 남는다
function progressBar(percent: number, width: number, theme: Theme, level: Role): Part[] {
  const filled = Math.min(width, Math.max(0, Math.round((percent / 100) * width)))
  const parts: Part[] = []
  if (filled > 0) parts.push(paint('█'.repeat(filled), theme, level))
  if (filled < width) parts.push(paint('░'.repeat(width - filled), theme, 'barEmpty'))
  return parts
}

function rateLimitWidget(rl: SessionRateLimit, label: string, s: Snapshot, theme: Theme): Part[] {
  // spend_limit은 한도를 넘으면 100을 넘는다 — 자르지 않고 그대로 보인다
  const pct = Math.floor(rl.percentUsed)
  const parts: Part[] = [paint(label + ': ', theme, 'secondary'), paint(pct + '%', theme, percentRole(pct))]
  const remaining = rl.resetsAt ? formatTimeRemaining(Date.parse(rl.resetsAt) - s.now, s.language) : ''
  if (remaining) parts.push({ text: ' (' + remaining + ')', dimColor: true })
  return parts
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
