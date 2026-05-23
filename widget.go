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
		SevenDSonnet string `json:"sevenDSonnet"`
	} `json:"labels"`
	Time struct {
		Days    string `json:"days"`
		Hours   string `json:"hours"`
		Minutes string `json:"minutes"`
		Seconds string `json:"seconds"`
	} `json:"time"`
	Widgets struct {
		ApiDuration string `json:"apiDuration"`
		BurnRate    string `json:"burnRate"`
		Cache       string `json:"cache"`
		Performance string `json:"performance"`
		Session     string `json:"session"`
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

// displayPresets defines built-in widget layouts. 현재는 compact 한 종류.
// 그 이상이 필요하면 사용자가 Config.Preset / Config.Lines로 custom 레이아웃을 정의한다.
var displayPresets = map[string][][]string{
	"compact": {
		{"projectInfo", "model", "context", "cost", "rateLimit5h", "rateLimit7d", "rateLimit7dSonnet"},
	},
}

// presetCharToWidget maps single characters to widget IDs for compact preset notation.
// 등록된 위젯에 대응하는 char만 매핑한다 (widgets_core.go, widgets_project.go, widgets_analytics.go).
var presetCharToWidget = map[byte]string{
	// core
	'M': "model",
	'C': "context",
	'$': "cost",
	'R': "rateLimit5h",
	'7': "rateLimit7d",
	'S': "rateLimit7dSonnet",
	'P': "projectInfo",
	// analytics
	'V': "version",
	'a': "apiDuration",
	'D': "sessionDuration",
	'B': "burnRate",
	'H': "cacheHit",
	'F': "performance",
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
	ProjectInfo string   // projectInfo rendered separately (always fresh)
	Lines       []string // rendered lines without projectInfo
	WidgetCount int      // count of non-projectInfo widgets
	HasProject  bool     // whether projectInfo was rendered
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
			if widgetID == "projectInfo" {
				result.ProjectInfo = rendered
				result.HasProject = true
			} else {
				parts = append(parts, rendered)
			}
		}
		if len(parts) > 0 {
			result.WidgetCount += len(parts)
			result.Lines = append(result.Lines, strings.Join(parts, sep))
		}
	}
	return result
}
