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
	sessionID := input.SessionId
	cached := loadSessionState(sessionID)

	ctx := &Context{
		Stdin:        input,
		Config:       cfg,
		ConfigDir:    configDir,
		Translations: translations,
		RateLimits:   rateLimits,
	}

	result := orchestrate(ctx)

	// Degraded input: current stdin rendered fewer widgets than the last good
	// render. Replay orchestrate with the cached stdin so rateLimit/cost come
	// from fresh values (account-global API cache + cached stdin cost) instead
	// of a frozen ANSI string.
	if cached != nil && result.WidgetCount < cached.WidgetCount {
		debugLog("main", "degraded input (widgets=%d, cached=%d), replaying with cached stdin", result.WidgetCount, cached.WidgetCount)
		ctx.Stdin = *cached.CachedStdin
		result = orchestrate(ctx)
	}

	sep := renderSeparator(cfg.Separator, getTheme(cfg.Theme))

	var partsOutput string
	if len(result.Lines) > 0 {
		partsOutput = strings.Join(result.Lines, "\n")
	}

	if partsOutput != "" {
		if result.HasProject {
			fmt.Print(result.ProjectInfo + sep + partsOutput)
		} else {
			fmt.Print(partsOutput)
		}
	} else if result.HasProject {
		fmt.Print(result.ProjectInfo)
	}

	// Save stdin (not rendered strings) so a future degrade can re-render with
	// fresh account-global values. Strip RateLimits: those must always come
	// from the live API cache, never from stale session memory.
	if result.WidgetCount >= 2 {
		snapshot := ctx.Stdin
		snapshot.RateLimits = nil
		saveSessionState(sessionID, &SessionState{
			CachedStdin: &snapshot,
			WidgetCount: result.WidgetCount,
		})
	}
}
