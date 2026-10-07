// 테마 8종의 역할별 색 표와 퍼센트 색 판정.
// $·런타임 import 없는 순수 계층이라 테스트가 직접 import한다.

export const THEME_NAMES = [
  'default',
  'minimal',
  'catppuccin',
  'dracula',
  'gruvbox',
  'nord',
  'tokyoNight',
  'solarized',
] as const

export type ThemeName = (typeof THEME_NAMES)[number]

export const DEFAULT_THEME: ThemeName = 'default'

// Go render.go의 역할 중 Info(worktree 표시)와 BarFilled(Go 코드에서 쓰지 않음)는 옮기지 않는다.
// Dim은 색이 아니라 속성이라 역할이 아니고 Part의 dimColor로 옮긴다.
export type Role = 'model' | 'folder' | 'branch' | 'safe' | 'warning' | 'danger' | 'secondary' | 'accent' | 'barEmpty'

export type Style = { color: string; bold?: boolean }

export type Theme = Readonly<Record<Role, Style>>

// Text color는 theme key·색 이름·hex만 보장되므로 Go의 ANSI 코드를 그 형태로 옮겼다.
// default는 256색 코드를 xterm 표의 hex로, truecolor 6종은 R;G;B를 그대로 #rrggbb로 옮겼다.
export const THEMES: Readonly<Record<ThemeName, Theme>> = {
  default: {
    model: { color: '#87d7ff' }, // 117
    folder: { color: '#ffd787' }, // 222
    branch: { color: '#ffafd7' }, // 218
    safe: { color: '#afd7af' }, // 151
    warning: { color: '#ffd787' }, // 222
    danger: { color: '#ff8787' }, // 210
    secondary: { color: '#b2b2b2' }, // 249
    accent: { color: '#ffd787' }, // 222
    barEmpty: { color: '#585858' }, // 240
  },
  // 37 white, 90 gray, 1;37 bold white
  minimal: {
    model: { color: 'white' },
    folder: { color: 'white' },
    branch: { color: 'white' },
    safe: { color: 'gray' },
    warning: { color: 'white' },
    danger: { color: 'white', bold: true },
    secondary: { color: 'gray' },
    accent: { color: 'white' },
    barEmpty: { color: 'gray' },
  },
  catppuccin: {
    model: { color: '#89b4fa' },
    folder: { color: '#f9e2af' },
    branch: { color: '#f5c2e7' },
    safe: { color: '#a6e3a1' },
    warning: { color: '#fab387' },
    danger: { color: '#f38ba8' },
    secondary: { color: '#7f849c' },
    accent: { color: '#f9e2af' },
    barEmpty: { color: '#45475a' },
  },
  dracula: {
    model: { color: '#bd93f9' },
    folder: { color: '#ffb86c' },
    branch: { color: '#ff79c6' },
    safe: { color: '#50fa7b' },
    warning: { color: '#f1fa8c' },
    danger: { color: '#ff5555' },
    secondary: { color: '#6272a4' },
    accent: { color: '#ffb86c' },
    barEmpty: { color: '#44475a' },
  },
  gruvbox: {
    model: { color: '#d79921' },
    folder: { color: '#fabd2f' },
    branch: { color: '#d3869b' },
    safe: { color: '#b8bb26' },
    warning: { color: '#fabd2f' },
    danger: { color: '#cc241d' },
    secondary: { color: '#a89984' },
    accent: { color: '#fabd2f' },
    barEmpty: { color: '#504945' },
  },
  nord: {
    model: { color: '#88c0d0' },
    folder: { color: '#ebcb8b' },
    branch: { color: '#b48ead' },
    safe: { color: '#a3be8c' },
    warning: { color: '#ebcb8b' },
    danger: { color: '#bf616a' },
    secondary: { color: '#4c566a' },
    accent: { color: '#ebcb8b' },
    barEmpty: { color: '#3b4252' },
  },
  tokyoNight: {
    model: { color: '#7aa2f7' },
    folder: { color: '#e0af68' },
    branch: { color: '#bb9af7' },
    safe: { color: '#9ece6a' },
    warning: { color: '#e0af68' },
    danger: { color: '#f7768e' },
    secondary: { color: '#565f89' },
    accent: { color: '#e0af68' },
    barEmpty: { color: '#3b4252' },
  },
  solarized: {
    model: { color: '#268bd2' },
    folder: { color: '#b58900' },
    branch: { color: '#d33682' },
    safe: { color: '#859900' },
    warning: { color: '#b58900' },
    danger: { color: '#dc322f' },
    secondary: { color: '#586e75' },
    accent: { color: '#b58900' },
    barEmpty: { color: '#586e75' },
  },
}

export function isThemeName(value: unknown): value is ThemeName {
  return typeof value === 'string' && (THEME_NAMES as readonly string[]).includes(value)
}

// Go getColorForPercent와 같은 경계 — 50 이하 safe, 80 이하 warning, 그 초과 danger
export function percentRole(percent: number): 'safe' | 'warning' | 'danger' {
  if (percent <= 50) return 'safe'
  if (percent <= 80) return 'warning'
  return 'danger'
}

// Go contextTokenWarn·contextTokenDanger와 같은 기준이다
const CONTEXT_TOKEN_WARN = 256_000
const CONTEXT_TOKEN_DANGER = 512_000

// 그 밖에는 Go 판의 기본색 대신 secondary다 — 터미널 기본 전경색이 테마 색 사이에서 혼자 튄다
export function tokenRole(tokens: number): 'secondary' | 'warning' | 'danger' {
  if (tokens >= CONTEXT_TOKEN_DANGER) return 'danger'
  if (tokens >= CONTEXT_TOKEN_WARN) return 'warning'
  return 'secondary'
}
