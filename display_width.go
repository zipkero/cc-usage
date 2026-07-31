package main

import "strings"

// runeRange is an inclusive [lo, hi] Unicode code point range.
type runeRange struct {
	lo, hi rune
}

// wideRuneRanges are the display-width-2 blocks this renderer recognizes:
// Hangul, CJK ideographs/symbols/compatibility, Kana, fullwidth forms, and
// the main emoji blocks (SPEC §5.7; ANALYSIS §5 D8). Sorted ascending by lo
// so runeWidth can stop scanning as soon as r is smaller than the current
// range's lo. Deliberately excludes Box Drawing (25xx region isn't here —
// see below), Geometric Shapes, Arrows, General Punctuation, and Dingbats:
// this renderer's own glyphs (◆ ◇ ○ ● ◈ ◎ █ ░ │ ↑ ↓ … ✓ ✗ ✎) live in those
// blocks and must keep their current width-1 rendering (East Asian
// Ambiguous, ANALYSIS §5 D8) — they fall through to runeWidth's default.
var wideRuneRanges = []runeRange{
	{0x1100, 0x115F},   // Hangul Jamo (leading consonants)
	{0x2E80, 0x303E},   // CJK Radicals Supplement .. CJK Symbols and Punctuation
	{0x3041, 0x33FF},   // Hiragana .. CJK Compatibility
	{0x3400, 0x4DBF},   // CJK Unified Ideographs Extension A
	{0x4E00, 0x9FFF},   // CJK Unified Ideographs
	{0xA000, 0xA4CF},   // Yi Syllables/Radicals
	{0xAC00, 0xD7A3},   // Hangul Syllables
	{0xF900, 0xFAFF},   // CJK Compatibility Ideographs
	{0xFE30, 0xFE4F},   // CJK Compatibility Forms
	{0xFF00, 0xFF60},   // Fullwidth Forms
	{0xFFE0, 0xFFE6},   // Fullwidth Signs
	{0x1F1E6, 0x1F1FF}, // Regional Indicator Symbols (flag pairs)
	{0x1F300, 0x1F5FF}, // Miscellaneous Symbols and Pictographs
	{0x1F600, 0x1F64F}, // Emoticons
	{0x1F680, 0x1F6FF}, // Transport and Map Symbols
	{0x1F900, 0x1F9FF}, // Supplemental Symbols and Pictographs
	{0x1FA70, 0x1FAFF}, // Symbols and Pictographs Extended-A
	{0x20000, 0x2FFFD}, // CJK Unified Ideographs Extension B and beyond (plane 2)
	{0x30000, 0x3FFFD}, // CJK Unified Ideographs Extension (plane 3)
}

// runeWidth reports r's terminal display width: 0 for C0 control characters
// and DEL (including the ESC that starts every escape sequence this file
// strips separately), 2 for the wideRuneRanges table, and 1 for everything
// else. The "everything else" default covers C1 control characters (no
// table entry, so they fall to 1) and Unicode's East Asian Ambiguous
// category too, which is where this renderer's own glyphs live — most
// terminals render Ambiguous as narrow, so leaving it at the default keeps
// their width unchanged (ANALYSIS §5 D8). Runes a real wide-aware terminal
// renders at 2 columns but that aren't in the table (rare CJK extension
// blocks, niche emoji not listed above) fall back to 1 here — a documented
// approximation, not a wcwidth-complete implementation; the zero-dependency
// constraint (SPEC §3) rules out pulling in one.
func runeWidth(r rune) int {
	if r < 0x20 || r == 0x7F {
		return 0
	}
	for _, rg := range wideRuneRanges {
		if r < rg.lo {
			break
		}
		if r <= rg.hi {
			return 2
		}
	}
	return 1
}

// nextDisplayToken returns the token starting at runes[i]: an ANSI escape
// sequence — a CSI sequence (ESC '[' ... final byte in 0x40-0x7E, e.g. SGR
// color codes) or an OSC sequence (ESC ']' ... terminated by BEL or ST/ESC
// '\', e.g. osc8Link's hyperlinks) — with width 0, or else a single visible
// rune with its runeWidth. next is always > i, so callers can loop without
// risking an infinite loop; an escape sequence missing its terminator simply
// runs to the end of the string and still counts as width 0.
func nextDisplayToken(runes []rune, i int) (token string, width int, next int) {
	if runes[i] == '\x1b' && i+1 < len(runes) {
		switch runes[i+1] {
		case '[':
			j := i + 2
			for j < len(runes) && !(runes[j] >= 0x40 && runes[j] <= 0x7E) {
				j++
			}
			if j < len(runes) {
				j++ // consume the final byte
			}
			return string(runes[i:j]), 0, j
		case ']':
			j := i + 2
			for j < len(runes) {
				if runes[j] == '\x07' {
					j++
					break
				}
				if runes[j] == '\x1b' && j+1 < len(runes) && runes[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			return string(runes[i:j]), 0, j
		}
	}
	return string(runes[i]), runeWidth(runes[i]), i + 1
}

// displayWidth reports s's terminal display width: ANSI escape sequences
// (CSI color codes, OSC 8 hyperlinks) count as 0, wide runes count as 2 per
// wideRuneRanges, and everything else counts as 1 (SPEC §5.7; ANALYSIS §5
// D8). This is the measurement layer the line-fitting step below relies on.
func displayWidth(s string) int {
	runes := []rune(s)
	total := 0
	for i := 0; i < len(runes); {
		_, w, next := nextDisplayToken(runes, i)
		total += w
		i = next
	}
	return total
}

// truncateToWidth cuts s to at most maxWidth display columns, always
// appending a single-column ellipsis ("…") plus RESET. RESET is
// unconditional so a cut never leaves a color code open, even when s had no
// open color to begin with (SPEC §5.7; ANALYSIS §5 D7). Escape sequences
// encountered before the cut point are preserved in full — dropping one
// mid-sequence would leak its raw bytes onto the line — so the visible
// budget is maxWidth-1 (reserving 1 column for the ellipsis) while escape
// sequences themselves don't consume any of it.
//
// This is a display-width analog of format.go's truncate, which counts
// runes instead of display columns; the two serve different truncation
// needs (path/name budgets vs. terminal-width fitting) and neither replaces
// the other (ANALYSIS §5 D8).
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return RESET
	}
	budget := maxWidth - 1
	runes := []rune(s)

	var b strings.Builder
	used := 0
	for i := 0; i < len(runes); {
		tok, w, next := nextDisplayToken(runes, i)
		if w > 0 && used+w > budget {
			break
		}
		b.WriteString(tok)
		used += w
		i = next
	}
	b.WriteString("…")
	b.WriteString(RESET)
	return b.String()
}

// fitLineWidth adjusts an already-rendered line to fit within maxWidth
// display columns. parts holds each widget's rendered output for the line
// and sep is the separator orchestrate() joins them with; this function
// never re-invokes a widget's GetData/Render, so the Widget contract stays
// untouched (ANALYSIS §5 D7 옵션 a). It drops whole widgets from the right
// one at a time — SPEC §5.3 keeps the leftmost widgets (path, model,
// context) the priority order the user wrote in lines/preset — and only
// truncates mid-widget as a last resort, when a single remaining widget
// still doesn't fit. Returns the fitted line and how many parts survived (a
// truncated lone widget still counts as 1).
func fitLineWidth(parts []string, sep string, maxWidth int) (string, int) {
	line := strings.Join(parts, sep)
	for len(parts) > 1 && displayWidth(line) > maxWidth {
		parts = parts[:len(parts)-1]
		line = strings.Join(parts, sep)
	}
	if displayWidth(line) <= maxWidth {
		return line, len(parts)
	}
	return truncateToWidth(line, maxWidth), len(parts)
}
