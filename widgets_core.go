package main

import (
	"fmt"
	"strings"
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

	name := shortenModelName(d.DisplayName)
	if name == "" {
		name = d.ID
	}

	return fmt.Sprintf("%s%s %s%s", theme.Model, emoji, name, RESET)
}

// --- context widget ---

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
	bar := renderProgressBar(d.Percent, theme)
	color := getColorForPercent(d.Percent, theme)
	return fmt.Sprintf("%s %s%d%%%s %s", bar, color, d.Percent, RESET, formatTokens(d.TotalTokens))
}

// --- cost widget ---

type costWidget struct{}

func (w costWidget) ID() string { return "cost" }

func (w costWidget) GetData(ctx *Context) (any, error) {
	return ctx.Stdin.Cost.TotalCostUsd, nil
}

func (w costWidget) Render(data any, ctx *Context) string {
	cost := data.(float64)
	theme := getTheme(ctx.Config.Theme)
	return fmt.Sprintf("%s%s%s", theme.Accent, formatCost(cost), RESET)
}

// --- registration ---

func init() {
	registerWidget(modelWidget{})
	registerWidget(contextWidget{})
	registerWidget(costWidget{})
}
