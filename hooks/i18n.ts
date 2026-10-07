// rate limit 라벨과 남은 시간 단위의 en/ko 표, 로캘 판정, 남은 시간 포맷.
// $·런타임 import 없는 순수 계층이라 테스트가 직접 import한다.

// Go 판 config.go의 language 선택지와 같다
export const LANGUAGE_OPTIONS = ['auto', 'en', 'ko'] as const

export type LanguageOption = (typeof LANGUAGE_OPTIONS)[number]

export type Language = Exclude<LanguageOption, 'auto'>

export const DEFAULT_LANGUAGE: LanguageOption = 'auto'

export function isLanguageOption(value: unknown): value is LanguageOption {
  return typeof value === 'string' && (LANGUAGE_OPTIONS as readonly string[]).includes(value)
}

type Translation = {
  labels: { fiveH: string; sevenD: string }
  time: { days: string; hours: string; minutes: string }
}

// deprecated-go locales/{en,ko}.json에서 band가 쓰는 항목만 옮겼다
export const TRANSLATIONS: Readonly<Record<Language, Translation>> = {
  en: { labels: { fiveH: '5h', sevenD: '7d' }, time: { days: 'd', hours: 'h', minutes: 'm' } },
  ko: { labels: { fiveH: '5시간', sevenD: '7일' }, time: { days: '일', hours: '시간', minutes: '분' } },
}

// Go 판 detectLanguage와 같다 — LC_ALL, LC_MESSAGES, LANG 값 중 하나라도 ko로 시작하면 한국어
export function detectLanguage(values: (string | undefined)[]): Language {
  return values.some((v) => v?.startsWith('ko')) ? 'ko' : 'en'
}

// auto면 로캘 값으로 판정하고, en·ko는 그대로 쓴다
export function resolveLanguage(option: LanguageOption, locales: (string | undefined)[]): Language {
  return option === 'auto' ? detectLanguage(locales) : option
}

// format.go의 formatTimeRemaining과 같은 규칙 — 1분 미만이나 지난 시각은 빈 문자열이라 생략된다
export function formatTimeRemaining(ms: number, lang: Language): string {
  const total = Math.floor(ms / 60_000)
  if (!(total > 0)) return ''
  const t = TRANSLATIONS[lang].time
  const days = Math.floor(total / (60 * 24))
  const hours = Math.floor((total % (60 * 24)) / 60)
  const minutes = total % 60
  if (days > 0) return days + t.days + ' ' + hours + t.hours
  if (hours > 0) return hours + t.hours + minutes + t.minutes
  return minutes + t.minutes
}
