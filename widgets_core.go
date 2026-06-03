package main

import (
	"fmt"
	"strings"
	"time"
)

// --- model widget ---

type modelWidget struct{}

type modelData struct {
	ID          string
	DisplayName string
}

func (w modelWidget) ID() string { return "model" }

func (w modelWidget) GetData(ctx *Context) (any, error) {
	if ctx.Stdin.Model.ID == "" && ctx.Stdin.Model.DisplayName == "" {
		return nil, nil
	}
	return &modelData{
		ID:          ctx.Stdin.Model.ID,
		DisplayName: ctx.Stdin.Model.DisplayName,
	}, nil
}

func (w modelWidget) Render(data any, ctx *Context) string {
	d := data.(*modelData)
	theme := getTheme(ctx.Config.Theme)

	emoji := "●"
	idLower := strings.ToLower(d.ID)
	if strings.Contains(idLower, "opus") {
		emoji = "◆"
	} else if strings.Contains(idLower, "sonnet") {
		emoji = "◇"
	} else if strings.Contains(idLower, "haiku") {
		emoji = "○"
	}

	name := d.ID
	if name == "" {
		name = d.DisplayName
	}

	return fmt.Sprintf("%s%s %s%s", theme.Model, emoji, name, RESET)
}

// --- context widget ---

const (
	contextTokenWarn   = 256_000
	contextTokenDanger = 512_000
)

type contextWidget struct{}

type contextData struct {
	Percent     int
	TotalTokens int
}

func (w contextWidget) ID() string { return "context" }

func (w contextWidget) GetData(ctx *Context) (any, error) {
	cw := ctx.Stdin.ContextWindow
	if cw.ContextWindowSize <= 0 {
		return nil, nil
	}

	var percent int
	if cw.UsedPercentage != nil {
		percent = *cw.UsedPercentage
	} else {
		total := cw.TotalInputTokens + cw.TotalOutputTokens
		percent = calculatePercent(total, cw.ContextWindowSize)
	}

	totalTokens := cw.TotalInputTokens + cw.TotalOutputTokens

	return &contextData{
		Percent:     percent,
		TotalTokens: totalTokens,
	}, nil
}

func (w contextWidget) Render(data any, ctx *Context) string {
	d := data.(*contextData)
	theme := getTheme(ctx.Config.Theme)
	bar := renderProgressBar(d.Percent, ctx.Config.ContextBarWidth(), theme)
	color := getColorForPercent(d.Percent, theme)

	var tokenColor, tokenReset string
	switch {
	case d.TotalTokens >= contextTokenDanger:
		tokenColor = theme.Danger
	case d.TotalTokens >= contextTokenWarn:
		tokenColor = theme.Warning
	}
	if tokenColor != "" {
		tokenReset = RESET
	}

	return fmt.Sprintf("%s %s%d%%%s %s%s%s", bar, color, d.Percent, RESET, tokenColor, formatTokens(d.TotalTokens), tokenReset)
}

// --- cost widget ---

type costWidget struct{}

func (w costWidget) ID() string { return "cost" }

func (w costWidget) GetData(ctx *Context) (any, error) {
	cost := ctx.Stdin.Cost.TotalCostUsd
	if cost < 0 {
		cost = 0
	}
	return cost, nil
}

func (w costWidget) Render(data any, ctx *Context) string {
	cost := data.(float64)
	theme := getTheme(ctx.Config.Theme)
	return fmt.Sprintf("%s%s%s", theme.Accent, formatCost(cost), RESET)
}

// --- rateLimit5h widget ---

type rateLimit5hWidget struct{}

type rateLimitData struct {
	Percent  int
	ResetsAt time.Time
}

func (w rateLimit5hWidget) ID() string { return "rateLimit5h" }

func (w rateLimit5hWidget) GetData(ctx *Context) (any, error) {
	if ctx.Stdin.RateLimits != nil && ctx.Stdin.RateLimits.FiveHour != nil {
		rl := ctx.Stdin.RateLimits.FiveHour
		return &rateLimitData{
			Percent:  clampPercent(float64(rl.UsedPercentage)),
			ResetsAt: time.Unix(rl.ResetsAt, 0),
		}, nil
	}
	return nil, nil
}

func (w rateLimit5hWidget) Render(data any, ctx *Context) string {
	return renderRateLimit(data, ctx.Translations.Labels.FiveH, ctx)
}

// --- rateLimit7d widget ---

type rateLimit7dWidget struct{}

func (w rateLimit7dWidget) ID() string { return "rateLimit7d" }

func (w rateLimit7dWidget) GetData(ctx *Context) (any, error) {
	if ctx.Stdin.RateLimits != nil && ctx.Stdin.RateLimits.SevenDay != nil {
		rl := ctx.Stdin.RateLimits.SevenDay
		return &rateLimitData{
			Percent:  clampPercent(float64(rl.UsedPercentage)),
			ResetsAt: time.Unix(rl.ResetsAt, 0),
		}, nil
	}
	return nil, nil
}

func (w rateLimit7dWidget) Render(data any, ctx *Context) string {
	return renderRateLimit(data, ctx.Translations.Labels.SevenD, ctx)
}

// renderRateLimit is the shared render logic for all rate limit widgets.
func renderRateLimit(data any, label string, ctx *Context) string {
	d := data.(*rateLimitData)
	theme := getTheme(ctx.Config.Theme)

	color := getColorForPercent(d.Percent, theme)
	result := fmt.Sprintf("%s%s: %s%d%%%s", theme.Secondary, label, color, d.Percent, RESET)

	if !d.ResetsAt.IsZero() {
		remaining := formatTimeRemaining(d.ResetsAt, time.Now(), ctx.Translations)
		if remaining != "0"+ctx.Translations.Time.Minutes {
			result += fmt.Sprintf(" %s(%s)%s", theme.Dim, remaining, RESET)
		}
	}

	return result
}

// --- registration ---

func init() {
	registerWidget(modelWidget{})
	registerWidget(contextWidget{})
	registerWidget(costWidget{})
	registerWidget(rateLimit5hWidget{})
	registerWidget(rateLimit7dWidget{})
}
