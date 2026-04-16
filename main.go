package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// version is set by ldflags at build time.
var version = "dev"

// debugLog prints debug messages to stderr when DEBUG=cc-usage or DEBUG=1.
func debugLog(context string, format string, args ...any) {
	dbg := os.Getenv("DEBUG")
	if dbg == "cc-usage" || dbg == "1" {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[cc-usage:%s] %s\n", context, msg)
	}
}

// defaultConfigPath returns ~/.claude/cc-usage.json.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "cc-usage.json")
}

func main() {
	configPath := flag.String("config", defaultConfigPath(), "config path")
	flag.Parse()

	// Determine configDir
	var configDir string
	if *configPath != "" {
		configDir = filepath.Dir(*configPath)
	} else {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".claude")
	}

	debugLog("main", "configPath=%s configDir=%s", *configPath, configDir)

	cfg := loadConfig(*configPath)
	debugLog("main", "config loaded: language=%s plan=%s displayMode=%s", cfg.Language, cfg.Plan, cfg.DisplayMode)

	input := parseStdin()
	debugLog("main", "stdin parsed: model=%s version=%s", input.Model.ID, input.Version)

	// credential + API
	token := getCredential(configDir)
	debugLog("main", "credential: len=%d", len(token))

	var rateLimits *UsageLimits
	if token != "" {
		rateLimits = fetchUsageLimits(token, cfg.Cache)
	}

	translations := loadTranslations(cfg.Language)
	debugLog("main", "translations loaded: lang=%s", cfg.Language)

	// Load cached session state
	cached := loadSessionState()

	ctx := &Context{
		Stdin:        input,
		Config:       cfg,
		ConfigDir:    configDir,
		Translations: translations,
		RateLimits:   rateLimits,
	}

	result := orchestrate(ctx)
	sep := renderSeparator(cfg.Separator, getTheme(cfg.Theme))

	// Degraded input: fewer widgets than last valid render → use cached parts
	var partsOutput string
	if cached != nil && cached.CachedParts != "" && result.WidgetCount < cached.WidgetCount {
		debugLog("main", "degraded input detected (widgets=%d, cached=%d), using cached parts", result.WidgetCount, cached.WidgetCount)
		partsOutput = cached.CachedParts
	} else if len(result.Lines) > 0 {
		partsOutput = strings.Join(result.Lines, "\n")
	}

	// Combine: projectInfo (always fresh) + cached/current parts
	if partsOutput != "" {
		if result.HasProject {
			// projectInfo goes first on the first line
			firstLine := result.ProjectInfo + sep + partsOutput
			fmt.Print(firstLine)
		} else {
			fmt.Print(partsOutput)
		}
	} else if result.HasProject {
		fmt.Print(result.ProjectInfo)
	}

	// Save non-projectInfo parts for degraded input detection
	if result.WidgetCount >= 2 && len(result.Lines) > 0 {
		saveSessionState(&SessionState{
			CachedParts: strings.Join(result.Lines, "\n"),
			WidgetCount: result.WidgetCount,
		})
	}
}
