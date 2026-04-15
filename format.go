package main

import (
	"fmt"
	"strings"
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

func formatTimeRemaining(resetAt time.Time, now time.Time, t *Translations) string {
	diff := resetAt.Sub(now)
	if diff <= 0 {
		return "0" + t.Time.Minutes
	}
	totalMinutes := int(diff.Minutes())
	days := totalMinutes / (60 * 24)
	hours := (totalMinutes % (60 * 24)) / 60
	minutes := totalMinutes % 60

	if days > 0 {
		return fmt.Sprintf("%d%s %d%s", days, t.Time.Days, hours, t.Time.Hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d%s%d%s", hours, t.Time.Hours, minutes, t.Time.Minutes)
	}
	return fmt.Sprintf("%d%s", minutes, t.Time.Minutes)
}

func formatDuration(ms int64) string {
	if ms <= 0 {
		return "0m"
	}
	totalMinutes := int(ms / 60000)
	hours := totalMinutes / 60
	minutes := totalMinutes % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func shortenModelName(displayName string) string {
	// displayName is already short like "Opus", "Sonnet", "Haiku"
	// Handle longer forms like "Claude 3.5 Sonnet" → "Sonnet"
	known := []string{"Opus", "Sonnet", "Haiku"}
	for _, k := range known {
		if strings.Contains(displayName, k) {
			return k
		}
	}
	return displayName
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
