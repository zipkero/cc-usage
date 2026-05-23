package main

import (
	"fmt"
)

// 분석 위젯들. stdin 페이로드(cost.*, context_window.current_usage.*, version)에 이미 들어있는
// 값만 활용. transcript 파싱이나 새 캐시 인프라는 사용하지 않는다.

// --- version widget ---

type versionWidget struct{}

func (w versionWidget) ID() string { return "version" }

func (w versionWidget) GetData(ctx *Context) (any, error) {
	if ctx.Stdin.Version == "" {
		return nil, nil
	}
	return ctx.Stdin.Version, nil
}

func (w versionWidget) Render(data any, ctx *Context) string {
	v := data.(string)
	theme := getTheme(ctx.Config.Theme)
	return fmt.Sprintf("%sv%s%s", theme.Dim, v, RESET)
}

// --- apiDuration widget ---

type apiDurationWidget struct{}

func (w apiDurationWidget) ID() string { return "apiDuration" }

func (w apiDurationWidget) GetData(ctx *Context) (any, error) {
	ms := ctx.Stdin.Cost.TotalApiDurationMs
	if ms == nil || *ms <= 0 {
		return nil, nil
	}
	return *ms, nil
}

func (w apiDurationWidget) Render(data any, ctx *Context) string {
	ms := data.(int64)
	theme := getTheme(ctx.Config.Theme)
	return fmt.Sprintf("%s%s: %s%s%s",
		theme.Secondary, ctx.Translations.Widgets.ApiDuration,
		theme.Accent, formatDuration(ms), RESET)
}

// --- sessionDuration widget ---

type sessionDurationWidget struct{}

func (w sessionDurationWidget) ID() string { return "sessionDuration" }

func (w sessionDurationWidget) GetData(ctx *Context) (any, error) {
	ms := ctx.Stdin.Cost.TotalDurationMs
	if ms == nil || *ms <= 0 {
		return nil, nil
	}
	return *ms, nil
}

func (w sessionDurationWidget) Render(data any, ctx *Context) string {
	ms := data.(int64)
	theme := getTheme(ctx.Config.Theme)
	return fmt.Sprintf("%s%s: %s%s%s",
		theme.Secondary, ctx.Translations.Widgets.Session,
		theme.Accent, formatDuration(ms), RESET)
}

// --- burnRate widget ---

type burnRateWidget struct{}

func (w burnRateWidget) ID() string { return "burnRate" }

func (w burnRateWidget) GetData(ctx *Context) (any, error) {
	cost := ctx.Stdin.Cost.TotalCostUsd
	ms := ctx.Stdin.Cost.TotalDurationMs
	if cost <= 0 || ms == nil || *ms <= 0 {
		return nil, nil
	}
	// $/hour: cost / (ms / 3,600,000)
	hours := float64(*ms) / 3_600_000.0
	return cost / hours, nil
}

func (w burnRateWidget) Render(data any, ctx *Context) string {
	rate := data.(float64)
	theme := getTheme(ctx.Config.Theme)
	return fmt.Sprintf("%s%s: %s$%.2f/h%s",
		theme.Secondary, ctx.Translations.Widgets.BurnRate,
		theme.Accent, rate, RESET)
}

// --- cacheHit widget ---

type cacheHitWidget struct{}

func (w cacheHitWidget) ID() string { return "cacheHit" }

func (w cacheHitWidget) GetData(ctx *Context) (any, error) {
	u := ctx.Stdin.ContextWindow.CurrentUsage
	total := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	if total <= 0 {
		return nil, nil
	}
	return calculatePercent(u.CacheReadInputTokens, total), nil
}

func (w cacheHitWidget) Render(data any, ctx *Context) string {
	pct := data.(int)
	theme := getTheme(ctx.Config.Theme)
	// 캐시 hit은 높을수록 좋음 — getColorForPercent는 "높을수록 위험" 기준이라 반전
	color := theme.Safe
	if pct < 50 {
		color = theme.Warning
	}
	if pct < 20 {
		color = theme.Danger
	}
	return fmt.Sprintf("%s%s: %s%d%%%s",
		theme.Secondary, ctx.Translations.Widgets.Cache,
		color, pct, RESET)
}

// --- performance widget ---

type performanceWidget struct{}

func (w performanceWidget) ID() string { return "performance" }

func (w performanceWidget) GetData(ctx *Context) (any, error) {
	api := ctx.Stdin.Cost.TotalApiDurationMs
	total := ctx.Stdin.Cost.TotalDurationMs
	if api == nil || total == nil || *total <= 0 {
		return nil, nil
	}
	return calculatePercent(int(*api), int(*total)), nil
}

func (w performanceWidget) Render(data any, ctx *Context) string {
	pct := data.(int)
	theme := getTheme(ctx.Config.Theme)
	// API time 비율 — 낮을수록 사용자/도구 처리에 시간 더 씀. 평가 중립.
	return fmt.Sprintf("%s%s: %s%d%%%s",
		theme.Secondary, ctx.Translations.Widgets.Performance,
		theme.Info, pct, RESET)
}

// --- registration ---

func init() {
	registerWidget(versionWidget{})
	registerWidget(apiDurationWidget{})
	registerWidget(sessionDurationWidget{})
	registerWidget(burnRateWidget{})
	registerWidget(cacheHitWidget{})
	registerWidget(performanceWidget{})
}
