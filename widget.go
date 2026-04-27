package main

import (
	_ "embed"
	"encoding/json"
	"os"
	"strings"
)

//go:embed locales/en.json
var localeEN []byte

//go:embed locales/ko.json
var localeKO []byte

// Translations holds all translatable strings.
type Translations struct {
	Model struct {
		Opus   string `json:"opus"`
		Sonnet string `json:"sonnet"`
		Haiku  string `json:"haiku"`
	} `json:"model"`
	Labels struct {
		FiveH        string `json:"fiveH"`
		SevenD       string `json:"sevenD"`
		SevenDAll    string `json:"sevenDAll"`
		SevenDSonnet string `json:"sevenDSonnet"`
		OneM         string `json:"oneM"`
	} `json:"labels"`
	Time struct {
		Days    string `json:"days"`
		Hours   string `json:"hours"`
		Minutes string `json:"minutes"`
		Seconds string `json:"seconds"`
	} `json:"time"`
	Errors struct {
		NoContext string `json:"noContext"`
	} `json:"errors"`
	Widgets struct {
		Tools          string `json:"tools"`
		Done           string `json:"done"`
		Running        string `json:"running"`
		Agent          string `json:"agent"`
		Todos          string `json:"todos"`
		ClaudeMd       string `json:"claudeMd"`
		AgentsMd       string `json:"agentsMd"`
		AddedDirs      string `json:"addedDirs"`
		Rules          string `json:"rules"`
		Mcps           string `json:"mcps"`
		Hooks          string `json:"hooks"`
		BurnRate       string `json:"burnRate"`
		Cache          string `json:"cache"`
		ToLimit        string `json:"toLimit"`
		Forecast       string `json:"forecast"`
		Budget         string `json:"budget"`
		Performance    string `json:"performance"`
		TokenBreakdown string `json:"tokenBreakdown"`
		TodayCost      string `json:"todayCost"`
		ApiDuration    string `json:"apiDuration"`
		PeakHours      string `json:"peakHours"`
		OffPeak        string `json:"offPeak"`
	} `json:"widgets"`
}

// loadTranslations loads the appropriate locale based on language setting.
func loadTranslations(lang string) *Translations {
	if lang == "auto" {
		lang = detectLanguage()
	}

	var data []byte
	switch lang {
	case "ko":
		data = localeKO
	default:
		data = localeEN
	}

	var t Translations
	if err := json.Unmarshal(data, &t); err != nil {
		debugLog("i18n", "parse error: %v, falling back to en", err)
		_ = json.Unmarshal(localeEN, &t)
	}
	return &t
}

// detectLanguage checks environment variables for locale hints.
func detectLanguage() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		val := os.Getenv(env)
		if strings.HasPrefix(val, "ko") {
			return "ko"
		}
	}
	return "en"
}

// Context holds all data needed by widgets.
type Context struct {
	Stdin        StdinInput
	Config       Config
	ConfigDir    string
	Translations *Translations
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
		{"projectInfo", "model", "context", "cost", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"},
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

// OrchestrateResult holds the result of widget orchestration.
type OrchestrateResult struct {
	Lines       []string
	WidgetCount int
}

// orchestrate runs all widgets according to the display configuration.
func orchestrate(ctx *Context) OrchestrateResult {
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

	result := OrchestrateResult{}
	sep := renderSeparator(ctx.Config.Separator, getTheme(ctx.Config.Theme))

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
			if rendered == "" {
				continue
			}
			parts = append(parts, rendered)
		}
		if len(parts) > 0 {
			result.WidgetCount += len(parts)
			result.Lines = append(result.Lines, strings.Join(parts, sep))
		}
	}
	return result
}
