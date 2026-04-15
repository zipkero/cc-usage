package main

import "strings"

// Context holds all data needed by widgets.
type Context struct {
	Stdin        StdinInput
	Config       Config
	ConfigDir    string
	Translations any // TODO: i18n 구현 시 타입 교체
	RateLimits   *UsageLimits
}

// Widget is the interface all widgets must implement.
type Widget interface {
	ID() string
	GetData(ctx *Context) (any, error)
	Render(data any, ctx *Context) string
}

// registry holds all registered widgets by ID.
var registry = map[string]Widget{}

func registerWidget(w Widget) {
	registry[w.ID()] = w
}

// displayPresets defines built-in widget layouts.
var displayPresets = map[string][][]string{
	"compact": {
		{"model", "context", "cost", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"},
	},
	"normal": {
		{"model", "context", "cost", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"},
		{"projectInfo", "sessionDuration", "burnRate", "todoProgress"},
	},
	"detailed": {
		{"model", "context", "cost", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"},
		{"projectInfo", "sessionName", "sessionDuration", "burnRate", "tokenSpeed", "depletionTime", "todoProgress"},
		{"configCounts", "toolActivity", "agentStatus", "cacheHit", "performance"},
	},
}

// presetCharToWidget maps single characters to widget IDs for compact preset notation.
var presetCharToWidget = map[byte]string{
	'M': "model",
	'C': "context",
	'$': "cost",
	'R': "rateLimit5h",
	'7': "rateLimit7d",
	'S': "rateLimit7dSonnet",
	'P': "projectInfo",
	'I': "sessionId",
	'D': "sessionDuration",
	'K': "configCounts",
	'F': "performance",
	'U': "budget",
	'T': "toolActivity",
	'A': "agentStatus",
	'O': "todoProgress",
	'B': "burnRate",
	'E': "depletionTime",
	'H': "cacheHit",
	'X': "codexUsage",
	'G': "geminiUsage",
	'Z': "zaiUsage",
	'N': "tokenBreakdown",
	'W': "forecast",
	'V': "version",
	'L': "linesChanged",
	'Y': "outputStyle",
	'Q': "tokenSpeed",
	'J': "sessionName",
	'@': "todayCost",
	'?': "lastPrompt",
	'm': "vimMode",
	'a': "apiDuration",
	'p': "peakHours",
	'g': "geminiUsageAll",
	'i': "sessionIdFull",
}

// resolvePreset parses Config.Preset into Config.Lines.
func resolvePreset(config *Config) {
	if config.Preset == "" {
		return
	}
	lineParts := strings.Split(config.Preset, "|")
	var lines [][]string
	for _, part := range lineParts {
		var widgets []string
		for j := 0; j < len(part); j++ {
			if widgetID, ok := presetCharToWidget[part[j]]; ok {
				widgets = append(widgets, widgetID)
			} else {
				debugLog("widget", "unknown preset char: %c", part[j])
			}
		}
		if len(widgets) > 0 {
			lines = append(lines, widgets)
		}
	}
	config.Lines = lines
	config.DisplayMode = "custom"
}

// orchestrate runs all widgets according to the display configuration.
func orchestrate(ctx *Context) []string {
	resolvePreset(&ctx.Config)

	var lines [][]string
	if ctx.Config.DisplayMode == "custom" && len(ctx.Config.Lines) > 0 {
		lines = ctx.Config.Lines
	} else if preset, ok := displayPresets[ctx.Config.DisplayMode]; ok {
		lines = preset
	} else {
		lines = displayPresets["compact"]
	}

	// Build disabled set
	disabled := make(map[string]bool)
	for _, id := range ctx.Config.DisabledWidgets {
		disabled[id] = true
	}

	var output []string
	for _, line := range lines {
		var parts []string
		for _, widgetID := range line {
			if disabled[widgetID] {
				continue
			}
			w, ok := registry[widgetID]
			if !ok {
				debugLog("widget", "unknown widget: %s", widgetID)
				continue
			}
			data, err := w.GetData(ctx)
			if err != nil || data == nil {
				continue
			}
			rendered := w.Render(data, ctx)
			if rendered != "" {
				parts = append(parts, rendered)
			}
		}
		if len(parts) > 0 {
			sep := renderSeparator(ctx.Config.Separator, getTheme(ctx.Config.Theme))
			output = append(output, strings.Join(parts, sep))
		}
	}
	return output
}
