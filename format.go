package main

import (
	"fmt"
	"time"
	"unicode/utf8"
)

func formatTokens(tokens int) string {
	if tokens < 0 {
		return "0"
	}
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1000000 {
		v := float64(tokens) / 1000.0
		if v < 10 {
			return fmt.Sprintf("%.1fK", v)
		}
		return fmt.Sprintf("%.0fK", v)
	}
	v := float64(tokens) / 1000000.0
	if v < 10 {
		return fmt.Sprintf("%.1fM", v)
	}
	return fmt.Sprintf("%.0fM", v)
}

func formatCost(cost float64) string {
	return fmt.Sprintf("$%.2f", cost)
}

// formatTimeRemaining reports the remaining time until resetAt as a display
// string and a bool telling the caller whether there is anything to show.
// The bool is false — and the string empty — when resetAt is the Go zero
// time (reset time unknown) or when the total whole minutes remaining is
// <= 0 (already past, or less than 60 seconds away). The threshold is
// int(diff.Minutes()) > 0 rather than diff > 0 so that sub-minute future
// gaps stay suppressed, matching the previous "0"+Minutes behavior for that
// window. Callers must use the bool, not the string contents, to decide
// whether to display the suffix.
func formatTimeRemaining(resetAt time.Time, now time.Time, t *Translations) (string, bool) {
	if resetAt.IsZero() {
		return "", false
	}
	diff := resetAt.Sub(now)
	totalMinutes := int(diff.Minutes())
	if totalMinutes <= 0 {
		return "", false
	}
	days := totalMinutes / (60 * 24)
	hours := (totalMinutes % (60 * 24)) / 60
	minutes := totalMinutes % 60

	if days > 0 {
		return fmt.Sprintf("%d%s %d%s", days, t.Time.Days, hours, t.Time.Hours), true
	}
	if hours > 0 {
		return fmt.Sprintf("%d%s%d%s", hours, t.Time.Hours, minutes, t.Time.Minutes), true
	}
	return fmt.Sprintf("%d%s", minutes, t.Time.Minutes), true
}

func calculatePercent(current, total int) int {
	if total <= 0 {
		return 0
	}
	p := current * 100 / total
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	count := utf8.RuneCountInString(s)
	if count <= maxLen {
		return s
	}
	runes := []rune(s)
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

func clampPercent(value float64) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return int(value)
}

func osc8Link(url, text string) string {
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, text)
}
