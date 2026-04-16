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

	// Detect session reset: model present but cost dropped → accumulate previous session
	accumulatedCost := cachedAccumulatedCost(cached)
	if cached != nil && input.Model.ID != "" && input.Cost.TotalCostUsd < cached.LastCost {
		accumulatedCost += cached.LastCost
		debugLog("main", "session reset detected: accumulated=%.2f + last=%.2f = %.2f",
			cached.AccumulatedCost, cached.LastCost, accumulatedCost)
	}

	// Inject accumulated cost into stdin before orchestrate
	input.Cost.TotalCostUsd += accumulatedCost

	ctx := &Context{
		Stdin:        input,
		Config:       cfg,
		ConfigDir:    configDir,
		Translations: translations,
		RateLimits:   rateLimits,
	}

	lines, widgetCount := orchestrate(ctx)

	// Degraded input: fewer widgets than last valid render → use cached output
	if cached != nil && cached.LastOutput != "" && widgetCount < cached.WidgetCount {
		debugLog("main", "degraded input detected (widgets=%d, cached=%d), using cached output", widgetCount, cached.WidgetCount)
		fmt.Print(cached.LastOutput)
		return
	}

	output := ""
	if len(lines) > 0 {
		output = strings.Join(lines, "\n")
		fmt.Print(output)
	}

	// Save state with raw stdin cost (before accumulation) for next reset detection
	if widgetCount >= 2 {
		saveSessionState(&SessionState{
			LastCost:        input.Cost.TotalCostUsd - accumulatedCost,
			LastOutput:      output,
			WidgetCount:     widgetCount,
			AccumulatedCost: accumulatedCost,
		})
	}
}
