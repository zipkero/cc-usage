// 터미널 표시폭 계산과 band 폭에 맞춘 줄 맞춤.
// $·런타임 import 없는 순수 계층이라 테스트가 직접 import한다.
import type { Part } from './widgets'

// Go 판 display_width.go의 wideRuneRanges와 같은 2칸 범위다(lo 오름차순, 양끝 포함).
// Box Drawing·Geometric Shapes·General Punctuation은 넣지 않는다 — band가 쓰는 ◆ █ ░ │ › · …는
// East Asian Ambiguous라 대부분의 터미널이 1칸으로 그린다.
const WIDE_RANGES: readonly (readonly [number, number])[] = [
  [0x1100, 0x115f], // Hangul Jamo (초성)
  [0x2e80, 0x303e], // CJK Radicals Supplement .. CJK Symbols and Punctuation
  [0x3041, 0x33ff], // Hiragana .. CJK Compatibility
  [0x3400, 0x4dbf], // CJK Unified Ideographs Extension A
  [0x4e00, 0x9fff], // CJK Unified Ideographs
  [0xa000, 0xa4cf], // Yi Syllables/Radicals
  [0xac00, 0xd7a3], // Hangul Syllables
  [0xf900, 0xfaff], // CJK Compatibility Ideographs
  [0xfe30, 0xfe4f], // CJK Compatibility Forms
  [0xff00, 0xff60], // Fullwidth Forms
  [0xffe0, 0xffe6], // Fullwidth Signs
  [0x1f1e6, 0x1f1ff], // Regional Indicator Symbols
  [0x1f300, 0x1f5ff], // Miscellaneous Symbols and Pictographs
  [0x1f600, 0x1f64f], // Emoticons
  [0x1f680, 0x1f6ff], // Transport and Map Symbols
  [0x1f900, 0x1f9ff], // Supplemental Symbols and Pictographs
  [0x1fa70, 0x1faff], // Symbols and Pictographs Extended-A
  [0x20000, 0x2fffd], // CJK Unified Ideographs Extension B 이후 (plane 2)
  [0x30000, 0x3fffd], // CJK Unified Ideographs Extension (plane 3)
]

const ELLIPSIS = '…'

// C0 제어문자와 DEL은 0칸, WIDE_RANGES는 2칸, 나머지는 1칸이다.
// 표에 없는 드문 2칸 문자는 1칸으로 잰다 — Go 판과 같은 근사이고, 넘친 몫은 바깥 Text의 truncate-end가 받는다.
function codePointWidth(cp: number): number {
  if (cp < 0x20 || cp === 0x7f) return 0
  for (const [lo, hi] of WIDE_RANGES) {
    if (cp < lo) break
    if (cp <= hi) return 2
  }
  return 1
}

export function displayWidth(text: string): number {
  let total = 0
  for (const ch of text) total += codePointWidth(ch.codePointAt(0)!)
  return total
}

function partsWidth(parts: readonly Part[]): number {
  return parts.reduce((sum, p) => sum + displayWidth(p.text), 0)
}

// 위젯 사이에 구분자 조각을 넣는다 — 구분자 조각은 줄마다 새로 복사해 서로 공유하지 않는다
function joinWidgets(widgets: readonly Part[][], separator: readonly Part[]): Part[] {
  const line: Part[] = []
  for (const parts of widgets) {
    if (line.length > 0) line.push(...separator.map((p) => ({ ...p })))
    line.push(...parts)
  }
  return line
}

// Go 판 fitLineWidth와 같은 순서다 — 표시폭이 budget을 넘으면 오른쪽 위젯을 앞 구분자와 함께 하나씩 빼고,
// 하나만 남아도 넘치면 표시폭 기준으로 자른다.
export function fitLine(widgets: readonly Part[][], separator: readonly Part[], budget: number): Part[] {
  let count = widgets.length
  let line = joinWidgets(widgets, separator)
  while (count > 1 && partsWidth(line) > budget) {
    count--
    line = joinWidgets(widgets.slice(0, count), separator)
  }
  return partsWidth(line) <= budget ? line : truncateParts(line, budget)
}

// budget-1칸 안에 들어가는 앞부분만 남기고 …를 붙인다(Go 판 truncateToWidth).
// 2칸 문자가 남은 칸에 다 들어가지 않으면 그 문자부터 버려 반으로 잘리지 않는다.
// …는 잘린 조각의 색을 그대로 이어받는다 — Go 판에서 …가 그 자리의 색 코드 뒤에 붙던 것과 같다.
function truncateParts(parts: readonly Part[], budget: number): Part[] {
  if (budget <= 0) return []
  const room = budget - 1
  const kept: Part[] = []
  let used = 0
  for (const part of parts) {
    let text = ''
    for (const ch of part.text) {
      const w = codePointWidth(ch.codePointAt(0)!)
      if (w > 0 && used + w > room) {
        kept.push({ ...part, text: text + ELLIPSIS })
        return kept
      }
      text += ch
      used += w
    }
    kept.push({ ...part, text })
  }
  // 줄 전체가 room 안에 들었다면 budget도 넘지 않았으므로 여기 오지 않는다
  kept.push({ text: ELLIPSIS })
  return kept
}
