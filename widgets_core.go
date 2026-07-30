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
	// Placeholder가 true면 첫 API 응답이 아직 도착하지 않은 상태다. zero
	// value(false)가 "실측"을 뜻해야 한다 — 기존 리터럴 &contextData{Percent:
	// ..., TotalTokens: ...} 4곳이 이 필드를 채우지 않고도 실측 케이스로
	// 남아야 하기 때문이다(ANALYSIS §5 D2).
	Placeholder bool
}

func (w contextWidget) ID() string { return "context" }

func (w contextWidget) GetData(ctx *Context) (any, error) {
	cw := ctx.Stdin.ContextWindow
	if cw.ContextWindowSize <= 0 {
		return nil, nil
	}

	var percent int
	if cw.UsedPercentage != nil {
		// Claude Code는 used_percentage를 소수(예: 8.4)로 보낼 수 있다 — clamp 후 정수 절삭.
		percent = clampPercent(*cw.UsedPercentage)
	} else {
		total := cw.TotalInputTokens + cw.TotalOutputTokens
		percent = calculatePercent(total, cw.ContextWindowSize)
	}

	totalTokens := cw.TotalInputTokens + cw.TotalOutputTokens

	// placeholder 상태에서도 퍼센트·토큰 계산은 그대로 수행한다 — 표시하지
	// 않을 뿐 값 산출 자체는 바꾸지 않는다(ANALYSIS §5 D2).
	return &contextData{
		Percent:     percent,
		TotalTokens: totalTokens,
		Placeholder: !ctx.FirstResponseReceived(),
	}, nil
}

func (w contextWidget) Render(data any, ctx *Context) string {
	d := data.(*contextData)
	theme := getTheme(ctx.Config.Theme)

	if d.Placeholder {
		// 빈 bar + 흐린 placeholder 하나. 퍼센트·토큰 자리를 따로 두지
		// 않는다 — 둘 다 같은 미측정에서 나온 값이라 나눠 보여주면 없는
		// 정보를 두 번 표시하게 된다(ANALYSIS §5 D3). bar는 기존
		// renderProgressBar(0, ...)를 그대로 쓰고 dim을 적용하지 않는다 —
		// 빈 bar는 이미 BarEmpty 색으로 muted 상태다(ANALYSIS §5 D4/D6).
		bar := renderProgressBar(0, ctx.Config.ContextBarWidth(), theme)
		return fmt.Sprintf("%s %s", bar, renderDimmed(placeholderChar, theme))
	}

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
	// Placeholder가 true면 이 rate limit이 아직 도착하지 않았고 첫 API
	// 응답도 아직 오지 않은 상태다. zero value(false)가 "실측"을 뜻해야
	// 한다 — contextData.Placeholder와 같은 극성이다(ANALYSIS §5 D2).
	Placeholder bool
}

func (w rateLimit5hWidget) ID() string { return "rateLimit5h" }

func (w rateLimit5hWidget) GetData(ctx *Context) (any, error) {
	if ctx.Stdin.RateLimits != nil && ctx.Stdin.RateLimits.FiveHour != nil {
		rl := ctx.Stdin.RateLimits.FiveHour
		return &rateLimitData{
			Percent:  clampPercent(rl.UsedPercentage),
			ResetsAt: time.Unix(rl.ResetsAt, 0),
		}, nil
	}
	// 포인터가 없어도 첫 응답이 이미 도착했으면 지금처럼 칸을 생략한다 —
	// rate_limits가 영구 부재인 비구독 계정에서 placeholder가 세션 내내
	// 고착되는 것을 막는다(ANALYSIS §5 D1, D2).
	if ctx.FirstResponseReceived() {
		return nil, nil
	}
	return &rateLimitData{Placeholder: true}, nil
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
			Percent:  clampPercent(rl.UsedPercentage),
			ResetsAt: time.Unix(rl.ResetsAt, 0),
		}, nil
	}
	if ctx.FirstResponseReceived() {
		return nil, nil
	}
	return &rateLimitData{Placeholder: true}, nil
}

func (w rateLimit7dWidget) Render(data any, ctx *Context) string {
	return renderRateLimit(data, ctx.Translations.Labels.SevenD, ctx)
}

// renderRateLimit is the shared render logic for all rate limit widgets.
func renderRateLimit(data any, label string, ctx *Context) string {
	d := data.(*rateLimitData)
	theme := getTheme(ctx.Config.Theme)

	if d.Placeholder {
		// 라벨 + ": " + placeholder 전체를 흐리게 감싼다 — rate limit
		// placeholder는 값 한 글자만이 아니라 칸 전체를 dim으로 낸다는
		// 점에서 context placeholder(bar는 dim 제외)와 다르다
		// (ANALYSIS §5 D4). placeholder 상태에는 reset 시각이 없으므로
		// formatTimeRemaining 경로를 타지 않는다(§5 D3).
		return renderDimmed(fmt.Sprintf("%s: %s", label, placeholderChar), theme)
	}

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
