// 플러그인 userConfig(options)를 band 설정으로 해석한다.
// $·런타임 import 없는 순수 계층이라 테스트가 직접 import한다.
import type { PluginOptions } from 'claude-code'
import { DEFAULT_LANGUAGE, isLanguageOption, type LanguageOption } from './i18n'
import { DEFAULT_THEME, isThemeName, type ThemeName } from './themes'

export type WidgetId =
  | 'projectInfo'
  | 'projectName'
  | 'repoInfo'
  | 'model'
  | 'context'
  | 'cost'
  | 'rateLimit5h'
  | 'rateLimit7d'
  | 'spendLimit'

// Go 판 presetCharToWidget과 같은 문자에 spend(L)를 더했다.
// Go 판에서 버린 f T E #와 재사용 금지 문자 S V a D B H F는 넣지 않아 무시된다.
const PRESET_CHARS: Readonly<Record<string, WidgetId>> = {
  P: 'projectInfo',
  N: 'projectName',
  G: 'repoInfo',
  M: 'model',
  C: 'context',
  $: 'cost',
  R: 'rateLimit5h',
  '7': 'rateLimit7d',
  L: 'spendLimit',
}

const WIDGET_IDS: ReadonlySet<string> = new Set<string>(Object.values(PRESET_CHARS))

// v0.6.2의 고정 순서(프로젝트 이름 │ 모델 │ 컨텍스트 │ 비용 │ 5h │ 7d │ spend)와 같다
export const DEFAULT_LAYOUT = 'NMC$R7L'

// Go 판 config.go의 separator 선택지와 같다
export const SEPARATOR_NAMES = ['pipe', 'dot', 'arrow', 'space'] as const

export type SeparatorName = (typeof SEPARATOR_NAMES)[number]

export const DEFAULT_SEPARATOR: SeparatorName = 'pipe'

function isSeparatorName(value: unknown): value is SeparatorName {
  return typeof value === 'string' && (SEPARATOR_NAMES as readonly string[]).includes(value)
}

// Go 판 config.go의 막대 폭 기본값·범위와 같다.
// manifest에 min·max를 두면 범위 밖 저장값이 모듈 로드를 막으므로 여기서 검증한다.
export const DEFAULT_CONTEXT_BAR_WIDTH = 8
const MIN_CONTEXT_BAR_WIDTH = 1
const MAX_CONTEXT_BAR_WIDTH = 40

function isContextBarWidth(value: unknown): value is number {
  return (
    typeof value === 'number' &&
    Number.isInteger(value) &&
    value >= MIN_CONTEXT_BAR_WIDTH &&
    value <= MAX_CONTEXT_BAR_WIDTH
  )
}

export type Settings = {
  theme: ThemeName
  separator: SeparatorName
  // auto는 세션 시작 때 로캘 환경변수로 en·ko 중 하나로 정해진다
  language: LanguageOption
  // 컨텍스트 막대 칸 수(1–40 정수)
  contextBarWidth: number
  lines: WidgetId[][]
  disabled: ReadonlySet<WidgetId>
}

// `|`로 줄을 나누고, 줄 안의 쉼표·공백 토큰이 위젯 ID면 그 위젯, 아니면 문자마다 preset 문자로 읽는다.
// 모르는 문자는 버리고 위젯이 없는 줄도 버린다.
export function parseWidgetTokens(text: string): WidgetId[][] {
  const lines: WidgetId[][] = []
  for (const line of text.split('|')) {
    const widgets: WidgetId[] = []
    for (const token of line.split(/[\s,]+/)) {
      if (!token) continue
      if (WIDGET_IDS.has(token)) {
        widgets.push(token as WidgetId)
        continue
      }
      for (const ch of token) {
        const id = PRESET_CHARS[ch]
        if (id) widgets.push(id)
      }
    }
    if (widgets.length > 0) lines.push(widgets)
  }
  return lines
}

// 항목마다 따로 검증해 틀린 항목만 기본값으로 돌린다
export function resolveSettings(options: PluginOptions): Settings {
  const layout = options['layout']
  let lines = typeof layout === 'string' ? parseWidgetTokens(layout) : []
  if (lines.length === 0) lines = parseWidgetTokens(DEFAULT_LAYOUT)

  // 끄기 목록은 줄 구분 없이 하나의 집합이다
  const disabledText = options['disabledWidgets']
  const disabled = new Set<WidgetId>(typeof disabledText === 'string' ? parseWidgetTokens(disabledText).flat() : [])

  const theme = options['theme']
  const separator = options['separator']
  const language = options['language']
  const contextBarWidth = options['contextBarWidth']

  return {
    theme: isThemeName(theme) ? theme : DEFAULT_THEME,
    separator: isSeparatorName(separator) ? separator : DEFAULT_SEPARATOR,
    language: isLanguageOption(language) ? language : DEFAULT_LANGUAGE,
    contextBarWidth: isContextBarWidth(contextBarWidth) ? contextBarWidth : DEFAULT_CONTEXT_BAR_WIDTH,
    lines,
    disabled,
  }
}

// 끈 위젯을 뺀 유효 배치에 id가 있는지 본다
export function isActive(settings: Settings, id: WidgetId): boolean {
  return !settings.disabled.has(id) && settings.lines.some((line) => line.includes(id))
}
